package main

import (
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/render"
)

// The list report accepts a trailing date, which reports the day as it stood
// then rather than today.
func TestListShowsAPastDay(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("add", "read")
	c.ok("done", "meditate", "yesterday")
	c.ok("done", "read")

	yesterday := hdate.Today().Add(-1).String()
	var rows []render.ListRow
	unmarshal(t, c.ok("list", "yesterday", "--json"), &rows)
	if len(rows) != 2 {
		t.Fatalf("rows = %v", rowNames(rows))
	}
	for _, r := range rows {
		if r.Date != yesterday {
			t.Errorf("%s row date = %q, want %q", r.Name, r.Date, yesterday)
		}
		if want := r.Name == "meditate"; r.DoneToday != want {
			t.Errorf("%s done on %s = %v, want %v", r.Name, yesterday, r.DoneToday, want)
		}
	}
}

// Rows still carry the day they describe when that day is today, so a consumer
// never has to guess which date the report is about.
func TestListRowsCarryTodaysDate(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")

	var rows []render.ListRow
	unmarshal(t, c.ok("list", "--json"), &rows)
	if len(rows) != 1 || rows[0].Date != hdate.Today().String() {
		t.Errorf("row date = %q, want %q", rows[0].Date, hdate.Today())
	}
}

// Tag filters and a date work together.
func TestListPastDayWithTagFilter(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+health")
	c.ok("add", "read", "+mind")
	c.ok("done", "meditate", "yesterday")

	var rows []render.ListRow
	unmarshal(t, c.ok("list", "+health", "yesterday", "--json"), &rows)
	if len(rows) != 1 || rows[0].Name != "meditate" || !rows[0].DoneToday {
		t.Errorf("+health yesterday = %v", rows)
	}
}

// Streaks and rates cannot look forward, so neither can the report.
func TestListRefusesADayThatHasNotArrived(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")

	if msg := c.fails("list", "tomorrow"); !strings.Contains(msg, "has not happened yet") {
		t.Errorf("error = %q", msg)
	}
}

// The table says which day it is about, and stops calling that day today.
func TestListPastDayLabelsTheDay(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("done", "meditate", "yesterday")

	out := c.ok("list", "yesterday")
	if !strings.Contains(out, hdate.Today().Add(-1).String()) {
		t.Errorf("output does not name the day:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "today") {
		t.Errorf("output still calls a past day today:\n%s", out)
	}
}
