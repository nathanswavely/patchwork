package eventsource

import (
	"context"
	"testing"
	"time"
)

// withZone sets the floating zone for one test and restores it after,
// so a test that leaves it set can't quietly decide another's outcome.
func withZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	prev := FloatingZone()
	SetFloatingZone(loc)
	t.Cleanup(func() { SetFloatingZone(prev) })
	return loc
}

// The reported bug, at its smallest: a feed says 7pm and means 7pm.
func TestParseICS_FloatingTimeIsNotUTC(t *testing.T) {
	withZone(t, "America/New_York")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:floating-1
SUMMARY:The Nancy Reagans
DTSTART:20260821T190000
DTEND:20260821T220000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	// 19:00 EDT is 23:00Z. Read as UTC it was 19:00Z, which renders back
	// in Eastern as 3:00 PM — the four-hour slip that started this.
	if items[0].StartsAt != "2026-08-21T23:00:00Z" {
		t.Errorf("StartsAt = %s, want 2026-08-21T23:00:00Z", items[0].StartsAt)
	}
	if items[0].EndsAt == nil || *items[0].EndsAt != "2026-08-22T02:00:00Z" {
		t.Errorf("EndsAt = %v, want 2026-08-22T02:00:00Z", items[0].EndsAt)
	}
}

// A calendar that names its own zone outranks the instance's.
func TestParseICS_XWRTimezoneWinsOverConfiguredZone(t *testing.T) {
	withZone(t, "America/New_York")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-TIMEZONE:America/Los_Angeles
BEGIN:VEVENT
UID:floating-2
SUMMARY:West Coast Show
DTSTART:20260821T190000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].StartsAt != "2026-08-22T02:00:00Z" {
		t.Fatalf("StartsAt = %v, want 2026-08-22T02:00:00Z (19:00 PDT)", items)
	}
}

// An all-day event is a date, and a date read in the wrong zone is a
// different day. Midnight UTC is the previous evening in Eastern.
func TestParseICS_AllDayDateLandsOnItsOwnDay(t *testing.T) {
	withZone(t, "America/New_York")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:allday-1
SUMMARY:Gallery Row Open House
DTSTART;VALUE=DATE:20260821
DTEND;VALUE=DATE:20260822
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].StartsAt != "2026-08-21T04:00:00Z" {
		t.Errorf("StartsAt = %s, want 2026-08-21T04:00:00Z (midnight EDT)", items[0].StartsAt)
	}
	// The day a reader in the venue's own zone sees.
	got := mustParse(t, items[0].StartsAt).In(FloatingZone()).Format("2006-01-02")
	if got != "2026-08-21" {
		t.Errorf("renders on %s, want 2026-08-21", got)
	}
}

// A TZID go's tzdata doesn't know used to make DateTimeStart fail, and a
// failed start drops the event from the desired set — which the
// reconciler cannot tell apart from a cancellation. Outlook writes these
// on every invitation it exports.
func TestParseICS_WindowsTZIDIsResolvedNotDropped(t *testing.T) {
	withZone(t, "UTC") // so a fallback to the instance zone would be visibly wrong
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:outlook-1
SUMMARY:Board Meeting
DTSTART;TZID="Eastern Standard Time":20260821T190000
DTEND;TZID="Eastern Standard Time":20260821T200000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("event was dropped: got %d items", len(items))
	}
	if items[0].StartsAt != "2026-08-21T23:00:00Z" {
		t.Errorf("StartsAt = %s, want 2026-08-21T23:00:00Z", items[0].StartsAt)
	}
}

// A TZID nothing can resolve still must not delete the event. It falls
// back to the calendar's zone, which is the same treatment a feed that
// named no zone at all gets.
func TestParseICS_UnresolvableTZIDFallsBackInsteadOfDropping(t *testing.T) {
	withZone(t, "America/New_York")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:mystery-1
SUMMARY:Somebody's Homegrown Feed
DTSTART;TZID=Customized Time Zone:20260821T190000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("event was dropped: got %d items", len(items))
	}
	if items[0].StartsAt != "2026-08-21T23:00:00Z" {
		t.Errorf("StartsAt = %s, want 2026-08-21T23:00:00Z", items[0].StartsAt)
	}
}

// A TZID that carries its own offset is honoured even though it names no
// tzdata zone.
func TestParseICS_TZIDCarryingItsOwnOffset(t *testing.T) {
	withZone(t, "UTC")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:offset-1
SUMMARY:Offset In The Name
DTSTART;TZID="(UTC-05:00) Eastern Time (US & Canada)":20260821T190000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].StartsAt != "2026-08-22T00:00:00Z" {
		t.Fatalf("StartsAt = %v, want 2026-08-22T00:00:00Z (19:00 at -05:00)", items)
	}
}

