package model

import (
	"testing"
	"time"

	"github.com/fhightower/habit-warrior/internal/hdate"
)

// sunday and the days around it, for readable rest assertions.
var (
	sunday   = hdate.New(2026, time.August, 2)
	saturday = sunday.Add(-1)
	monday   = sunday.Add(1)
)

func TestSabbathZeroValueRestsNever(t *testing.T) {
	var s Sabbath
	for _, d := range []hdate.Date{saturday, sunday, monday} {
		if s.Rest(d) {
			t.Errorf("zero Sabbath rests on %s", d)
		}
	}
}

func TestSabbathRestsOnItsWeekdayOnly(t *testing.T) {
	s := Sabbath{Day: time.Sunday, On: true}
	if !s.Rest(sunday) {
		t.Error("Sunday sabbath does not rest on Sunday")
	}
	for _, d := range []hdate.Date{saturday, monday, sunday.Add(7).Add(-1)} {
		if s.Rest(d) {
			t.Errorf("Sunday sabbath rests on %s (%s)", d, d.Weekday())
		}
	}
	if !s.Rest(sunday.Add(7)) {
		t.Error("Sunday sabbath does not rest on the following Sunday")
	}
}

func TestStoreSabbathDefaultsToSundayOn(t *testing.T) {
	s := NewStore()
	got := s.Sabbath()
	if !got.On || got.Day != time.Sunday {
		t.Errorf("Sabbath() = %+v, want Sunday on", got)
	}
}

func TestStoreSabbathReadsConfig(t *testing.T) {
	cases := []struct {
		value   string
		wantOn  bool
		wantDay time.Weekday
	}{
		{"sunday", true, time.Sunday},
		{"saturday", true, time.Saturday},
		{"WED", true, time.Wednesday},
		{"none", false, 0},
		{"off", false, 0},
		{"never", false, 0},
		{"", true, time.Sunday},        // unset falls back to the default
		{"someday", true, time.Sunday}, // hand-edited garbage degrades, never fails
	}
	for _, c := range cases {
		s := NewStore()
		v := c.value
		s.Config = &Config{Sabbath: &v}
		got := s.Sabbath()
		if got.On != c.wantOn {
			t.Errorf("Sabbath(%q).On = %v, want %v", c.value, got.On, c.wantOn)
			continue
		}
		if got.On && got.Day != c.wantDay {
			t.Errorf("Sabbath(%q).Day = %s, want %s", c.value, got.Day, c.wantDay)
		}
	}
}

