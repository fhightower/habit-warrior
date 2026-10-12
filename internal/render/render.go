// Package render turns habits into the three reports: list, calendar, stats.
package render

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/model"
)

// Glyphs is the density ramp used by the calendar, from none to full.
var Glyphs = [5]string{"·", "░", "▒", "▓", "█"}

// RestGlyph marks a weekly rest day that was not done. It is deliberately not
// part of the density ramp: a rest day is skipped, not a low score.
const RestGlyph = "–"

const (
	reset  = "\033[0m"
	dim    = "\033[2m"
	bold   = "\033[1m"
	yellow = "\033[33m"
)

// levelColors shades the density ramp from faint to full green.
var levelColors = [5]string{"\033[38;5;238m", "\033[38;5;22m", "\033[38;5;28m", "\033[38;5;34m", "\033[38;5;40m"}

func paint(s, code string, on bool) string {
	if !on || code == "" {
		return s
	}
	return code + s + reset
}

// width counts display columns, which for our glyphs equals rune count.
func width(s string) int { return utf8.RuneCountInString(s) }

// pad right-aligns or left-aligns s in n columns, measuring the uncolored text.
func pad(s string, n int, right bool) string {
	gap := n - width(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// ListRow is one line of the list report, and its JSON shape.
//
// The report is about one day, which is today unless a report was asked for a
// past one. Date names that day, and the fields whose names say today are
// about it: on a report of an earlier day, done_today means done that day.
type ListRow struct {
	ID        int      `json:"id"`
	Date      string   `json:"date"`
	Name      string   `json:"name"`
	Tags      []string `json:"tags"`
	DoneToday bool     `json:"done_today"`
	Streak    int      `json:"streak"`
	AtRisk    bool     `json:"at_risk"`
	Last30    int      `json:"last_30"`
	// Last30Of is how many of the last 30 days were eligible, which is fewer
	// than 30 once rest days are taken out.
	Last30Of int  `json:"last_30_of"`
	Archived bool `json:"archived"`
	// RestToday reports whether the habit is resting today, whether from the
	// weekly sabbath or a skip, so an unmarked habit reads as resting rather
	// than missed.
	RestToday bool `json:"rest_today"`
}

// BuildRows derives the list report from habits, as the given day stood.
//
// Every figure looks backwards from day and no further forward, so a past day
// reports what was true then rather than a truncated view of now. At risk
// keeps its meaning on a past day, since a day already gone can still be
// backfilled: it says the streak survives only if that day gets done.
func BuildRows(habits []*model.Habit, day hdate.Date, sab model.Sabbath) []ListRow {
	rows := make([]ListRow, 0, len(habits))
	for _, h := range habits {
		streak, atRisk := h.CurrentStreak(day, sab)
		tags := h.Tags
		if tags == nil {
			tags = []string{}
		}
		last30, last30of := h.CountLast(day, 30, sab)
		rows = append(rows, ListRow{
			ID:        h.ID,
			Date:      day.String(),
			Name:      h.Name,
			Tags:      tags,
			DoneToday: h.IsDone(day),
			Streak:    streak,
			AtRisk:    atRisk,
			Last30:    last30,
			Last30Of:  last30of,
			Archived:  h.Archived,
			RestToday: h.Rests(day, sab),
		})
	}
	return rows
}

// Score is the day at a glance: how many of the habits due today are done.
type Score struct {
	Done  int
	Total int
	// Rest reports that no habit is due today, when a percentage would read as
	// a failing grade rather than a day off.
	Rest bool
}

// BuildScore summarizes the rows. Archived habits are left out: they are not
// being tracked, so they cannot be behind.
//
// A habit resting today is not due and leaves the denominator, unless it was
// done anyway, which keeps a bonus day inside the total it is counted in.
func BuildScore(rows []ListRow) Score {
	var s Score
	tracked := 0
	resting := 0
	for _, r := range rows {
		if r.Archived {
			continue
		}
		tracked++
		if r.DoneToday {
			s.Done++
		}
		if r.RestToday {
			resting++
		}
		if !r.RestToday || r.DoneToday {
			s.Total++
		}
	}
	s.Rest = tracked > 0 && resting == tracked
	return s
}

// Percent is the share of today's habits completed, rounded to the nearest
// whole number but never all the way to either end: a day with anything left
// cannot show 100%, and a day with anything done cannot show 0%.
func (s Score) Percent() int {
	if s.Total <= 0 || s.Done <= 0 {
		return 0
	}
	if s.Done >= s.Total {
		return 100
	}
	p := int(math.Round(float64(s.Done) / float64(s.Total) * 100))
	if p >= 100 {
		return 99
	}
	if p <= 0 {
		return 1
	}
	return p
}

// scoreBar is how many columns the progress bar occupies.
const scoreBar = 20

// scoreLine renders the day's headline: a percentage, a bar, and the counts
// behind them. when names the day the counts are about, "today" on a report of
// today and "done" on one of an earlier day, whose date the title already gave.
func scoreLine(s Score, when string, color bool) string {
	if s.Rest {
		note := "nothing due"
		if s.Done > 0 {
			note = fmt.Sprintf("%d done anyway", s.Done)
		}
		return fmt.Sprintf(" %s  %s",
			paint("Rest day", bold, color), paint(note, dim, color))
	}

	pct := s.Percent()
	filled := s.Done * scoreBar / s.Total
	if s.Done > 0 && filled == 0 {
		filled = 1 // any progress at all should be visible
	}
	bar := paint(strings.Repeat(Glyphs[4], filled), levelColors[4], color) +
		paint(strings.Repeat(Glyphs[1], scoreBar-filled), levelColors[0], color)

	return fmt.Sprintf(" %s  %s  %s",
		paint(pad(fmt.Sprintf("%d%%", pct), 4, true), shade(pct), color),
		bar,
		paint(fmt.Sprintf("%d of %d %s", s.Done, s.Total, when), dim, color))
}

// shade colors a percentage: green when complete, yellow partway, dim at none.
func shade(pct int) string {
	switch {
	case pct == 0:
		return dim
	case pct < 100:
		return yellow
	}
	return levelColors[4]
}

// rank orders a row into one of three groups: what is still left to do today,
// what is settled, and what is no longer tracked.
//
// A habit resting today is settled rather than behind, since it is not due:
// that covers a day off it was skipped for as much as the weekly sabbath, and
// it is the same reason rest leaves the score's denominator. An archived habit
// sinks whether or not it was marked, for the same reason it is left out of
// the score entirely.
func rank(r ListRow) int {
	switch {
	case r.Archived:
		return 2
	case !r.DoneToday && !r.RestToday:
		return 0
	default:
		return 1
	}
}

// order returns the rows to print: what is left to do first, then the rest,
// each group still in the order it arrived. The caller's slice is left alone,
// so JSON consumers keep the ID ordering they were given.
func order(rows []ListRow) []ListRow {
	sorted := make([]ListRow, len(rows))
	copy(sorted, rows)
	sort.SliceStable(sorted, func(i, j int) bool { return rank(sorted[i]) < rank(sorted[j]) })
	return sorted
}

// List writes the habit table for day, which is today unless an earlier day
// was asked for. A past day is named in a title and drops the word today from
// the report, since every figure in it is about that day rather than this one.
// A weekly score, when given, follows the day's headline; nil leaves it out.
func List(w io.Writer, rows []ListRow, day, today hdate.Date, weekly *Weekly, color bool) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No habits. Add one with: hw add <name>")
		return
	}

	past := !day.Equal(today)
	when, todayCol := "today", "Today"
	if past {
		when, todayCol = "done", "Done"
		fmt.Fprintf(w, " %s\n", paint(fmt.Sprintf("%s %s", day.Weekday(), day), bold, color))
	}

	// A day where everything rests has no total to show, but is exactly the day
	// the headline most needs to say so.
	if score := BuildScore(rows); score.Total > 0 || score.Rest {
		fmt.Fprintln(w, scoreLine(score, when, color))
	}
	if weekly != nil {
		fmt.Fprintln(w, " "+weeklyLine(*weekly, color))
	}

	rows = order(rows)

	headers := []string{"ID", "Habit", "Tags", todayCol, "Streak", "Last 30"}
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		name := r.Name
		if r.Archived {
			name += " (archived)"
		}
		mark := Glyphs[0]
		switch {
		case r.DoneToday:
			mark = "✓"
		case r.RestToday:
			mark = RestGlyph
		}
		streak := fmt.Sprintf("%d", r.Streak)
		if r.AtRisk && r.Streak > 0 {
			streak += "!"
		}
		cells = append(cells, []string{
			fmt.Sprintf("%d", r.ID),
			name,
			strings.Join(r.Tags, ","),
			mark,
			streak,
			fmt.Sprintf("%d/%d (%d%%)", r.Last30, r.Last30Of,
				Score{Done: r.Last30, Total: r.Last30Of}.Percent()),
		})
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = width(h)
	}
	for _, row := range cells {
		for i, c := range row {
			if width(c) > widths[i] {
				widths[i] = width(c)
			}
		}
	}
	rightAlign := []bool{true, false, false, false, true, true}

	var head []string
	for i, h := range headers {
		head = append(head, pad(h, widths[i], rightAlign[i]))
	}
	fmt.Fprintln(w, paint(strings.TrimRight(" "+strings.Join(head, "  "), " "), bold, color))

	for i, row := range cells {
		out := make([]string, len(row))
		for j, c := range row {
			padded := pad(c, widths[j], rightAlign[j])
			switch {
			case j == 3 && rows[i].DoneToday:
				padded = paint(padded, levelColors[4], color)
			case j == 3:
				padded = paint(padded, dim, color)
			case j == 2:
				padded = paint(padded, dim, color)
			case j == 4 && rows[i].AtRisk && rows[i].Streak > 0:
				padded = paint(padded, yellow, color)
			}
			out[j] = padded
		}
		fmt.Fprintln(w, strings.TrimRight(" "+strings.Join(out, "  "), " "))
	}

	if color {
		// A past day can still be backfilled, so its streak is at risk in the
		// same sense today's is: it survives if that day gets done.
		dayWord := "today"
		if past {
			dayWord = "that day"
		}
		note := "  ! streak survives only if done " + dayWord
		// Any habit resting earns the legend: rest is now per habit, so the
		// glyph can appear on one row while the rest of the table is ordinary.
		for _, r := range rows {
			if r.RestToday {
				note += fmt.Sprintf("   %s resting %s", RestGlyph, dayWord)
				break
			}
		}
		fmt.Fprintln(w, paint(note, dim, color))
	}
}

