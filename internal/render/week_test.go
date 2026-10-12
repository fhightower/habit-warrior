package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/model"
)

// today is Thursday 2026-08-06, so this week opened on Sunday 2026-08-02 and
// last week ran 2026-07-26 through 2026-08-01, offsets 11 back to 5.
const (
	thisWeek = "2026-08-02"
	lastWeek = "2026-07-26"
)

// offsets returns every offset back from today from first down to last.
func offsets(first, last int) []int {
	var out []int
	for o := first; o >= last; o-- {
		out = append(out, o)
	}
	return out
}

func TestWeekScoreCountsTheWeekSoFar(t *testing.T) {
	h := habit("read", 3, 2) // Monday and Tuesday
	got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}).ThisWeek

	// Sunday through Thursday is five days due, today included.
	if got.Start != thisWeek || got.Done != 2 || got.Of != 5 {
		t.Errorf("this week = %+v, want 2 of 5 from %s", got, thisWeek)
	}
	if got.Percent() != 40 {
		t.Errorf("Percent = %d, want 40", got.Percent())
	}
}

func TestWeekScoreAddsUpEveryHabit(t *testing.T) {
	a := habit("read", 0, 1)
	b := habit("meditate", 0)
	b.ID = 2
	got := BuildWeekly([]*model.Habit{a, b}, today, model.Sabbath{}).ThisWeek
	if got.Done != 3 || got.Of != 10 {
		t.Errorf("this week = %d of %d, want 3 of 10", got.Done, got.Of)
	}
}

func TestWeekScoreLeavesOutRestAndSkippedDays(t *testing.T) {
	h := skipped(habit("read", 3, 2, 1), 0) // Monday to Wednesday, Thursday off
	got := BuildWeekly([]*model.Habit{h}, today, sundaySabbath).ThisWeek
	if got.Done != 3 || got.Of != 3 {
		t.Errorf("this week = %d of %d, want 3 of 3: Sunday rests and Thursday was skipped", got.Done, got.Of)
	}
}

func TestWeekScoreCountsFromTheDayAHabitStarted(t *testing.T) {
	h := startedOn(habit("read", 1, 0), 1)
	got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}).ThisWeek
	if got.Done != 2 || got.Of != 2 {
		t.Errorf("this week = %d of %d, want 2 of 2 for a habit added yesterday", got.Done, got.Of)
	}
}

func TestWeeklyLastWeekIsTheWholePreviousWeek(t *testing.T) {
	h := habit("read", 7, 6, 2)
	got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}).LastWeek
	if got.Start != lastWeek || got.Done != 2 || got.Of != 7 {
		t.Errorf("last week = %+v, want 2 of 7 from %s", got, lastWeek)
	}
}

// The week in progress can be perfect after a single day, so it does not get
// to claim the best score until it is over.
func TestWeeklyBestIgnoresTheWeekInProgress(t *testing.T) {
	h := habit("read", append(offsets(4, 0), 6, 7)...)
	got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{})
	if got.ThisWeek.Percent() != 100 {
		t.Fatalf("this week = %d%%, want a perfect week so far", got.ThisWeek.Percent())
	}
	if got.Best == nil || got.Best.Start != lastWeek {
		t.Errorf("Best = %+v, want last week", got.Best)
	}
}

func TestWeeklyBestPrefersTheMostRecentTie(t *testing.T) {
	// The weeks of 2026-07-12 and 2026-07-26 are both perfect.
	done := append(offsets(25, 19), offsets(11, 5)...)
	h := habit("read", done...)
	got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}).Best
	if got == nil || got.Start != lastWeek || got.Percent() != 100 {
		t.Errorf("Best = %+v, want the later perfect week %s", got, lastWeek)
	}
}

func TestWeeklyBestSkipsWeeksWithNothingDue(t *testing.T) {
	h := skipped(habit("read", 18), offsets(11, 5)...) // all of last week off
	got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}).Best
	if got == nil || got.Start != "2026-07-19" {
		t.Errorf("Best = %+v, want the week of 2026-07-19 rather than a week off", got)
	}
}

func TestWeeklyHasNoBestBeforeAWeekHasFinished(t *testing.T) {
	h := startedOn(habit("read", 0), 2)
	if got := BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}).Best; got != nil {
		t.Errorf("Best = %+v, want none in a habit's first week", got)
	}
}

func TestWeeklyLine(t *testing.T) {
	h := habit("read", 0, 1, 6, 7, 8, 9, 10, 11) // last week 6 of 7, this week 2 of 5
	got := weeklyLine(BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}), false)
	want := "This week 40%   Last week 86%   Best 86% (week of 2026-07-26)"
	if got != want {
		t.Errorf("weeklyLine =\n%q\nwant\n%q", got, want)
	}
}

func TestWeeklyLineWithNothingToScore(t *testing.T) {
	h := startedOn(habit("read"), 0)
	got := weeklyLine(BuildWeekly([]*model.Habit{h}, today, model.Sabbath{}), false)
	want := "This week 0%   Last week –   Best –"
	if got != want {
		t.Errorf("weeklyLine =\n%q\nwant\n%q", got, want)
	}
}

