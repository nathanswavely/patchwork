package eventsource

import (
	"context"
	"testing"
	"time"
)

// The real defect, from Tellus360's markup: the page says "21+ | 7pm" and
// the schema.org block beside it says "2026-08-28T15:00:00-04:00". That is
// 19:00Z — a faithful rendering of the wrong instant, which is why every
// parser believed it.
func TestReinterpretUTCAsLocal_TheTellus360Case(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	// What the feed publishes for a 7pm show, in the two forms it can take.
	for _, published := range []string{
		"2026-08-28T15:00:00-04:00", // as printed
		"2026-08-28T19:00:00Z",      // the same instant, normalized
	} {
		got := ReinterpretUTCAsLocal(published, ny)
		if got != "2026-08-28T23:00:00Z" {
			t.Errorf("%s → %s, want 2026-08-28T23:00:00Z (7pm EDT)", published, got)
		}
	}
}

// The reason this is a switch and not a number of hours. The publisher's
// error equals the venue's UTC offset, so it is four hours in summer and
// five in winter; a stored "+4" would be right today and silently wrong
// from the first Sunday in November.
func TestReinterpretUTCAsLocal_SurvivesDaylightSaving(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	cases := []struct {
		name, published, want string
		shift                 time.Duration
	}{
		{"August, EDT", "2026-08-28T19:00:00Z", "2026-08-28T23:00:00Z", 4 * time.Hour},
		{"January, EST", "2026-01-15T19:00:00Z", "2026-01-16T00:00:00Z", 5 * time.Hour},
	}
	for _, c := range cases {
		got := ReinterpretUTCAsLocal(c.published, ny)
		if got != c.want {
			t.Errorf("%s: %s → %s, want %s", c.name, c.published, got, c.want)
		}
		// And the same 7pm wall clock comes back out in both seasons,
		// which is the property a fixed offset cannot hold.
		if local := mustParse(t, got).In(ny); local.Hour() != 19 {
			t.Errorf("%s: renders locally at %s, want 19:00", c.name, local.Format("15:04"))
		}
		if gap := mustParse(t, got).Sub(mustParse(t, c.published)); gap != c.shift {
			t.Errorf("%s: shifted by %s, want %s", c.name, gap, c.shift)
		}
	}
}

func TestReinterpretUTCAsLocal_LeavesUnparsableValuesAlone(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	for _, s := range []string{"", "not-a-time", "2026-08-28"} {
		if got := ReinterpretUTCAsLocal(s, ny); got != s {
			t.Errorf("%q → %q, want it untouched", s, got)
		}
	}
	if got := ReinterpretUTCAsLocal("2026-08-28T19:00:00Z", nil); got != "2026-08-28T19:00:00Z" {
		t.Errorf("nil zone changed the value: %s", got)
	}
}

// Occurrence is the reconciler's identity key, not a time anyone reads.
// Shifting it would turn the first sync after the switch into a
// delete-and-reinsert of every recurring event.
func TestCorrectStampedUTC_LeavesTheIdentityKeyAlone(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	ends := "2026-08-28T21:00:00Z"
	items := []Item{{
		UID:        "weekly@test",
		Occurrence: "2026-08-28T19:00:00Z",
		StartsAt:   "2026-08-28T19:00:00Z",
		EndsAt:     &ends,
	}}
	out := correctStampedUTC(items, ny)
	if out[0].Occurrence != "2026-08-28T19:00:00Z" {
		t.Errorf("Occurrence moved to %s — the event would be re-keyed", out[0].Occurrence)
	}
	if out[0].StartsAt != "2026-08-28T23:00:00Z" {
		t.Errorf("StartsAt = %s, want 2026-08-28T23:00:00Z", out[0].StartsAt)
	}
	if *out[0].EndsAt != "2026-08-29T01:00:00Z" {
		t.Errorf("EndsAt = %s, want 2026-08-29T01:00:00Z", *out[0].EndsAt)
	}
}

// End to end: a feed publishing a correct-looking but wrong offset, on a
// source with the switch on.
func TestSync_CorrectsAPublisherThatStampsLocalAsUTC(t *testing.T) {
	db := setupTestDB(t)

	// A 7pm show, published the way Tellus360 publishes it.
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour).Format("20060102T150405Z")
	feed := newFeedServer(t, wrap(vevent("stamped@test", "A 7pm Show", start)))
	sourceID := seedSource(t, db, feed.srv.URL)
	setPatchZone(t, db, sourceID, "America/New_York")

	// Believed as published, first.
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var id, before string
	if err := db.QueryRow(`SELECT id, starts_at FROM events WHERE source_id = ?`, sourceID).
		Scan(&id, &before); err != nil {
		t.Fatalf("read: %v", err)
	}

	// The admin looks at the venue's page, sees the markup disagrees with
	// it, and says so.
	if _, err := db.Exec(`UPDATE event_sources SET local_time_stamped_utc = 1, etag = NULL, last_modified = NULL WHERE id = ?`, sourceID); err != nil {
		t.Fatalf("set switch: %v", err)
	}
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("resync: %v", err)
	}

	var afterID, after string
	if err := db.QueryRow(`SELECT id, starts_at FROM events WHERE source_id = ?`, sourceID).
		Scan(&afterID, &after); err != nil {
		t.Fatalf("read after: %v", err)
	}
	if afterID != id {
		t.Errorf("event was replaced (%s → %s), not corrected in place", id, afterID)
	}
	ny, _ := time.LoadLocation("America/New_York")
	wantShift := mustParse(t, before).In(ny)
	_, off := wantShift.Zone()
	if gap := mustParse(t, after).Sub(mustParse(t, before)); gap != time.Duration(-off)*time.Second {
		t.Errorf("moved by %s, want %s (the venue's offset)", gap, time.Duration(-off)*time.Second)
	}
	// The published wall clock is what the event now starts at locally.
	pubClock := mustParse(t, before).UTC().Format("15:04")
	if got := mustParse(t, after).In(ny).Format("15:04"); got != pubClock {
		t.Errorf("local clock = %s, want %s (the digits the publisher wrote)", got, pubClock)
	}
}

// The switch off is the default and changes nothing.
func TestSync_UntouchedSourcesAreStillBelieved(t *testing.T) {
	db := setupTestDB(t)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour).Format("20060102T150405Z")
	feed := newFeedServer(t, wrap(vevent("plain@test", "Correctly Published", start)))
	sourceID := seedSource(t, db, feed.srv.URL)
	setPatchZone(t, db, sourceID, "America/New_York")

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var got string
	if err := db.QueryRow(`SELECT starts_at FROM events WHERE source_id = ?`, sourceID).Scan(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	want := mustParse(t, time.Now().UTC().Add(48*time.Hour).Truncate(time.Hour).Format(time.RFC3339)).Format(time.RFC3339)
	if got != want {
		t.Errorf("starts_at = %s, want %s — a source nobody flagged must be believed", got, want)
	}
}