// CalDay is one cell of the calendar: how many of the selected habits were
// done that day.
type CalDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
	// Of is how many of the habits were due that day: the ones that had started
	// and were not resting, plus any that rested and were done anyway. It is
	// the denominator behind Count, and is smaller than the habit total on a day
	// some of them had off or had not been added yet.
	Of int `json:"of"`
	// Rest marks a day no habit was due once tracking had begun, so a consumer
	// can tell an intentional gap from a missed day. Days before the calendar's
	// Since are not rest: nothing was being tracked.
	Rest bool `json:"rest,omitempty"`
}

// Calendar is the heatmap data: every day from Start (a Sunday) through End
// (today), plus the number of habits it covers and how each week scored.
type Calendar struct {
	Start string `json:"start"`
	End   string `json:"end"`
	// Since is the first day any of the habits counts. Days before it were not
	// being tracked: nothing was due on them, and nothing rested either.
	Since  string   `json:"since"`
	Habits int      `json:"habits"`
	Days   []CalDay `json:"days"`
	// Weeks scores each week shown, one per column, oldest first.
	Weeks []WeekScore `json:"weeks"`
	// Weekly is the summary under the grid. It looks at the habits' whole
	// history rather than only the weeks shown, so its best week can be one
	// the grid has scrolled past.
	Weekly

	start hdate.Date
	end   hdate.Date
	since hdate.Date
}

