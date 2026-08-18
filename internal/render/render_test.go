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
	List(&buf, BuildRows([]*model.Habit{done, pending}, today, model.Sabbath{}), today, today, false)
	out := buf.String()

	lines := tableLines(out)
	if len(lines) != 3 {
		t.Fatalf("want a header and two rows, got:\n%s", out)
	}
	// Rows are ordered by what is left to do, so find each one by name.
	if row := rowFor(out, "meditate"); !strings.Contains(row, "✓") || !strings.Contains(row, "health") {
		t.Errorf("done row = %q", row)
	}
	// The second habit was done yesterday but not today: streak survives, flagged.
	if row := rowFor(out, "read"); !strings.Contains(row, "2!") {
		t.Errorf("at-risk row = %q, want the streak marked with !", row)
	}
	if strings.Contains(out, "\033[") {
		t.Error("color escapes leaked into uncolored output")
	}
}

func TestListColumnsLineUpWithWideGlyphs(t *testing.T) {
	var buf bytes.Buffer
	rows := BuildRows([]*model.Habit{habit("a", 0), habit("bbbbbbb")}, today, model.Sabbath{})
	List(&buf, rows, today, today, false)

	lines := tableLines(buf.String())
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
	doneRow, pendingRow := rowFor(buf.String(), "a"), rowFor(buf.String(), "bbbbbbb")
	markerCol := indexRune(doneRow, '✓')
	if markerCol < 0 || markerCol != indexRune(pendingRow, '·') {
		t.Errorf("marker column drifted: ✓ at %d, · at %d", markerCol, indexRune(pendingRow, '·'))
	}
}

// tableLines returns the habit table from List output, dropping the score
// headline above it so tests can index rows without counting banner lines.
func tableLines(out string) []string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "ID") {
			return lines[i:]
		}
	}
	return lines
}

// rowFor returns the table row for the named habit, empty when there is none.
func rowFor(out, name string) string {
	for _, l := range tableLines(out)[1:] { // skip the header
		if f := strings.Fields(l); len(f) > 1 && f[1] == name {
			return l
		}
	}
	return ""
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
	List(&buf, nil, today, today, false)
	if !strings.Contains(buf.String(), "hw add") {
		t.Errorf("empty list should point at hw add, got %q", buf.String())
	}
}

func TestBuildScoreCountsActiveHabits(t *testing.T) {
	archived := habit("old", 0)
	archived.Archived = true
	rows := BuildRows([]*model.Habit{
		habit("a", 0), habit("b", 0), habit("c"), archived,
	}, today, model.Sabbath{})

	got := BuildScore(rows)
	if got.Done != 2 || got.Total != 3 {
		t.Errorf("BuildScore = %d of %d, want 2 of 3: archived habits are not due", got.Done, got.Total)
	}
	if got.Rest {
		t.Error("Rest = true on an ordinary day")
	}
}

func TestScorePercent(t *testing.T) {
	cases := []struct {
		done, total, want int
		why               string
	}{
		{5, 8, 63, "rounds to nearest"},
		{0, 3, 0, "nothing done"},
		{3, 3, 100, "all done"},
		{0, 0, 0, "no habits"},
		{1, 3, 33, "rounds down"},
		{2, 3, 67, "rounds up"},
		{999, 1000, 99, "never rounds up to a complete day"},
		{1, 1000, 1, "never rounds down to nothing when something was done"},
	}
	for _, c := range cases {
		if got := (Score{Done: c.done, Total: c.total}).Percent(); got != c.want {
			t.Errorf("Score{%d, %d}.Percent() = %d, want %d (%s)", c.done, c.total, got, c.want, c.why)
		}
	}
}

func TestListShowsTheDailyScore(t *testing.T) {
	var buf bytes.Buffer
	rows := BuildRows([]*model.Habit{
		habit("a", 0), habit("b", 0), habit("c"), habit("d"), habit("e"),
	}, today, model.Sabbath{})
	List(&buf, rows, today, today, false)

	first := strings.SplitN(buf.String(), "\n", 2)[0]
	if !strings.Contains(first, "40%") {
		t.Errorf("first line = %q, want the score leading the output", first)
	}
	if !strings.Contains(first, "2 of 5") {
		t.Errorf("first line = %q, want the raw counts", first)
	}
}

