package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-074: setting a patch's timezone silently moved every event it had.
//
// It did not move them, which is the whole trouble: the stored instants
// stayed exactly where they were and every *reading* changed, because an
// event inherits its patch's zone (docs/adr/045) and the reading is what
// a person sees. Four rehearsals typed as 7pm began saying 2pm, with no
// warning and no way back. Both honest answers are here, neither is a
// default, and each says what it did and to how many.

func patchNodeZone(t *testing.T, db *database.DB, slug, token string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/nodes/"+slug, body, token)
	return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
}

// insertZonedEvent puts an event on the calendar at an exact instant, with
// an optional zone of its own.
func insertZonedEvent(t *testing.T, db *database.DB, nodeID, ownerID, title, startsAt, endsAt, tz string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	var zone any
	if tz != "" {
		zone = tz
	}
	var ends any
	if endsAt != "" {
		ends = endsAt
	}
	_, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location,
		 starts_at, ends_at, timezone, recurrence, visibility, status)
		 VALUES (?, ?, ?, ?, '', '', ?, ?, ?, '', 'public', 'active')`,
		id, nodeID, ownerID, title, startsAt, ends, zone,
	)
	if err != nil {
		t.Fatalf("insert event %s: %v", title, err)
	}
	return id
}

func startsAtOf(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT starts_at FROM events WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatalf("read starts_at: %v", err)
	}
	return s
}

// choirWithFourRehearsals is June's patch: four 7pm rehearsals typed while
// the patch had no zone of its own, so they were written as 7pm UTC.
func choirWithFourRehearsals(t *testing.T, db *database.DB) (nodeID, ownerID, token string, ids []string) {
	t.Helper()
	owner, tok := createTestUser(t, db, "june", "member")
	nodeID = createTestNode(t, db, owner.ID, "Choir", "choir", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	for _, d := range []string{"2027-01-05", "2027-01-12", "2027-01-19", "2027-01-26"} {
		ids = append(ids, insertZonedEvent(t, db, nodeID, owner.ID, "Rehearsal",
			d+"T19:00:00.000Z", d+"T21:00:00.000Z", ""))
	}
	return nodeID, owner.ID, tok, ids
}

func TestSettingAPatchTimezoneRefusesToGuessAboutExistingEvents(t *testing.T) {
	db := setupTestDB(t)
	_, _, token, ids := choirWithFourRehearsals(t, db)

	w := patchNodeZone(t, db, "choir", token, map[string]interface{}{"timezone": "America/New_York"})
	if w.Code != http.StatusConflict {
		t.Fatalf("set timezone with no answer: got %d, want 409 — %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error          string   `json:"error"`
		Code           string   `json:"code"`
		From           string   `json:"from"`
		To             string   `json:"to"`
		EventsAffected int      `json:"events_affected"`
		Choices        []string `json:"choices"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode 409: %v", err)
	}
	if resp.Code != "timezone_events_undecided" {
		t.Errorf("code = %q", resp.Code)
	}
	if resp.EventsAffected != 4 {
		t.Errorf("events_affected = %d, want 4", resp.EventsAffected)
	}
	if resp.From != "UTC" || resp.To != "America/New_York" {
		t.Errorf("from/to = %q/%q", resp.From, resp.To)
	}
	if len(resp.Choices) != 2 {
		t.Errorf("choices = %v", resp.Choices)
	}
	if !strings.Contains(resp.Error, "4 events") {
		t.Errorf("the message must name the count, got %q", resp.Error)
	}

	// Nothing happened: not the zone, and not one instant. A refusal that
	// half-applied would be worse than the silence it replaces.
	var stored string
	db.QueryRow(`SELECT COALESCE(timezone,'') FROM nodes WHERE slug = 'choir'`).Scan(&stored)
	if stored != "" {
		t.Errorf("timezone stored despite the refusal: %q", stored)
	}
	for _, id := range ids {
		if got := startsAtOf(t, db, id); !strings.HasSuffix(got, "T19:00:00.000Z") {
			t.Errorf("event moved during a refusal: %s", got)
		}
	}
}

