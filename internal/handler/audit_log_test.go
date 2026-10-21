package handler_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// seedAuditDay writes one entry at each edge of 2026-10-19 and one either side
// of it. The action names the moment, so a test can say which ones it got.
func seedAuditDay(t *testing.T, db *database.DB) {
	t.Helper()
	for _, row := range []struct{ action, at string }{
		{"day-before", "2026-10-18T23:59:59.999Z"},
		{"first-moment", "2026-10-19T00:00:00.000Z"},
		{"evening", "2026-10-19T21:47:38.770Z"},
		{"last-moment", "2026-10-19T23:59:59.999Z"},
		{"day-after", "2026-10-20T00:00:00.000Z"},
	} {
		if _, err := db.Exec(
			`INSERT INTO audit_log (id, action, entity_type, entity_id, created_at) VALUES (?, ?, 'test', 'x', ?)`,
			auth.NewUUIDv7(), row.action, row.at,
		); err != nil {
			t.Fatalf("seed %s: %v", row.action, err)
		}
	}
}

func auditActions(t *testing.T, db *database.DB, query string) []string {
	t.Helper()
	r := authedRequest("GET", "/api/v1/admin/audit-log?limit=50&"+query, nil, "")
	w := servePublicMux(t, "GET", "/api/v1/admin/audit-log", handler.AuditLog(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var actions []string
	for _, it := range decodeJSON(t, w)["items"].([]interface{}) {
		actions = append(actions, it.(map[string]interface{})["action"].(string))
	}
	return actions
}

func assertActions(t *testing.T, got, want []string, label string) {
	t.Helper()
	have := map[string]bool{}
	for _, a := range got {
		have[a] = true
	}
	if len(got) != len(want) {
		t.Fatalf("%s: expected %v, got %v", label, want, got)
	}
	for _, a := range want {
		if !have[a] {
			t.Fatalf("%s: expected %v, got %v", label, want, got)
		}
	}
}

// created_at is a full timestamp compared as text, and a bare date sorts
// before every timestamp on the day it names. So `to=2026-10-19` returned
// nothing from Oct 19, and a single-day filter returned nothing at all.
func TestAuditLog_DateOnlyBoundsIncludeTheirDay(t *testing.T) {
	db := setupTestDB(t)
	seedAuditDay(t, db)

	got := auditActions(t, db, "from=2026-10-19&to=2026-10-19")
	assertActions(t, got, []string{"first-moment", "evening", "last-moment"}, "single-day filter")

	got = auditActions(t, db, "to=2026-10-19")
	assertActions(t, got, []string{"day-before", "first-moment", "evening", "last-moment"}, "open-ended to")
}

// The admin page sends the instants bounding the reader's local day, and
// those must be honoured as given rather than widened to a UTC day.
func TestAuditLog_InstantBoundsAreHonoured(t *testing.T) {
	db := setupTestDB(t)
	seedAuditDay(t, db)

	// Oct 19 in New York (EDT): 04:00Z to 03:59:59Z the next day.
	query := "from=" + url.QueryEscape("2026-10-19T04:00:00.000Z") +
		"&to=" + url.QueryEscape("2026-10-20T03:59:59Z")
	got := auditActions(t, db, query)
	assertActions(t, got, []string{"evening", "last-moment", "day-after"}, "local-day instants")
}
