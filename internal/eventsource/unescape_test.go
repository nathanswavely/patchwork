package eventsource

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
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

	items, err := ParseSquarespace([]byte(body), testNow, "https://lancworkshop.example/events")
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

// The bug this file's helper was rewritten for: entity encoding applied
// twice. Squarespace served PCA&D's address as "PCA&amp;amp;D", one pass
// left "PCA&amp;D", and the events list rendered that literally — while
// the same document's titles, encoded once, looked fine.
func TestPlainTextDecodesToAFixedPoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"PCA&amp;amp;D", "PCA&D"},
		{"PCA&amp;D", "PCA&D"},
		{"PCA&D", "PCA&D"},
		{"5 & Dime", "5 & Dime"},
		{"Mickey&#39;s Black Box", "Mickey's Black Box"},
		{"Tapes &amp;amp;amp; zines", "Tapes & zines"},
		{"", ""},
		{"no entities here", "no entities here"},
	}
	for _, c := range cases {
		if got := plainText(c.in); got != c.want {
			t.Errorf("plainText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseSquarespaceDecodesDoubleEncodedAddress(t *testing.T) {
	start := testNow.Add(48 * time.Hour).UnixMilli()
	body := fmt.Sprintf(`{
	  "collection": {"typeName": "events-stacked"},
	  "upcoming": [
	  {"id": "bbb", "title": "CCE Pre-College Summer Course Spotlight", "startDate": %d,
	   "excerpt": "<p>Come make art</p>",
	   "location": {"addressTitle": "PCA&amp;amp;D",
	                "addressLine1": "204 N Prince St", "addressLine2": "Lancaster, PA"}}
	]}`, start)

	items, err := ParseSquarespace([]byte(body), testNow, "https://pcad.example/events")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := "PCA&D, 204 N Prince St, Lancaster, PA"; items[0].Location != want {
		t.Errorf("location = %q, want %q", items[0].Location, want)
	}
}

// A Place's name is the field a venue's own schema.org markup almost
// always uses, and it was the one location shape that decoded nothing.
func TestParseJSONLDDecodesPlaceEntities(t *testing.T) {
	it := Item{}
	fillJSONLDLocation(map[string]any{
		"name":    "Lanc Workshop &amp; Tool Library",
		"address": map[string]any{"streetAddress": "433 Ice Avenue"},
	}, &it)
	if want := "Lanc Workshop & Tool Library, 433 Ice Avenue"; it.Location != want {
		t.Errorf("location = %q, want %q", it.Location, want)
	}
}

func TestHealKeyDropsEntityLeftoversOnly(t *testing.T) {
	cases := []struct{ in, want string }{
		// What NameKey produced while the location was encoded, against
		// what it produces once decoded.
		{"pca amp d", "pca d"},
		{"lanc workshop amp tool library", "lanc workshop tool library"},
		{"mickey 39 s black box", "mickey s black box"},
		// Names that merely contain an entity's letters. An entity always
		// has text on both sides of it; a name may not.
		{"amp room", "amp room"},
		{"the amp", "the amp"},
		{"amp", "amp"},
		{"binns park", "binns park"},
	}
	for _, c := range cases {
		if got := healKey(c.in); got != c.want {
			t.Errorf("healKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// The heal has to agree with the reader, or a mapping made before the
	// fix points at a key no listing carries again.
	if healKey(NameKey("PCA&amp;D, 204 N Prince St")) != NameKey("PCA&D, 204 N Prince St") {
		t.Error("healed key does not match the decoded name's key")
	}
}

// A sync only rewrites a listing it still finds. Four of Lancaster
// Patchwork's five encoded rows were past events of a venue that has since
// renamed itself — nothing would ever have touched them again.
func TestHealEncodedEntitiesFixesStoredRows(t *testing.T) {
	db := setupTestDB(t)
	sourceID := seedSource(t, db, "https://example.com/feed.ics")

	var nodeID, userID string
	if err := db.QueryRow(
		`SELECT node_id, added_by FROM event_sources WHERE id = ?`, sourceID,
	).Scan(&nodeID, &userID); err != nil {
		t.Fatalf("read source: %v", err)
	}

	eventID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		eventID, nodeID, userID,
		"Coffee &amp; Cassettes", "Tapes &amp;amp; zines",
		"Lanc Workshop &amp; Tool Library, 433 Ice Avenue", future(48*time.Hour),
	); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE event_sources SET name_key = 'lanc workshop amp tool library' WHERE id = ?`,
		sourceID,
	); err != nil {
		t.Fatalf("seed crosswalk key: %v", err)
	}

	n, err := HealEncodedEntities(db)
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if n != 4 {
		t.Errorf("healed %d fields, want 4", n)
	}

	var title, desc, loc, nameKey string
	if err := db.QueryRow(
		`SELECT title, description, location FROM events WHERE id = ?`, eventID,
	).Scan(&title, &desc, &loc); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if title != "Coffee & Cassettes" {
		t.Errorf("title = %q", title)
	}
	if desc != "Tapes & zines" {
		t.Errorf("description = %q", desc)
	}
	if loc != "Lanc Workshop & Tool Library, 433 Ice Avenue" {
		t.Errorf("location = %q", loc)
	}
	if err := db.QueryRow(
		`SELECT name_key FROM event_sources WHERE id = ?`, sourceID,
	).Scan(&nameKey); err != nil {
		t.Fatalf("read key: %v", err)
	}
	if nameKey != "lanc workshop tool library" {
		t.Errorf("name_key = %q", nameKey)
	}

	// Idempotent: a second start must find nothing left to do.
	if n, err := HealEncodedEntities(db); err != nil || n != 0 {
		t.Errorf("second heal = %d, %v; want 0, nil", n, err)
	}
}