// A weekly show is at the same o'clock all year. Expanded in UTC it
// slides an hour the week the clocks change; expanded in the calendar's
// zone it doesn't.
func TestParseICS_RecurrenceHoldsItsClockAcrossDST(t *testing.T) {
	withZone(t, "America/New_York")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:weekly-1
SUMMARY:Open Mic
DTSTART:20261023T190000
RRULE:FREQ=WEEKLY;COUNT=6
END:VEVENT
END:VCALENDAR`
	// US DST ends 2026-11-01, so this run straddles it.
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("expected 6 occurrences, got %d", len(items))
	}
	for _, it := range items {
		local := mustParse(t, it.StartsAt).In(FloatingZone())
		if local.Hour() != 19 || local.Minute() != 0 {
			t.Errorf("occurrence %s renders locally at %s, want 19:00", it.StartsAt, local.Format("15:04"))
		}
	}
}

// Times that carry their own offset are untouched by any of this.
func TestParseICS_ZonedTimesIgnoreTheFallbackZone(t *testing.T) {
	withZone(t, "Asia/Tokyo") // deliberately nothing to do with the feed
	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-TIMEZONE:Asia/Tokyo
BEGIN:VEVENT
UID:utc-1
SUMMARY:Stated In UTC
DTSTART:20260821T190000Z
END:VEVENT
BEGIN:VEVENT
UID:tzid-1
SUMMARY:Stated With A TZID
DTSTART;TZID=America/New_York:20260821T190000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byUID := map[string]Item{}
	for _, it := range items {
		byUID[it.UID] = it
	}
	if got := byUID["utc-1"].StartsAt; got != "2026-08-21T19:00:00Z" {
		t.Errorf("Z time = %s, want 2026-08-21T19:00:00Z", got)
	}
	if got := byUID["tzid-1"].StartsAt; got != "2026-08-21T23:00:00Z" {
		t.Errorf("TZID time = %s, want 2026-08-21T23:00:00Z", got)
	}
}

// The same slip on the other door: a CMS that writes schema.org markup
// straight out of its local wall clock.
func TestParseJSONLD_FloatingTimeIsNotUTC(t *testing.T) {
	withZone(t, "America/New_York")
	page := `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"MusicEvent","name":"The Nancy Reagans",
 "url":"https://venue.example/nancy","startDate":"2026-08-21T19:00:00",
 "endDate":"2026-08-21T22:00:00"}
</script></head><body></body></html>`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseJSONLD([]byte(page), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].StartsAt != "2026-08-21T23:00:00Z" {
		t.Errorf("StartsAt = %s, want 2026-08-21T23:00:00Z", items[0].StartsAt)
	}
	if items[0].EndsAt == nil || *items[0].EndsAt != "2026-08-22T02:00:00Z" {
		t.Errorf("EndsAt = %v, want 2026-08-22T02:00:00Z", items[0].EndsAt)
	}
}

// A schema.org startDate that states an offset keeps it.
func TestParseJSONLD_StatedOffsetsIgnoreTheFallbackZone(t *testing.T) {
	withZone(t, "Asia/Tokyo")
	page := `<html><script type="application/ld+json">
{"@type":"Event","name":"Stated","url":"https://venue.example/stated",
 "startDate":"2026-08-21T19:00:00-04:00"}
</script></html>`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseJSONLD([]byte(page), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].StartsAt != "2026-08-21T23:00:00Z" {
		t.Fatalf("StartsAt = %v, want 2026-08-21T23:00:00Z", items)
	}
}

// A bare date in markup is a day, not an instant at Greenwich.
func TestParseJSONLD_BareDateLandsOnItsOwnDay(t *testing.T) {
	withZone(t, "America/New_York")
	page := `<html><script type="application/ld+json">
{"@type":"Event","name":"All Day","url":"https://venue.example/allday",
 "startDate":"2026-08-21"}
