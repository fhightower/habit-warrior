package model

import (
	"testing"

	"github.com/fhightower/habit-warrior/internal/hdate"
)

// skipDays marks the given offsets back from today as skipped.
func skipDays(h *Habit, offsets ...int) *Habit {
	for _, o := range offsets {
		h.Skip(today.Add(-o))
	}
	return h
}

func TestSkipKeepsDatesSortedAndUnique(t *testing.T) {
	h := skipDays(habitDoneOn(), 5, 1, 3, 1, 0)
	want := []string{"2026-08-01", "2026-08-03", "2026-08-05", "2026-08-06"}
	if len(h.Skipped) != len(want) {
		t.Fatalf("Skipped = %v, want %v", h.Skipped, want)
	}
	for i, w := range want {
		if h.Skipped[i] != w {
			t.Fatalf("Skipped = %v, want %v", h.Skipped, want)
		}
	}
	if h.Skip(today) {
		t.Error("Skip of an already-skipped day reported a change")
	}
}

func TestUnskip(t *testing.T) {
	h := skipDays(habitDoneOn(), 0, 1)
	if !h.Unskip(today) {
		t.Fatal("Unskip of a skipped day reported no change")
	}
	if h.IsSkipped(today) {
		t.Error("still skipped after Unskip")
	}
	if h.Unskip(today) {
		t.Error("Unskip of a day that was not skipped reported a change")
	}
	if len(h.Skipped) != 1 {
		t.Errorf("Skipped = %v, want one entry left", h.Skipped)
	}
}

// Rest has two sources, the weekly sabbath and a one-off skip, and every report
// asks the same question of a day rather than checking them separately.
func TestRestsCoversSabbathAndSkip(t *testing.T) {
	sab := Sabbath{Day: today.Weekday(), On: true} // today is the sabbath
	h := skipDays(habitDoneOn(), 1)

	cases := []struct {
		name string
		day  hdate.Date
		sab  Sabbath
		want bool
	}{
		{"sabbath day", today, sab, true},
		{"skipped day", today.Add(-1), sab, true},
		{"skipped day with sabbath off", today.Add(-1), Sabbath{}, true},
		{"ordinary day", today.Add(-2), sab, false},
		{"sabbath day with sabbath off", today, Sabbath{}, false},
	}
	for _, c := range cases {
		if got := h.Rests(c.day, c.sab); got != c.want {
			t.Errorf("%s: Rests(%s) = %v, want %v", c.name, c.day, got, c.want)
		}
	}
}

// Doing a habit on a day off is the truth about that day, so the skip goes.
func TestMarkDoneClearsSkip(t *testing.T) {
	h := skipDays(habitDoneOn(), 0)
	if !h.MarkDone(today) {
		t.Fatal("MarkDone on a skipped day reported no change")
	}
	if h.IsSkipped(today) {
		t.Error("day is still skipped after being marked done")
	}
	if !h.IsDone(today) {
		t.Error("day is not done after MarkDone")
	}
}

func TestCurrentStreakPassesThroughSkippedDays(t *testing.T) {
	cases := []struct {
		name       string
		done       []int
		skipped    []int
		want       int
		wantAtRisk bool
	}{
		{"skip bridges a gap", []int{0, 1, 3}, []int{2}, 3, false},
		{"a skipped run bridges a wider gap", []int{0, 1, 5}, []int{2, 3, 4}, 3, false},
		{"skipped today is not at risk", []int{1, 2}, []int{0}, 2, false},
		{"skipped today with nothing behind it", nil, []int{0}, 0, false},
		{"an undone ordinary day still breaks the run", []int{0, 1, 4}, []int{2}, 2, false},
		{"not done today, yesterday skipped, streak survives at risk", []int{2, 3}, []int{1}, 2, true},
	}

	for _, c := range cases {
		h := skipDays(habitDoneOn(c.done...), c.skipped...)
		got, atRisk := h.CurrentStreak(today, Sabbath{})
		if got != c.want || atRisk != c.wantAtRisk {
			t.Errorf("%s: CurrentStreak = (%d, %v), want (%d, %v)", c.name, got, atRisk, c.want, c.wantAtRisk)
		}
	}
}

