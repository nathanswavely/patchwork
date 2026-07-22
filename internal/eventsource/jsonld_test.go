package eventsource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// Fixture modeled on a real Humanitix host page: an ItemList of Events
// (offsets without colons), plus a standalone MusicEvent, a cancelled
// event, an ignored WebSite block, and one malformed block.
func ldFixture(now time.Time) string {
	// Humanitix writes offsets without a colon; render the instant in a
	// fixed -4 zone so the string genuinely carries that offset.
	edt := time.FixedZone("EDT", -4*3600)
	ev1 := now.Add(48 * time.Hour).In(edt).Format("2006-01-02T15:04:05-0700")
	ev1end := now.Add(51 * time.Hour).In(edt).Format("2006-01-02T15:04:05-0700")
	ev2 := now.Add(96 * time.Hour).Format(time.RFC3339)
	cancelled := now.Add(72 * time.Hour).Format(time.RFC3339)
	return `<!doctype html><html><head>
<script type="application/ld+json">{"@context":"https://schema.org","url":"https://example.test","name":"Site","@type":"WebSite"}</script>
<script type="application/ld+json">{not valid json</script>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"ItemList","itemListElement":[
 {"@type":"ListItem","position":0,"item":{"@type":"Event","name":"Latin Social Dance!","url":"https://events.example/latin-social",
  "startDate":"` + ev1 + `","endDate":"` + ev1end + `",
  "location":{"@type":"Place","name":"West Art","address":{"@type":"PostalAddress","streetAddress":"816 Buchanan Ave, Lancaster, PA"}},
  "eventStatus":"https://schema.org/EventScheduled","description":"<p>Monthly Latin dance nights &amp; more</p>"}},
 {"@type":"ListItem","position":1,"item":{"@type":"Event","name":"Cancelled Show","url":"https://events.example/cancelled",
  "startDate":"` + cancelled + `","eventStatus":"https://schema.org/EventCancelled"}}
]}</script>
</head><body>
<script type="application/ld+json">{"@type":"MusicEvent","name":"Standalone Gig","url":"https://events.example/gig",
 "startDate":"` + ev2 + `","location":"The Back Room",
 "geo-free":"x"}</script>
</body></html>`
}

func TestParseJSONLD(t *testing.T) {
	now := time.Now().UTC()
	items, err := ParseJSONLD([]byte(ldFixture(now)), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (cancelled dropped, WebSite ignored), got %d: %+v", len(items), items)
	}
	byUID := map[string]Item{}
	for _, it := range items {
		byUID[it.UID] = it
	}

	dance, ok := byUID["https://events.example/latin-social"]
	if !ok {
		t.Fatalf("ItemList event missing: %v", byUID)
	}
	if dance.Title != "Latin Social Dance!" {
		t.Errorf("title: %q", dance.Title)
	}
	if dance.Location != "West Art, 816 Buchanan Ave, Lancaster, PA" {
		t.Errorf("location: %q", dance.Location)
	}
	if dance.Description != "Monthly Latin dance nights & more" {
		t.Errorf("description strip: %q", dance.Description)
	}
	if dance.EndsAt == nil {
		t.Error("end date lost")
	}
	// The -0400 offset must land as the right UTC instant.
	want := time.Now().UTC().Add(48 * time.Hour)
	got, _ := time.Parse(time.RFC3339, dance.StartsAt)
	if d := got.Sub(want); d > 2*time.Minute || d < -2*time.Minute {
		t.Errorf("offset conversion off: %s vs ~%s", dance.StartsAt, want)
	}

	gig, ok := byUID["https://events.example/gig"]
	if !ok || gig.Location != "The Back Room" {
		t.Errorf("standalone MusicEvent with string location: %+v", gig)
	}

	if _, cancelled := byUID["https://events.example/cancelled"]; cancelled {
		t.Error("cancelled event emitted")
	}
}

func TestParseJSONLD_NoMarkupErrors(t *testing.T) {
	_, err := ParseJSONLD([]byte("<html><body>plain page</body></html>"), time.Now().UTC())
	if err == nil {
		t.Fatal("a page without Event markup must error so detection can move on")
	}
}

// Pasting a page with schema.org Event markup auto-detects the jsonld
// type from the body already fetched — no second request needed.
func TestSync_AutoDetectsJSONLD(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, ldFixture(now))
	}))
	t.Cleanup(srv.Close)
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })

	sourceID := seedSource(t, db, srv.URL+"/host/west-art")

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var typ string
	db.QueryRow(`SELECT type FROM event_sources WHERE id = ?`, sourceID).Scan(&typ)
	if typ != "jsonld" {
		t.Errorf("detected type: %q", typ)
	}
	if requests != 1 {
		t.Errorf("detection should reuse the fetched body, made %d requests", requests)
	}
	if n := countEvents(t, db, sourceID); n != 2 {
		t.Errorf("expected 2 imported events, got %d", n)
	}
	status, lastError := sourceState(t, db, sourceID)
	if status != "ok" || lastError != nil {
		t.Errorf("source state: %s / %v", status, lastError)
	}
}