func TestSetSabbathCanonicalizes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sat", "saturday"},
		{"Sunday", "sunday"},
		{"off", "none"},
		{"never", "none"},
		{"none", "none"},
		{"on", "sunday"}, // re-enable at the default day
	}
	for _, c := range cases {
		s := NewStore()
		if err := s.SetSabbath(c.in); err != nil {
			t.Errorf("SetSabbath(%q): %v", c.in, err)
			continue
		}
		if s.Config == nil || s.Config.Sabbath == nil {
			t.Errorf("SetSabbath(%q) left the config unset", c.in)
			continue
		}
		if got := *s.Config.Sabbath; got != c.want {
			t.Errorf("SetSabbath(%q) stored %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSetSabbathRejectsNonsense(t *testing.T) {
	for _, bad := range []string{"", "someday", "yesterday", "2026-08-02"} {
		s := NewStore()
		if err := s.SetSabbath(bad); err == nil {
			t.Errorf("SetSabbath(%q) was accepted", bad)
		}
	}
}

// sundaySabbath is the default rest day, used by the bridging tests.
var sundaySabbath = Sabbath{Day: time.Sunday, On: true}

// habitDoneBack builds a habit completed on the given offsets back from anchor.
func habitDoneBack(anchor hdate.Date, offsets ...int) *Habit {
	h := &Habit{ID: 1, Name: "test", Created: anchor.Add(-365).String(), Done: []string{}}
	for _, o := range offsets {
		h.MarkDone(anchor.Add(-o))
	}
	return h
}

// Offsets back from Thursday 2026-08-06: 3 is Monday, 4 is the Sunday rest
// day, 5 is Saturday.
func TestCurrentStreakBridgesRestDay(t *testing.T) {
	cases := []struct {
		name       string
		anchor     hdate.Date
		offsets    []int
		sabbath    Sabbath
		want       int
		wantAtRisk bool
	}{
		{"rest day skipped, not counted", today, []int{0, 1, 2, 3, 5}, sundaySabbath, 5, false},
		{"same run without sabbath stops at the gap", today, []int{0, 1, 2, 3, 5}, Sabbath{}, 4, false},
		{"rest day done anyway counts", today, []int{0, 1, 2, 3, 4, 5}, sundaySabbath, 6, false},
		{"bridged run is still at risk when today is missed", today, []int{1, 2, 3, 5}, sundaySabbath, 4, true},
		{"two missed days still break the run", today, []int{0, 3, 5}, sundaySabbath, 1, false},
		{"today is the rest day and undone, not at risk", sunday, []int{1, 2}, sundaySabbath, 2, false},
		{"today is the rest day and done", sunday, []int{0, 1}, sundaySabbath, 2, false},
		{"nothing logged", today, nil, sundaySabbath, 0, false},
		{"only the rest day logged", today, []int{4}, sundaySabbath, 0, false},
	}

	for _, c := range cases {
		h := habitDoneBack(c.anchor, c.offsets...)
		got, atRisk := h.CurrentStreak(c.anchor, c.sabbath)
		if got != c.want || atRisk != c.wantAtRisk {
			t.Errorf("%s: CurrentStreak = (%d, %v), want (%d, %v)", c.name, got, atRisk, c.want, c.wantAtRisk)
		}
	}
}

func TestLongestStreakBridgesRestDay(t *testing.T) {
	// Friday, Saturday, Monday, Tuesday around the Sunday rest day.
	h := habitDoneBack(today, 6, 5, 3, 2)
	if got := h.LongestStreak(sundaySabbath); got != 4 {
		t.Errorf("LongestStreak with sabbath = %d, want 4", got)
	}
	if got := h.LongestStreak(Sabbath{}); got != 2 {
		t.Errorf("LongestStreak without sabbath = %d, want 2", got)
	}
}

func TestLongestStreakDoesNotBridgeOrdinaryGaps(t *testing.T) {
	// Wednesday and Monday, with Tuesday missed: a one-day gap that is not a
	// rest day must not join the runs.
	h := habitDoneBack(today, 1, 3)
	if got := h.LongestStreak(sundaySabbath); got != 1 {
		t.Errorf("LongestStreak across a missed Tuesday = %d, want 1", got)
	}

	// Friday and Tuesday, three days apart across the rest day: too wide to
	// bridge even though a rest day sits inside the gap.
	wide := habitDoneBack(today, 6, 2)
	if got := wide.LongestStreak(sundaySabbath); got != 1 {
		t.Errorf("LongestStreak across a three-day gap = %d, want 1", got)
	}
}

// The 14 days ending Thursday 2026-08-06 contain two Sundays, 2026-07-26 and
// 2026-08-02.
func TestCountLastReportsEligibleDays(t *testing.T) {
	h := habitDoneBack(today, 0, 1, 2)

	done, eligible := h.CountLast(today, 14, sundaySabbath)
	if done != 3 || eligible != 12 {
		t.Errorf("CountLast = (%d, %d), want (3, 12): two rest days come out of the window", done, eligible)
	}

	if done, eligible := h.CountLast(today, 14, Sabbath{}); done != 3 || eligible != 14 {
		t.Errorf("CountLast with sabbath off = (%d, %d), want (3, 14)", done, eligible)
	}
}

func TestCountLastCountsRestDaysThatWereDone(t *testing.T) {
	// Offset 4 is Sunday 2026-08-02, done as a bonus.
	h := habitDoneBack(today, 0, 1, 2, 4)
	done, eligible := h.CountLast(today, 14, sundaySabbath)
	if done != 4 || eligible != 13 {
		t.Errorf("CountLast = (%d, %d), want (4, 13): a rest day done anyway rejoins the window", done, eligible)
	}
}

// statsHabit is created on Friday 2026-07-31, six days before today.
func statsHabit(offsets ...int) *Habit {
	h := &Habit{ID: 1, Name: "test", Created: today.Add(-6).String(), Done: []string{}}
	for _, o := range offsets {
		h.MarkDone(today.Add(-o))
	}
	return h
}

func TestStatsExcludesRestDaysFromTheRate(t *testing.T) {
	// Every day since Friday except the Sunday rest day.
	h := statsHabit(6, 5, 3, 2, 1, 0)

	s := h.Stats(today, sundaySabbath)
	if s.TrackedDays != 6 || s.Total != 6 || s.Rate != 1 {
		t.Errorf("Stats = %d of %d (%.2f), want 6 of 6 (1.00): resting must not cost anything",
			s.Total, s.TrackedDays, s.Rate)
	}

	off := h.Stats(today, Sabbath{})
	if off.TrackedDays != 7 || off.Rate == 1 {
		t.Errorf("Stats with sabbath off = %d of %d (%.2f), want 6 of 7", off.Total, off.TrackedDays, off.Rate)
	}
}

func TestStatsRateCannotExceedFullWithBonusRestDays(t *testing.T) {
	// Every day since Friday, the Sunday rest day included.
	h := statsHabit(6, 5, 4, 3, 2, 1, 0)
	s := h.Stats(today, sundaySabbath)
	if s.Total != 7 || s.TrackedDays != 7 || s.Rate != 1 {
		t.Errorf("Stats = %d of %d (%.2f), want 7 of 7 (1.00)", s.Total, s.TrackedDays, s.Rate)
	}
}

func TestStatsReportsTheRestDay(t *testing.T) {
	h := statsHabit(0)
	if got := h.Stats(today, sundaySabbath).Sabbath; got != "Sunday" {
		t.Errorf("Stats.Sabbath = %q, want Sunday", got)
	}
	if got := h.Stats(today, Sabbath{}).Sabbath; got != "" {
		t.Errorf("Stats.Sabbath with sabbath off = %q, want empty", got)
	}
}

func TestStatsRestDayTodayIsNotAMiss(t *testing.T) {
	// Today is the rest day, and it has not been done.
	h := &Habit{ID: 1, Name: "test", Created: sunday.Add(-6).String(), Done: []string{}}
	h.MarkDone(sunday.Add(-1))
	s := h.Stats(sunday, sundaySabbath)
	if s.RestToday != true {
		t.Error("Stats.RestToday = false on the rest day")
	}
	if s.AtRisk {
		t.Error("Stats.AtRisk = true on a rest day: there is nothing left to do")
	}
}

func TestSabbathString(t *testing.T) {
	if got := (Sabbath{Day: time.Saturday, On: true}).String(); got != "Saturday" {
		t.Errorf("String() = %q, want Saturday", got)
	}
	if got := (Sabbath{}).String(); got != "off" {
		t.Errorf("String() = %q, want off", got)
	}
}
