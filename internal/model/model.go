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

// Version is the on-disk schema version. Version 2 added the config object,
// whose sabbath setting an older binary would ignore while reporting the wrong
// streaks. Version 3 added per-completion notes, which an older binary would
// drop on its next write. Version 4 added skipped days, which an older binary
// would ignore while reporting exactly the missed days and broken streaks the
// skip was taken to avoid. Every bump exists so that older binary refuses the
// file instead.
const Version = 4

// Habit is one binary daily habit: on any given day it was done or it wasn't.
type Habit struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags,omitempty"`
	Created  string   `json:"created"`
	Archived bool     `json:"archived,omitempty"`
	Done     []string `json:"done"`              // ISO dates, sorted and unique
	Skipped  []string `json:"skipped,omitempty"` // ISO dates, sorted and unique
	// Notes holds what you wrote about a completion, keyed by ISO date. It
	// rides alongside Done rather than inside it so that the completion list
	// stays a plain sorted array of dates.
	Notes map[string]string `json:"notes,omitempty"`
}

// IsSkipped reports whether the habit was deliberately taken off on d.
func (h *Habit) IsSkipped(d hdate.Date) bool { return containsDate(h.Skipped, d.String()) }

// Skip takes a day off, reporting whether anything changed.
func (h *Habit) Skip(d hdate.Date) bool {
	list, changed := insertDate(h.Skipped, d.String())
	h.Skipped = list
	return changed
}

// Unskip puts a day back on, reporting whether anything changed.
func (h *Habit) Unskip(d hdate.Date) bool {
	list, changed := removeDate(h.Skipped, d.String())
	h.Skipped = list
	return changed
}

// Rests reports whether d is a day off for this habit, from either source: the
// weekly sabbath, or a skip taken on that one day. Resting is skipped, not
// failed — a rest day neither breaks a streak nor counts against a completion
// rate — so every report asks this one question rather than checking the two
// sources separately.
func (h *Habit) Rests(d hdate.Date, sab Sabbath) bool {
	return sab.Rest(d) || h.IsSkipped(d)
}

// containsDate reports whether a sorted list of ISO dates holds s.
func containsDate(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}

// insertDate adds s to a sorted list, reporting whether anything changed.
func insertDate(list []string, s string) ([]string, bool) {
	i := sort.SearchStrings(list, s)
	if i < len(list) && list[i] == s {
		return list, false
	}
	list = append(list, "")
	copy(list[i+1:], list[i:])
	list[i] = s
	return list, true
}

// removeDate drops s from a sorted list, reporting whether anything changed.
func removeDate(list []string, s string) ([]string, bool) {
	i := sort.SearchStrings(list, s)
	if i >= len(list) || list[i] != s {
		return list, false
	}
	return append(list[:i], list[i+1:]...), true
}

// Note is what was written about the completion on d, empty when there is
// nothing.
func (h *Habit) Note(d hdate.Date) string { return h.Notes[d.String()] }

// SetNote records text against a day, reporting whether anything changed.
// Empty text removes the note.
func (h *Habit) SetNote(d hdate.Date, text string) bool {
	s := d.String()
	text = strings.TrimSpace(text)
	if h.Notes[s] == text {
		return false
	}
	if text == "" {
		delete(h.Notes, s)
		return true
	}
	if h.Notes == nil {
		h.Notes = map[string]string{}
	}
	h.Notes[s] = text
	return true
}

