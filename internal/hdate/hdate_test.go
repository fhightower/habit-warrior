package hdate

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	// A Thursday, chosen so weekday lookups have to wrap.
	today := New(2026, time.August, 6)

	cases := []struct {
		in   string
		want Date
	}{
		{"today", today},
		{"TODAY", today},
		{"t", today},
		{"yesterday", New(2026, time.August, 5)},
		{"y", New(2026, time.August, 5)},
		{"tomorrow", New(2026, time.August, 7)},
		{"2026-08-01", New(2026, time.August, 1)},
		{"3d", New(2026, time.August, 3)},
		{"3d ago", New(2026, time.August, 3)},
		{"3 days ago", New(2026, time.August, 3)},
		{"1 day ago", New(2026, time.August, 5)},
		{"0d", today},
		{"  2  days   ago ", New(2026, time.August, 4)},
		{"thu", today},      // today is a Thursday
		{"thursday", today}, // same, spelled out
		{"wed", New(2026, time.August, 5)},
		{"fri", New(2026, time.July, 31)}, // last Friday, not the coming one
		{"Mon", New(2026, time.August, 3)},
	}

	for _, c := range cases {
		got, err := Parse(c.in, today)
		if err != nil {
			t.Errorf("Parse(%q) returned error: %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("Parse(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	today := New(2026, time.August, 6)
	for _, in := range []string{"", "   ", "someday", "2026-13-01", "-3", "d", "many days ago", "2026/08/01"} {
		if got, err := Parse(in, today); err == nil {
			t.Errorf("Parse(%q) = %s, want an error", in, got)
		}
	}
}

func TestAddAndSince(t *testing.T) {
	d := New(2026, time.August, 6)
	if got := d.Add(1).String(); got != "2026-08-07" {
		t.Errorf("Add(1) = %s", got)
	}
	if got := d.Add(-6).String(); got != "2026-07-31" {
		t.Errorf("Add(-6) = %s, want the month to roll back", got)
	}
	if got := d.Add(3).Since(d); got != 3 {
		t.Errorf("Since = %d, want 3", got)
	}
	if got := d.Since(d); got != 0 {
		t.Errorf("Since(self) = %d, want 0", got)
	}
	if got := d.Add(-2).Since(d); got != -2 {
		t.Errorf("Since across a backwards gap = %d, want -2", got)
	}
}

// Day arithmetic must not skip or repeat a day across a DST boundary, which is
// why dates are held at midnight UTC.
func TestAddAcrossDST(t *testing.T) {
	// US DST began 2026-03-08.
	d := New(2026, time.March, 7)
	want := []string{"2026-03-08", "2026-03-09", "2026-03-10"}
	for i, w := range want {
		if got := d.Add(i + 1).String(); got != w {
			t.Errorf("Add(%d) = %s, want %s", i+1, got, w)
		}
	}
}

func TestFromTimeUsesLocalCalendarDay(t *testing.T) {
	loc := time.FixedZone("UTC-8", -8*3600)
	// 2026-08-06 23:30 local is already the 7th in UTC; the local day wins.
	got := FromTime(time.Date(2026, time.August, 6, 23, 30, 0, 0, loc))
	if want := New(2026, time.August, 6); !got.Equal(want) {
		t.Errorf("FromTime = %s, want %s", got, want)
	}
}

func TestParseISORoundTrip(t *testing.T) {
	d, err := ParseISO("2026-02-29")
	if err == nil {
		t.Fatalf("ParseISO accepted a non-existent date: %s", d)
	}
	d, err = ParseISO("2024-02-29")
	if err != nil {
		t.Fatalf("ParseISO(leap day) failed: %v", err)
	}
	if got := d.String(); got != "2024-02-29" {
		t.Errorf("round trip = %s", got)
	}
}