func TestListLast30CarriesItsPercentage(t *testing.T) {
	var buf bytes.Buffer
	// Thirteen of the last 26 eligible days. The Sundays sit at offsets 4, 11,
	// 18 and 25, and none are used, so the denominator stays 26.
	h := habit("meditate", 1, 2, 3, 5, 6, 8, 10, 12, 14, 15, 16, 17, 19)
	List(&buf, BuildRows([]*model.Habit{h}, today, sundaySabbath), today, today, false)

	row := tableLines(buf.String())[1]
	if !strings.Contains(row, "13/26 (50%)") {
		t.Errorf("row = %q, want the last-30 column to carry its percentage", row)
	}
}

func TestListLast30PercentageMatchesTheScoreRounding(t *testing.T) {
	var buf bytes.Buffer
	// Nothing at all in the window: no percentage should imply otherwise.
	List(&buf, BuildRows([]*model.Habit{habit("meditate")}, today, model.Sabbath{}), today, today, false)
	if row := tableLines(buf.String())[1]; !strings.Contains(row, "0/30 (0%)") {
		t.Errorf("row = %q, want 0/30 (0%%)", row)
	}
}

func TestListScoreIsAbsentWithoutHabits(t *testing.T) {
	var buf bytes.Buffer
	List(&buf, nil, today, today, false)
	if strings.Contains(buf.String(), "%") {
		t.Errorf("empty list should show no score:\n%s", buf.String())
	}
}

func TestListScoreOnARestDay(t *testing.T) {
	restDay := hdate.New(2026, time.August, 2)
	idle := &model.Habit{ID: 1, Name: "a", Created: restDay.Add(-30).String(), Done: []string{}}

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{idle}, restDay, sundaySabbath), restDay, restDay, false)
	out := buf.String()
	if !strings.Contains(out, "Rest day") {
		t.Errorf("rest day list does not say so:\n%s", out)
	}
	// The headline specifically: a rest day has no score, and 0% would read as
	// a failing grade. The Last 30 column keeps its own percentage.
	headline := strings.SplitN(out, "\n", 2)[0]
	if strings.Contains(headline, "%") {
		t.Errorf("a rest day was given a score: %q", headline)
	}
}

func TestListScoreCountsBonusWorkOnARestDay(t *testing.T) {
	restDay := hdate.New(2026, time.August, 2)
	worked := &model.Habit{ID: 1, Name: "a", Created: restDay.Add(-30).String(), Done: []string{}}
	worked.MarkDone(restDay)

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{worked}, restDay, sundaySabbath), restDay, restDay, false)
	out := buf.String()
	if !strings.Contains(out, "Rest day") || !strings.Contains(out, "1") {
		t.Errorf("rest day list does not report the bonus:\n%s", out)
	}
}

// sundaySabbath rests on Sunday, which for the 2026-08-06 anchor is 2026-08-02.
var sundaySabbath = model.Sabbath{Day: time.Sunday, On: true}

func TestListShowsRestInsteadOfAMiss(t *testing.T) {
	// Anchor on the rest day itself, with the habit not done.
	restDay := hdate.New(2026, time.August, 2)
	h := &model.Habit{ID: 1, Name: "meditate", Created: restDay.Add(-30).String(), Done: []string{}}

	var buf bytes.Buffer
	rows := BuildRows([]*model.Habit{h}, restDay, sundaySabbath)
	if !rows[0].RestToday {
		t.Fatal("RestToday = false on the rest day")
	}
	List(&buf, rows, restDay, restDay, false)

	out := buf.String()
	if !strings.Contains(out, RestGlyph) {
		t.Errorf("list does not mark the rest day with %q:\n%s", RestGlyph, out)
	}
	if strings.Contains(out, Glyphs[0]) {
		t.Errorf("rest day still reads as a miss:\n%s", out)
	}
}

