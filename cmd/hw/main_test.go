package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/render"
)

// cli runs hw against a data file in a temp directory, returning stdout.
type cli struct {
	t    *testing.T
	path string
}

func newCLI(t *testing.T) *cli {
	t.Helper()
	return &cli{t: t, path: filepath.Join(t.TempDir(), "habits.json")}
}

func (c *cli) run(args ...string) (string, error) {
	c.t.Helper()
	var buf bytes.Buffer
	full := append([]string{"--data", c.path, "--no-color"}, args...)
	err := run(full, &buf)
	return buf.String(), err
}

// ok runs a command that must succeed.
func (c *cli) ok(args ...string) string {
	c.t.Helper()
	out, err := c.run(args...)
	if err != nil {
		c.t.Fatalf("hw %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// fails runs a command that must fail, returning the error message.
func (c *cli) fails(args ...string) string {
	c.t.Helper()
	_, err := c.run(args...)
	if err == nil {
		c.t.Fatalf("hw %s unexpectedly succeeded", strings.Join(args, " "))
	}
	return err.Error()
}

func TestAddListDoneFlow(t *testing.T) {
	c := newCLI(t)

	if out := c.ok("add", "meditate", "+health"); !strings.Contains(out, "Added meditate (#1)") {
		t.Errorf("add output = %q", out)
	}
	c.ok("add", "read a book", "+mind")

	out := c.ok("list")
	if !strings.Contains(out, "meditate") || !strings.Contains(out, "read a book") {
		t.Fatalf("list output:\n%s", out)
	}

	c.ok("done", "meditate")
	out = c.ok("list")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.Contains(lines[1], "✓") {
		t.Errorf("meditate not marked done:\n%s", out)
	}
	if strings.Contains(lines[2], "✓") {
		t.Errorf("the wrong habit was marked done:\n%s", out)
	}

	// done is idempotent.
	if out := c.ok("done", "meditate"); !strings.Contains(out, "No change") {
		t.Errorf("second done = %q", out)
	}

	if out := c.ok("undo", "meditate"); !strings.Contains(out, "Unmarked") {
		t.Errorf("undo = %q", out)
	}
	if out := c.ok("list"); strings.Contains(out, "✓") {
		t.Errorf("undo did not clear the mark:\n%s", out)
	}
}

func TestDoneAcceptsHumanDates(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("done", "meditate", "yesterday")
	c.ok("done", "meditate", "3", "days", "ago")

	var stats map[string]any
	out := c.ok("stats", "meditate", "--json")
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("stats --json: %v\n%s", err, out)
	}
	if stats["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", stats["total"])
	}
	// Done yesterday but not today: the streak survives and is flagged.
	if stats["current_streak"].(float64) != 1 || stats["at_risk"] != true {
		t.Errorf("streak = %v, at_risk = %v", stats["current_streak"], stats["at_risk"])
	}
}

func TestDoneRejectsFutureDates(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	if msg := c.fails("done", "meditate", "tomorrow"); !strings.Contains(msg, "has not happened yet") {
		t.Errorf("error = %q", msg)
	}
	if msg := c.fails("done", "meditate", "someday"); !strings.Contains(msg, "cannot understand date") {
		t.Errorf("error = %q", msg)
	}
}

func TestBulkDoneByTag(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+morning")
	c.ok("add", "stretch", "+morning")
	c.ok("add", "read", "+evening")

	c.ok("done", "+morning")

	var rows []render.ListRow
	if err := json.Unmarshal([]byte(c.ok("list", "--json")), &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		want := r.Name != "read"
		if r.DoneToday != want {
			t.Errorf("%s done = %v, want %v", r.Name, r.DoneToday, want)
		}
	}

	if msg := c.fails("done", "+nosuchtag"); !strings.Contains(msg, "no habits match") {
		t.Errorf("error = %q", msg)
	}
}

// A mistyped relative date must not silently become a bulk operation over
// every habit.
func TestNegativeNumberIsNotATagExclusion(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("add", "read")

	if msg := c.fails("done", "meditate", "-3"); !strings.Contains(msg, "cannot understand date") {
		t.Errorf("error = %q", msg)
	}
	var rows []render.ListRow
	unmarshal(t, c.ok("list", "--json"), &rows)
	for _, r := range rows {
		if r.DoneToday {
			t.Errorf("%s was marked done by a failed command", r.Name)
		}
	}
}

func TestHabitAndTagFiltersCannotBeMixed(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+health")

	for _, args := range [][]string{
		{"done", "+health", "meditate"},
		{"cal", "+health", "meditate"},
		{"stats", "+health", "meditate"},
	} {
		if msg := c.fails(args...); !strings.Contains(msg, "not both") {
			t.Errorf("hw %s: error = %q", strings.Join(args, " "), msg)
		}
	}
}

func TestListFilters(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+health", "+morning")
	c.ok("add", "run", "+health")
	c.ok("add", "read", "+mind")

	var rows []render.ListRow
	unmarshal(t, c.ok("list", "+health", "--json"), &rows)
	if len(rows) != 2 {
		t.Errorf("+health = %v", rowNames(rows))
	}

	unmarshal(t, c.ok("list", "+health", "-morning", "--json"), &rows)
	if len(rows) != 1 || rows[0].Name != "run" {
		t.Errorf("+health -morning = %v", rowNames(rows))
	}

	if msg := c.fails("list", "meditate"); !strings.Contains(msg, "only tag filters") {
		t.Errorf("error = %q", msg)
	}
}

func TestSelectorResolution(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("add", "read")
	c.ok("add", "read fiction")

	c.ok("done", "1")       // by ID
	c.ok("done", "med")     // prefix, same habit
	c.ok("done", "fiction") // substring

	var rows []render.ListRow
	unmarshal(t, c.ok("list", "--json"), &rows)
	byName := map[string]render.ListRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	if !byName["meditate"].DoneToday || !byName["read fiction"].DoneToday || byName["read"].DoneToday {
		t.Errorf("wrong habits marked: %+v", rows)
	}

	if msg := c.fails("done", "rea"); !strings.Contains(msg, "matches") {
		t.Errorf("ambiguous selector error = %q", msg)
	}
	if msg := c.fails("done", "swim"); !strings.Contains(msg, "no habit matching") {
		t.Errorf("unknown selector error = %q", msg)
	}
}

func TestRenameTagArchiveDelete(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+health")
	c.ok("add", "read")

	if out := c.ok("rename", "meditate", "morning", "sit"); !strings.Contains(out, "Renamed meditate to morning sit") {
		t.Errorf("rename = %q", out)
	}
	if msg := c.fails("rename", "morning sit", "read"); !strings.Contains(msg, "already exists") {
		t.Errorf("duplicate rename = %q", msg)
	}

	if out := c.ok("tag", "morning sit", "+calm", "-health"); !strings.Contains(out, "+calm") || strings.Contains(out, "+health") {
		t.Errorf("tag = %q", out)
	}
	if msg := c.fails("tag", "read"); !strings.Contains(msg, "+tag") {
		t.Errorf("tag without changes = %q", msg)
	}

	c.ok("archive", "read")
	var rows []render.ListRow
	unmarshal(t, c.ok("list", "--json"), &rows)
	if len(rows) != 1 {
		t.Errorf("archived habit still listed: %v", rowNames(rows))
	}
	unmarshal(t, c.ok("list", "--all", "--json"), &rows)
	if len(rows) != 2 {
		t.Errorf("--all did not include the archived habit: %v", rowNames(rows))
	}
	if msg := c.fails("archive", "read"); !strings.Contains(msg, "already archived") {
		t.Errorf("double archive = %q", msg)
	}
	c.ok("unarchive", "read")

	if msg := c.fails("delete", "read"); !strings.Contains(msg, "--force") {
		t.Errorf("delete without --force = %q", msg)
	}
	if out := c.ok("delete", "read", "--force"); !strings.Contains(out, "Deleted read") {
		t.Errorf("delete = %q", out)
	}
	unmarshal(t, c.ok("list", "--all", "--json"), &rows)
	if len(rows) != 1 {
		t.Errorf("delete left %v", rowNames(rows))
	}
}

func TestCalOutput(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("done", "meditate")

	out := c.ok("cal", "meditate")
	if !strings.Contains(out, "meditate") || !strings.Contains(out, "Less") {
		t.Errorf("cal output:\n%s", out)
	}

	var cal render.Calendar
	unmarshal(t, c.ok("cal", "--weeks", "2", "--json"), &cal)
	if len(cal.Days) < 8 || len(cal.Days) > 14 {
		t.Errorf("two weeks produced %d days", len(cal.Days))
	}
	if cal.End != hdate.Today().String() {
		t.Errorf("End = %s", cal.End)
	}

	if msg := c.fails("cal", "--weeks", "0"); !strings.Contains(msg, "positive number") {
		t.Errorf("error = %q", msg)
	}
	if msg := c.fails("cal", "--weeks"); !strings.Contains(msg, "needs a value") {
		t.Errorf("error = %q", msg)
	}
}

func TestStatsForAllHabits(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("add", "read")

	out := c.ok("stats")
	if !strings.Contains(out, "meditate") || !strings.Contains(out, "read") {
		t.Errorf("stats for all habits:\n%s", out)
	}

	var many []map[string]any
	unmarshal(t, c.ok("stats", "--json"), &many)
	if len(many) != 2 {
		t.Errorf("stats --json returned %d entries", len(many))
	}
}

func TestEmptyStoreReports(t *testing.T) {
	c := newCLI(t)
	if out := c.ok("list"); !strings.Contains(out, "No habits") {
		t.Errorf("list on an empty store = %q", out)
	}
	if msg := c.fails("cal"); !strings.Contains(msg, "no habits") {
		t.Errorf("cal on an empty store = %q", msg)
	}
	if msg := c.fails("stats"); !strings.Contains(msg, "no habits") {
		t.Errorf("stats on an empty store = %q", msg)
	}
}

func TestHelpAndVersionAndUnknownCommand(t *testing.T) {
	c := newCLI(t)
	if out := c.ok("--help"); !strings.Contains(out, "hw add") {
		t.Errorf("help = %q", out)
	}
	if out := c.ok(); !strings.Contains(out, "Usage:") {
		t.Errorf("bare hw = %q", out)
	}
	if out := c.ok("--version"); !strings.Contains(out, version) {
		t.Errorf("version = %q", out)
	}
	if msg := c.fails("frobnicate"); !strings.Contains(msg, "unknown command") {
		t.Errorf("unknown command = %q", msg)
	}
}

func TestNoDataFileIsWrittenUntilSomethingChanges(t *testing.T) {
	c := newCLI(t)
	c.ok("list")
	if _, err := os.Stat(c.path); !os.IsNotExist(err) {
		t.Error("a read-only command created the data file")
	}
	c.ok("add", "meditate")
	if _, err := os.Stat(c.path); err != nil {
		t.Errorf("add did not create the data file: %v", err)
	}
}

func TestCorruptDataFileExitsWithIOCode(t *testing.T) {
	c := newCLI(t)
	if err := os.WriteFile(c.path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := c.run("list")
	if err == nil {
		t.Fatal("corrupt file was accepted")
	}
	var ee exitError
	if !errorsAs(err, &ee) || ee.code != 2 {
		t.Errorf("exit code = %+v, want 2", err)
	}
}

func TestUserErrorsExitWithCodeOne(t *testing.T) {
	c := newCLI(t)
	_, err := c.run("done", "nothing")
	var ee exitError
	if !errorsAs(err, &ee) || ee.code != 1 {
		t.Errorf("exit code = %+v, want 1", err)
	}
}

func unmarshal(t *testing.T, data string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, data)
	}
}

func rowNames(rows []render.ListRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}
