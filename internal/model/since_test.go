package model

import "testing"

func TestSinceIsTheDayTheHabitWasAdded(t *testing.T) {
	h := habitDoneOn(0, 1)
	h.Created = today.Add(-10).String()
	if got := h.Since(); !got.Equal(today.Add(-10)) {
		t.Errorf("Since = %s, want the created date %s", got, today.Add(-10))
	}
}

// Backfilling a day older than the habit moves its start back to that day, so
// the completion is never counted against a day the habit did not exist.
func TestSinceReachesBackToABackfilledCompletion(t *testing.T) {
	h := habitDoneOn(0, 20)
	h.Created = today.Add(-10).String()
	if got := h.Since(); !got.Equal(today.Add(-20)) {
		t.Errorf("Since = %s, want the oldest completion %s", got, today.Add(-20))
	}
}

func TestDueBeforeTheHabitStartedIsFalse(t *testing.T) {
	h := habitDoneOn()
	h.Created = today.String()
	if h.Due(today.Add(-1), Sabbath{}) {
		t.Error("Due = true the day before the habit was added")
	}
	if !h.Due(today, Sabbath{}) {
		t.Error("Due = false on the day the habit was added")
	}
}

func TestDueLeavesOutRestDaysUnlessDone(t *testing.T) {
	sab := Sabbath{Day: sunday.Weekday(), On: true}
	h := habitDoneOn()
	if h.Due(sunday, sab) {
		t.Error("Due = true on the sabbath with nothing done")
	}
	if !h.Due(monday, sab) {
		t.Error("Due = false on an ordinary day")
	}

	h.MarkDone(sunday)
	if !h.Due(sunday, sab) {
		t.Error("Due = false on a sabbath that was done anyway")
	}

	h.Skip(monday)
	if h.Due(monday, sab) {
		t.Error("Due = true on a skipped day")
	}
}