func TestListLast30DropsRestDaysFromTheDenominator(t *testing.T) {
	// Done every day for the last 30, rest days included in the window.
	offsets := make([]int, 30)
	for i := range offsets {
		offsets[i] = i
	}
	h := habit("meditate", offsets...)

	var buf bytes.Buffer
	rows := BuildRows([]*model.Habit{h}, today, sundaySabbath)
	List(&buf, rows, today, today, false)
	if !strings.Contains(buf.String(), "30/30") {
		t.Errorf("a perfect month should read 30/30 when rest days were done anyway:\n%s", buf.String())
	}

	// Nothing done: the four Sundays in the window leave 26 eligible days.
	empty := habit("skip")
	var buf2 bytes.Buffer
	List(&buf2, BuildRows([]*model.Habit{empty}, today, sundaySabbath), today, today, false)
	if !strings.Contains(buf2.String(), "0/26") {
		t.Errorf("want 0/26 with four rest days excluded:\n%s", buf2.String())
	}
}

func TestListColumnsLineUpWithTheRestGlyph(t *testing.T) {
	restDay := hdate.New(2026, time.August, 2)
	done := &model.Habit{ID: 1, Name: "a", Created: restDay.Add(-30).String(), Done: []string{}}
	done.MarkDone(restDay)
	resting := &model.Habit{ID: 2, Name: "b", Created: restDay.Add(-30).String(), Done: []string{}}

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{done, resting}, restDay, sundaySabbath), restDay, restDay, false)

	lines := tableLines(buf.String())
	if len(lines) < 3 {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
	markerCol := indexRune(lines[1], '✓')
	restCol := indexRune(lines[2], []rune(RestGlyph)[0])
	if markerCol < 0 || markerCol != restCol {
		t.Errorf("marker column drifted: ✓ at %d, rest at %d", markerCol, restCol)
	}
}

// habitOrder returns the habit names down the table, in the order listed.
func habitOrder(out string) []string {
	lines := tableLines(out)
	names := make([]string, 0, len(lines))
	for _, l := range lines[1:] { // skip the header
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		names = append(names, f[1])
	}
	return names
}

func TestListPutsUnresolvedHabitsFirst(t *testing.T) {
	done := habit("meditate", 0, 1)
	pending := habit("read", 1)
	pending.ID = 2
	alsoDone := habit("stretch", 0)
	alsoDone.ID = 3
	stillPending := habit("walk")
	stillPending.ID = 4

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{done, pending, alsoDone, stillPending}, today, model.Sabbath{}), today, today, false)

	got := strings.Join(habitOrder(buf.String()), ",")
	if want := "read,walk,meditate,stretch"; got != want {
		t.Errorf("order = %q, want %q: what is left to do comes first, ID order within each group", got, want)
	}
}

func TestListPutsArchivedHabitsLast(t *testing.T) {
	// Archived and unmarked, which would sort it first if archiving were ignored.
	archived := habit("old")
	archived.Archived = true
	pending := habit("read")
	pending.ID = 2
	done := habit("meditate", 0)
	done.ID = 3

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{archived, pending, done}, today, model.Sabbath{}), today, today, false)

	got := strings.Join(habitOrder(buf.String()), ",")
	if want := "read,meditate,old"; got != want {
		t.Errorf("order = %q, want %q: an archived habit is not being tracked, so it cannot be behind", got, want)
	}
}

func TestListKeepsIDOrderOnARestDay(t *testing.T) {
	restDay := hdate.New(2026, time.August, 2)
	done := &model.Habit{ID: 1, Name: "a", Created: restDay.Add(-30).String(), Done: []string{}}
	done.MarkDone(restDay)
	resting := &model.Habit{ID: 2, Name: "b", Created: restDay.Add(-30).String(), Done: []string{}}

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{done, resting}, restDay, sundaySabbath), restDay, restDay, false)

	got := strings.Join(habitOrder(buf.String()), ",")
	if want := "a,b"; got != want {
		t.Errorf("order = %q, want %q: nothing is due on a rest day, so an unmarked habit is not unresolved", got, want)
	}
}

