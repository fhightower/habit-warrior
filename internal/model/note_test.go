package model

import (
	"testing"
)

func TestSetNoteRecordsTextAgainstTheDay(t *testing.T) {
	h := habitDoneOn(0)
	if !h.SetNote(today, "3 x max pullups") {
		t.Fatal("SetNote reported no change on a fresh note")
	}
	if got := h.Note(today); got != "3 x max pullups" {
		t.Errorf("Note = %q, want the text just set", got)
	}
	if got := h.Note(today.Add(-1)); got != "" {
		t.Errorf("Note on another day = %q, want empty", got)
	}
}

func TestSetNoteReportsWhetherAnythingChanged(t *testing.T) {
	h := habitDoneOn(0)
	h.SetNote(today, "same")
	if h.SetNote(today, "same") {
		t.Error("SetNote reported a change when the text was identical")
	}
	if !h.SetNote(today, "different") {
		t.Error("SetNote reported no change when the text differed")
	}
	if got := h.Note(today); got != "different" {
		t.Errorf("Note = %q, want the replacement", got)
	}
}

func TestSetNoteClearsWithEmptyText(t *testing.T) {
	h := habitDoneOn(0)
	h.SetNote(today, "something")
	if !h.SetNote(today, "") {
		t.Error("clearing an existing note reported no change")
	}
	if got := h.Note(today); got != "" {
		t.Errorf("Note = %q, want it cleared", got)
	}
	if h.Notes != nil && len(h.Notes) != 0 {
		t.Errorf("Notes = %v, want the empty entry removed", h.Notes)
	}
}

func TestUndoDiscardsTheNote(t *testing.T) {
	h := habitDoneOn(0)
	h.SetNote(today, "3 x max pullups")

	h.Undo(today)
	if got := h.Note(today); got != "" {
		t.Errorf("Note = %q after undo, want it gone: a note describes a completion", got)
	}

	// Redoing the day must not resurrect the old text.
	h.MarkDone(today)
	if got := h.Note(today); got != "" {
		t.Errorf("Note = %q after redoing the day, want empty", got)
	}
}

func TestNotesSurviveOtherDaysBeingUndone(t *testing.T) {
	h := habitDoneOn(0, 1)
	h.SetNote(today, "today's note")
	h.SetNote(today.Add(-1), "yesterday's note")

	h.Undo(today.Add(-1))
	if got := h.Note(today); got != "today's note" {
		t.Errorf("Note = %q, want the untouched day to keep its note", got)
	}
}
