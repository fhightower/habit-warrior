package main

import (
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/render"
)

func TestBareListShowsTheWeeklyScore(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+calm")
	c.ok("done", "meditate")

	for _, args := range [][]string{{}, {"list"}, {"ls"}} {
		out := c.ok(args...)
		for _, want := range []string{"This week", "Last week", "Best"} {
			if !strings.Contains(out, want) {
				t.Errorf("hw %s is missing %q:\n%s", strings.Join(args, " "), want, out)
			}
		}
	}
}

// The weekly score is about every habit as of today, so a list narrowed by a
// filter or moved to another day leaves it out rather than mislabel it.
func TestListWithArgumentsLeavesOutTheWeeklyScore(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate", "+calm")
	c.ok("done", "meditate", "yesterday")

	for _, args := range [][]string{{"list", "+calm"}, {"list", "yesterday"}, {"list", "--all"}} {
		if out := c.ok(args...); strings.Contains(out, "This week") {
			t.Errorf("hw %s shows the weekly score:\n%s", strings.Join(args, " "), out)
		}
	}
}

func TestBareListJSONIsStillTheRows(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")

	var rows []render.ListRow
	unmarshal(t, c.ok("--json"), &rows)
	if len(rows) != 1 {
		t.Errorf("hw --json = %+v, want just the habit rows", rows)
	}
}

func TestCalShowsWeeklyScores(t *testing.T) {
	c := newCLI(t)
	c.ok("add", "meditate")
	c.ok("done", "meditate")

	out := c.ok("cal", "--weeks", "2")
	if !strings.Contains(out, "\n%") || !strings.Contains(out, "This week") {
		t.Errorf("cal is missing the week scores or the summary:\n%s", out)
	}

	var cal render.Calendar
	unmarshal(t, c.ok("cal", "--weeks", "2", "--json"), &cal)
	if len(cal.Weeks) != 2 {
		t.Errorf("weeks = %d, want one per column", len(cal.Weeks))
	}
	if cal.ThisWeek.Done != 1 {
		t.Errorf("this_week = %+v, want today's completion counted", cal.ThisWeek)
	}
	// A habit added today has no finished week behind it.
	if cal.Best != nil {
		t.Errorf("best = %+v, want none yet", cal.Best)
	}
	if raw := c.ok("cal", "--json"); !strings.Contains(raw, `"this_week"`) || !strings.Contains(raw, `"best": null`) {
		t.Errorf("cal JSON does not carry the weekly summary at the top level:\n%s", raw)
	}
}
