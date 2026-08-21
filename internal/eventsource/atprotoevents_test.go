package eventsource_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/atproto"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
)

func rec(rkey string, value map[string]any) atproto.Record {
	b, _ := json.Marshal(value)
	return atproto.Record{URI: "at://did:plc:abc/community.lexicon.calendar.event/" + rkey, Value: b}
}

func mustParse(t *testing.T, records []atproto.Record, now time.Time) []eventsource.Item {
	t.Helper()
	items, err := eventsource.ParseATProtoEvents(records, now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return items
}

func TestParseATProtoEvents_ReadsARecord(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	items := mustParse(t, []atproto.Record{
		rec("3labc", map[string]any{
			"name":        "Irish Session Night",
			"description": "Weekly session.",
			"createdAt":   "2026-08-01T00:00:00Z",
			"startsAt":    "2026-08-24T23:00:00Z",
			"endsAt":      "2026-08-25T02:00:00Z",
			"locations": []any{map[string]any{
				"$type": "community.lexicon.location.address",
				"name":  "The Selvage Pub", "street": "119 N Water St",
			}},
			"uris": []any{map[string]any{"uri": "https://tellus.example/shows/1"}},
		}),
	}, now)

	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	it := items[0]
	if it.UID != "3labc" {
		t.Errorf("UID = %q, want the rkey", it.UID)
	}
	if it.Occurrence != "" {
		t.Errorf("Occurrence = %q; the lexicon has no recurrence", it.Occurrence)
	}
	if it.Title != "Irish Session Night" {
		t.Errorf("Title = %q", it.Title)
	}
	if it.StartsAt != "2026-08-24T23:00:00Z" {
		t.Errorf("StartsAt = %q", it.StartsAt)
	}
	if it.EndsAt == nil || *it.EndsAt != "2026-08-25T02:00:00Z" {
		t.Errorf("EndsAt = %v", it.EndsAt)
	}
	if it.Location != "The Selvage Pub, 119 N Water St" {
		t.Errorf("Location = %q, want name-first (docs/adr/046)", it.Location)
	}
	if it.URL != "https://tellus.example/shows/1" {
		t.Errorf("URL = %q", it.URL)
	}
}

// docs/adr/063 decision 3. The lexicon requires only createdAt and name,
// so this record is valid and still cannot become an event. Defaulting to
// createdAt would publish a show dated three weeks before it happens.
func TestParseATProtoEvents_SkipsRecordsWithNoStart(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	items := mustParse(t, []atproto.Record{
		rec("nostart", map[string]any{
			"name":      "Someday, Maybe",
			"createdAt": "2026-08-01T00:00:00Z",
		}),
		rec("badstart", map[string]any{
			"name":      "Sometime",
			"createdAt": "2026-08-01T00:00:00Z",
			"startsAt":  "next tuesday",
		}),
	}, now)

	if len(items) != 0 {
		t.Fatalf("want nothing published, got %d: %+v", len(items), items)
	}
}

// One bad record must not cost a venue its whole calendar.
func TestParseATProtoEvents_SurvivesAMalformedRecord(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	good := rec("good", map[string]any{
		"name": "Live Music", "createdAt": "2026-08-01T00:00:00Z",
		"startsAt": "2026-09-11T21:00:00Z",
	})
	bad := atproto.Record{
		URI:   "at://did:plc:abc/community.lexicon.calendar.event/bad",
		Value: json.RawMessage(`{"name": 12345}`),
	}
	items := mustParse(t, []atproto.Record{bad, good}, now)
	if len(items) != 1 || items[0].UID != "good" {
		t.Fatalf("want the good record only, got %+v", items)
	}
}

// The same window ICS sources use: yesterday through the horizon.
func TestParseATProtoEvents_WindowsByStart(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	items := mustParse(t, []atproto.Record{
		rec("old", map[string]any{"name": "Last Month", "createdAt": "2026-07-01T00:00:00Z", "startsAt": "2026-07-04T20:00:00Z"}),
		rec("soon", map[string]any{"name": "Tomorrow", "createdAt": "2026-08-01T00:00:00Z", "startsAt": "2026-08-22T20:00:00Z"}),
		rec("far", map[string]any{"name": "Next Year", "createdAt": "2026-08-01T00:00:00Z", "startsAt": "2027-08-22T20:00:00Z"}),
	}, now)

	if len(items) != 1 || items[0].UID != "soon" {
		t.Fatalf("want only the in-window item, got %+v", items)
	}
}

// An end before the start is discarded rather than stored: the reconciler
// would otherwise write an event that finishes before it begins.
func TestParseATProtoEvents_IgnoresAnEndBeforeTheStart(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	items := mustParse(t, []atproto.Record{
		rec("backwards", map[string]any{
			"name": "Backwards", "createdAt": "2026-08-01T00:00:00Z",
			"startsAt": "2026-08-24T23:00:00Z", "endsAt": "2026-08-24T09:00:00Z",
		}),
	}, now)
	if len(items) != 1 {
		t.Fatalf("want the event kept, got %d", len(items))
	}
	if items[0].EndsAt != nil {
		t.Errorf("EndsAt = %v, want it dropped", *items[0].EndsAt)
	}
}

func TestParseATURI(t *testing.T) {
	did, coll, err := atproto.ParseATURI("at://did:plc:abc/community.lexicon.calendar.event")
	if err != nil || did != "did:plc:abc" || coll != atproto.EventCollection {
		t.Fatalf("got %q %q %v", did, coll, err)
	}
	// A handle is not storable — decision 1 is that the DID is what lasts.
	for _, bad := range []string{"at://tellus.example/x", "https://x.example", "at://", "at://did:plc:abc"} {
		if _, _, err := atproto.ParseATURI(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}
