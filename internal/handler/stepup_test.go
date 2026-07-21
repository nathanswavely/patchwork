package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// serveSudoAdmin mounts a handler behind the real gate chain: admin, then
// step-up (docs/adr/017).
func serveSudoAdmin(db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AdminRequired(db, middleware.SudoRequired(db, h)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func bodyCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return body["code"]
}

// Instance wipe erases every patch, person, and governance record, and takes
// the audit log with it. An admin cookie alone must not be enough.
func TestWipeRejectsSessionWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "wipe-admin", "admin")

	name := settings.EffectiveName(db, cfg)
	r := authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": name}, token)
	w := serveSudoAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("wipe without step-up returned %d, want 403: %s", w.Code, w.Body.String())
	}

	// The instance must still be there. A gate that returns 403 after doing
	// the work is not a gate.
	var nodes int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&nodes)
	if nodes == 0 {
		t.Fatal("wipe ran despite being rejected")
	}
}

// Export carries every member's email address off the instance (docs/adr/012).
func TestExportRejectsSessionWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "export-admin", "admin")

	r := authedRequest("GET", "/api/v1/admin/export", nil, token)
	w := serveSudoAdmin(db, "GET", "/api/v1/admin/export", handler.AdminExport(db, cfg), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("export without step-up returned %d, want 403: %s", w.Code, w.Body.String())
	}
}

// Promotion to instance admin hands someone the wipe button.
func TestPromoteToAdminRejectsSessionWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "promoter", "admin")
	targetUser, _ := createTestUser(t, db, "hopeful", "member")
	targetID := targetUser.ID

	r := authedRequest("PATCH", "/api/v1/admin/users/"+targetID,
		map[string]string{"role": "admin"}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/users/{id}", handler.UpdateUser(db), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("promotion without step-up returned %d, want 403: %s", w.Code, w.Body.String())
	}

	var role string
	db.QueryRow("SELECT role FROM users WHERE id = ?", targetID).Scan(&role)
	if role != "member" {
		t.Fatalf("target role is %q — promotion happened despite the 403", role)
	}
}

// With a live window, the same promotion goes through. The gate must be
// passable, not merely present.
func TestPromoteToAdminSucceedsWithStepUp(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "promoter2", "admin")
	targetUser, _ := createTestUser(t, db, "hopeful2", "member")
	targetID := targetUser.ID

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	r := authedRequest("PATCH", "/api/v1/admin/users/"+targetID,
		map[string]string{"role": "admin"}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/users/{id}", handler.UpdateUser(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("promotion with step-up returned %d, want 200: %s", w.Code, w.Body.String())
	}

	var role string
	db.QueryRow("SELECT role FROM users WHERE id = ?", targetID).Scan(&role)
	if role != "admin" {
		t.Fatalf("target role is %q, want admin", role)
	}
}

// ADR 017 gates promotion, not demotion: taking privilege away from an
// account you have just lost trust in should not require ceremony.
func TestDemotionNeedsNoStepUp(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "demoter", "admin")
	targetUser, _ := createTestUser(t, db, "outgoing", "admin")
	targetID := targetUser.ID

	r := authedRequest("PATCH", "/api/v1/admin/users/"+targetID,
		map[string]string{"role": "member"}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/users/{id}", handler.UpdateUser(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("demotion returned %d, want 200: %s", w.Code, w.Body.String())
	}

	var role string
	db.QueryRow("SELECT role FROM users WHERE id = ?", targetID).Scan(&role)
	if role != "member" {
		t.Fatalf("target role is %q, want member", role)
	}
}

// Routine admin work must stay unprompted — ADR 017 rejects step-up on every
// admin action, because reflexive approval is how step-up stops working.
func TestRoutineAdminActionsAreNotGated(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "routine-admin", "admin")

	r := authedRequest("PATCH", "/api/v1/admin/settings",
		map[string]string{"name": "Still Fine"}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)

	if w.Code != http.StatusOK {
		t.Fatalf("routine settings update returned %d, want 200: %s", w.Code, w.Body.String())
	}
}

// The 403 distinguishes "confirm with your passkey" from "you have no
// passkey", because those need different UI.
func TestGateReportsMissingPasskeyDistinctly(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	actor, token := createTestUser(t, db, "no-passkey-admin", "admin")
	userID := actor.ID

	r := authedRequest("GET", "/api/v1/admin/export", nil, token)
	w := serveSudoAdmin(db, "GET", "/api/v1/admin/export", handler.AdminExport(db, cfg), r)
	if got := bodyCode(t, w); got != "passkey_required" {
		t.Fatalf("code = %q, want passkey_required", got)
	}

	// With a passkey enrolled, the same rejection asks for confirmation
	// rather than enrollment.
	if _, err := db.Exec(
		`INSERT INTO credentials (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count) VALUES (?, ?, ?, ?, 'none', ?, 0)`,
		auth.NewUUIDv7(), userID, []byte("cred"), []byte("key"), make([]byte, 16),
	); err != nil {
		t.Fatalf("insert credential: %v", err)
	}

	r2 := authedRequest("GET", "/api/v1/admin/export", nil, token)
	w2 := serveSudoAdmin(db, "GET", "/api/v1/admin/export", handler.AdminExport(db, cfg), r2)
	if got := bodyCode(t, w2); got != "sudo_required" {
		t.Fatalf("code = %q, want sudo_required", got)
	}
}

// A live window lets the export through.
func TestExportSucceedsWithStepUp(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "export-admin2", "admin")

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	r := authedRequest("GET", "/api/v1/admin/export", nil, token)
	w := serveSudoAdmin(db, "GET", "/api/v1/admin/export", handler.AdminExport(db, cfg), r)

	if w.Code != http.StatusOK {
		t.Fatalf("export with step-up returned %d, want 200: %s", w.Code, w.Body.String())
	}
}

// The status endpoint is what the admin UI reads to warn about a missing
// passkey before someone hits the wall.
func TestStepUpStatusReportsPasskeyAbsence(t *testing.T) {
	db := setupTestDB(t)
	actor, token := createTestUser(t, db, "status-user", "admin")
	userID := actor.ID

	get := func() map[string]interface{} {
		r := authedRequest("GET", "/api/v1/auth/step-up", nil, token)
		w := httptest.NewRecorder()
		middleware.AuthRequired(db, handler.StepUpStatus(db))(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status returned %d: %s", w.Code, w.Body.String())
		}
		var out map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}

	if out := get(); out["has_passkey"] != false {
		t.Fatalf("has_passkey = %v, want false", out["has_passkey"])
	}

	if _, err := db.Exec(
		`INSERT INTO credentials (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count) VALUES (?, ?, ?, ?, 'none', ?, 0)`,
		auth.NewUUIDv7(), userID, []byte("cred"), []byte("key"), make([]byte, 16),
	); err != nil {
		t.Fatalf("insert credential: %v", err)
	}

	out := get()
	if out["has_passkey"] != true {
		t.Fatalf("has_passkey = %v, want true", out["has_passkey"])
	}
	if out["active"] != false {
		t.Fatalf("active = %v, want false before any ceremony", out["active"])
	}
}