func TestListDoesNotReorderTheCallersRows(t *testing.T) {
	done := habit("meditate", 0)
	pending := habit("read")
	pending.ID = 2
	rows := BuildRows([]*model.Habit{done, pending}, today, model.Sabbath{})

	var buf bytes.Buffer
	List(&buf, rows, today, today, false)

	if rows[0].Name != "meditate" || rows[1].Name != "read" {
		t.Errorf("List reordered the caller's slice: %q then %q", rows[0].Name, rows[1].Name)
	}
}

func TestBuildCalFlagsRestDays(t *testing.T) {
	h := habit("meditate", 0)
	cal := BuildCal([]*model.Habit{h}, today, 4, sundaySabbath)

	rest := map[string]bool{}
	for _, d := range cal.Days {
		rest[d.Date] = d.Rest
	}
	if !rest["2026-08-02"] {
		t.Error("Sunday 2026-08-02 is not flagged as a rest day")
	}
	if rest["2026-08-03"] {
		t.Error("Monday 2026-08-03 is flagged as a rest day")
	}
	if !rest["2026-07-26"] {
		t.Error("the earlier Sunday is not flagged as a rest day")
	}
}

func TestCalDrawsRestDaysDistinctly(t *testing.T) {
	var buf bytes.Buffer
	cal := BuildCal([]*model.Habit{habit("meditate", 0)}, today, 4, sundaySabbath)
	Cal(&buf, cal, "meditate", false)

	out := buf.String()
	if !strings.Contains(out, RestGlyph) {
		t.Errorf("calendar does not draw rest days with %q:\n%s", RestGlyph, out)
	}
	if !strings.Contains(out, "rest") {
		t.Errorf("calendar legend does not explain the rest glyph:\n%s", out)
	}
}

func TestCalWithSabbathOffDrawsNoRestGlyph(t *testing.T) {
	var buf bytes.Buffer
	cal := BuildCal([]*model.Habit{habit("meditate", 0)}, today, 4, model.Sabbath{})
	Cal(&buf, cal, "meditate", false)
	if strings.Contains(buf.String(), RestGlyph) {
		t.Errorf("rest glyph drawn with sabbath off:\n%s", buf.String())
	}
}

func TestStatsShowsTheRestDay(t *testing.T) {
	var buf bytes.Buffer
	Stats(&buf, habit("meditate", 0, 1).Stats(today, sundaySabbath), false)
	out := buf.String()
	if !strings.Contains(out, "Sabbath") || !strings.Contains(out, "Sunday") {
		t.Errorf("stats does not report the rest day:\n%s", out)
	}
}

func TestStatsOmitsSabbathWhenOff(t *testing.T) {
	var buf bytes.Buffer
	Stats(&buf, habit("meditate", 0, 1).Stats(today, model.Sabbath{}), false)
	if strings.Contains(buf.String(), "Sabbath") {
		t.Errorf("stats mentions a sabbath that is off:\n%s", buf.String())
	}
}

func TestBuildCalCoversWholeWeeks(t *testing.T) {
	h := habit("meditate", 0, 3)
	cal := BuildCal([]*model.Habit{h}, today, 4, model.Sabbath{})

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
	cal := BuildCal([]*model.Habit{a, b, c}, today, 1, model.Sabbath{})

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
	cal := BuildCal([]*model.Habit{habit("meditate", 0)}, today, 3, model.Sabbath{})
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
	cal := BuildCal([]*model.Habit{habit("meditate")}, today, 12, model.Sabbath{})
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
	Stats(&buf, h.Stats(today, model.Sabbath{}), false)
	out := buf.String()

	for _, want := range []string{"meditate (#1)", "+health", "Current streak", "3 days", "Longest streak", "Best weekday"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats output missing %q:\n%s", want, out)
		}
	}
}

func TestStatsFlagsAtRisk(t *testing.T) {
	var buf bytes.Buffer
	Stats(&buf, habit("meditate", 1, 2).Stats(today, model.Sabbath{}), false)
	if !strings.Contains(buf.String(), "not done today") {
		t.Errorf("at-risk streak not flagged:\n%s", buf.String())
	}
}
