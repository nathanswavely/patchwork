package eventsource

import (
	"strings"
	"testing"
	"time"
)

// fixed "now" for every parser test: window is [Jul 20, Oct 19] 2026.
var testNow = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

func ics(body string) []byte {
	doc := "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\n" + body + "END:VCALENDAR\n"
	return []byte(strings.ReplaceAll(doc, "\n", "\r\n"))
}

func TestParseICS_SingleEventFields(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:one@test
SUMMARY:Open Mic
DESCRIPTION:Bring a song
LOCATION:The Selvage
GEO:40.038;-76.305
DTSTART:20260722T190000Z
DTEND:20260722T220000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(items), items)
	}
	it := items[0]
	if it.UID != "one@test" || it.Occurrence != "" {
		t.Errorf("identity: %q / %q", it.UID, it.Occurrence)
	}
	if it.Title != "Open Mic" || it.Description != "Bring a song" || it.Location != "The Selvage" {
		t.Errorf("fields: %+v", it)
	}
	if it.StartsAt != "2026-07-22T19:00:00Z" {
		t.Errorf("starts_at: %s", it.StartsAt)
	}
	if it.EndsAt == nil || *it.EndsAt != "2026-07-22T22:00:00Z" {
		t.Errorf("ends_at: %v", it.EndsAt)
	}
	if it.Latitude == nil || *it.Latitude != 40.038 || it.Longitude == nil || *it.Longitude != -76.305 {
		t.Errorf("geo: %v %v", it.Latitude, it.Longitude)
	}
}

// A weekly series: 5 occurrences, one EXDATEd away, one moved and
// retitled by an override. The override keeps the ORIGINAL occurrence as
// its identity, so a moved show updates in place instead of duplicating.
func TestParseICS_RecurrenceExpansion(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:weekly@test
SUMMARY:Weekly
DTSTART:20260722T230000Z
DTEND:20260723T010000Z
RRULE:FREQ=WEEKLY;COUNT=5
EXDATE:20260729T230000Z
END:VEVENT
BEGIN:VEVENT
UID:weekly@test
RECURRENCE-ID:20260805T230000Z
SUMMARY:Weekly (special)
DTSTART:20260805T200000Z
DTEND:20260805T220000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items (5 - exdate, override folded in), got %d: %+v", len(items), items)
	}

	byOcc := map[string]Item{}
	for _, it := range items {
		byOcc[it.Occurrence] = it
	}
	if _, exdated := byOcc["2026-07-29T23:00:00Z"]; exdated {
		t.Error("EXDATE occurrence survived")
	}

	plain, ok := byOcc["2026-07-22T23:00:00Z"]
	if !ok || plain.Title != "Weekly" || plain.StartsAt != "2026-07-22T23:00:00Z" {
		t.Errorf("plain occurrence: %+v", plain)
	}
	if plain.EndsAt == nil || *plain.EndsAt != "2026-07-23T01:00:00Z" {
		t.Errorf("duration-derived end: %v", plain.EndsAt)
	}

	special, ok := byOcc["2026-08-05T23:00:00Z"]
	if !ok {
		t.Fatalf("override occurrence missing: %+v", byOcc)
	}
	if special.Title != "Weekly (special)" || special.StartsAt != "2026-08-05T20:00:00Z" {
		t.Errorf("override should keep original occurrence identity but its own fields: %+v", special)
	}
}

func TestParseICS_Cancellations(t *testing.T) {
	// A cancelled single event is simply absent.
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:gone@test
SUMMARY:Cancelled Show
STATUS:CANCELLED
DTSTART:20260722T190000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("cancelled event emitted: %+v", items)
	}

	// A cancelled override removes just that occurrence.
	items, err = ParseICS(ics(`BEGIN:VEVENT
UID:series@test
SUMMARY:Series
DTSTART:20260722T190000Z
RRULE:FREQ=WEEKLY;COUNT=2
END:VEVENT
BEGIN:VEVENT
UID:series@test
RECURRENCE-ID:20260729T190000Z
STATUS:CANCELLED
SUMMARY:Series
DTSTART:20260729T190000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].Occurrence != "2026-07-22T19:00:00Z" {
		t.Errorf("expected only the first occurrence, got %+v", items)
	}
}

func TestParseICS_PrivateItemsNeverLeaveTheParser(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:private@test
SUMMARY:Board Meeting
CLASS:PRIVATE
DTSTART:20260722T190000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("private item emitted: %+v", items)
	}
}

func TestParseICS_Window(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:past@test
SUMMARY:Long Over
DTSTART:20260701T190000Z
END:VEVENT
BEGIN:VEVENT
UID:far@test
SUMMARY:Beyond Horizon
DTSTART:20261101T190000Z
END:VEVENT
BEGIN:VEVENT
UID:in@test
SUMMARY:Inside
DTSTART:20260801T190000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].UID != "in@test" {
		t.Errorf("window filter: %+v", items)
	}
}

func TestParseICS_TZIDConvertsToUTC(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:tz@test
SUMMARY:Evening Show
DTSTART;TZID=America/New_York:20260722T190000
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	// 7pm Eastern in July is EDT (UTC-4).
	if items[0].StartsAt != "2026-07-22T23:00:00Z" {
		t.Errorf("tz conversion: %s", items[0].StartsAt)
	}
}

func TestParseICS_UntitledAndMissingUID(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
SUMMARY:No UID
DTSTART:20260722T190000Z
END:VEVENT
BEGIN:VEVENT
UID:blank@test
DTSTART:20260723T190000Z
END:VEVENT
`), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected only the UID-bearing item, got %+v", items)
	}
	if items[0].Title != "(untitled)" {
		t.Errorf("untitled fallback: %q", items[0].Title)
	}
}
