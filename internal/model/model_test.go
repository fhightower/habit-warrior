package model

import (
	"testing"
	"time"

	"github.com/fhightower/habit-warrior/internal/hdate"
)

var today = hdate.New(2026, time.August, 6)

// habitDoneOn builds a habit completed on the given offsets back from today.
func habitDoneOn(offsets ...int) *Habit {
	h := &Habit{ID: 1, Name: "test", Created: today.Add(-365).String(), Done: []string{}}
	for _, o := range offsets {
		h.MarkDone(today.Add(-o))
	}
	return h
}

func TestMarkDoneKeepsDatesSortedAndUnique(t *testing.T) {
	h := habitDoneOn(5, 1, 3, 1, 0)
	want := []string{"2026-08-01", "2026-08-03", "2026-08-05", "2026-08-06"}
	if len(h.Done) != len(want) {
		t.Fatalf("Done = %v, want %v", h.Done, want)
	}
	for i, w := range want {
		if h.Done[i] != w {
			t.Fatalf("Done = %v, want %v", h.Done, want)
		}
	}
	if h.MarkDone(today) {
		t.Error("MarkDone on an already-done day reported a change")
	}
}

func TestUndo(t *testing.T) {
	h := habitDoneOn(0, 1)
	if !h.Undo(today) {
		t.Fatal("Undo of a done day reported no change")
	}
	if h.IsDone(today) {
		t.Error("still done after Undo")
	}
	if h.Undo(today) {
		t.Error("Undo of a day that was not done reported a change")
	}
	if len(h.Done) != 1 {
		t.Errorf("Done = %v, want one entry left", h.Done)
	}
}

func TestCurrentStreak(t *testing.T) {
	cases := []struct {
		name       string
		offsets    []int
		want       int
		wantAtRisk bool
	}{
		{"nothing logged", nil, 0, false},
		{"today only", []int{0}, 1, false},
		{"today and back three days", []int{0, 1, 2}, 3, false},
		{"not today but yesterday, streak survives at risk", []int{1, 2, 3}, 3, true},
		{"gap before today breaks the streak", []int{0, 2, 3}, 1, false},
		{"stale streak, two days idle", []int{2, 3, 4}, 0, false},
		{"today plus an old unrelated run", []int{0, 10, 11}, 1, false},
	}

	for _, c := range cases {
		h := habitDoneOn(c.offsets...)
		got, atRisk := h.CurrentStreak(today)
		if got != c.want || atRisk != c.wantAtRisk {
			t.Errorf("%s: CurrentStreak = (%d, %v), want (%d, %v)", c.name, got, atRisk, c.want, c.wantAtRisk)
		}
	}
}

func TestLongestStreak(t *testing.T) {
	cases := []struct {
		offsets []int
		want    int
	}{
		{nil, 0},
		{[]int{0}, 1},
		{[]int{0, 1, 2}, 3},
		{[]int{0, 2, 3, 4, 7}, 3},
		{[]int{10, 11, 12, 13, 0}, 4},
	}
	for _, c := range cases {
		if got := habitDoneOn(c.offsets...).LongestStreak(); got != c.want {
			t.Errorf("LongestStreak(%v) = %d, want %d", c.offsets, got, c.want)
		}
	}
}

func TestLongestStreakAcrossMonthBoundary(t *testing.T) {
	h := &Habit{Done: []string{"2026-07-30", "2026-07-31", "2026-08-01"}}
	if got := h.LongestStreak(); got != 3 {
		t.Errorf("LongestStreak = %d, want 3 across the month boundary", got)
	}
}

func TestCountLast(t *testing.T) {
	h := habitDoneOn(0, 1, 8, 40)
	if got := h.CountLast(today, 7); got != 2 {
		t.Errorf("CountLast(7) = %d, want 2", got)
	}
	if got := h.CountLast(today, 30); got != 3 {
		t.Errorf("CountLast(30) = %d, want 3", got)
	}
	if got := h.CountLast(today, 0); got != 0 {
		t.Errorf("CountLast(0) = %d, want 0", got)
	}
	// The window includes its first day: a completion exactly 6 days back is
	// inside a 7-day window.
	edge := habitDoneOn(6)
	if got := edge.CountLast(today, 7); got != 1 {
		t.Errorf("CountLast(7) at the window edge = %d, want 1", got)
	}
	if got := habitDoneOn(7).CountLast(today, 7); got != 0 {
		t.Errorf("CountLast(7) just outside the window = %d, want 0", got)
	}
}