// BuildCal collects completions for the last weeks weeks, ending today. The
// range starts on the Sunday that begins the earliest week shown, so the grid
// has whole columns.
//
// Each habit counts from the day it started, so a day before it was added is
// neither a miss nor a rest. A day before every habit started is not rest
// either: nothing was being tracked yet.
func BuildCal(habits []*model.Habit, today hdate.Date, weeks int, sab model.Sabbath) Calendar {
	if weeks < 1 {
		weeks = 1
	}
	// Walk back to the Sunday of the current week, then back weeks-1 more.
	start := sundayOf(today).Add(-7 * (weeks - 1))
	since := firstStart(habits, today)

	cal := Calendar{
		Start:  start.String(),
		End:    today.String(),
		Since:  since.String(),
		Habits: len(habits),
		Days:   []CalDay{},
		Weeks:  []WeekScore{},
		Weekly: BuildWeekly(habits, today, sab),
		start:  start,
		end:    today,
		since:  since,
	}
	for s := start; !s.After(today); s = s.Add(7) {
		cal.Weeks = append(cal.Weeks, scoreWeek(habits, s, today, sab))
	}
	for d := start; !d.After(today); d = d.Add(1) {
		n, of := 0, 0
		for _, h := range habits {
			if h.IsDone(d) {
				n++
			}
			if h.Due(d, sab) {
				of++
			}
		}
		rest := of == 0 && !d.Before(since)
		cal.Days = append(cal.Days, CalDay{Date: d.String(), Count: n, Of: of, Rest: rest})
	}
	return cal
}

