// Package model holds habits, the collection they live in, and everything
// derived from them: streaks, stats, selectors and tag filters.
package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fhightower/habit-warrior/internal/hdate"
)

// Version is the on-disk schema version.
const Version = 1

// Habit is one binary daily habit: on any given day it was done or it wasn't.
type Habit struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags,omitempty"`
	Created  string   `json:"created"`
	Archived bool     `json:"archived,omitempty"`
	Done     []string `json:"done"` // ISO dates, sorted and unique
}

// Store is the whole data file.
type Store struct {
	Version int     `json:"version"`
	NextID  int     `json:"next_id"`
	Habits  []Habit `json:"habits"`
}

// NewStore returns an empty store at the current schema version.
func NewStore() *Store {
	return &Store{Version: Version, NextID: 1, Habits: []Habit{}}
}

// CreatedDate is the day the habit was added. A store hand-edited into an
// unparsable created date falls back to the earliest completion, then today,
// so reports degrade instead of failing.
func (h *Habit) CreatedDate() hdate.Date {
	if d, err := hdate.ParseISO(h.Created); err == nil {
		return d
	}
	if len(h.Done) > 0 {
		if d, err := hdate.ParseISO(h.Done[0]); err == nil {
			return d
		}
	}
	return hdate.Today()
}

// IsDone reports whether the habit was completed on d.
func (h *Habit) IsDone(d hdate.Date) bool {
	s := d.String()
	i := sort.SearchStrings(h.Done, s)
	return i < len(h.Done) && h.Done[i] == s
}

// MarkDone records a completion, reporting whether anything changed.
func (h *Habit) MarkDone(d hdate.Date) bool {
	s := d.String()
	i := sort.SearchStrings(h.Done, s)
	if i < len(h.Done) && h.Done[i] == s {
		return false
	}
	h.Done = append(h.Done, "")
	copy(h.Done[i+1:], h.Done[i:])
	h.Done[i] = s
	return true
}

// Undo removes a completion, reporting whether anything changed.
func (h *Habit) Undo(d hdate.Date) bool {
	s := d.String()
	i := sort.SearchStrings(h.Done, s)
	if i >= len(h.Done) || h.Done[i] != s {
		return false
	}
	h.Done = append(h.Done[:i], h.Done[i+1:]...)
	return true
}

// CurrentStreak counts consecutive completed days ending today. A habit done
// yesterday but not yet today keeps its streak and is reported at risk, so an
// unfinished morning does not read as a broken streak.
func (h *Habit) CurrentStreak(today hdate.Date) (n int, atRisk bool) {
	cursor := today
	if !h.IsDone(cursor) {
		cursor = cursor.Add(-1)
		if !h.IsDone(cursor) {
			return 0, false
		}
		atRisk = true
	}
	for h.IsDone(cursor) {
		n++
		cursor = cursor.Add(-1)
	}
	return n, atRisk
}