// The sabbath bridge only ever had to span a single day. A skipped run can be
// any width, so the bridge spans whatever rests.
func TestLongestStreakBridgesSkippedRun(t *testing.T) {
	h := &Habit{Done: []string{"2026-08-01", "2026-08-02", "2026-08-06", "2026-08-07"}}
	h.Skipped = []string{"2026-08-03", "2026-08-04", "2026-08-05"}
	if got := h.LongestStreak(Sabbath{}); got != 4 {
		t.Errorf("LongestStreak = %d, want 4 across a three-day skip", got)
	}

	// One undone ordinary day inside the gap is enough to break it.
	h.Skipped = []string{"2026-08-03", "2026-08-05"}
	if got := h.LongestStreak(Sabbath{}); got != 2 {
		t.Errorf("LongestStreak = %d, want 2 when the gap is not fully rested", got)
	}
}

func TestCountLastDropsSkippedDays(t *testing.T) {
	h := skipDays(habitDoneOn(0), 1, 2)
	done, eligible := h.CountLast(today, 7, Sabbath{})
	if done != 1 || eligible != 5 {
		t.Errorf("CountLast(7) = (%d, %d), want (1, 5)", done, eligible)
	}

	// A day skipped and then done anyway counts on both sides, so done can never
	// come out above eligible.
	h2 := skipDays(habitDoneOn(), 0)
	h2.MarkDone(today)
	done, eligible = h2.CountLast(today, 7, Sabbath{})
	if done != 1 || eligible != 7 {
		t.Errorf("CountLast(7) after doing a skipped day = (%d, %d), want (1, 7)", done, eligible)
	}
}

func TestStatsCountsSkippedDays(t *testing.T) {
	h := habitDoneOn(3, 4)
	h.Created = today.Add(-4).String()
	skipDays(h, 0, 1, 2)

	s := h.Stats(today, Sabbath{})
	if s.Skipped != 3 {
		t.Errorf("Skipped = %d, want 3", s.Skipped)
	}
	// Five days since tracking began, three of them skipped.
	if s.TrackedDays != 2 {
		t.Errorf("TrackedDays = %d, want 2", s.TrackedDays)
	}
	if s.Rate != 1 {
		t.Errorf("Rate = %v, want 1: every day that counted was done", s.Rate)
	}
	if !s.RestToday {
		t.Error("RestToday = false, want true: today is skipped")
	}
}

// A sabbath and a skip on the same day is not a contradiction, and neither is
// counted twice.
func TestSkipOnTheSabbathIsHarmless(t *testing.T) {
	sab := Sabbath{Day: today.Add(-1).Weekday(), On: true}
	h := skipDays(habitDoneOn(0), 1)
	if got, _ := h.CurrentStreak(today, sab); got != 1 {
		t.Errorf("CurrentStreak = %d, want 1", got)
	}
	_, eligible := h.CountLast(today, 7, sab)
	if eligible != 6 {
		t.Errorf("eligible = %d, want 6: the doubly-rested day comes out once", eligible)
	}
}

// A hand-edited file can mark a day both done and skipped. Done is the stronger
// claim: the work happened.
func TestDoneWinsOverSkipOnTheSameDay(t *testing.T) {
	h := &Habit{
		Created: today.Add(-1).String(),
		Done:    []string{today.String()},
		Skipped: []string{today.String()},
	}
	if got, _ := h.CurrentStreak(today, Sabbath{}); got != 1 {
		t.Errorf("CurrentStreak = %d, want 1", got)
	}
	done, eligible := h.CountLast(today, 1, Sabbath{})
	if done != 1 || eligible != 1 {
		t.Errorf("CountLast(1) = (%d, %d), want (1, 1)", done, eligible)
	}
}