</script></html>`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseJSONLD([]byte(page), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].StartsAt != "2026-08-21T04:00:00Z" {
		t.Fatalf("StartsAt = %v, want 2026-08-21T04:00:00Z (midnight EDT)", items)
	}
}

func TestResolveTZID(t *testing.T) {
	cases := []struct {
		tzid string
		want string // IANA name, or "" when the result is a fixed zone
		ok   bool
	}{
		{"America/New_York", "America/New_York", true},
		{`"America/New_York"`, "America/New_York", true},
		{"Eastern Standard Time", "America/New_York", true},
		{"eastern standard time", "America/New_York", true},
		// Half the Windows ids carry a period and half don't; both forms
		// have to land on the same zone.
		{"W. Europe Standard Time", "Europe/Berlin", true},
		{"W Europe Standard Time", "Europe/Berlin", true},
		{"E. South America Standard Time", "America/Sao_Paulo", true},
		{"  Eastern   Standard Time ", "America/New_York", true},
		{"(UTC-05:00) Eastern Time (US & Canada)", "", true},
		{"UTC", "UTC", true},
		{"Customized Time Zone", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		loc, iana, ok := resolveTZID(c.tzid)
		if ok != c.ok {
			t.Errorf("resolveTZID(%q) ok = %v, want %v", c.tzid, ok, c.ok)
			continue
		}
		if ok && loc == nil {
			t.Errorf("resolveTZID(%q) returned ok with a nil location", c.tzid)
		}
		if iana != c.want {
			t.Errorf("resolveTZID(%q) iana = %q, want %q", c.tzid, iana, c.want)
		}
	}
}

func TestFloatingZoneDefaultsToUTC(t *testing.T) {
	prev := FloatingZone()
	t.Cleanup(func() { SetFloatingZone(prev) })
	SetFloatingZone(nil)
	if FloatingZone() != time.UTC {
		t.Errorf("FloatingZone() = %v, want UTC", FloatingZone())
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

// End to end, because the parser being right is not the same as the
// right instant reaching the events table.
func TestSync_FloatingFeedTimeIsStoredInTheVenuesZone(t *testing.T) {
	withZone(t, "America/New_York")
	db := setupTestDB(t)

	// A floating DTSTART two days out, written as a bare wall clock the
	// way a CMS export does.
	local := time.Now().In(FloatingZone()).Add(48 * time.Hour).Truncate(time.Hour)
	wall := local.Format("20060102T150405")
	feed := newFeedServer(t, wrap(vevent("floating@test", "The Nancy Reagans", wall)))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var startsAt string
	if err := db.QueryRow(
		`SELECT starts_at FROM events WHERE source_id = ?`, sourceID,
	).Scan(&startsAt); err != nil {
		t.Fatalf("read stored event: %v", err)
	}
	// Read back in the venue's zone, it must be the o'clock the feed said.
	got := mustParse(t, startsAt).In(FloatingZone())
	if !got.Equal(local) {
		t.Errorf("stored %s, which is %s locally; feed said %s",
			startsAt, got.Format(time.RFC3339), local.Format(time.RFC3339))
	}
}

// Migration 056's premise: a source that already holds a wrong time gets
// it corrected by an ordinary re-sync, in place, without the event
// losing its id.
func TestSync_RepairsAWrongStoredTimeInPlace(t *testing.T) {
	withZone(t, "America/New_York")
	db := setupTestDB(t)

	local := time.Now().In(FloatingZone()).Add(48 * time.Hour).Truncate(time.Hour)
	wall := local.Format("20060102T150405")
	feed := newFeedServer(t, wrap(vevent("floating@test", "The Nancy Reagans", wall)))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	var id, before string
	if err := db.QueryRow(
		`SELECT id, starts_at FROM events WHERE source_id = ?`, sourceID,
	).Scan(&id, &before); err != nil {
		t.Fatalf("read stored event: %v", err)
	}

	// Put the row back the way the old parser would have left it: the
	// same wall clock, stamped UTC.
	wrong := local.UTC().Add(-time.Duration(zoneOffset(local)) * time.Second).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE events SET starts_at = ? WHERE id = ?`, wrong, id); err != nil {
		t.Fatalf("plant wrong time: %v", err)
	}

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("resync: %v", err)
	}
	var afterID, after string
	if err := db.QueryRow(
		`SELECT id, starts_at FROM events WHERE source_id = ?`, sourceID,
	).Scan(&afterID, &after); err != nil {
		t.Fatalf("read repaired event: %v", err)
	}
	if afterID != id {
		t.Errorf("event was replaced (%s → %s), not repaired", id, afterID)
	}
	if after != before {
		t.Errorf("starts_at = %s after resync, want %s", after, before)
	}
}

// zoneOffset is the offset in seconds that t's zone was at, which is
// exactly what the old UTC reading threw away.
func zoneOffset(t time.Time) int {
	_, off := t.Zone()
	return off
}

// A TZID is allowed to be a quoted string, and go-ical hands the quotes
// straight to time.LoadLocation. A perfectly ordinary IANA zone written
// the quoted way must not fall off the ladder.
func TestParseICS_QuotedIANATZID(t *testing.T) {
	withZone(t, "UTC")
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:quoted-1
SUMMARY:Quoted Zone
DTSTART;TZID="America/New_York":20260821T190000
END:VEVENT
END:VCALENDAR`
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	items, err := ParseICS([]byte(ics), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].StartsAt != "2026-08-21T23:00:00Z" {
		t.Fatalf("StartsAt = %v, want 2026-08-21T23:00:00Z", items)
	}
}

// Every key has to be in its folded form, or it is a row that can never
// be reached — "Cen. Australia Standard Time" folds to a key without the
// period, and a table entry that kept the period would look present and
// never match.
func TestWindowsZoneTableIsFoldedAndResolvable(t *testing.T) {
	for key, iana := range windowsZones {
		if folded := foldZoneName(key); folded != key {
			t.Errorf("key %q is unreachable: folds to %q", key, folded)
		}
		if _, err := time.LoadLocation(iana); err != nil {
			t.Errorf("key %q maps to %q, which tzdata does not have: %v", key, iana, err)
		}
	}
}
