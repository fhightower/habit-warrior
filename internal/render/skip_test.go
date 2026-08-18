package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/model"
)

// skipped marks days off on a habit, counting back from today.
func skipped(h *model.Habit, offsets ...int) *model.Habit {
	for _, o := range offsets {
		h.Skip(today.Add(-o))
	}
	return h
}

func TestBuildRowsMarksASkippedHabitAsResting(t *testing.T) {
	off := skipped(habit("read"), 0)
	on := habit("meditate")
	on.ID = 2

	rows := BuildRows([]*model.Habit{off, on}, today, model.Sabbath{})
	if !rows[0].RestToday {
		t.Error("skipped habit: RestToday = false, want true")
	}
	if rows[1].RestToday {
		t.Error("ordinary habit: RestToday = true, want false")
	}
}

func TestListSinksASkippedHabitBelowTheUnfinishedOnes(t *testing.T) {
	// A day off is not work you owe, so it belongs with the settled habits even
	// though it carries the same bare mark as one you have yet to get to.
	off := skipped(habit("read"), 0)
	pending := habit("meditate")
	pending.ID = 2
	done := habit("stretch", 0)
	done.ID = 3

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{off, pending, done}, today, model.Sabbath{}), today, today, false)

	got := strings.Join(habitOrder(buf.String()), ",")
	if want := "meditate,read,stretch"; got != want {
		t.Errorf("order = %q, want %q: a skipped habit is resting, not behind", got, want)
	}
}

func TestListDrawsSkippedHabitWithTheRestGlyph(t *testing.T) {
	off := skipped(habit("read"), 0)
	on := habit("meditate", 0)
	on.ID = 2

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{off, on}, today, model.Sabbath{}), today, today, false)
	lines := tableLines(buf.String())
	if len(lines) != 3 {
		t.Fatalf("want a header and two rows, got:\n%s", buf.String())
	}
	if !strings.Contains(lines[1], RestGlyph) {
		t.Errorf("skipped row = %q, want the rest glyph", lines[1])
	}
	if !strings.Contains(lines[2], "✓") {
		t.Errorf("done row = %q", lines[2])
	}
}

// A habit taken off today is not due, so it cannot be behind: it leaves the
// denominator rather than counting as an unfinished one.
func TestScoreDropsRestingHabitsFromTheDenominator(t *testing.T) {
	off := skipped(habit("read"), 0)
	done := habit("meditate", 0)
	done.ID = 2
	pending := habit("stretch")
	pending.ID = 3

	s := BuildScore(BuildRows([]*model.Habit{off, done, pending}, today, model.Sabbath{}))
	if s.Done != 1 || s.Total != 2 {
		t.Errorf("Score = %d of %d, want 1 of 2", s.Done, s.Total)
	}
	if s.Rest {
		t.Error("Rest = true with a habit still due")
	}
}

func TestScoreIsRestOnlyWhenEveryHabitRests(t *testing.T) {
	a := skipped(habit("read"), 0)
	b := skipped(habit("meditate"), 0)
	b.ID = 2

	if s := BuildScore(BuildRows([]*model.Habit{a, b}, today, model.Sabbath{})); !s.Rest {
		t.Error("Rest = false with every habit skipped")
	}

	// One habit still due is an ordinary day, however the rows are ordered.
	due := habit("stretch")
	due.ID = 3
	for _, order := range [][]*model.Habit{{a, b, due}, {due, a, b}, {a, due, b}} {
		if s := BuildScore(BuildRows(order, today, model.Sabbath{})); s.Rest {
			t.Error("Rest = true with a habit still due")
		}
	}
}

// Doing a habit on a day off counts, and the total has to make room for it or
// the score reads as more done than there were habits.
func TestScoreCountsAHabitDoneOnItsDayOff(t *testing.T) {
	sab := model.Sabbath{Day: today.Weekday(), On: true}
	worked := habit("read", 0) // done on the sabbath anyway
	rested := habit("meditate")
	rested.ID = 2

	s := BuildScore(BuildRows([]*model.Habit{worked, rested}, today, sab))
	if s.Done != 1 || s.Total != 1 {
		t.Errorf("Score = %d of %d, want 1 of 1: only the day that was worked counts", s.Done, s.Total)
	}
	if s.Percent() != 100 {
		t.Errorf("Percent = %d, want 100", s.Percent())
	}
	if !s.Rest {
		t.Error("Rest = false on the sabbath")
	}
}

// The calendar aggregates habits, so a day's denominator is how many of them
// were actually due that day rather than how many exist.
func TestCalCountsOnlyTheHabitsDueThatDay(t *testing.T) {
	off := skipped(habit("read"), 1)
	done := habit("meditate", 1)
	done.ID = 2

	cal := BuildCal([]*model.Habit{off, done}, today, 2, model.Sabbath{})
	byDate := map[string]CalDay{}
	for _, d := range cal.Days {
		byDate[d.Date] = d
	}

	yesterday := byDate[today.Add(-1).String()]
	if yesterday.Count != 1 || yesterday.Of != 1 {
		t.Errorf("yesterday = %d of %d, want 1 of 1: one habit was off", yesterday.Count, yesterday.Of)
	}
	if yesterday.Rest {
		t.Error("yesterday marked as rest with a habit still due")
	}

	now := byDate[today.String()]
	if now.Of != 2 {
		t.Errorf("today = %d of %d, want both habits due", now.Count, now.Of)
	}
}

func TestCalMarksADayNoHabitWasDue(t *testing.T) {
	a := skipped(habit("read"), 1)
	b := skipped(habit("meditate"), 1)
	b.ID = 2

	cal := BuildCal([]*model.Habit{a, b}, today, 2, model.Sabbath{})
	for _, d := range cal.Days {
		if d.Date != today.Add(-1).String() {
			continue
		}
		if !d.Rest || d.Of != 0 {
			t.Errorf("day off = rest %v, of %d, want rest with nothing due", d.Rest, d.Of)
		}
	}

	var buf bytes.Buffer
	Cal(&buf, cal, "", false)
	if !strings.Contains(buf.String(), RestGlyph) {
		t.Errorf("calendar drew no rest glyph:\n%s", buf.String())
	}
}

func TestStatsShowsSkippedDays(t *testing.T) {
	h := skipped(habit("read", 2), 0, 1)

	var buf bytes.Buffer
	Stats(&buf, h.Stats(today, model.Sabbath{}), false)
	out := buf.String()
	if !strings.Contains(out, "Skipped") || !strings.Contains(out, "2 days") {
		t.Errorf("stats did not report skipped days:\n%s", out)
	}

	// Nothing skipped, nothing to say.
	buf.Reset()
	Stats(&buf, habit("meditate", 0).Stats(today, model.Sabbath{}), false)
	if strings.Contains(buf.String(), "Skipped") {
		t.Errorf("stats mentioned skips for a habit with none:\n%s", buf.String())
	}
}
