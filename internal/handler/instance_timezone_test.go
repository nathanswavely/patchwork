package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Moving the quilt's zone is the patch-level change at a wider radius
// (docs/adr/105). It asks the same question, counts the patches as well as
// the events, and — the part that makes it safe to ask at all — touches only
// what actually inherits.

// quiltZone sets the configured default for one test and puts it back
// afterwards. The default is process-global (settings.SetTimezoneDefault is
// called once at startup in a real server), so a test that leaves it set is a
// test that changes what every later test in this package reads.
func quiltZone(t *testing.T, name string) {
	t.Helper()
	settings.SetTimezoneDefault(name)
	t.Cleanup(func() { settings.SetTimezoneDefault("") })
}

func patchQuiltZone(t *testing.T, db *database.DB, token, tz, mode string) (int, map[string]interface{}) {
	t.Helper()
	body := map[string]interface{}{"timezone": tz}
	if mode != "" {
		body["timezone_events"] = mode
	}
	r := authedRequest("PATCH", "/api/v1/admin/settings", body, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, testConfig()), r)
	var out map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// The refusal, and what it counts.
func TestQuiltZone_RefusesUntilSomebodySaysWhichTheyMeant(t *testing.T) {
	db := setupTestDB(t)
	quiltZone(t, "UTC")
	owner, token := createTestUser(t, db, "zoneboss", "admin")

	// Two patches that inherit, and one that keeps its own time.
	inheritA := createTestNode(t, db, owner.ID, "Inherit A", "inherit-a", "open")
	inheritB := createTestNode(t, db, owner.ID, "Inherit B", "inherit-b", "open")
	itsOwn := createTestNode(t, db, owner.ID, "Its Own", "its-own", "open")
	db.Exec(`UPDATE nodes SET timezone = 'Europe/Berlin' WHERE id = ?`, itsOwn)

	// 7pm UTC, which is a different wall clock in New York.
	const at = "2027-03-05T19:00:00.000Z"
	a1 := insertZonedEvent(t, db, inheritA, owner.ID, "Rehearsal", at, "", "")
	a2 := insertZonedEvent(t, db, inheritA, owner.ID, "Meeting", at, "", "")
	b1 := insertZonedEvent(t, db, inheritB, owner.ID, "Opening", at, "", "")
	// Neither of these inherits the quilt: one names its own zone, and the
	// other belongs to a patch that does.
	pinned := insertZonedEvent(t, db, inheritA, owner.ID, "Pinned", at, "", "Asia/Tokyo")
	elsewhere := insertZonedEvent(t, db, itsOwn, owner.ID, "Berlin gig", at, "", "")

	code, body := patchQuiltZone(t, db, token, "America/New_York", "")
	if code != http.StatusConflict {
		t.Fatalf("expected a 409 asking which was meant, got %d: %v", code, body)
	}
	if body["code"] != "timezone_events_undecided" {
		t.Errorf("expected the undecided code, got %v", body["code"])
	}
	if got := body["events_affected"].(float64); got != 3 {
		t.Errorf("expected the three inheriting events, got %v", got)
	}
	if got := body["patches_affected"].(float64); got != 2 {
		t.Errorf("expected two patches counted, got %v", got)
	}

	// And nothing was written: not the setting, not an instant.
	if got := settings.EffectiveTimezone(db); got != "UTC" {
		t.Errorf("a refused change must not save the zone, got %q", got)
	}
	for _, id := range []string{a1, a2, b1, pinned, elsewhere} {
		if got := startsAtOf(t, db, id); got != at {
			t.Errorf("a refused change must move nothing, %s is %q", id, got)
		}
	}
}

