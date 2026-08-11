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

	if wd, ok := ParseWeekday(norm); ok {
		back := (int(today.Weekday()) - int(wd) + 7) % 7
		return today.Add(-back), nil
	}

	return Date{}, fmt.Errorf("cannot understand date %q", s)
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
