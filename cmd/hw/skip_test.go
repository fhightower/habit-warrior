package main

import (
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/render"
)

func TestSkipTakesADayOff(t *testing.T) {
	c := newCLI(t)
	c.ok("config", "sabbath", "off") // so the rest glyph can only come from the skip
	c.ok("add", "read")

	if out := c.ok("skip", "read"); !strings.Contains(out, "read") {
		t.Errorf("skip output = %q", out)
	}

	out := c.ok("list")
	if !strings.Contains(out, render.RestGlyph) {
		t.Errorf("list does not show the habit resting:\n%s", out)
	}
}

// A day off is skipped, not failed: the run carries straight through it, the
// same way the sabbath does.
func TestSkipBridgesAStreakEndToEnd(t *testing.T) {
	now := hdate.Today()
	c := newCLI(t)
	c.ok("config", "sabbath", "off")
	c.ok("add", "meditate")
	c.ok("done", "meditate", now.Add(-1).String())
	c.ok("done", "meditate", now.Add(-3).String())

	var broken map[string]any
	unmarshal(t, c.ok("stats", "meditate", "--json"), &broken)
	if broken["current_streak"].(float64) != 1 {
		t.Fatalf("streak before the skip = %v, want 1", broken["current_streak"])
	}

	c.ok("skip", "meditate", now.Add(-2).String())
	var bridged map[string]any
	unmarshal(t, c.ok("stats", "meditate", "--json"), &bridged)
	if bridged["current_streak"].(float64) != 2 {
		t.Errorf("streak across the skipped day = %v, want 2", bridged["current_streak"])
	}
	if bridged["skipped"].(float64) != 1 {
		t.Errorf("skipped = %v, want 1", bridged["skipped"])
	}
}

func TestSkipAcceptsARange(t *testing.T) {
	now := hdate.Today()
	c := newCLI(t)
	c.ok("add", "read")

	out := c.ok("skip", "read", "3d..today")
	if !strings.Contains(out, "4 days") {
		t.Errorf("range skip output = %q, want it to report 4 days", out)
	}

	var stats map[string]any
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 4 {
		t.Errorf("skipped = %v, want 4", stats["skipped"])
	}

	// Both endpoints are included.
	c.ok("unskip", "read", now.Add(-3).String())
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 3 {
		t.Errorf("skipped after unskipping an endpoint = %v, want 3", stats["skipped"])
	}
}

// Booking time off before it arrives is the point, so skip takes future days
// where done refuses them.
func TestSkipAcceptsFutureDays(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "read")
	c.ok("skip", "read", "tomorrow")
	// Overlapping the day already off, so the count also shows a repeated skip
	// is not double counted.
	c.ok("skip", "read", "tomorrow..in 5 days")

	var stats map[string]any
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 5 {
		t.Errorf("skipped = %v, want 5", stats["skipped"])
	}

	// Streaks and rates stop at today, so days booked ahead change nothing yet.
	if stats["tracked_days"].(float64) != 1 {
		t.Errorf("tracked_days = %v, want 1: future days off are not tracked yet", stats["tracked_days"])
	}
}

// "next mon" and "next fri" each land inside the coming week, so which comes
// first depends on today. A range that runs backwards is refused rather than
// quietly swapped, and that has to hold on every day of the week.
func TestNextWeekdayRangeDependsOnToday(t *testing.T) {
	now := hdate.Today()
	c := newCLI(t)
	c.ok("add", "read")

	mon, err := hdate.Parse("next mon", now)
	if err != nil {
		t.Fatal(err)
	}
	fri, err := hdate.Parse("next fri", now)
	if err != nil {
		t.Fatal(err)
	}
	if fri.Before(mon) {
		if msg := c.fails("skip", "read", "next mon..next fri"); !strings.Contains(msg, "before") {
			t.Errorf("error = %q, want a backwards range refused", msg)
		}
		return
	}
	out := c.ok("skip", "read", "next mon..next fri")
	if !strings.Contains(out, mon.String()) || !strings.Contains(out, fri.String()) {
		t.Errorf("skip output = %q, want %s..%s", out, mon, fri)
	}
}