// LongestStreak is the longest run of consecutive days ever completed.
func (h *Habit) LongestStreak() int {
	best, run := 0, 0
	var prev hdate.Date
	for _, s := range h.Done {
		d, err := hdate.ParseISO(s)
		if err != nil {
			continue
		}
		if !prev.IsZero() && prev.Add(1).Equal(d) {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
		prev = d
	}
	return best
}

// CountLast counts completions in the window of days ending today inclusive.
func (h *Habit) CountLast(today hdate.Date, days int) int {
	if days <= 0 {
		return 0
	}
	start := today.Add(-(days - 1))
	n := 0
	for _, s := range h.Done {
		d, err := hdate.ParseISO(s)
		if err != nil || d.Before(start) || d.After(today) {
			continue
		}
		n++
	}
	return n
}

// Stats is the derived summary shown by the stats report.
type Stats struct {
	ID   int      `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
	// Since is the first day the completion rate counts, which is the day the
	// habit was added unless older days were backfilled.
	Since       string  `json:"since"`
	Total       int     `json:"total"`
	Current     int     `json:"current_streak"`
	AtRisk      bool    `json:"at_risk"`
	Longest     int     `json:"longest_streak"`
	TrackedDays int     `json:"tracked_days"`
	Rate        float64 `json:"rate"`
	Last7       int     `json:"last_7"`
	Last30      int     `json:"last_30"`
	Last365     int     `json:"last_365"`
	BestWeekday string  `json:"best_weekday"`
	DoneToday   bool    `json:"done_today"`
}

// Stats summarizes the habit as of today.
func (h *Habit) Stats(today hdate.Date) Stats {
	cur, atRisk := h.CurrentStreak(today)
	// Backfilling a day older than the habit itself extends the window, so the
	// completion rate can never exceed 100%.
	since := h.CreatedDate()
	if len(h.Done) > 0 {
		if first, err := hdate.ParseISO(h.Done[0]); err == nil && first.Before(since) {
			since = first
		}
	}
	tracked := today.Since(since) + 1
	if tracked < 1 {
		tracked = 1
	}
	rate := 0.0
	if tracked > 0 {
		rate = float64(len(h.Done)) / float64(tracked)
	}
	tags := h.Tags
	if tags == nil {
		tags = []string{}
	}
	return Stats{
		ID:          h.ID,
		Name:        h.Name,
		Tags:        tags,
		Since:       since.String(),
		Total:       len(h.Done),
		Current:     cur,
		AtRisk:      atRisk,
		Longest:     h.LongestStreak(),
		TrackedDays: tracked,
		Rate:        rate,
		Last7:       h.CountLast(today, 7),
		Last30:      h.CountLast(today, 30),
		Last365:     h.CountLast(today, 365),
		BestWeekday: h.bestWeekday(),
		DoneToday:   h.IsDone(today),
	}
}

// bestWeekday is the weekday with the most completions, ties going to the
// earlier day of the week. Empty when nothing has been logged.
func (h *Habit) bestWeekday() string {
	var counts [7]int
	for _, s := range h.Done {
		d, err := hdate.ParseISO(s)
		if err != nil {
			continue
		}
		counts[int(d.Weekday())]++
	}
	best, bestN := -1, 0
	for i, n := range counts {
		if n > bestN {
			best, bestN = i, n
		}
	}
	if best < 0 {
		return ""
	}
	return time.Weekday(best).String()
}

// HasTag reports whether the habit carries tag, compared case-insensitively.
func (h *Habit) HasTag(tag string) bool {
	for _, t := range h.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// AddTag adds a tag if it is not already present.
func (h *Habit) AddTag(tag string) bool {
	if tag == "" || h.HasTag(tag) {
		return false
	}
	h.Tags = append(h.Tags, tag)
	sort.Strings(h.Tags)
	return true
}

// RemoveTag drops a tag if present.
func (h *Habit) RemoveTag(tag string) bool {
	for i, t := range h.Tags {
		if strings.EqualFold(t, tag) {
			h.Tags = append(h.Tags[:i], h.Tags[i+1:]...)
			return true
		}
	}
	return false
}

// Add creates a habit. Names are unique, compared case-insensitively, so that
// selectors stay unambiguous.
func (s *Store) Add(name string, tags []string, today hdate.Date) (*Habit, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("habit name cannot be empty")
	}
	if _, err := strconv.Atoi(name); err == nil {
		return nil, fmt.Errorf("habit name cannot be a number: it would collide with an ID")
	}
	for i := range s.Habits {
		if strings.EqualFold(s.Habits[i].Name, name) {
			return nil, fmt.Errorf("habit %q already exists", s.Habits[i].Name)
		}
	}
	if s.NextID < 1 {
		s.NextID = 1
	}
	h := Habit{ID: s.NextID, Name: name, Created: today.String(), Done: []string{}}
	for _, t := range tags {
		h.AddTag(t)
	}
	s.NextID++
	s.Habits = append(s.Habits, h)
	return &s.Habits[len(s.Habits)-1], nil
}

// Delete removes a habit by ID, reporting whether it was found.
func (s *Store) Delete(id int) bool {
	for i := range s.Habits {
		if s.Habits[i].ID == id {
			s.Habits = append(s.Habits[:i], s.Habits[i+1:]...)
			return true
		}
	}
	return false
}

// Find resolves a selector to exactly one habit, trying in order: numeric ID,
// exact name, unique name prefix, unique substring. Archived habits are
// included so they can be found and restored. Ambiguity is an error rather
// than a guess.
func (s *Store) Find(sel string) (*Habit, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return nil, fmt.Errorf("no habit given")
	}
	if id, err := strconv.Atoi(sel); err == nil {
		for i := range s.Habits {
			if s.Habits[i].ID == id {
				return &s.Habits[i], nil
			}
		}
		return nil, fmt.Errorf("no habit with ID %d", id)
	}

	lower := strings.ToLower(sel)
	for i := range s.Habits {
		if strings.EqualFold(s.Habits[i].Name, sel) {
			return &s.Habits[i], nil
		}
	}

	for _, match := range []func(string) bool{
		func(name string) bool { return strings.HasPrefix(name, lower) },
		func(name string) bool { return strings.Contains(name, lower) },
	} {
		var hits []int
		for i := range s.Habits {
			if match(strings.ToLower(s.Habits[i].Name)) {
				hits = append(hits, i)
			}
		}
		if len(hits) == 1 {
			return &s.Habits[hits[0]], nil
		}
		if len(hits) > 1 {
			names := make([]string, len(hits))
			for j, i := range hits {
				names[j] = s.Habits[i].Name
			}
			return nil, fmt.Errorf("%q matches %s", sel, strings.Join(names, ", "))
		}
	}

	return nil, fmt.Errorf("no habit matching %q", sel)
}

// Filter holds a parsed tag query: every include tag must be present and no
// exclude tag may be.
type Filter struct {
	Include []string
	Exclude []string
}

// ParseArgs splits command arguments into tag filters and the remaining
// positional arguments, in order.
//
// A numeric "-3" is left positional rather than read as excluding the tag "3".
// Someone reaching for a relative date should get a date error, not a silent
// bulk operation over every habit that lacks a tag they never had.
func ParseArgs(args []string) (Filter, []string) {
	var f Filter
	var rest []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "+") && len(a) > 1:
			f.Include = append(f.Include, a[1:])
		case strings.HasPrefix(a, "-") && len(a) > 1 && !strings.HasPrefix(a, "--") && !isNumeric(a[1:]):
			f.Exclude = append(f.Exclude, a[1:])
		default:
			rest = append(rest, a)
		}
	}
	return f, rest
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Empty reports whether the filter selects everything.
func (f Filter) Empty() bool { return len(f.Include) == 0 && len(f.Exclude) == 0 }

// Match reports whether a habit satisfies the filter.
func (f Filter) Match(h *Habit) bool {
	for _, t := range f.Include {
		if !h.HasTag(t) {
			return false
		}
	}
	for _, t := range f.Exclude {
		if h.HasTag(t) {
			return false
		}
	}
	return true
}

// Select returns the habits matching the filter, in ID order. Archived habits
// are left out unless includeArchived is set.
func (s *Store) Select(f Filter, includeArchived bool) []*Habit {
	out := []*Habit{}
	for i := range s.Habits {
		h := &s.Habits[i]
		if h.Archived && !includeArchived {
			continue
		}
		if f.Match(h) {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
