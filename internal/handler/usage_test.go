package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// The Usage tab and its switch
// (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).

func getUsage(t *testing.T, db *database.DB, token, query string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/admin/usage"+query, nil, token)
	w := serveAdmin(db, "GET", "/api/v1/admin/usage", handler.AdminUsage(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET usage: got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return resp
}

func TestUsageIsOffUntilAnAdminTurnsItOn(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "boss", "admin")

	if getUsage(t, db, token, "")["enabled"] != false {
		t.Fatal("a fresh instance counts nothing")
	}

	r := authedRequest("PATCH", "/api/v1/admin/settings", map[string]interface{}{"usage_stats": true}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("turn on: got %d: %s", w.Code, w.Body.String())
	}
	if !settings.UsageStatsEnabled(db) || getUsage(t, db, token, "")["enabled"] != true {
		t.Fatal("the switch should be on")
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'admin.usage_stats_set' AND metadata = '{"enabled":true}'`).Scan(&n)
	if n != 1 {
		t.Errorf("turning counting on is audited on its own line, got %d rows", n)
	}
}

func TestUsageReportsDailyTotalsAndNeverSumsVisitors(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")
	if err := settings.Set(db, settings.KeyUsageStats, "true"); err != nil {
		t.Fatal(err)
	}

	counter := middleware.NewUsageCounter(db, func() bool { return true })
	counter.Record("203.0.113.7", "Mozilla/5.0", "/")
	counter.Record("203.0.113.7", "Mozilla/5.0", "/patches/{slug}")
	counter.Record("203.0.113.9", "Mozilla/5.0", "/")
	counter.Flush()

	// A second flush with nothing new writes nothing more.
	counter.Flush()

	// Yesterday, written directly: two visitors, so a naive sum would be 4.
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	db.Exec(`INSERT INTO usage_days (day, path, views) VALUES (?, '/', 5)`, yesterday)
	db.Exec(`INSERT INTO usage_visitors (day, visitors) VALUES (?, 2)`, yesterday)

	resp := getUsage(t, db, token, "?days=7")
	if resp["days"] != float64(7) {
		t.Errorf("days = %v", resp["days"])
	}
	if resp["views"] != float64(8) {
		t.Errorf("views over the window = %v, want 8", resp["views"])
	}
	if resp["peak_visitors"] != float64(2) {
		t.Errorf("peak visitors = %v, want 2 (never a sum across days)", resp["peak_visitors"])
	}
	series := resp["series"].([]interface{})
	if len(series) != 7 {
		t.Fatalf("a 7-day window has 7 days, got %d", len(series))
	}
	today := series[6].(map[string]interface{})
	if today["views"] != float64(3) || today["visitors"] != float64(2) {
		t.Errorf("today = %v", today)
	}
	paths := resp["paths"].([]interface{})
	top := paths[0].(map[string]interface{})
	if top["path"] != "/" || top["views"] != float64(7) {
		t.Errorf("top path = %v", top)
	}

	// An unknown window clamps to the default rather than erroring.
	if getUsage(t, db, token, "?days=12")["days"] != float64(30) {
		t.Error("an unlisted window should clamp to 30")
	}
}

func TestUsageActivityComesFromTheRecordsTheQuiltKeepsAnyway(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")
	before := getUsage(t, db, token, "?days=7")["activity"].(map[string]interface{})["accounts"].(float64)
	createTestUser(t, db, "newcomer", "member")

	resp := getUsage(t, db, token, "?days=7")
	activity := resp["activity"].(map[string]interface{})
	if activity["accounts"] != before+1 {
		t.Errorf("accounts created = %v, want %v", activity["accounts"], before+1)
	}
	if resp["enabled"] != false {
		t.Error("activity should be answered with counting off")
	}
}

func TestClearUsageDeletesEveryRowAndIsAudited(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")
	db.Exec(`INSERT INTO usage_days (day, path, views) VALUES ('2026-09-01', '/', 5)`)
	db.Exec(`INSERT INTO usage_visitors (day, visitors) VALUES ('2026-09-01', 2)`)

	counter := middleware.NewUsageCounter(db, func() bool { return true })
	counter.Record("203.0.113.7", "Mozilla/5.0", "/")

	r := authedRequest("DELETE", "/api/v1/admin/usage", nil, token)
	w := serveAdmin(db, "DELETE", "/api/v1/admin/usage", handler.AdminClearUsage(db, counter), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("clear: got %d: %s", w.Code, w.Body.String())
	}

	// What the counter held in memory must not come back on the next flush.
	counter.Flush()
	var days, visitors, audits int
	db.QueryRow(`SELECT COUNT(*) FROM usage_days`).Scan(&days)
	db.QueryRow(`SELECT COUNT(*) FROM usage_visitors`).Scan(&visitors)
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'admin.usage_cleared'`).Scan(&audits)
	if days != 0 || visitors != 0 {
		t.Errorf("after clear: %d day rows, %d visitor rows", days, visitors)
	}
	if audits != 1 {
		t.Errorf("clear should be audited once, got %d", audits)
	}
}