// Store is the whole data file.
type Store struct {
	Version int     `json:"version"`
	NextID  int     `json:"next_id"`
	Config  *Config `json:"config,omitempty"`
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
func (h *Habit) IsDone(d hdate.Date) bool { return containsDate(h.Done, d.String()) }

// MarkDone records a completion, reporting whether anything changed. Doing a
// day that had been taken off clears the skip: the work is the truth about that
// day, and a day cannot be both done and off.
func (h *Habit) MarkDone(d hdate.Date) bool {
	list, changed := insertDate(h.Done, d.String())
	h.Done = list
	return h.Unskip(d) || changed
}

// Undo removes a completion, reporting whether anything changed. Any note goes
// with it: a note describes a completion, and leaving it behind would attach
// stale text to whatever gets logged for that day next. A day undone is an
// ordinary day again, not a day off.
func (h *Habit) Undo(d hdate.Date) bool {
	list, changed := removeDate(h.Done, d.String())
	h.Done = list
	if changed {
		delete(h.Notes, d.String())
	}
	return changed
}

// CurrentStreak counts consecutive completed days ending today. A habit done
// yesterday but not yet today keeps its streak and is reported at risk, so an
// unfinished morning does not read as a broken streak.
//
// Rest days are skipped rather than failed: an undone rest day neither adds to
// the count nor ends it, so a run continues straight through. A rest day that
// was done anyway counts like any other day. Today being a rest day is never
// at risk, since there is nothing left to do.
func (h *Habit) CurrentStreak(today hdate.Date, sab Sabbath) (n int, atRisk bool) {
	cursor := today
	if !h.IsDone(cursor) {
		if h.Rests(cursor, sab) {
			cursor = cursor.Add(-1)
		} else {
			cursor = cursor.Add(-1)
			for !h.IsDone(cursor) && h.Rests(cursor, sab) {
				cursor = cursor.Add(-1)
			}
			if !h.IsDone(cursor) {
				return 0, false
			}
			atRisk = true
		}
	}
	for {
		switch {
		case h.IsDone(cursor):
			n++
		case h.Rests(cursor, sab):
			// Skipped, not counted: the run passes through it.
		default:
			return n, atRisk
		}
		cursor = cursor.Add(-1)
	}
}

// LongestStreak is the longest run of consecutive days ever completed. Two
// days also count as consecutive when every day between them rests, which is
// the historical form of the bridge CurrentStreak walks. A sabbath gap is
// always one day wide, but a skipped run can be any width, so the bridge is
// measured rather than assumed.
func (h *Habit) LongestStreak(sab Sabbath) int {
	best, run := 0, 0
	var prev hdate.Date
	for _, s := range h.Done {
		d, err := hdate.ParseISO(s)
		if err != nil {
			continue
		}
		if !prev.IsZero() && (prev.Add(1).Equal(d) || h.bridges(prev, d, sab)) {
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

// bridges reports whether the gap between two completions is made up entirely
// of rest days, which makes them consecutive as far as a streak is concerned.
// Adjacent days have no gap to bridge and are handled by the caller.
func (h *Habit) bridges(prev, d hdate.Date, sab Sabbath) bool {
	if !d.After(prev.Add(1)) {
		return false
	}
	for c := prev.Add(1); c.Before(d); c = c.Add(1) {
		if !h.Rests(c, sab) {
			return false
		}
	}
	return true
}

// CountLast counts completions in the window of days ending today inclusive,
// alongside how many days of that window were eligible. Rest days are not
// eligible unless they were done anyway, which keeps done from ever exceeding
// eligible.
func (h *Habit) CountLast(today hdate.Date, days int, sab Sabbath) (done, eligible int) {
	if days <= 0 {
		return 0, 0
	}
	start := today.Add(-(days - 1))
	for _, s := range h.Done {
		d, err := hdate.ParseISO(s)
		if err != nil || d.Before(start) || d.After(today) {
			continue
		}
		done++
	}
	for d := start; !d.After(today); d = d.Add(1) {
		if !h.Rests(d, sab) || h.IsDone(d) {
			eligible++
		}
	}
	return done, eligible
}

// eligibleDays counts the days from start through today that count toward a
// completion rate: everything but rest days left undone.
func (h *Habit) eligibleDays(start, today hdate.Date, sab Sabbath) int {
	n := 0
	for d := start; !d.After(today); d = d.Add(1) {
		if !h.Rests(d, sab) || h.IsDone(d) {
			n++
		}
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
	Since   string `json:"since"`
	Total   int    `json:"total"`
	Current int    `json:"current_streak"`
	AtRisk  bool   `json:"at_risk"`
	Longest int    `json:"longest_streak"`
	// TrackedDays counts only the days that could have been completed: rest
	// days are left out unless they were done anyway.
	TrackedDays int `json:"tracked_days"`
	// Skipped is how many days were taken off one at a time, as opposed to the
	// weekly sabbath.
	Skipped     int     `json:"skipped"`
	Rate        float64 `json:"rate"`
	Last7       int     `json:"last_7"`
	Last7Of     int     `json:"last_7_of"`
	Last30      int     `json:"last_30"`
	Last30Of    int     `json:"last_30_of"`
	Last365     int     `json:"last_365"`
	Last365Of   int     `json:"last_365_of"`
	BestWeekday string  `json:"best_weekday"`
	DoneToday   bool    `json:"done_today"`
	// Notes is what was written about each completion, keyed by ISO date.
	Notes map[string]string `json:"notes,omitempty"`
	// Sabbath names the weekly rest day, empty when sabbath mode is off.
	Sabbath string `json:"sabbath"`
	// RestToday reports whether today is the rest day.
	RestToday bool `json:"rest_today"`
}

// Stats summarizes the habit as of today.
func (h *Habit) Stats(today hdate.Date, sab Sabbath) Stats {
	cur, atRisk := h.CurrentStreak(today, sab)
	// Backfilling a day older than the habit itself extends the window, so the
	// completion rate can never exceed 100%.
	since := h.CreatedDate()
	if len(h.Done) > 0 {
		if first, err := hdate.ParseISO(h.Done[0]); err == nil && first.Before(since) {
			since = first
		}
	}
	tracked := h.eligibleDays(since, today, sab)
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
	last7, last7of := h.CountLast(today, 7, sab)
	last30, last30of := h.CountLast(today, 30, sab)
	last365, last365of := h.CountLast(today, 365, sab)
	return Stats{
		ID:          h.ID,
		Name:        h.Name,
		Tags:        tags,
		Since:       since.String(),
		Total:       len(h.Done),
		Current:     cur,
		AtRisk:      atRisk,
		Longest:     h.LongestStreak(sab),
		TrackedDays: tracked,
		Skipped:     len(h.Skipped),
		Rate:        rate,
		Last7:       last7,
		Last7Of:     last7of,
		Last30:      last30,
		Last30Of:    last30of,
		Last365:     last365,
		Last365Of:   last365of,
		BestWeekday: h.bestWeekday(),
		DoneToday:   h.IsDone(today),
		Notes:       h.Notes,
		Sabbath:     sabbathName(sab),
		RestToday:   h.Rests(today, sab),
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
