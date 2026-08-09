package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/model"
)

var today = hdate.New(2026, time.August, 6) // a Thursday

func habit(name string, offsets ...int) *model.Habit {
	h := &model.Habit{ID: 1, Name: name, Created: today.Add(-30).String(), Done: []string{}}
	for _, o := range offsets {
		h.MarkDone(today.Add(-o))
	}
	return h
}

func TestListMarksTodayAndStreak(t *testing.T) {
	done := habit("meditate", 0, 1, 2)
	done.Tags = []string{"health"}
	pending := habit("read", 1, 2)
	pending.ID = 2

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{done, pending}, today), false)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two rows, got:\n%s", out)
	}
	if !strings.Contains(lines[1], "✓") || !strings.Contains(lines[1], "health") {
		t.Errorf("done row = %q", lines[1])
	}
	// The second habit was done yesterday but not today: streak survives, flagged.
	if !strings.Contains(lines[2], "2!") {
		t.Errorf("at-risk row = %q, want the streak marked with !", lines[2])
	}
	if strings.Contains(out, "\033[") {
		t.Error("color escapes leaked into uncolored output")
	}
}

func TestListColumnsLineUpWithWideGlyphs(t *testing.T) {
	var buf bytes.Buffer
	rows := BuildRows([]*model.Habit{habit("a", 0), habit("bbbbbbb")}, today)
	List(&buf, rows, false)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
	// One row's marker is ✓ and the other's is ·, both three bytes but one
	// column wide. Padding that counted bytes would leave the columns ragged,
	// so both rows must be the same number of display columns.
	first, second := []rune(lines[1]), []rune(lines[2])
	if len(first) != len(second) {
		t.Errorf("rows have different display widths:\n%q (%d)\n%q (%d)",
			lines[1], len(first), lines[2], len(second))
	}
	// The marker sits at the same offset on both rows.
	markerCol := indexRune(lines[1], '✓')
	if markerCol < 0 || markerCol != indexRune(lines[2], '·') {
		t.Errorf("marker column drifted: ✓ at %d, · at %d", markerCol, indexRune(lines[2], '·'))
	}
}

// indexRune reports the display column of the first occurrence of r.
func indexRune(s string, r rune) int {
	for i, got := range []rune(s) {
		if got == r {
			return i
		}
	}
	return -1
}

func TestListEmpty(t *testing.T) {
	var buf bytes.Buffer
	List(&buf, nil, false)
	if !strings.Contains(buf.String(), "hw add") {
		t.Errorf("empty list should point at hw add, got %q", buf.String())
	}
}

func TestBuildCalCoversWholeWeeks(t *testing.T) {
	h := habit("meditate", 0, 3)
	cal := BuildCal([]*model.Habit{h}, today, 4)

	// Four columns ending with the current week: the grid opens on the Sunday
	// three weeks before this week's Sunday (2026-08-02).
	if cal.Start != "2026-07-12" {
		t.Errorf("Start = %s, want 2026-07-12", cal.Start)
	}
	if cal.End != today.String() {
		t.Errorf("End = %s, want today", cal.End)
	}
	// Sunday 2026-07-12 through Thursday 2026-08-06 is 26 days.
	if len(cal.Days) != 26 {
		t.Errorf("Days = %d, want 26", len(cal.Days))
	}
	if cal.Days[0].Date != cal.Start {
		t.Errorf("first day = %s", cal.Days[0].Date)
	}
	if last := cal.Days[len(cal.Days)-1]; last.Date != today.String() || last.Count != 1 {
		t.Errorf("last day = %+v, want today counted", last)
	}
	if cal.Habits != 1 {
		t.Errorf("Habits = %d", cal.Habits)
	}
}

