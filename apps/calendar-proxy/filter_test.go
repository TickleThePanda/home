package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Fixtures are synthetic: made-up summaries, UIDs and example.com people.

var now = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

const self = "me@example.com"

func calendar(events ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:Europe/London\r\nEND:VTIMEZONE\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:Asia/Tokyo\r\nEND:VTIMEZONE\r\n" +
		strings.Join(events, "") + "END:VCALENDAR\r\n")
}

func event(uid, summary string, lines ...string) string {
	return "BEGIN:VEVENT\r\nUID:" + uid + "\r\nSUMMARY:" + summary + "\r\n" +
		strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\n"
}

func run(t *testing.T, src []byte, opts ...func(*Options)) (string, Stats) {
	t.Helper()
	o := Options{Now: now, PastDays: 90, MaxBytes: 1 << 20, Self: self}
	for _, f := range opts {
		f(&o)
	}
	out, st, err := Filter(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out); err != nil {
		t.Fatalf("output doesn't parse: %v", err)
	}
	return string(out), st
}

func assertHas(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

func assertLacks(t *testing.T, out string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(out, u) {
			t.Errorf("output has %q:\n%s", u, out)
		}
	}
}

func TestStandaloneEvents(t *testing.T) {
	out, st := run(t, calendar(
		event("a", "Future meeting", "DTSTART:20261120T100000Z", "DTEND:20261120T110000Z"),
		event("b", "Cancelled meeting", "DTSTART:20261121T100000Z", "DTEND:20261121T110000Z", "STATUS:CANCELLED"),
		event("c", "Declined meeting", "DTSTART:20261122T100000Z", "DTEND:20261122T110000Z",
			"ATTENDEE;CN=Me;PARTSTAT=DECLINED:mailto:ME@example.com",
			"ATTENDEE;PARTSTAT=ACCEPTED:mailto:other@example.com"),
		event("d", "Someone else declined", "DTSTART:20261123T100000Z", "DTEND:20261123T110000Z",
			"ATTENDEE;PARTSTAT=ACCEPTED:mailto:me@example.com",
			"ATTENDEE;PARTSTAT=DECLINED:mailto:other@example.com"),
		event("e", "Recent past", "DTSTART:20261001T100000Z", "DTEND:20261001T110000Z"),
		event("f", "Ancient past", "DTSTART:20200101T100000Z", "DTEND:20200101T110000Z"),
		event("g", "Ongoing trip", "DTSTART;VALUE=DATE:20260701", "DTEND;VALUE=DATE:20261201"),
		event("h", "Old all-day", "DTSTART;VALUE=DATE:20250101"),
	))
	assertHas(t, out, "Future meeting", "Someone else declined", "Recent past", "Ongoing trip")
	assertLacks(t, out, "Cancelled meeting", "Declined meeting", "Ancient past", "Old all-day", "ATTENDEE")
	want := map[string]int{"cancelled": 1, "declined": 1, "old": 2}
	if fmt.Sprint(st.Dropped) != fmt.Sprint(want) {
		t.Errorf("dropped = %v, want %v", st.Dropped, want)
	}
}

