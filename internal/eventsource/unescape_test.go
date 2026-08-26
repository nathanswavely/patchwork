package eventsource

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Feeds generated out of a CMS carry HTML entities through into fields that
// are not HTML. Only the JSON-LD reader unescaped them, so an ICS or
// Squarespace calendar published "Lanc Workshop &amp; Tool Library" and the
// UI rendered it literally.
//
// Location is where it showed worst: it is name-first (docs/adr/046), so the
// entity lands in the half that survives truncation on a narrow row while
// the ellipsis eats the harmless postal tail.

func TestParseICSUnescapesEntities(t *testing.T) {
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:entities@test
SUMMARY:Coffee &amp; Cassettes
DESCRIPTION:Tapes\, records &amp; zines
LOCATION:Lanc Workshop &amp; Tool Library\, 433 Ice Avenue
DTSTART:20260722T190000Z
END:VEVENT
`), testNow, testZone())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Title != "Coffee & Cassettes" {
		t.Errorf("title = %q, want the ampersand unescaped", it.Title)
	}
	if it.Description != "Tapes, records & zines" {
		t.Errorf("description = %q, want the ampersand unescaped", it.Description)
	}
	if it.Location != "Lanc Workshop & Tool Library, 433 Ice Avenue" {
		t.Errorf("location = %q, want the ampersand unescaped", it.Location)
	}
}

func TestParseICSLeavesOrdinaryTextAlone(t *testing.T) {
	// Unescaping must not become a licence to mangle. An ampersand that was
	// never an entity stays exactly one ampersand.
	items, err := ParseICS(ics(`BEGIN:VEVENT
UID:plain@test
SUMMARY:Coffee & Cassettes
LOCATION:5 & Dime
DTSTART:20260722T190000Z
END:VEVENT
`), testNow, testZone())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if items[0].Title != "Coffee & Cassettes" || items[0].Location != "5 & Dime" {
		t.Errorf("plain text changed: %q / %q", items[0].Title, items[0].Location)
	}
}

func TestParseSquarespaceUnescapesTitleAndLocation(t *testing.T) {
	// The excerpt already went through stripHTML, which unescaped; the title
	// and the joined address did not. A title kept its &amp; while its own
	// description lost one.
	start := testNow.Add(48 * time.Hour).UnixMilli()
	body := fmt.Sprintf(`{
	  "collection": {"typeName": "events-stacked"},
	  "upcoming": [
	  {"id": "aaa", "title": "Coffee &amp; Cassettes", "startDate": %d,
	   "excerpt": "<p>Tapes &amp; zines</p>",
	   "location": {"addressTitle": "Lanc Workshop &amp; Tool Library",
	                "addressLine1": "433 Ice Avenue", "addressLine2": ""}}
	]}`, start)

	items, err := ParseSquarespace([]byte(body), testNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Title != "Coffee & Cassettes" {
		t.Errorf("title = %q, want the ampersand unescaped", it.Title)
	}
	if it.Location != "Lanc Workshop & Tool Library, 433 Ice Avenue" {
		t.Errorf("location = %q, want the ampersand unescaped", it.Location)
	}
	if !strings.Contains(it.Description, "&") || strings.Contains(it.Description, "&amp;") {
		t.Errorf("description = %q, want the ampersand unescaped", it.Description)
	}
}