func TestListShowsTheWeeklyScoreUnderTheDailyOne(t *testing.T) {
	habits := []*model.Habit{habit("read", 0, 6)}
	weekly := BuildWeekly(habits, today, model.Sabbath{})

	var buf bytes.Buffer
	List(&buf, BuildRows(habits, today, model.Sabbath{}), today, today, &weekly, false)
	lines := strings.Split(buf.String(), "\n")
	if !strings.Contains(lines[0], "1 of 1 today") || !strings.HasPrefix(lines[1], " This week") {
		t.Errorf("want the daily score then the weekly one, got:\n%s", buf.String())
	}

	buf.Reset()
	List(&buf, BuildRows(habits, today, model.Sabbath{}), today, today, nil, false)
	if strings.Contains(buf.String(), "This week") {
		t.Errorf("weekly score shown without being asked for:\n%s", buf.String())
	}
}

func TestBuildCalScoresEachWeekShown(t *testing.T) {
	h := habit("read", 0, 6, 13)
	cal := BuildCal([]*model.Habit{h}, today, 3, model.Sabbath{})
	if len(cal.Weeks) != 3 {
		t.Fatalf("Weeks = %d, want one per column", len(cal.Weeks))
	}
	for i, want := range []string{"2026-07-19", lastWeek, thisWeek} {
		if cal.Weeks[i].Start != want {
			t.Errorf("week %d starts %s, want %s", i, cal.Weeks[i].Start, want)
		}
	}
	if cal.Weeks[2] != cal.ThisWeek {
		t.Errorf("last column %+v disagrees with this week %+v", cal.Weeks[2], cal.ThisWeek)
	}
}

// calLines renders a calendar and splits it into lines.
func calLines(t *testing.T, cal Calendar) []string {
	t.Helper()
	var buf bytes.Buffer
	Cal(&buf, cal, "read", false)
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// weekRow returns the line of week scores from Cal output.
func weekRow(lines []string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, "%") {
			return l
		}
	}
	return ""
}

func TestCalPrintsEachWeeksScoreUnderItsColumn(t *testing.T) {
	h := habit("read", 0, 1, 5, 6, 7) // last week 3 of 7, this week 2 of 5
	lines := calLines(t, BuildCal([]*model.Habit{h}, today, 2, model.Sabbath{}))

	row := weekRow(lines)
	if got := strings.Fields(row); strings.Join(got, " ") != "% 43 40" {
		t.Fatalf("week row = %q, want 43 then 40", row)
	}
	// Each score ends in the column its week's glyphs are drawn in.
	saturday, thursday := []rune(lines[2+6]), []rune(lines[2+4])
	if end := len([]rune(strings.TrimRight(row, " "))) - 1; end != len(thursday)-1 {
		t.Errorf("this week's score ends at column %d, its glyphs at %d:\n%s\n%s", end, len(thursday)-1, lines[2+4], row)
	}
	if at := strings.Index(row, "43") + 1; at != len(saturday)-1 {
		t.Errorf("last week's score ends at column %d, its glyphs at %d:\n%s\n%s", at, len(saturday)-1, lines[2+6], row)
	}
}

// A perfect week is three digits wide, and every column widens to keep it
// clear of its neighbours.
func TestCalWidensColumnsForAPerfectWeek(t *testing.T) {
	h := habit("read", append(offsets(11, 5), 0, 1)...)
	lines := calLines(t, BuildCal([]*model.Habit{h}, today, 2, model.Sabbath{}))
	if got := strings.Fields(weekRow(lines)); strings.Join(got, " ") != "% 100 40" {
		t.Errorf("week row = %q, want 100 then 40", weekRow(lines))
	}
	if sat := lines[2+6]; sat != "Sat   "+Glyphs[4] {
		t.Errorf("Saturday row = %q, want the glyph under the last digit of 100", sat)
	}
}

// Months starting in neighbouring columns sit one column apart, which in a
// widened grid is exactly a label's width. The later one gives way rather
// than run into the first.
func TestCalMonthLabelsStayApartInWideColumns(t *testing.T) {
	h := habit("read", 0, 1) // this week 40%, so columns are three wide
	lines := calLines(t, BuildCal([]*model.Habit{h}, today, 3, model.Sabbath{}))
	if header := lines[1]; strings.Contains(header, "JulAug") || !strings.Contains(header, "Jul") {
		t.Errorf("month labels = %q, want Jul alone with no label run into it", header)
	}
}

func TestCalWeekRowMarksUntrackedAndRestingWeeks(t *testing.T) {
	// Started the week of 2026-07-19 with one day done, then took last week off.
	h := skipped(startedOn(habit("read", 13), 13), offsets(11, 5)...)
	lines := calLines(t, BuildCal([]*model.Habit{h}, today, 4, model.Sabbath{}))
	row := weekRow(lines)
	if got := strings.Fields(row); strings.Join(got, " ") != "% 50 – 0" {
		t.Errorf("week row = %q, want blank before the start, then 50, a rest week and 0", row)
	}
	if !strings.HasPrefix(row, "%     ") {
		t.Errorf("week row = %q, want the week before the habit started left blank", row)
	}
}

func TestCalShowsTheWeeklySummary(t *testing.T) {
	h := habit("read", 0, 1, 6, 7, 8, 9, 10, 11)
	lines := calLines(t, BuildCal([]*model.Habit{h}, today, 2, model.Sabbath{}))
	last := lines[len(lines)-1]
	if last != "This week 40%   Last week 86%   Best 86% (week of 2026-07-26)" {
		t.Errorf("summary = %q", last)
	}
	if lines[len(lines)-2] != "" {
		t.Errorf("want a blank line between the legend and the summary, got %q", lines[len(lines)-2])
	}
}