func TestSeries(t *testing.T) {
	out, st := run(t, calendar(
		// Weekly from 2024, ongoing, with:
		event("weekly", "Weekly sync",
			"DTSTART;TZID=Europe/London:20240108T090000", "DTEND;TZID=Europe/London:20240108T093000",
			"RRULE:FREQ=WEEKLY;BYDAY=MO"),
		// a future instance cancelled,
		event("weekly", "Weekly sync", "RECURRENCE-ID;TZID=Europe/London:20261123T090000",
			"DTSTART;TZID=Europe/London:20261123T090000", "DTEND;TZID=Europe/London:20261123T093000",
			"STATUS:CANCELLED"),
		// a future instance declined,
		event("weekly", "Weekly sync", "RECURRENCE-ID;TZID=Europe/London:20261130T090000",
			"DTSTART;TZID=Europe/London:20261130T090000", "DTEND;TZID=Europe/London:20261130T093000",
			"ATTENDEE;PARTSTAT=DECLINED:mailto:me@example.com"),
		// a future instance moved,
		event("weekly", "Weekly sync (moved)", "RECURRENCE-ID;TZID=Europe/London:20261207T090000",
			"DTSTART;TZID=Europe/London:20261208T140000", "DTEND;TZID=Europe/London:20261208T143000"),
		// and an old moved instance.
		event("weekly", "Weekly sync (old move)", "RECURRENCE-ID;TZID=Europe/London:20250106T090000",
			"DTSTART;TZID=Europe/London:20250107T090000", "DTEND;TZID=Europe/London:20250107T093000"),

		event("until", "Ended by UNTIL", "DTSTART:20240101T100000Z", "DTEND:20240101T110000Z",
			"RRULE:FREQ=DAILY;UNTIL=20240301T000000Z"),
		event("until", "Ended by UNTIL", "RECURRENCE-ID:20240105T100000Z",
			"DTSTART:20240105T120000Z", "DTEND:20240105T130000Z"),
		event("count", "Ended by COUNT", "DTSTART:20250101T100000Z", "DTEND:20250101T110000Z",
			"RRULE:FREQ=WEEKLY;COUNT=10"),
		event("recent", "Ended recently", "DTSTART:20260101T100000Z", "DTEND:20260101T110000Z",
			"RRULE:FREQ=WEEKLY;UNTIL=20261101T000000Z"),
		// Ended, but one instance was moved into the future.
		event("moved", "Moved past the end", "DTSTART:20250101T100000Z", "DTEND:20250101T110000Z",
			"RRULE:FREQ=WEEKLY;COUNT=3"),
		event("moved", "Moved past the end", "RECURRENCE-ID:20250108T100000Z",
			"DTSTART:20261201T100000Z", "DTEND:20261201T110000Z"),
		// The whole series declined.
		event("declined", "Declined series", "DTSTART:20260101T100000Z", "DTEND:20260101T110000Z",
			"RRULE:FREQ=WEEKLY", "ATTENDEE;PARTSTAT=DECLINED:mailto:me@example.com"),
		event("declined", "Declined series", "RECURRENCE-ID:20261123T100000Z",
			"DTSTART:20261123T110000Z", "DTEND:20261123T120000Z"),
	))

	assertHas(t, out,
		"Weekly sync (moved)",
		// Dropped instances become EXDATEs in the original format.
		"EXDATE;TZID=Europe/London:20261123T090000",
		"EXDATE;TZID=Europe/London:20261130T090000",
		"EXDATE;TZID=Europe/London:20250106T090000",
		"Ended recently",
		"Moved past the end",
	)
	assertLacks(t, out, "Weekly sync (old move)", "Ended by UNTIL", "Ended by COUNT", "Declined series",
		"RECURRENCE-ID;TZID=Europe/London:20261123T090000",
		"RECURRENCE-ID;TZID=Europe/London:20261130T090000")
	if got := strings.Count(out, "BEGIN:VEVENT"); got != 5 {
		t.Errorf("kept %d events, want 5:\n%s", got, out)
	}
	if st.Dropped["old"] != 4 || st.Dropped["cancelled"] != 1 || st.Dropped["declined"] != 3 {
		t.Errorf("dropped = %v", st.Dropped)
	}
	// Asia/Tokyo is unused now.
	assertLacks(t, out, "Asia/Tokyo")
	assertHas(t, out, "TZID:Europe/London")
}

func TestOrphanOverride(t *testing.T) {
	// An invite to one instance of a series whose master isn't in the feed.
	out, _ := run(t, calendar(
		event("o1", "Single instance", "RECURRENCE-ID:20261125T100000Z",
			"DTSTART:20261125T100000Z", "DTEND:20261125T110000Z"),
		event("o2", "Old single instance", "RECURRENCE-ID:20200101T100000Z",
			"DTSTART:20200101T100000Z", "DTEND:20200101T110000Z"),
	))
	assertHas(t, out, "Single instance")
	assertLacks(t, out, "Old single instance", "EXDATE")
}

