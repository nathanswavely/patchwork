package eventsource

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Every ingest path carries the event's own page (docs/adr/079), and every
// one of them checks the scheme first: this value is rendered as an href
// and a feed is somebody else's input.

func TestParseICS_CarriesEventURL(t *testing.T) {
	now := time.Now().UTC()
	body := wrap(
		"BEGIN:VEVENT\nUID:linked@test\nSUMMARY:Linked Show\nDTSTART:" + future(48*time.Hour) +
			"\nURL:https://elcapitan.example/shows/linked\nEND:VEVENT\n" +
			"BEGIN:VEVENT\nUID:hostile@test\nSUMMARY:Hostile Show\nDTSTART:" + future(72*time.Hour) +
			"\nURL:javascript:alert(1)\nEND:VEVENT\n")
	items, err := ParseICS([]byte(body), now, time.UTC)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byUID := map[string]Item{}
	for _, it := range items {
		byUID[it.UID] = it
	}
	if got := byUID["linked@test"].URL; got != "https://elcapitan.example/shows/linked" {
		t.Errorf("ICS URL not carried: %q", got)
	}
	if got := byUID["hostile@test"].URL; got != "" {
		t.Errorf("a javascript: URL survived the parser: %q", got)
	}
}

func TestParseJSONLD_URLAndOfferFallback(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(48 * time.Hour).Format(time.RFC3339)
	// Three shapes: url on the Event, no url but an offer that has one
	// (the ticketing-platform shape), and neither.
	page := fmt.Sprintf(`<html><script type="application/ld+json">
	[
	  {"@type":"MusicEvent","name":"Direct","startDate":%q,
	   "url":"https://venue.example/direct"},
	  {"@type":"MusicEvent","name":"Via Offer","startDate":%q,"@id":"urn:offer-show",
	   "offers":{"@type":"Offer","url":"https://tickets.example/buy/42"}},
	  {"@type":"MusicEvent","name":"Bare","startDate":%q,"@id":"urn:bare-show"}
	]
	</script></html>`, start, start, start)

	items, err := ParseJSONLD([]byte(page), now, time.UTC)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byTitle := map[string]Item{}
	for _, it := range items {
		byTitle[it.Title] = it
	}
	if len(byTitle) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(byTitle), byTitle)
	}
	if got := byTitle["Direct"].URL; got != "https://venue.example/direct" {
		t.Errorf("Event url: %q", got)
	}
	// Ticketing platforms describe the show on the Event and put the buy
	// link on the offer, and the buy link is what the reader wanted.
	if got := byTitle["Via Offer"].URL; got != "https://tickets.example/buy/42" {
		t.Errorf("offers.url fallback: %q", got)
	}
	if got := byTitle["Bare"].URL; got != "" {
		t.Errorf("expected no link, got %q", got)
	}
}

func TestParseSquarespace_ResolvesItemLinks(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(48 * time.Hour).UnixMilli()
	// baseUrl wins over the pasted address: a venue on a custom domain
	// must not get links to its .squarespace.com staging host.
	body := fmt.Sprintf(`{
	  "collection": {"typeName": "events-stacked"},
	  "website": {"baseUrl": "https://elcapitan.example"},
	  "upcoming": [
	    {"id": "aaa", "title": "Relative", "startDate": %d, "fullUrl": "/events/relative"},
	    {"id": "bbb", "title": "Absolute", "startDate": %d, "fullUrl": "https://other.example/x"},
	    {"id": "ccc", "title": "None", "startDate": %d},
	    {"id": "ddd", "title": "Hostile", "startDate": %d, "fullUrl": "javascript:alert(1)"}
	  ]}`, start, start, start, start)

	items, err := ParseSquarespace([]byte(body), now, "https://elcapitan-56tk.squarespace.com/events")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byUID := map[string]Item{}
	for _, it := range items {
		byUID[it.UID] = it
	}
	if got := byUID["aaa"].URL; got != "https://elcapitan.example/events/relative" {
		t.Errorf("relative fullUrl: %q", got)
	}
	if got := byUID["bbb"].URL; got != "https://other.example/x" {
		t.Errorf("absolute fullUrl: %q", got)
	}
	if got := byUID["ccc"].URL; got != "" {
		t.Errorf("expected no link, got %q", got)
	}
	// A ref carrying its own scheme is returned by ResolveReference
	// unchanged, so the scheme is checked after resolution too.
	if got := byUID["ddd"].URL; got != "" {
		t.Errorf("a javascript: fullUrl survived resolution: %q", got)
	}

	// With no declared site, the pasted address's origin is the base.
	noSite := fmt.Sprintf(`{"collection":{"typeName":"events-stacked"},
	  "upcoming":[{"id":"aaa","title":"Relative","startDate":%d,"fullUrl":"/events/relative"}]}`, start)
	items, err = ParseSquarespace([]byte(noSite), now, "https://fallback.example/calendar?format=json")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := items[0].URL; got != "https://fallback.example/events/relative" {
		t.Errorf("fallback base: %q", got)
	}
}

// The link is part of what the source is authoritative about, which is
// also the whole backfill story (docs/adr/079): an event imported before
// this column existed gets its link on the next sync, because the
// reconciler now sees the row as differing from the feed.
func TestSync_EventURLLandsAndBackfills(t *testing.T) {
	db := setupTestDB(t)
	withLink := func(url string) string {
		return wrap("BEGIN:VEVENT\nUID:a@test\nSUMMARY:Show A\nDTSTART:" +
			future(48*time.Hour) + "\nURL:" + url + "\nEND:VEVENT\n")
	}
	feed := newFeedServer(t, wrap(vevent("a@test", "Show A", future(48*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	storedURL := func() string {
		var s string
		if err := db.QueryRow(
			`SELECT COALESCE(event_url,'') FROM events WHERE source_id = ?`, sourceID,
		).Scan(&s); err != nil {
			t.Fatalf("read event_url: %v", err)
		}
		return s
	}
	if storedURL() != "" {
		t.Fatalf("a feed with no URL gave one: %q", storedURL())
	}

	// This is the deploy case: the same events, now read with the column
	// in place. Nothing about the show changed but the link, and that
	// alone has to count as a change.
	feed.set(withLink("https://elcapitan.example/shows/a"), 200)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if storedURL() != "https://elcapitan.example/shows/a" {
		t.Fatalf("link did not backfill: %q", storedURL())
	}
	if n := countEvents(t, db, sourceID); n != 1 {
		t.Fatalf("expected the same event updated in place, got %d rows", n)
	}

	// A venue moving a show to a new ticket link propagates the same way.
	feed.set(withLink("https://elcapitan.example/shows/a-moved"), 200)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if storedURL() != "https://elcapitan.example/shows/a-moved" {
		t.Fatalf("link did not follow the feed: %q", storedURL())
	}

	// And dropping it from the feed drops it here: the source is
	// authoritative in both directions.
	feed.set(wrap(vevent("a@test", "Show A", future(48*time.Hour))), 200)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("fourth sync: %v", err)
	}
	if storedURL() != "" {
		t.Fatalf("link outlived the feed that carried it: %q", storedURL())
	}
}