func TestSkipRefusesADayAlreadyDone(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "read")
	c.ok("done", "read")

	msg := c.fails("skip", "read")
	if !strings.Contains(msg, "undo") {
		t.Errorf("error = %q, want it to point at undo", msg)
	}

	// Nothing was written: the day is still done and not skipped.
	var stats map[string]any
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 0 || stats["total"].(float64) != 1 {
		t.Errorf("refused skip changed the data: %v", stats)
	}
}

// A range is all or nothing, like every other command that names several days
// or habits at once.
func TestSkipRangeRefusedWholeWhenOneDayIsDone(t *testing.T) {
	now := hdate.Today()
	c := newCLI(t)
	c.ok("add", "read")
	c.ok("done", "read", now.Add(-2).String())

	msg := c.fails("skip", "read", "4d..today")
	if !strings.Contains(msg, now.Add(-2).String()) {
		t.Errorf("error = %q, want it to name the day that is done", msg)
	}

	var stats map[string]any
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 0 {
		t.Errorf("skipped = %v, want the whole range refused", stats["skipped"])
	}
}

func TestDoneClearsASkip(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "read")
	c.ok("skip", "read")
	c.ok("done", "read")

	var stats map[string]any
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 0 {
		t.Errorf("skipped = %v, want the skip cleared by done", stats["skipped"])
	}
	if !stats["done_today"].(bool) {
		t.Error("done_today = false after done")
	}
}

func TestUnskipPutsADayBackOn(t *testing.T) {
	c := newCLI(t)
	c.ok("config", "sabbath", "off") // today must be an ordinary day for rest_today to mean the skip
	c.ok("add", "read")
	c.ok("skip", "read")
	c.ok("unskip", "read")

	var stats map[string]any
	unmarshal(t, c.ok("stats", "read", "--json"), &stats)
	if stats["skipped"].(float64) != 0 {
		t.Errorf("skipped = %v, want 0", stats["skipped"])
	}
	if stats["rest_today"].(bool) {
		t.Error("rest_today = true after unskip")
	}

	if out := c.ok("unskip", "read"); !strings.Contains(out, "No change") {
		t.Errorf("unskipping a day that was not off = %q", out)
	}
}

func TestSkipByTag(t *testing.T) {
	c := newCLI(t)
	c.ok("config", "sabbath", "off") // the untagged habit must not be resting for another reason
	c.ok("add", "read", "+mind")
	c.ok("add", "meditate", "+mind")
	c.ok("add", "run", "+health")

	c.ok("skip", "+mind")

	var rows []render.ListRow
	unmarshal(t, c.ok("list", "--json"), &rows)
	for _, r := range rows {
		want := r.Name != "run"
		if r.RestToday != want {
			t.Errorf("%s: rest_today = %v, want %v", r.Name, r.RestToday, want)
		}
	}
}

func TestSkipRejectsBadRanges(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "read")

	now := hdate.Today()
	backwards := now.String() + ".." + now.Add(-3).String()
	if msg := c.fails("skip", "read", backwards); msg == "" {
		t.Error("a backwards range was accepted")
	}
	// A typo in the year should not write years of days off.
	if msg := c.fails("skip", "read", "2026-01-01..2036-01-01"); !strings.Contains(msg, "366") {
		t.Errorf("error = %q, want it to name the limit", msg)
	}
}

func TestSkipRejectsANote(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "read")
	if msg := c.fails("skip", "read", "--note", "sick"); !strings.Contains(msg, "note") {
		t.Errorf("error = %q, want it to explain that skip takes no note", msg)
	}
}

func TestSkipUsageErrors(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "read")
	if msg := c.fails("skip"); !strings.Contains(msg, "usage") {
		t.Errorf("bare skip = %q", msg)
	}
	if msg := c.fails("unskip"); !strings.Contains(msg, "usage") {
		t.Errorf("bare unskip = %q", msg)
	}
	if msg := c.fails("skip", "nosuchhabit"); !strings.Contains(msg, "nosuchhabit") {
		t.Errorf("unknown habit = %q", msg)
	}
}

func TestHelpMentionsSkip(t *testing.T) {
	c := newCLI(t)
	out := c.ok("help")
	if !strings.Contains(out, "hw skip") || !strings.Contains(out, "hw unskip") {
		t.Errorf("help does not document skip:\n%s", out)
	}
}