// keep_clock re-anchors what inherits, and only what inherits.
func TestQuiltZone_KeepClockMovesOnlyWhatInherits(t *testing.T) {
	db := setupTestDB(t)
	quiltZone(t, "UTC")
	owner, token := createTestUser(t, db, "zoneboss2", "admin")

	inherits := createTestNode(t, db, owner.ID, "Inherits", "inherits", "open")
	itsOwn := createTestNode(t, db, owner.ID, "Its Own Two", "its-own-two", "open")
	db.Exec(`UPDATE nodes SET timezone = 'Europe/Berlin' WHERE id = ?`, itsOwn)

	const at = "2027-03-05T19:00:00.000Z"
	moving := insertZonedEvent(t, db, inherits, owner.ID, "Rehearsal", at, "", "")
	pinned := insertZonedEvent(t, db, inherits, owner.ID, "Pinned", at, "", "Asia/Tokyo")
	elsewhere := insertZonedEvent(t, db, itsOwn, owner.ID, "Berlin gig", at, "", "")

	code, body := patchQuiltZone(t, db, token, "America/New_York", "keep_clock")
	if code != http.StatusOK {
		t.Fatalf("expected the answered change to go through, got %d: %v", code, body)
	}
	if got := settings.EffectiveTimezone(db); got != "America/New_York" {
		t.Fatalf("expected the zone saved, got %q", got)
	}
	change, _ := body["timezone_change"].(map[string]interface{})
	if change == nil {
		t.Fatal("expected the response to say what was done")
	}
	if got := change["events_moved"].(float64); got != 1 {
		t.Errorf("expected one instant rewritten, got %v", got)
	}

	// 7pm UTC read as 7pm; in New York that is midnight UTC.
	if got := startsAtOf(t, db, moving); got != "2027-03-06T00:00:00.000Z" {
		t.Errorf("keep_clock: 7pm should still read 7pm, got %q", got)
	}
	for _, id := range []string{pinned, elsewhere} {
		if got := startsAtOf(t, db, id); got != at {
			t.Errorf("an event that does not inherit the quilt must not move, got %q", got)
		}
	}
}

// keep_instant saves the zone and rewrites nothing.
func TestQuiltZone_KeepInstantLeavesEveryRowAlone(t *testing.T) {
	db := setupTestDB(t)
	quiltZone(t, "UTC")
	owner, token := createTestUser(t, db, "zoneboss3", "admin")
	nodeID := createTestNode(t, db, owner.ID, "Inherits Three", "inherits-three", "open")

	const at = "2027-03-05T19:00:00.000Z"
	id := insertZonedEvent(t, db, nodeID, owner.ID, "Rehearsal", at, "", "")

	code, body := patchQuiltZone(t, db, token, "America/New_York", "keep_instant")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", code, body)
	}
	if got := startsAtOf(t, db, id); got != at {
		t.Errorf("keep_instant must leave the row alone, got %q", got)
	}
	if got := settings.EffectiveTimezone(db); got != "America/New_York" {
		t.Errorf("expected the zone saved, got %q", got)
	}
	var audited int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'admin.instance_timezone_changed'`).Scan(&audited)
	if audited != 1 {
		t.Errorf("expected the change on the record, got %d entries", audited)
	}
}

// Nothing to decide, nothing asked. A question with no consequences is how
// people learn to click through the ones that have them (docs/adr/101).
func TestQuiltZone_NoQuestionWhereNothingReadsDifferently(t *testing.T) {
	db := setupTestDB(t)
	quiltZone(t, "America/New_York")
	owner, token := createTestUser(t, db, "zoneboss4", "admin")
	nodeID := createTestNode(t, db, owner.ID, "Detroit", "detroit", "open")
	insertZonedEvent(t, db, nodeID, owner.ID, "Rehearsal", "2027-03-05T19:00:00.000Z", "", "")

	// Two names for one clock (docs/adr/067 decision 4).
	if code, body := patchQuiltZone(t, db, token, "America/Detroit", ""); code != http.StatusOK {
		t.Fatalf("expected no question between agreeing zones, got %d: %v", code, body)
	}

	// And a quilt with nothing inheriting is not asked either.
	db.Exec(`UPDATE nodes SET timezone = 'Europe/Berlin' WHERE id = ?`, nodeID)
	if code, body := patchQuiltZone(t, db, token, "Asia/Tokyo", ""); code != http.StatusOK {
		t.Fatalf("expected no question with nothing inheriting, got %d: %v", code, body)
	}
}

// A fixed offset is not a place, and the admin box refuses it like every
// other write path (docs/adr/101).
func TestQuiltZone_RefusesAFixedOffset(t *testing.T) {
	db := setupTestDB(t)
	quiltZone(t, "UTC")
	_, token := createTestUser(t, db, "zoneboss5", "admin")
	if code, _ := patchQuiltZone(t, db, token, "EST", ""); code != http.StatusBadRequest {
		t.Errorf("expected EST refused, got %d", code)
	}
}