func TestBuildCalCountsAcrossHabits(t *testing.T) {
	a := habit("a", 0)
	b := habit("b", 0)
	c := habit("c", 1)
	cal := BuildCal([]*model.Habit{a, b, c}, today, 1)

	byDate := map[string]int{}
	for _, d := range cal.Days {
		byDate[d.Date] = d.Count
	}
	if byDate[today.String()] != 2 {
		t.Errorf("today's count = %d, want 2", byDate[today.String()])
	}
	if byDate[today.Add(-1).String()] != 1 {
		t.Errorf("yesterday's count = %d, want 1", byDate[today.Add(-1).String()])
	}
}

func TestLevel(t *testing.T) {
	cases := []struct{ count, total, want int }{
		{0, 5, 0},
		{1, 1, 4}, // a single habit is all-or-nothing
		{5, 5, 4},
		{4, 5, 3},
		{2, 5, 2},
		{1, 5, 1},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := level(c.count, c.total); got != c.want {
			t.Errorf("level(%d, %d) = %d, want %d", c.count, c.total, got, c.want)
		}
	}
}

func TestCalGrid(t *testing.T) {
	var buf bytes.Buffer
	cal := BuildCal([]*model.Habit{habit("meditate", 0)}, today, 3)
	Cal(&buf, cal, "meditate", false)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// title, month labels, seven weekday rows, legend
	if len(lines) != 10 {
		t.Fatalf("want 10 lines, got %d:\n%s", len(lines), buf.String())
	}
	if lines[0] != "meditate" {
		t.Errorf("title = %q", lines[0])
	}
	for i, want := range []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"} {
		if !strings.HasPrefix(lines[2+i], want) {
			t.Errorf("row %d = %q, want it to start with %s", i, lines[2+i], want)
		}
	}
	// Today is a Thursday and is done, so the Thursday row ends with a full block.
	thursday := strings.TrimRight(lines[2+4], " ")
	if !strings.HasSuffix(thursday, Glyphs[4]) {
		t.Errorf("Thursday row = %q, want it to end with a completed day", thursday)
	}
	// Days after today are blank, so Friday and Saturday are shorter.
	if len([]rune(strings.TrimRight(lines[2+5], " "))) >= len([]rune(thursday)) {
		t.Errorf("future days were rendered:\nThu %q\nFri %q", thursday, lines[2+5])
	}
	if !strings.Contains(lines[9], "Less") {
		t.Errorf("legend = %q", lines[9])
	}
}

// Every month the grid spans gets a label, positioned over the column where
// that month starts.
func TestCalMonthLabels(t *testing.T) {
	var buf bytes.Buffer
	cal := BuildCal([]*model.Habit{habit("meditate")}, today, 12)
	Cal(&buf, cal, "", false)

	header := strings.Split(buf.String(), "\n")[0]
	for _, month := range []string{"May", "Jun", "Jul", "Aug"} {
		if !strings.Contains(header, month) {
			t.Errorf("month labels %q are missing %s", header, month)
		}
	}
	// Labels appear in calendar order.
	last := -1
	for _, month := range []string{"May", "Jun", "Jul", "Aug"} {
		at := strings.Index(header, month)
		if at <= last {
			t.Errorf("month labels out of order in %q", header)
		}
		last = at
	}
	// A label must sit over its own month's column, not run into its neighbor.
	if strings.Contains(header, "JunJul") || strings.Contains(header, "MayJun") {
		t.Errorf("labels collided: %q", header)
	}
}

func TestStatsOutput(t *testing.T) {
	h := habit("meditate", 0, 1, 2)
	h.Tags = []string{"health"}

	var buf bytes.Buffer
	Stats(&buf, h.Stats(today), false)
	out := buf.String()

	for _, want := range []string{"meditate (#1)", "+health", "Current streak", "3 days", "Longest streak", "Best weekday"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats output missing %q:\n%s", want, out)
		}
	}
}

func TestStatsFlagsAtRisk(t *testing.T) {
	var buf bytes.Buffer
	Stats(&buf, habit("meditate", 1, 2).Stats(today), false)
	if !strings.Contains(buf.String(), "not done today") {
		t.Errorf("at-risk streak not flagged:\n%s", buf.String())
	}
}
