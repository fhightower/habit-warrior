package render

import (
	"fmt"
	"strconv"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/model"
)

// WeekScore is one week's score: how many habit-days were done out of how many
// were due, Sunday through Saturday. The week in progress counts through today,
// today included, the way the daily score counts the day so far.
type WeekScore struct {
	// Start is the Sunday the week opens on.
	Start string `json:"start"`
	Done  int    `json:"done"`
	// Of is how many habit-days were due, which is what Done is out of. Rest
	// days left undone are not due, and neither is any day before a habit
	// started, so a week with nothing due has an Of of zero.
	Of int `json:"of"`
}

// Percent is the share of the week's due habit-days that were done, rounded
// the way the daily score is: never all the way to either end.
func (w WeekScore) Percent() int { return Score{Done: w.Done, Total: w.Of}.Percent() }

// Weekly is the weekly score at a glance.
type Weekly struct {
	ThisWeek WeekScore `json:"this_week"`
	LastWeek WeekScore `json:"last_week"`
	// Best is the finished week with the highest score, the most recent on a
	// tie. The week in progress is left out, since one good day would otherwise
	// claim it. Nil until a finished week has had something due.
	Best *WeekScore `json:"best"`
}

// sundayOf is the Sunday that opens d's week.
func sundayOf(d hdate.Date) hdate.Date { return d.Add(-int(d.Weekday())) }

// firstStart is the earliest day any of the habits counts, or today when there
// are none to count.
func firstStart(habits []*model.Habit, today hdate.Date) hdate.Date {
	since := today
	for _, h := range habits {
		if s := h.Since(); s.Before(since) {
			since = s
		}
	}
	return since
}

// scoreWeek scores the habits over the week opening on start, looking no
// further forward than today.
func scoreWeek(habits []*model.Habit, start, today hdate.Date, sab model.Sabbath) WeekScore {
	w := WeekScore{Start: start.String()}
	for d := start; d.Before(start.Add(7)) && !d.After(today); d = d.Add(1) {
		for _, h := range habits {
			if !h.Due(d, sab) {
				continue
			}
			w.Of++
			if h.IsDone(d) {
				w.Done++
			}
		}
	}
	return w
}

// BuildWeekly scores this week and last, and finds the best finished week in
// the habits' whole history.
func BuildWeekly(habits []*model.Habit, today hdate.Date, sab model.Sabbath) Weekly {
	this := sundayOf(today)
	wk := Weekly{
		ThisWeek: scoreWeek(habits, this, today, sab),
		LastWeek: scoreWeek(habits, this.Add(-7), today, sab),
	}
	for s := sundayOf(firstStart(habits, today)); s.Before(this); s = s.Add(7) {
		w := scoreWeek(habits, s, today, sab)
		if w.Of == 0 {
			continue
		}
		// Weeks are walked oldest first, so letting an equal score replace the
		// best so far leaves the most recent of a tie. The ratios are compared
		// exactly rather than as rounded percentages.
		if wk.Best == nil || w.Done*wk.Best.Of >= wk.Best.Done*w.Of {
			wk.Best = &w
		}
	}
	return wk
}

// weekPercent is a week's score as a percentage, or a dash for a week nothing
// was due, where a percentage would read as a failing grade.
func weekPercent(w WeekScore, color bool) string {
	if w.Of == 0 {
		return paint(RestGlyph, dim, color)
	}
	pct := w.Percent()
	return paint(strconv.Itoa(pct)+"%", shade(pct), color)
}

// weeklyLine renders the weekly summary: the week so far, last week, and the
// best finished week along with when it was.
func weeklyLine(wk Weekly, color bool) string {
	best := paint(RestGlyph, dim, color)
	if wk.Best != nil {
		best = weekPercent(*wk.Best, color) + " " +
			paint(fmt.Sprintf("(week of %s)", wk.Best.Start), dim, color)
	}
	return fmt.Sprintf("%s %s   %s %s   %s %s",
		paint("This week", dim, color), weekPercent(wk.ThisWeek, color),
		paint("Last week", dim, color), weekPercent(wk.LastWeek, color),
		paint("Best", dim, color), best)
}
