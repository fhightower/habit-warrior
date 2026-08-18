package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhightower/habit-warrior/internal/model"
)

// A report of an earlier day names that day and stops calling it today, down
// to the legend: an earlier day is still open to a backfill, so the streak
// note holds, but it is about the day shown rather than this one.
func TestListOfAPastDayNamesItEverywhere(t *testing.T) {
	yesterday := today.Add(-1)
	h := habit("meditate", 2) // done the day before, leaving yesterday at risk

	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{h}, yesterday, model.Sabbath{}), yesterday, today, true)
	out := buf.String()

	if !strings.Contains(out, yesterday.String()) {
		t.Errorf("output does not name the day:\n%s", out)
	}
	if strings.Contains(out, "today") {
		t.Errorf("a report of %s still says today:\n%s", yesterday, out)
	}
	if !strings.Contains(out, "that day") {
		t.Errorf("the legend does not speak of the day shown:\n%s", out)
	}
	if !strings.Contains(out, "Done") {
		t.Errorf("the marker column is not headed for the day shown:\n%s", out)
	}
}

// A report of today is unchanged: no title, and the day is called today.
func TestListOfTodayKeepsItsWording(t *testing.T) {
	var buf bytes.Buffer
	List(&buf, BuildRows([]*model.Habit{habit("meditate", 1)}, today, model.Sabbath{}), today, today, true)
	out := buf.String()

	if strings.Contains(out, today.String()) {
		t.Errorf("a report of today should not need a date title:\n%s", out)
	}
	if !strings.Contains(out, "done today") {
		t.Errorf("the legend no longer speaks of today:\n%s", out)
	}
}