func TestStats(t *testing.T) {
	h := habitDoneOn(0, 1, 2)
	h.Created = today.Add(-9).String() // ten tracked days
	s := h.Stats(today)

	if s.Total != 3 || s.Current != 3 || s.Longest != 3 {
		t.Errorf("Stats totals = %+v", s)
	}
	if s.TrackedDays != 10 {
		t.Errorf("TrackedDays = %d, want 10", s.TrackedDays)
	}
	if s.Rate < 0.29 || s.Rate > 0.31 {
		t.Errorf("Rate = %v, want ~0.3", s.Rate)
	}
	if !s.DoneToday {
		t.Error("DoneToday = false, want true")
	}
	if s.BestWeekday == "" {
		t.Error("BestWeekday is empty despite completions")
	}
}

// Backfilling days from before the habit was added must widen the window the
// rate is measured over, not report more completions than days tracked.
func TestStatsCountsBackfilledDaysBeforeCreation(t *testing.T) {
	h := habitDoneOn(0, 1, 2, 3)
	h.Created = today.String() // added today, history typed in afterwards

	s := h.Stats(today)
	if s.Since != today.Add(-3).String() {
		t.Errorf("Since = %s, want the oldest completion %s", s.Since, today.Add(-3))
	}
	if s.TrackedDays != 4 {
		t.Errorf("TrackedDays = %d, want 4", s.TrackedDays)
	}
	if s.Rate != 1 {
		t.Errorf("Rate = %v, want 1", s.Rate)
	}
}

func TestStatsOnAnEmptyHabit(t *testing.T) {
	h := &Habit{Name: "fresh", Created: today.String(), Done: []string{}}
	s := h.Stats(today)
	if s.Total != 0 || s.Current != 0 || s.Longest != 0 {
		t.Errorf("Stats = %+v, want zeros", s)
	}
	if s.TrackedDays != 1 {
		t.Errorf("TrackedDays = %d, want 1 on its first day", s.TrackedDays)
	}
	if s.BestWeekday != "" {
		t.Errorf("BestWeekday = %q, want empty", s.BestWeekday)
	}
}

func TestBestWeekday(t *testing.T) {
	// Three Mondays, one Tuesday.
	h := &Habit{Done: []string{"2026-08-03", "2026-08-04", "2026-08-10", "2026-08-17"}}
	if got := h.bestWeekday(); got != "Monday" {
		t.Errorf("bestWeekday = %q, want Monday", got)
	}
}

func TestAddAssignsIDsAndRejectsDuplicates(t *testing.T) {
	s := NewStore()
	a, err := s.Add("meditate", []string{"health"}, today)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if a.ID != 1 || a.Created != today.String() {
		t.Errorf("first habit = %+v", a)
	}
	b, err := s.Add("read", nil, today)
	if err != nil || b.ID != 2 {
		t.Fatalf("second habit = %+v, err %v", b, err)
	}
	if _, err := s.Add("MEDITATE", nil, today); err == nil {
		t.Error("Add accepted a duplicate name differing only in case")
	}
	if _, err := s.Add("  ", nil, today); err == nil {
		t.Error("Add accepted a blank name")
	}
	if _, err := s.Add("42", nil, today); err == nil {
		t.Error("Add accepted a numeric name, which would collide with an ID")
	}
}

func TestIDsAreNotReused(t *testing.T) {
	s := NewStore()
	s.Add("one", nil, today)
	s.Add("two", nil, today)
	s.Delete(2)
	h, err := s.Add("three", nil, today)
	if err != nil {
		t.Fatal(err)
	}
	if h.ID != 3 {
		t.Errorf("new habit ID = %d, want 3 (IDs are never reused)", h.ID)
	}
}

func TestFind(t *testing.T) {
	s := NewStore()
	s.Add("meditate", nil, today)
	s.Add("read", nil, today)
	s.Add("read fiction", nil, today)

	cases := []struct {
		sel  string
		want string
	}{
		{"1", "meditate"},
		{"meditate", "meditate"},
		{"MEDITATE", "meditate"},
		{"med", "meditate"},         // unique prefix
		{"read", "read"},            // exact name beats the prefix collision
		{"read f", "read fiction"},  // unique prefix among the two "read" habits
		{"fiction", "read fiction"}, // unique substring
		{"ditat", "meditate"},       // substring, not a prefix
	}
	for _, c := range cases {
		h, err := s.Find(c.sel)
		if err != nil {
			t.Errorf("Find(%q): %v", c.sel, err)
			continue
		}
		if h.Name != c.want {
			t.Errorf("Find(%q) = %s, want %s", c.sel, h.Name, c.want)
		}
	}
}