// level maps a day's count onto the density ramp.
func level(count, total int) int {
	if count <= 0 {
		return 0
	}
	if total <= 1 {
		return 4
	}
	switch {
	case count >= total:
		return 4
	case count*3 >= total*2:
		return 3
	case count*3 >= total:
		return 2
	default:
		return 1
	}
}

// Cal writes the heatmap grid.
func Cal(w io.Writer, cal Calendar, title string, color bool) {
	start, end := cal.start, cal.end
	if start.IsZero() {
		if d, err := hdate.ParseISO(cal.Start); err == nil {
			start = d
		}
	}
	if end.IsZero() {
		if d, err := hdate.ParseISO(cal.End); err == nil {
			end = d
		}
	}
	since := cal.since
	if since.IsZero() {
		since = start
		if d, err := hdate.ParseISO(cal.Since); err == nil {
			since = d
		}
	}

	counts := make(map[string]int, len(cal.Days))
	due := make(map[string]int, len(cal.Days))
	rest := make(map[string]bool, len(cal.Days))
	anyRest := false
	for _, d := range cal.Days {
		counts[d.Date] = d.Count
		due[d.Date] = d.Of
		if d.Rest {
			rest[d.Date] = true
			anyRest = true
		}
	}

	if title != "" {
		fmt.Fprintln(w, paint(title, bold, color))
	}

	cols := end.Since(start)/7 + 1
	const labelWidth = 4

	// Each week's score prints under its column: blank for a week that ended
	// before tracking began, a dash for one where nothing was due.
	scores := make([]string, cols)
	for c := 0; c < cols && c < len(cal.Weeks); c++ {
		wk := cal.Weeks[c]
		switch {
		case start.Add(c*7 + 6).Before(since):
			// Left blank, like the column above it.
		case wk.Of == 0:
			scores[c] = RestGlyph
		default:
			scores[c] = fmt.Sprintf("%d", wk.Percent())
		}
	}
	// Every column is as wide as the widest score plus a space, so neighbours
	// never run together; a glyph sits over its score's last digit.
	cellW := 2
	for _, s := range scores {
		if width(s)+1 > cellW {
			cellW = width(s) + 1
		}
	}
	lead := strings.Repeat(" ", cellW-2)

	// Month labels sit above the first column of each new month.
	months := make([]string, cols)
	prev := ""
	for c := 0; c < cols; c++ {
		colStart := start.Add(c * 7)
		m := colStart.Add(6).Month().String()[:3] // the week's midpoint month
		if m != prev {
			months[c] = m
			prev = m
		}
	}
	// Each label sits above the column its month starts in. A label is dropped
	// only when it would not leave a space after the previous one, which
	// happens when two months start within a column of each other.
	var head strings.Builder
	head.WriteString(strings.Repeat(" ", labelWidth))
	pos := 0
	for c := 0; c < cols; c++ {
		want := c*cellW + len(lead) // over the column's glyph
		if months[c] == "" || (pos > 0 && pos >= want) {
			continue
		}
		head.WriteString(strings.Repeat(" ", want-pos))
		head.WriteString(months[c])
		pos = want + width(months[c])
	}
	fmt.Fprintln(w, paint(strings.TrimRight(head.String(), " "), dim, color))

	weekdayNames := [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for row := 0; row < 7; row++ {
		var line strings.Builder
		line.WriteString(paint(pad(weekdayNames[row], labelWidth, false), dim, color))
		for c := 0; c < cols; c++ {
			d := start.Add(c*7 + row)
			// Days before tracking began are as blank as days still to come:
			// nothing could have been done on either.
			if d.After(end) || d.Before(start) || d.Before(since) {
				line.WriteString(strings.Repeat(" ", cellW))
				continue
			}
			key := d.String()
			line.WriteString(lead)
			// A rest day nobody worked shows as rest; one that was done anyway
			// keeps its density, since the work really happened.
			if rest[key] && counts[key] == 0 {
				line.WriteString(paint(RestGlyph, dim, color))
				line.WriteString(" ")
				continue
			}
			// The denominator is the habits due that day, not every habit: a
			// day two of three had off is full when the third one was done.
			lv := level(counts[key], due[key])
			line.WriteString(paint(Glyphs[lv], levelColors[lv], color))
			line.WriteString(" ")
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
	}

	var weekRow strings.Builder
	weekRow.WriteString(paint(pad("%", labelWidth, false), dim, color))
	for c, s := range scores {
		weekRow.WriteString(strings.Repeat(" ", cellW-1-width(s)))
		switch s {
		case "":
		case RestGlyph:
			weekRow.WriteString(paint(s, dim, color))
		default:
			weekRow.WriteString(paint(s, shade(cal.Weeks[c].Percent()), color))
		}
		weekRow.WriteString(" ")
	}
	fmt.Fprintln(w, strings.TrimRight(weekRow.String(), " "))

	var legend strings.Builder
	legend.WriteString("Less ")
	for i, g := range Glyphs {
		legend.WriteString(paint(g, levelColors[i], color))
		legend.WriteString(" ")
	}
	legend.WriteString("More")
	if anyRest {
		legend.WriteString("   ")
		legend.WriteString(paint(RestGlyph, dim, color))
		legend.WriteString(" rest")
	}
	fmt.Fprintln(w, paint(legend.String(), dim, color))

	fmt.Fprintln(w)
	fmt.Fprintln(w, weeklyLine(cal.Weekly, color))
}

// Stats writes the per-habit summary.
func Stats(w io.Writer, s model.Stats, color bool) {
	title := fmt.Sprintf("%s (#%d)", s.Name, s.ID)
	if len(s.Tags) > 0 {
		title += "  +" + strings.Join(s.Tags, " +")
	}
	fmt.Fprintln(w, paint(title, bold, color))

	streak := fmt.Sprintf("%d days", s.Current)
	if s.AtRisk && s.Current > 0 {
		streak = paint(streak+" (not done today)", yellow, color)
	}
	todayMark := paint("no", dim, color)
	switch {
	case s.DoneToday:
		todayMark = paint("yes", levelColors[4], color)
	case s.RestToday:
		todayMark = paint("rest day", dim, color)
	}

	rows := [][2]string{
		{"Today", todayMark},
		{"Current streak", streak},
		{"Longest streak", fmt.Sprintf("%d days", s.Longest)},
		{"Completed", fmt.Sprintf("%d of %d days (%.0f%%)", s.Total, s.TrackedDays, s.Rate*100)},
		{"Last 7 days", fmt.Sprintf("%d of %d", s.Last7, s.Last7Of)},
		{"Last 30 days", fmt.Sprintf("%d of %d", s.Last30, s.Last30Of)},
		{"Last 365 days", fmt.Sprintf("%d of %d", s.Last365, s.Last365Of)},
		{"Best weekday", orDash(s.BestWeekday)},
		{"Tracking since", s.Since},
	}
	if s.Skipped > 0 {
		rows = append(rows, [2]string{"Skipped", fmt.Sprintf("%d days (taken off, not counted)", s.Skipped)})
	}
	if s.Sabbath != "" {
		rows = append(rows, [2]string{"Sabbath", s.Sabbath + " (rested, not counted)"})
	}
	labelWidth := 0
	for _, r := range rows {
		if width(r[0]) > labelWidth {
			labelWidth = width(r[0])
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %s  %s\n", paint(pad(r[0], labelWidth, false), dim, color), r[1])
	}

	if dates := recentNoteDates(s.Notes, noteLimit); len(dates) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, paint("  Recent notes", bold, color))
		for _, d := range dates {
			fmt.Fprintf(w, "  %s  %s\n", paint(d, dim, color), s.Notes[d])
		}
	}
}

// noteLimit is how many notes the stats report shows, newest first. The whole
// set is still in the JSON for anyone who wants it.
const noteLimit = 5

// recentNoteDates returns up to n note dates, newest first.
func recentNoteDates(notes map[string]string, n int) []string {
	if len(notes) == 0 {
		return nil
	}
	dates := make([]string, 0, len(notes))
	for d := range notes {
		dates = append(dates, d)
	}
	// ISO dates sort chronologically as strings.
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	if len(dates) > n {
		dates = dates[:n]
	}
	return dates
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
