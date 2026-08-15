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

// Forward forms exist so a day off can be booked before it arrives. A bare
// weekday keeps pointing at the most recent one, which is what done relies on.
func TestParseForwardForms(t *testing.T) {
	// A Thursday.
	today := New(2026, time.August, 6)

	cases := []struct {
		in   string
		want Date
	}{
		{"next mon", New(2026, time.August, 10)},
		{"next monday", New(2026, time.August, 10)},
		{"NEXT Fri", New(2026, time.August, 7)},
		{"next thu", New(2026, time.August, 13)}, // today is a Thursday: the coming one, not today
		{"in 3 days", New(2026, time.August, 9)},
		{"in 1 day", New(2026, time.August, 7)},
		{"in 0 days", today},
		{"  in   2 days  ", New(2026, time.August, 8)},
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

func TestParseRange(t *testing.T) {
	today := New(2026, time.August, 6)

	cases := []struct {
		in         string
		start, end Date
	}{
		{"2026-08-20..2026-08-27", New(2026, time.August, 20), New(2026, time.August, 27)},
		{"2026-08-20 .. 2026-08-27", New(2026, time.August, 20), New(2026, time.August, 27)},
		{"3 days ago..today", New(2026, time.August, 3), today},
		{"today..next mon", today, New(2026, time.August, 10)},
		{"2026-08-20..2026-08-20", New(2026, time.August, 20), New(2026, time.August, 20)},
		// A lone date is a range of one day, so callers have a single shape.
		{"tomorrow", New(2026, time.August, 7), New(2026, time.August, 7)},
		{"2026-08-01", New(2026, time.August, 1), New(2026, time.August, 1)},
	}

	for _, c := range cases {
		start, end, err := ParseRange(c.in, today)
		if err != nil {
			t.Errorf("ParseRange(%q) returned error: %v", c.in, err)
			continue
		}
		if !start.Equal(c.start) || !end.Equal(c.end) {
			t.Errorf("ParseRange(%q) = %s..%s, want %s..%s", c.in, start, end, c.start, c.end)
		}
	}
}

func TestParseRangeRejects(t *testing.T) {
	today := New(2026, time.August, 6)
	cases := []struct {
		in  string
		why string
	}{
		{"2026-08-27..2026-08-20", "backwards range"},
		{"..today", "missing start"},
		{"today..", "missing end"},
		{"..", "no dates at all"},
		{"someday..today", "unparsable start"},
		{"today..someday", "unparsable end"},
		{"a..b..c", "more than two endpoints"},
		{"", "empty"},
	}
	for _, c := range cases {
		if start, end, err := ParseRange(c.in, today); err == nil {
			t.Errorf("ParseRange(%q) = %s..%s, want an error (%s)", c.in, start, end, c.why)
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

func TestParseWeekday(t *testing.T) {
	cases := []struct {
		in   string
		want time.Weekday
	}{
		{"sun", time.Sunday},
		{"Sunday", time.Sunday},
		{"SAT", time.Saturday},
		{"  weds  ", time.Wednesday},
		{"thurs", time.Thursday},
	}
	for _, c := range cases {
		got, ok := ParseWeekday(c.in)
		if !ok {
			t.Errorf("ParseWeekday(%q) not recognized", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("ParseWeekday(%q) = %s, want %s", c.in, got, c.want)
		}
	}

	for _, bad := range []string{"", "someday", "sundae", "1"} {
		if got, ok := ParseWeekday(bad); ok {
			t.Errorf("ParseWeekday(%q) = %s, want rejection", bad, got)
		}
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