func TestFindRejectsAmbiguityAndMisses(t *testing.T) {
	s := NewStore()
	s.Add("read", nil, today)
	s.Add("reading list", nil, today)

	if h, err := s.Find("rea"); err == nil {
		t.Errorf("Find(\"rea\") = %s, want an ambiguity error", h.Name)
	}
	if _, err := s.Find("swim"); err == nil {
		t.Error("Find on an unknown name did not error")
	}
	if _, err := s.Find("99"); err == nil {
		t.Error("Find on an unknown ID did not error")
	}
	if _, err := s.Find(""); err == nil {
		t.Error("Find on an empty selector did not error")
	}
}

func TestFindSeesArchivedHabits(t *testing.T) {
	s := NewStore()
	h, _ := s.Add("meditate", nil, today)
	h.Archived = true
	if _, err := s.Find("meditate"); err != nil {
		t.Errorf("archived habits must stay findable so they can be restored: %v", err)
	}
}

func TestParseArgs(t *testing.T) {
	f, rest := ParseArgs([]string{"+health", "run", "-evening", "3", "days", "ago", "--all"})
	if len(f.Include) != 1 || f.Include[0] != "health" {
		t.Errorf("Include = %v", f.Include)
	}
	if len(f.Exclude) != 1 || f.Exclude[0] != "evening" {
		t.Errorf("Exclude = %v", f.Exclude)
	}
	want := []string{"run", "3", "days", "ago", "--all"}
	if len(rest) != len(want) {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
	for i := range want {
		if rest[i] != want[i] {
			t.Fatalf("rest = %v, want %v", rest, want)
		}
	}
}

// "-3" must stay positional. Read as a tag exclusion it would turn a single
// habit command into a bulk one over everything.
func TestParseArgsLeavesNegativeNumbersPositional(t *testing.T) {
	f, rest := ParseArgs([]string{"meditate", "-3"})
	if len(f.Exclude) != 0 {
		t.Errorf("Exclude = %v, want -3 left alone", f.Exclude)
	}
	if len(rest) != 2 || rest[1] != "-3" {
		t.Errorf("rest = %v, want [meditate -3]", rest)
	}
	// A real tag that merely starts with a digit still excludes.
	f, _ = ParseArgs([]string{"-3am"})
	if len(f.Exclude) != 1 || f.Exclude[0] != "3am" {
		t.Errorf("Exclude = %v, want [3am]", f.Exclude)
	}
}

func TestParseArgsLeavesLongFlagsAlone(t *testing.T) {
	f, rest := ParseArgs([]string{"--weeks", "12"})
	if len(f.Exclude) != 0 {
		t.Errorf("a --flag was read as a tag exclusion: %v", f.Exclude)
	}
	if len(rest) != 2 {
		t.Errorf("rest = %v, want the flag and its value untouched", rest)
	}
}

func TestSelectFiltersAndOrders(t *testing.T) {
	s := NewStore()
	s.Add("meditate", []string{"health", "morning"}, today)
	s.Add("read", []string{"mind"}, today)
	s.Add("run", []string{"health", "evening"}, today)
	s.Habits[2].Archived = true

	got := s.Select(Filter{Include: []string{"health"}}, false)
	if len(got) != 1 || got[0].Name != "meditate" {
		t.Errorf("archived habits leaked into Select: %v", names(got))
	}

	got = s.Select(Filter{Include: []string{"health"}}, true)
	if len(got) != 2 {
		t.Errorf("--all Select = %v, want both health habits", names(got))
	}
	if got[0].ID > got[1].ID {
		t.Error("Select is not in ID order")
	}

	got = s.Select(Filter{Include: []string{"health"}, Exclude: []string{"evening"}}, true)
	if len(got) != 1 || got[0].Name != "meditate" {
		t.Errorf("exclusion ignored: %v", names(got))
	}

	if got := s.Select(Filter{}, false); len(got) != 2 {
		t.Errorf("empty filter = %v, want both unarchived habits", names(got))
	}
}

func TestTagEditing(t *testing.T) {
	h := &Habit{Name: "run"}
	if !h.AddTag("health") || h.AddTag("HEALTH") {
		t.Errorf("tags are not case-insensitively unique: %v", h.Tags)
	}
	h.AddTag("cardio")
	if h.Tags[0] != "cardio" {
		t.Errorf("tags are not sorted: %v", h.Tags)
	}
	if !h.RemoveTag("HEALTH") {
		t.Error("RemoveTag is case-sensitive")
	}
	if h.RemoveTag("nope") {
		t.Error("RemoveTag reported a change for a tag that was not there")
	}
}

func names(hs []*Habit) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.Name
	}
	return out
}
