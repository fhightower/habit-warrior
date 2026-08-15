// Package hdate handles civil dates: whole days with no time-of-day or zone.
//
// Dates are held as midnight UTC so that day arithmetic and comparison are
// unaffected by DST transitions in the user's local zone. "Today" is still
// derived from the local clock.
package hdate

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Layout is the wire format for dates: ISO 8601 calendar dates.
const Layout = "2006-01-02"

// Date is a single calendar day.
type Date struct {
	t time.Time
}

// New builds a Date from its parts.
func New(y int, m time.Month, d int) Date {
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// FromTime drops the time-of-day from t, keeping the calendar day as seen in
// t's own location.
func FromTime(t time.Time) Date {
	y, m, d := t.Date()
	return New(y, m, d)
}

// Today is the current calendar day in the local zone.
func Today() Date { return FromTime(time.Now()) }

// ParseISO reads a date in Layout format.
func ParseISO(s string) (Date, error) {
	t, err := time.Parse(Layout, s)
	if err != nil {
		return Date{}, fmt.Errorf("not a date: %q", s)
	}
	return FromTime(t), nil
}

func (d Date) String() string        { return d.t.Format(Layout) }
func (d Date) IsZero() bool          { return d.t.IsZero() }
func (d Date) Add(days int) Date     { return Date{d.t.AddDate(0, 0, days)} }
func (d Date) Equal(o Date) bool     { return d.t.Equal(o.t) }
func (d Date) Before(o Date) bool    { return d.t.Before(o.t) }
func (d Date) After(o Date) bool     { return d.t.After(o.t) }
func (d Date) Weekday() time.Weekday { return d.t.Weekday() }
func (d Date) Month() time.Month     { return d.t.Month() }
func (d Date) Day() int              { return d.t.Day() }
func (d Date) Year() int             { return d.t.Year() }

// Since counts whole days from o to d. Same day is 0, tomorrow is 1.
func (d Date) Since(o Date) int {
	return int(d.t.Sub(o.t).Hours() / 24)
}

// Parse reads a human date expression relative to today. Accepted forms:
//
//	today, yesterday, tomorrow
//	2026-08-01
//	3d, 3 days ago, 3d ago
//	mon, monday (the most recent Monday, today included)
//	next mon, next monday (the coming Monday, never today)
//	in 3 days, in 1 day
//
// A bare weekday looks backwards and "next" looks forwards, which keeps the
// short form meaning what it always has while still giving a way to name a day
// that has not arrived.
func Parse(s string, today Date) (Date, error) {
	norm := strings.ToLower(strings.Join(strings.Fields(s), " "))
	if norm == "" {
		return Date{}, fmt.Errorf("empty date")
	}

	switch norm {
	case "today", "t":
		return today, nil
	case "yesterday", "yest", "y":
		return today.Add(-1), nil
	case "tomorrow", "tom":
		return today.Add(1), nil
	}

	if d, err := ParseISO(norm); err == nil {
		return d, nil
	}

	if n, ok := parseDaysAgo(norm); ok {
		return today.Add(-n), nil
	}

	if n, ok := parseDaysAhead(norm); ok {
		return today.Add(n), nil
	}

	if d, ok := parseNextWeekday(norm, today); ok {
		return d, nil
	}

	if wd, ok := ParseWeekday(norm); ok {
		back := (int(today.Weekday()) - int(wd) + 7) % 7
		return today.Add(-back), nil
	}

	return Date{}, fmt.Errorf("cannot understand date %q", s)
}

// RangeSep separates the two endpoints of a date range.
const RangeSep = ".."

// ParseRange reads a range of days: two dates around "..", or a lone date,
// which is a range of one day so that callers have a single shape to work
// with. Endpoints go through Parse, so anything a single date accepts works on
// either side. A backwards range is an error rather than a silently swapped
// one: it more often means a typo than an intention.
func ParseRange(s string, today Date) (start, end Date, err error) {
	if !strings.Contains(s, RangeSep) {
		d, err := Parse(s, today)
		if err != nil {
			return Date{}, Date{}, err
		}
		return d, d, nil
	}
	parts := strings.Split(s, RangeSep)
	if len(parts) != 2 {
		return Date{}, Date{}, fmt.Errorf("a range is two dates around %q, got %q", RangeSep, s)
	}
	if start, err = Parse(parts[0], today); err != nil {
		return Date{}, Date{}, fmt.Errorf("range start: %w", err)
	}
	if end, err = Parse(parts[1], today); err != nil {
		return Date{}, Date{}, fmt.Errorf("range end: %w", err)
	}
	if end.Before(start) {
		return Date{}, Date{}, fmt.Errorf("range %s..%s ends before it starts", start, end)
	}
	return start, end, nil
}

// parseDaysAgo reads "3d", "3d ago", "3 days ago" and friends.
func parseDaysAgo(s string) (int, bool) {
	s = strings.TrimSuffix(s, " ago")
	s = strings.TrimSpace(s)
	for _, suffix := range []string{" days", " day", "days", "day", "d"} {
		if rest, ok := strings.CutSuffix(s, suffix); ok {
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil || n < 0 {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// parseDaysAhead reads "in 3 days", "in 1 day". The "in" is required: a bare
// "3 days" already means three days ago.
func parseDaysAhead(s string) (int, bool) {
	rest, ok := strings.CutPrefix(s, "in ")
	if !ok {
		return 0, false
	}
	for _, suffix := range []string{" days", " day", "days", "day", "d"} {
		if num, ok := strings.CutSuffix(strings.TrimSpace(rest), suffix); ok {
			n, err := strconv.Atoi(strings.TrimSpace(num))
			if err != nil || n < 0 {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// parseNextWeekday reads "next mon", "next monday": the coming weekday, which
// is a week out when today already is that day.
func parseNextWeekday(s string, today Date) (Date, bool) {
	rest, ok := strings.CutPrefix(s, "next ")
	if !ok {
		return Date{}, false
	}
	wd, ok := ParseWeekday(rest)
	if !ok {
		return Date{}, false
	}
	ahead := (int(wd) - int(today.Weekday()) + 7) % 7
	if ahead == 0 {
		ahead = 7
	}
	return today.Add(ahead), true
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

// ParseWeekday reads a weekday name or abbreviation, ignoring case and
// surrounding space: "sun", "Sunday", "SAT", "thurs".
func ParseWeekday(s string) (time.Weekday, bool) {
	wd, ok := weekdays[strings.ToLower(strings.TrimSpace(s))]
	return wd, ok
}
