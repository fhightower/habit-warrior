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
	// This test is about parsing dates, not resting: with the default sabbath
	// on, whether the two-day gap below is bridged would depend on the weekday
	// the suite happens to run.
	c.ok("config", "sabbath", "off")
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
	if out := c.ok("help"); !strings.Contains(out, "Usage:") {
		t.Errorf("hw help = %q", out)
	}
	if out := c.ok("--version"); !strings.Contains(out, version) {
		t.Errorf("version = %q", out)
	}
	if msg := c.fails("frobnicate"); !strings.Contains(msg, "unknown command") {
		t.Errorf("unknown command = %q", msg)
	}
}

func TestBareCommandListsHabits(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+calm")
	c.ok("add", "workout")
	c.ok("done", "meditate")

	bare := c.ok()
	if strings.Contains(bare, "Usage:") {
		t.Fatalf("bare hw still prints the help message:\n%s", bare)
	}
	if bare != c.ok("list") {
		t.Errorf("bare hw and hw list disagree:\n%s\n---\n%s", bare, c.ok("list"))
	}
	for _, want := range []string{"meditate", "workout", "calm", "✓"} {
		if !strings.Contains(bare, want) {
			t.Errorf("bare hw output is missing %q:\n%s", want, bare)
		}
	}
}

func TestBareCommandOnAnEmptyStorePointsAtAdd(t *testing.T) {
	c := newCLI(t)
	out := c.ok()
	if strings.Contains(out, "Usage:") {
		t.Errorf("bare hw on an empty store printed the help message:\n%s", out)
	}
	if !strings.Contains(out, "hw add") {
		t.Errorf("bare hw on an empty store should point at hw add, got:\n%s", out)
	}
}

func TestBareCommandHonoursGlobalFlags(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")

	var rows []render.ListRow
	unmarshal(t, c.ok("--json"), &rows)
	if len(rows) != 1 || rows[0].Name != "meditate" {
		t.Errorf("hw --json = %+v, want the habit list", rows)
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

// TestSabbathBridgesAStreakEndToEnd pins the rest day to the weekday two days
// back, so the same completions are tested against a rest day and an ordinary
// day no matter which day the suite runs on.
func TestSabbathBridgesAStreakEndToEnd(t *testing.T) {
	now := hdate.Today()
	restDay := now.Add(-2)

	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("done", "meditate", now.Add(-1).String())
	c.ok("done", "meditate", now.Add(-3).String())

	c.ok("config", "sabbath", restDay.Weekday().String())
	var bridged map[string]any
	unmarshal(t, c.ok("stats", "meditate", "--json"), &bridged)
	if bridged["current_streak"].(float64) != 2 {
		t.Errorf("streak across the rest day = %v, want 2", bridged["current_streak"])
	}

	c.ok("config", "sabbath", "off")
	var broken map[string]any
	unmarshal(t, c.ok("stats", "meditate", "--json"), &broken)
	if broken["current_streak"].(float64) != 1 {
		t.Errorf("streak with sabbath off = %v, want 1", broken["current_streak"])
	}
}

func TestDoneOnTheSabbathIsAllowedAndCounts(t *testing.T) {
	now := hdate.Today()

	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("config", "sabbath", now.Add(-1).Weekday().String())
	c.ok("done", "meditate", now.Add(-1).String())
	c.ok("done", "meditate")

	var stats map[string]any
	unmarshal(t, c.ok("stats", "meditate", "--json"), &stats)
	if stats["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2: a rest day marked done is still recorded", stats["total"])
	}
	if stats["current_streak"].(float64) != 2 {
		t.Errorf("streak = %v, want 2: a rest day done anyway counts", stats["current_streak"])
	}
}

func TestConfigShowsDefaults(t *testing.T) {
	c := newCLI(t)
	out := c.ok("config")
	if !strings.Contains(out, "sabbath") || !strings.Contains(strings.ToLower(out), "sunday") {
		t.Errorf("config listing does not report the default sabbath:\n%s", out)
	}
}

func TestConfigSetsAndReadsBackSabbath(t *testing.T) {
	c := newCLI(t)
	if out := c.ok("config", "sabbath", "sat"); !strings.Contains(strings.ToLower(out), "saturday") {
		t.Errorf("setting sabbath to sat reported %q", out)
	}
	// The setting survives the process boundary, i.e. it was written to disk.
	if out := c.ok("config", "sabbath"); !strings.Contains(strings.ToLower(out), "saturday") {
		t.Errorf("sabbath did not persist, got %q", out)
	}
}

func TestConfigDisablesAndReenablesSabbath(t *testing.T) {
	c := newCLI(t)
	if out := c.ok("config", "sabbath", "off"); !strings.Contains(strings.ToLower(out), "off") {
		t.Errorf("disabling reported %q", out)
	}
	if out := c.ok("config", "sabbath"); !strings.Contains(strings.ToLower(out), "off") {
		t.Errorf("sabbath did not stay off, got %q", out)
	}
	if out := c.ok("config", "sabbath", "on"); !strings.Contains(strings.ToLower(out), "sunday") {
		t.Errorf("re-enabling did not restore the default day, got %q", out)
	}
}

func TestConfigRejectsBadKeysAndValues(t *testing.T) {
	c := newCLI(t)
	if msg := c.fails("config", "sabatical", "sunday"); !strings.Contains(msg, "sabbath") {
		t.Errorf("unknown key error does not list the valid keys: %q", msg)
	}
	if msg := c.fails("config", "sabbath", "someday"); !strings.Contains(msg, "weekday") {
		t.Errorf("bad value error = %q", msg)
	}
}

func TestConfigJSON(t *testing.T) {
	c := newCLI(t)
	c.ok("config", "sabbath", "monday")
	var cfg map[string]any
	unmarshal(t, c.ok("config", "--json"), &cfg)
	if cfg["sabbath"] != "monday" {
		t.Errorf("config --json sabbath = %v, want monday", cfg["sabbath"])
	}
}

func TestConfigLeavesHabitsIntact(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+calm")
	c.ok("done", "meditate")
	c.ok("config", "sabbath", "friday")

	var rows []render.ListRow
	unmarshal(t, c.ok("list", "--json"), &rows)
	if len(rows) != 1 || rows[0].Name != "meditate" || !rows[0].DoneToday {
		t.Errorf("writing config disturbed the habits: %+v", rows)
	}
}

func TestHelpDocumentsEveryCommand(t *testing.T) {
	c := newCLI(t)
	out := c.ok("--help")

	// Only the command list counts: prose further down mentions commands too,
	// and a command missing from the list is exactly the bug being guarded.
	_, after, ok := strings.Cut(out, "Usage:\n")
	if !ok {
		t.Fatalf("help has no Usage section:\n%s", out)
	}
	list, _, _ := strings.Cut(after, "\n\n")

	for _, cmd := range []string{"add", "done", "undo", "list", "cal", "stats",
		"rename", "tag", "archive", "unarchive", "delete", "config"} {
		if !strings.Contains(list, "  hw "+cmd+" ") {
			t.Errorf("the command list does not include hw %s:\n%s", cmd, list)
		}
	}
	if !strings.Contains(out, "sabbath") {
		t.Errorf("help does not explain the sabbath:\n%s", out)
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