func TestDescriptionTruncated(t *testing.T) {
	long := strings.Repeat("é", 600) + `\n` + strings.Repeat("x", 600)
	out, _ := run(t, calendar(
		event("a", "Meeting", "DTSTART:20261120T100000Z", "DTEND:20261120T110000Z",
			"DESCRIPTION:"+long, "X-ALT-DESC;FMTTYPE=text/html:<p>big</p>"),
	))
	root, _ := Parse([]byte(out))
	desc := firstEvent(root).Value("DESCRIPTION")
	if len(desc) > descriptionLimit+len("…") || !strings.HasSuffix(desc, "…") {
		t.Errorf("description not truncated: %d bytes", len(desc))
	}
	assertLacks(t, out, "X-ALT-DESC")
}

func TestSizeDegradation(t *testing.T) {
	var events []string
	// 60 days of past and future short meetings, each with a description.
	for d := -60; d <= 60; d++ {
		day := now.AddDate(0, 0, d).Format("20060102")
		events = append(events, event(fmt.Sprintf("m%d", d), fmt.Sprintf("Meeting %d", d),
			"DTSTART:"+day+"T100000Z", "DTEND:"+day+"T110000Z",
			"DESCRIPTION:"+strings.Repeat("agenda ", 50)))
	}
	events = append(events,
		event("trip", "Far trip", "DTSTART;VALUE=DATE:20270301", "DTEND;VALUE=DATE:20270305"),
		event("long", "Far workshop", "DTSTART:20270310T090000Z", "DTEND:20270310T170000Z"),
	)
	src := calendar(events...)

	full, st := run(t, src)
	if len(st.Degraded) != 0 {
		t.Fatalf("degraded without a limit: %v", st.Degraded)
	}

	// Dropping descriptions alone is enough.
	out, st := run(t, src, func(o *Options) { o.MaxBytes = len(full) - 1000 })
	if fmt.Sprint(st.Degraded) != "[description]" || st.PastDays != 90 {
		t.Errorf("degraded = %v, past = %d", st.Degraded, st.PastDays)
	}
	assertLacks(t, out, "DESCRIPTION")

	// Tight: the past window shrinks to nothing, then far-future short
	// meetings go, but the trip and the long workshop stay.
	out, st = run(t, src, func(o *Options) { o.MaxBytes = 4000 })
	if fmt.Sprint(st.Degraded) != "[description past-window short-events]" || st.PastDays != 0 {
		t.Errorf("degraded = %v, past = %d", st.Degraded, st.PastDays)
	}
	if len(out) > 4000 {
		t.Errorf("output %d bytes, over limit", len(out))
	}
	assertHas(t, out, "Far trip", "Far workshop", "Meeting 1\r\n")
	assertLacks(t, out, "Meeting -1\r\n", "Meeting 60\r\n")
	if st.Dropped["size"] == 0 {
		t.Errorf("no size drops counted: %v", st.Dropped)
	}
}

func TestRoundTripFolding(t *testing.T) {
	long := strings.Repeat("ü", 100)
	src := calendar(event("a", long, "DTSTART:20261120T100000Z", "X-CUSTOM;X-P=\"a:b;c\":keep me"))
	out, _ := run(t, src)
	for _, line := range bytes.Split([]byte(out), []byte("\r\n")) {
		if len(line) > 75 {
			t.Errorf("line over 75 octets: %d", len(line))
		}
	}
	root, err := Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	ev := firstEvent(root)
	if ev.Value("SUMMARY") != long {
		t.Errorf("summary mangled by folding")
	}
	if p := ev.Prop("X-CUSTOM"); p == nil || p.Value != "keep me" || p.Param("X-P") != "a:b;c" {
		t.Errorf("X-CUSTOM = %+v", p)
	}
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"PT1H30M": 90 * time.Minute,
		"P2D":     48 * time.Hour,
		"P1W":     7 * 24 * time.Hour,
		"-PT15M":  -15 * time.Minute,
		"P1DT2H":  26 * time.Hour,
	} {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}

func firstEvent(cal *Component) *Component {
	for _, c := range cal.Children {
		if c.Name == "VEVENT" {
			return c
		}
	}
	return nil
}
