package eventsource

// End-to-end: an atproto source through resolution, listRecords, parsing,
// and the ADR 031 reconciler that every source type shares.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/atproto"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// stubResolver serves canned documents. A did:web resolves over https by
// definition, so there is no httptest server that could stand in.
func stubResolver(t *testing.T, pages map[string]string) {
	t.Helper()
	prev := atprotoResolver
	t.Cleanup(func() { atprotoResolver = prev })
	atprotoResolver = func() atproto.Resolver {
		return atproto.Resolver{
			Get: func(url string) (*http.Response, error) {
				body, ok := pages[url]
				if !ok {
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			},
		}
	}
}

func lexRecord(rkey, name, startsAt string) map[string]any {
	return map[string]any{
		"uri": "at://did:web:tellus.example/community.lexicon.calendar.event/" + rkey,
		"cid": "bafy" + rkey,
		"value": map[string]any{
			"name": name, "createdAt": "2026-08-01T00:00:00Z", "startsAt": startsAt,
		},
	}
}

func listBody(t *testing.T, records ...map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"records": records})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const didDoc = `{
  "id": "did:web:tellus.example",
  "alsoKnownAs": ["at://tellus.example"],
  "service": [{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds.example"}]
}`

func atprotoPages(t *testing.T, records ...map[string]any) map[string]string {
	t.Helper()
	listURL := "https://pds.example/xrpc/com.atproto.repo.listRecords?collection=" +
		"community.lexicon.calendar.event&limit=100&repo=did%3Aweb%3Atellus.example"
	return map[string]string{
		"https://tellus.example/.well-known/did.json": didDoc,
		listURL: listBody(t, records...),
	}
}

func seedATProtoSource(t *testing.T, db *database.DB) string {
	t.Helper()
	return seedSourceOfType(t, db, "atproto", "at://did:web:tellus.example/"+atproto.EventCollection)
}

func TestSync_ATProtoSourceImportsEvents(t *testing.T) {
	db := setupTestDB(t)
	start := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	stubResolver(t, atprotoPages(t,
		lexRecord("3lone", "Irish Session Night", start),
		lexRecord("3ltwo", "Roots Revival", time.Now().UTC().Add(72*time.Hour).Format(time.RFC3339)),
	))
	sourceID := seedATProtoSource(t, db)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n := countEvents(t, db, sourceID); n != 2 {
		t.Fatalf("want 2 imported events, got %d", n)
	}

	// ADR 031: attaching vouched once, so imported events publish.
	var status, visibility string
	if err := db.QueryRow(`SELECT status, visibility FROM events WHERE source_id = ? LIMIT 1`, sourceID).Scan(&status, &visibility); err != nil {
		t.Fatalf("inspect event: %v", err)
	}
	if status != "active" || visibility != "public" {
		t.Errorf("imported event is %s/%s, want active/public", status, visibility)
	}
}

// The ADR 031 rule that matters most, now reached through a new transport:
// a repository that stops answering must not wipe a venue's calendar.
func TestSync_ATProtoUnreachableRepoDeletesNothing(t *testing.T) {
	db := setupTestDB(t)
	start := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	stubResolver(t, atprotoPages(t, lexRecord("3lone", "Irish Session Night", start)))
	sourceID := seedATProtoSource(t, db)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// The PDS goes away entirely.
	stubResolver(t, map[string]string{})
	if err := Sync(context.Background(), db, nil, sourceID); err == nil {
		t.Fatal("expected the failed sync to report an error")
	}
	if n := countEvents(t, db, sourceID); n != 1 {
		t.Errorf("an unreachable repo removed events: %d left", n)
	}
	status, lastError := sourceState(t, db, sourceID)
	if status != "error" || lastError == nil {
		t.Errorf("source state after failure: %s / %v", status, lastError)
	}
}

// A record dropped from the collection withdraws its event — the single
// removal path (docs/adr/063 rejects trusting the lexicon's status field).
// A future event the feed no longer carries is a promise withdrawn, so the
// row goes; ADR 031's protection is for PAST events, covered by the ICS
// tests and shared by every source type.
func TestSync_ATProtoVanishedRecordWithdrawsTheEvent(t *testing.T) {
	db := setupTestDB(t)
	start := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	both := atprotoPages(t,
		lexRecord("3lone", "Irish Session Night", start),
		lexRecord("3ltwo", "Roots Revival", time.Now().UTC().Add(72*time.Hour).Format(time.RFC3339)),
	)
	stubResolver(t, both)
	sourceID := seedATProtoSource(t, db)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if n := countEvents(t, db, sourceID); n != 2 {
		t.Fatalf("setup: want 2 events, got %d", n)
	}

	stubResolver(t, atprotoPages(t, lexRecord("3lone", "Irish Session Night", start)))
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if n := countEvents(t, db, sourceID); n != 1 {
		t.Errorf("want the dropped record's event withdrawn, %d events remain", n)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM events WHERE source_id = ?`, sourceID).Scan(&title); err != nil {
		t.Fatalf("read survivor: %v", err)
	}
	if title != "Irish Session Night" {
		t.Errorf("the wrong event survived: %q", title)
	}
}
