package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/model"
)

// startedOn sets the day a habit was added, counting back from today.
func startedOn(h *model.Habit, offset int) *model.Habit {
	h.Created = today.Add(-offset).String()
	return h
}

func calDays(cal Calendar) map[string]CalDay {
	byDate := map[string]CalDay{}
	for _, d := range cal.Days {
		byDate[d.Date] = d
	}
	return byDate
}

// A day before a habit was added is not one it could have missed, and not one
// it rested on either: it simply was not being tracked yet.
func TestBuildCalLeavesOutDaysBeforeAHabitStarted(t *testing.T) {
	h := startedOn(habit("read"), 2)
	cal := BuildCal([]*model.Habit{h}, today, 2, model.Sabbath{})

	if cal.Since != today.Add(-2).String() {
		t.Errorf("Since = %s, want the day the habit was added %s", cal.Since, today.Add(-2))
	}
	days := calDays(cal)
	before := days[today.Add(-3).String()]
	if before.Of != 0 || before.Rest {
		t.Errorf("day before the start = %d of %d, rest %v; want nothing due and no rest", before.Count, before.Of, before.Rest)
	}
	if first := days[today.Add(-2).String()]; first.Of != 1 {
		t.Errorf("first day = %d of %d, want the habit due", first.Count, first.Of)
	}
}

func TestBuildCalCountsEachHabitFromItsOwnStart(t *testing.T) {
	old := habit("meditate", 3)
	fresh := startedOn(habit("read"), 1)
	fresh.ID = 2

	days := calDays(BuildCal([]*model.Habit{old, fresh}, today, 1, model.Sabbath{}))
	if d := days[today.Add(-3).String()]; d.Count != 1 || d.Of != 1 {
		t.Errorf("before read started = %d of %d, want 1 of 1", d.Count, d.Of)
	}
	if d := days[today.String()]; d.Of != 2 {
		t.Errorf("today = %d of %d, want both habits due", d.Count, d.Of)
	}
}

// The calendar starts where the oldest habit does, which backfilling can
// move earlier than any habit was added.
func TestBuildCalSinceFollowsABackfilledCompletion(t *testing.T) {
	h := startedOn(habit("read", 4), 1)
	cal := BuildCal([]*model.Habit{h}, today, 1, model.Sabbath{})
	if cal.Since != today.Add(-4).String() {
		t.Errorf("Since = %s, want the backfilled day %s", cal.Since, today.Add(-4))
	}
}

// Untracked days are drawn blank, like the days still to come: a miss glyph
// would read as failure and a rest glyph as a day off, and neither happened.
func TestCalLeavesDaysBeforeTrackingBlank(t *testing.T) {
	h := startedOn(habit("read", 0), 0) // added Thursday, done the same day
	var buf bytes.Buffer
	Cal(&buf, BuildCal([]*model.Habit{h}, today, 1, sundaySabbath), "read", false)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, day := range []string{"Sun", "Mon", "Tue", "Wed"} {
		if row := strings.TrimRight(lines[2+i], " "); row != day {
			t.Errorf("%s row = %q, want it blank before the habit started", day, row)
		}
	}
	if thu := lines[2+4]; !strings.HasSuffix(thu, Glyphs[4]) {
		t.Errorf("Thursday row = %q, want the completion drawn", thu)
	}
	if strings.Contains(buf.String(), "rest") {
		t.Errorf("legend explains a rest glyph that was never drawn:\n%s", buf.String())
	}
}
