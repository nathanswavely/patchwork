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

func ssFixture(now time.Time) string {
	upcoming := now.Add(48 * time.Hour).UnixMilli()
	upcomingEnd := now.Add(51 * time.Hour).UnixMilli()
	beyond := now.Add(120 * 24 * time.Hour).UnixMilli()
	recent := now.Add(-12 * time.Hour).UnixMilli()
	longPast := now.Add(-30 * 24 * time.Hour).UnixMilli()
	return fmt.Sprintf(`{
	  "collection": {"typeName": "events-stacked"},
	  "upcoming": [
	    {"id": "aaa", "title": " Big Show ", "startDate": %d, "endDate": %d,
	     "excerpt": "<p>Doors at <b>7</b> &amp; music at 8</p>",
	     "location": {"addressTitle": "El Capitan", "addressLine1": "123 Main St",
	                  "markerLat": 40.05, "markerLng": -76.3}},
	    {"id": "bbb", "title": "Default Location Show", "startDate": %d,
	     "location": {"addressTitle": "", "addressLine1": "",
	                  "markerLat": 40.7207559, "markerLng": -74.0007613}},
	    {"id": "far", "title": "Beyond Horizon", "startDate": %d}
	  ],
	  "past": [
	    {"id": "recent", "title": "Last Night", "startDate": %d},
	    {"id": "old", "title": "Long Ago", "startDate": %d}
	  ]
	}`, upcoming, upcomingEnd, upcoming, beyond, recent, longPast)
}

func TestParseSquarespace(t *testing.T) {
	now := time.Now().UTC()
	items, err := ParseSquarespace([]byte(ssFixture(now)), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byUID := map[string]Item{}
	for _, it := range items {
		byUID[it.UID] = it
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (window drops far+old), got %d: %v", len(items), byUID)
	}

	show := byUID["aaa"]
	if show.Title != "Big Show" {
		t.Errorf("title: %q", show.Title)
	}
	if show.Description != "Doors at 7 & music at 8" {
		t.Errorf("excerpt strip: %q", show.Description)
	}
	if show.Location != "El Capitan, 123 Main St" {
		t.Errorf("location: %q", show.Location)
	}
	if show.Latitude == nil || *show.Latitude != 40.05 {
		t.Errorf("marker lat: %v", show.Latitude)
	}
	if show.EndsAt == nil {
		t.Error("end date lost")
	}

	// Squarespace's default map marker without a typed address is noise,
	// not a location.
	noLoc := byUID["bbb"]
	if noLoc.Location != "" || noLoc.Latitude != nil {
		t.Errorf("default marker imported as a location: %q %v", noLoc.Location, noLoc.Latitude)
	}

	if _, ok := byUID["recent"]; !ok {
		t.Error("recent past event inside the grace window missing")
	}
}

func TestParseSquarespace_RejectsNonEventsCollections(t *testing.T) {
	_, err := ParseSquarespace([]byte(`{"collection":{"typeName":"page"},"items":[]}`), time.Now().UTC())
	if err == nil {
		t.Fatal("a plain page must not parse as an events collection")
	}
}

// Pasting a Squarespace events page as an 'ics' source auto-detects the
// JSON view, persists the type, and imports — one URL box for admins.
func TestSync_AutoDetectsSquarespace(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(ssFixture(now)))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><html><body>a calendar page</body></html>"))
	}))
	t.Cleanup(srv.Close)
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })

	sourceID := seedSource(t, db, srv.URL+"/events")

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var typ string
	db.QueryRow(`SELECT type FROM event_sources WHERE id = ?`, sourceID).Scan(&typ)
	if typ != "squarespace" {
		t.Errorf("detected type not persisted: %q", typ)
	}
	if n := countEvents(t, db, sourceID); n != 3 {
		t.Errorf("expected 3 imported events, got %d", n)
	}
	status, lastError := sourceState(t, db, sourceID)
	if status != "ok" || lastError != nil {
		t.Errorf("source state: %s / %v", status, lastError)
	}

	// Second sync goes straight to the JSON view and stays healthy.
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if n := countEvents(t, db, sourceID); n != 3 {
		t.Errorf("second sync changed the set: %d", n)
	}
}

// A URL that is neither ICS nor Squarespace reports the ICS error — the
// admin pasted a calendar address, so that's the failure that matters.
func TestSync_NonCalendarReportsICSError(t *testing.T) {
	db := setupTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><html><body>just a homepage</body></html>"))
	}))
	t.Cleanup(srv.Close)
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })

	sourceID := seedSource(t, db, srv.URL)
	if err := Sync(context.Background(), db, nil, sourceID); err == nil {
		t.Fatal("expected error for a non-calendar page")
	}
	status, lastError := sourceState(t, db, sourceID)
	if status != "error" || lastError == nil {
		t.Fatalf("source state: %s / %v", status, lastError)
	}
	var typ string
	db.QueryRow(`SELECT type FROM event_sources WHERE id = ?`, sourceID).Scan(&typ)
	if typ != "ics" {
		t.Errorf("type must not flip on failed detection: %q", typ)
	}
}