func TestKeepInstantLeavesEveryStoredInstantAlone(t *testing.T) {
	db := setupTestDB(t)
	_, _, token, ids := choirWithFourRehearsals(t, db)
	before := make([]string, len(ids))
	for i, id := range ids {
		before[i] = startsAtOf(t, db, id)
	}

	w := patchNodeZone(t, db, "choir", token, map[string]interface{}{
		"timezone": "America/New_York", "timezone_events": "keep_instant",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("keep_instant: got %d — %s", w.Code, w.Body.String())
	}
	var resp struct {
		Timezone       string `json:"timezone"`
		TimezoneChange struct {
			Mode           string `json:"mode"`
			EventsAffected int    `json:"events_affected"`
			EventsMoved    int    `json:"events_moved"`
		} `json:"timezone_change"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Timezone != "America/New_York" {
		t.Errorf("node timezone = %q", resp.Timezone)
	}
	if resp.TimezoneChange.Mode != "keep_instant" {
		t.Errorf("mode = %q", resp.TimezoneChange.Mode)
	}
	if resp.TimezoneChange.EventsAffected != 4 {
		t.Errorf("events_affected = %d, want 4", resp.TimezoneChange.EventsAffected)
	}
	if resp.TimezoneChange.EventsMoved != 0 {
		t.Errorf("events_moved = %d, want 0 — keep_instant rewrites nothing", resp.TimezoneChange.EventsMoved)
	}
	for i, id := range ids {
		if got := startsAtOf(t, db, id); got != before[i] {
			t.Errorf("event %d: starts_at %s → %s", i, before[i], got)
		}
	}
}

func TestKeepClockMovesTheInstantsSoSevenPMStaysSevenPM(t *testing.T) {
	db := setupTestDB(t)
	_, _, token, ids := choirWithFourRehearsals(t, db)

	w := patchNodeZone(t, db, "choir", token, map[string]interface{}{
		"timezone": "America/New_York", "timezone_events": "keep_clock",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("keep_clock: got %d — %s", w.Code, w.Body.String())
	}
	var resp struct {
		TimezoneChange struct {
			Mode        string `json:"mode"`
			EventsMoved int    `json:"events_moved"`
		} `json:"timezone_change"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TimezoneChange.Mode != "keep_clock" || resp.TimezoneChange.EventsMoved != 4 {
		t.Fatalf("report = %+v, want keep_clock/4", resp.TimezoneChange)
	}

	// January in Lancaster is UTC−5, so 7pm Eastern is midnight UTC on the
	// following day. The assertion that matters is the one the bandleader
	// made: the page still says 7:00 PM.
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	for _, id := range ids {
		got := startsAtOf(t, db, id)
		ts, err := time.Parse(time.RFC3339, got)
		if err != nil {
			t.Fatalf("parse %q: %v", got, err)
		}
		if h, m := ts.In(ny).Hour(), ts.In(ny).Minute(); h != 19 || m != 0 {
			t.Errorf("reads as %02d:%02d in America/New_York, want 19:00 (%s)", h, m, got)
		}
		// Written-by-the-browser precision is preserved: starts_at holds
		// two of them and every range comparison is lexicographic.
		if !strings.Contains(got, ".000Z") {
			t.Errorf("precision changed on rewrite: %s", got)
		}
	}

	// ends_at travels with it, or a two-hour rehearsal becomes a
	// seven-hour one.
	var ends string
	db.QueryRow(`SELECT ends_at FROM events WHERE id = ?`, ids[0]).Scan(&ends)
	et, err := time.Parse(time.RFC3339, ends)
	if err != nil {
		t.Fatalf("parse ends_at %q: %v", ends, err)
	}
	if h := et.In(ny).Hour(); h != 21 {
		t.Errorf("ends_at reads as %02d:00, want 21:00", h)
	}
}

func TestZoneChangeNeverTouchesPinnedOrImportedEvents(t *testing.T) {
	db := setupTestDB(t)
	nodeID, ownerID, token, _ := choirWithFourRehearsals(t, db)

	// An event pinning its own zone is not inheriting, so the patch's zone
	// is not its zone and nothing about it changes.
	pinned := insertZonedEvent(t, db, nodeID, ownerID, "Out-of-town date",
		"2027-02-02T19:00:00.000Z", "", "America/Los_Angeles")

	// An imported event's instant is the feed's fact, arrived at through
	// the ingest chain docs/adr/067 built. Re-anchoring it would undo that
	// reading with a guess — and the next sync would overwrite the guess.
	srcID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, url, type, added_by) VALUES (?, ?, 'https://example.test/cal.ics', 'ics', ?)`,
		srcID, nodeID, ownerID,
	); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	imported := insertZonedEvent(t, db, nodeID, ownerID, "From the feed", "2027-02-09T19:00:00Z", "", "")
	if _, err := db.Exec(`UPDATE events SET source_id = ? WHERE id = ?`, srcID, imported); err != nil {
		t.Fatalf("attach source: %v", err)
	}

	w := patchNodeZone(t, db, "choir", token, map[string]interface{}{
		"timezone": "America/New_York", "timezone_events": "keep_clock",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("keep_clock: got %d — %s", w.Code, w.Body.String())
	}
	var resp struct {
		TimezoneChange struct {
			EventsAffected int `json:"events_affected"`
			EventsMoved    int `json:"events_moved"`
		} `json:"timezone_change"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TimezoneChange.EventsAffected != 4 || resp.TimezoneChange.EventsMoved != 4 {
		t.Errorf("report = %+v, want 4 affected and 4 moved — the pinned and imported events are neither",
			resp.TimezoneChange)
	}
	if got := startsAtOf(t, db, pinned); got != "2027-02-02T19:00:00.000Z" {
		t.Errorf("pinned event moved: %s", got)
	}
	if got := startsAtOf(t, db, imported); got != "2027-02-09T19:00:00Z" {
		t.Errorf("imported event moved: %s", got)
	}
}

func TestZonesThatAgreeOnTheClockAskNoQuestion(t *testing.T) {
	db := setupTestDB(t)
	_, _, token, ids := choirWithFourRehearsals(t, db)

	// New York first, deliberately, so the second change is between two
	// names for one clock.
	if w := patchNodeZone(t, db, "choir", token, map[string]interface{}{
		"timezone": "America/New_York", "timezone_events": "keep_instant",
	}); w.Code != http.StatusOK {
		t.Fatalf("first change: got %d — %s", w.Code, w.Body.String())
	}
	before := startsAtOf(t, db, ids[0])

	// docs/adr/067 decision 4: America/New_York and America/Detroit never
	// disagree, so asking a question with no consequences would be noise
	// exactly where a question claims to be care.
	w := patchNodeZone(t, db, "choir", token, map[string]interface{}{"timezone": "America/Detroit"})
	if w.Code != http.StatusOK {
		t.Fatalf("same-clock change: got %d, want 200 — %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "timezone_change") {
		t.Errorf("reported a change that changes nothing: %s", w.Body.String())
	}
	if got := startsAtOf(t, db, ids[0]); got != before {
		t.Errorf("same-clock change moved an event: %s → %s", before, got)
	}
}

func TestZoneChangeOnAPatchWithNoEventsJustSaves(t *testing.T) {
	db := setupTestDB(t)
	owner, token := createTestUser(t, db, "founder", "member")
	nodeID := createTestNode(t, db, owner.ID, "New Patch", "new-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	w := patchNodeZone(t, db, "new-patch", token, map[string]interface{}{"timezone": "Europe/Berlin"})
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 — %s", w.Code, w.Body.String())
	}
	var stored string
	db.QueryRow(`SELECT timezone FROM nodes WHERE slug = 'new-patch'`).Scan(&stored)
	if stored != "Europe/Berlin" {
		t.Errorf("timezone = %q", stored)
	}
}

func TestClearingAPatchTimezoneAlsoAsks(t *testing.T) {
	db := setupTestDB(t)
	_, _, token, _ := choirWithFourRehearsals(t, db)
	if w := patchNodeZone(t, db, "choir", token, map[string]interface{}{
		"timezone": "America/New_York", "timezone_events": "keep_clock",
	}); w.Code != http.StatusOK {
		t.Fatalf("set: got %d — %s", w.Code, w.Body.String())
	}

	// Going back to inheriting the quilt's zone is the same move in the
	// other direction and gets the same question.
	w := patchNodeZone(t, db, "choir", token, map[string]interface{}{"timezone": ""})
	if w.Code != http.StatusConflict {
		t.Fatalf("clear: got %d, want 409 — %s", w.Code, w.Body.String())
	}
}

// Every write path that takes a zone refuses a name that is not a place.
// docs/adr/045 and docs/adr/067 say an event's time belongs to a place;
// "EST" is a fixed −05:00 that never observes daylight saving, so a patch
// wearing it is exactly right until the second Sunday of March and an hour
// early after it, with nothing anywhere complaining.
func TestNonIANAZonesAreRefusedAtEveryWritePath(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	owner, token := createTestUser(t, db, "zoner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Venue", "venue", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	admin, adminToken := createTestUser(t, db, "quiltboss", "admin")
	_ = admin

	eventID := insertZonedEvent(t, db, nodeID, owner.ID, "Show", "2027-03-20T19:00:00Z", "", "")

	// "EST" is the one the audit named; the rest are the family it belongs
	// to, all of which time.LoadLocation resolves without complaint.
	bad := []string{"EST", "MST", "HST", "EST5EDT", "CET", "GMT", "Local", "Etc/GMT+5", "Lancaster/PA"}

	for _, tz := range bad {
		t.Run("node/"+tz, func(t *testing.T) {
			w := patchNodeZone(t, db, "venue", token, map[string]interface{}{"timezone": tz})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("PATCH /nodes/{slug} accepted %q: %d", tz, w.Code)
			}
			assertZoneMessage(t, w.Body.String(), tz)
		})

		t.Run("event-create/"+tz, func(t *testing.T) {
			body := map[string]interface{}{
				"node_id": nodeID, "title": "Show", "starts_at": "2027-03-20T19:00:00Z", "timezone": tz,
			}
			r := authedRequest("POST", "/api/v1/events", body, token)
			w := serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, cfg), r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("POST /events accepted %q: %d", tz, w.Code)
			}
			assertZoneMessage(t, w.Body.String(), tz)
		})

		t.Run("event-update/"+tz, func(t *testing.T) {
			r := authedRequest("PATCH", "/api/v1/events/"+eventID, map[string]interface{}{"timezone": tz}, token)
			w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("PATCH /events/{id} accepted %q: %d", tz, w.Code)
			}
			assertZoneMessage(t, w.Body.String(), tz)
		})

		t.Run("instance/"+tz, func(t *testing.T) {
			r := authedRequest("PATCH", "/api/v1/admin/settings", map[string]interface{}{"timezone": tz}, adminToken)
			w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("PATCH /admin/settings accepted %q: %d", tz, w.Code)
			}
			assertZoneMessage(t, w.Body.String(), tz)
		})
	}

	// And the names that are places still go through, along with UTC, the
	// terminating rung of the resolution chain.
	for _, tz := range []string{"America/New_York", "Europe/Berlin", "Pacific/Auckland", "UTC"} {
		w := patchNodeZone(t, db, "venue", token, map[string]interface{}{
			"timezone": tz, "timezone_events": "keep_instant",
		})
		if w.Code != http.StatusOK {
			t.Errorf("PATCH /nodes/{slug} refused %q: %d — %s", tz, w.Code, w.Body.String())
		}
	}
}

// The refusal has to say what to type instead, or it is a dead end for the
// person who typed the only zone abbreviation they know.
func assertZoneMessage(t *testing.T, body, tz string) {
	t.Helper()
	if !strings.Contains(body, "America/New_York") {
		t.Errorf("refusing %q said nothing about what to type instead: %s", tz, body)
	}
}
