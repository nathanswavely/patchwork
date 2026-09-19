package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// A recovery code proves presence too (docs/adr/099): the way through for a
// person whose device makes no passkey, which before this was no way at all.

// agedCodes gives the user a batch of recovery codes and backdates both the
// batch and the session so the batch predates the sign-in, which is the
// ordinary shape — codes on paper from weeks ago, a session from today.
func agedCodes(t *testing.T, db *database.DB, userID, token string) []string {
	t.Helper()
	codes, err := auth.GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatalf("generate codes: %v", err)
	}
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	recent := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE recovery_codes SET created_at = ? WHERE user_id = ?`, old, userID)
	db.Exec(`UPDATE sessions SET created_at = ? WHERE token = ?`, recent, auth.HashToken(token))
	return codes
}

func postRecoveryStepUp(t *testing.T, db *database.DB, token, code string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/auth/step-up/recovery", map[string]string{"code": code}, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/step-up/recovery", middleware.AuthRequired(db, handler.StepUpRecovery(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// The whole point: a code opens the window, and the window then carries an
// action that a session alone cannot.
func TestStepUpRecovery_OpensTheWindowAndCarriesAWipe(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	admin, token := createTestUser(t, db, "rec-admin", "admin")
	codes := agedCodes(t, db, admin.ID, token)

	w := postRecoveryStepUp(t, db, token, codes[0])
	if w.Code != http.StatusOK {
		t.Fatalf("step-up by recovery code returned %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["active"] != true {
		t.Errorf("response did not report an open window: %v", body)
	}
	if n, _ := body["codes_remaining"].(float64); int(n) != auth.RecoveryCodeCount-1 {
		t.Errorf("codes_remaining = %v, want %d", body["codes_remaining"], auth.RecoveryCodeCount-1)
	}

	// Checked before the wipe, which takes the audit log with it.
	var audited int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'auth.step_up' AND user_id = ?`, admin.ID).Scan(&audited)
	if audited != 1 {
		t.Errorf("expected one auth.step_up audit row, got %d", audited)
	}

	// And the gate the person was stuck behind now opens.
	name := settings.EffectiveName(db, cfg)
	r := authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": name}, token)
	got := serveSudoAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r)
	if got.Code == http.StatusForbidden {
		t.Fatalf("the window did not carry the action: %s", got.Body.String())
	}
}

// A batch minted inside this session is refused, by its own name, and the
// gate stays shut. This is the property that keeps a stolen cookie from
// minting its own second factor.
func TestStepUpRecovery_RefusesCodesMintedInThisSession(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	admin, token := createTestUser(t, db, "rec-fresh", "admin")
	codes, err := auth.GenerateRecoveryCodes(db, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Session older than the codes, which is what "made during this
	// session" looks like from the database's side.
	old := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE sessions SET created_at = ? WHERE token = ?`, old, auth.HashToken(token))

	w := postRecoveryStepUp(t, db, token, codes[0])
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := bodyCode(t, w); code != "recovery_codes_too_new" {
		t.Errorf("error code = %q, want recovery_codes_too_new", code)
	}

	name := settings.EffectiveName(db, cfg)
	r := authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": name}, token)
	if got := serveSudoAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r); got.Code != http.StatusForbidden {
		t.Fatalf("the gate opened on a refused code: %d", got.Code)
	}
}

func TestStepUpRecovery_NamesItsRefusals(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "rec-refuse", "admin")

	// Nothing generated yet.
	if w := postRecoveryStepUp(t, db, token, "aaaa-bbbb-cccc"); bodyCode(t, w) != "no_recovery_codes" {
		t.Errorf("with no batch: got %q, want no_recovery_codes", bodyCode(t, w))
	}

	agedCodes(t, db, admin.ID, token)
	if w := postRecoveryStepUp(t, db, token, "zzzz-zzzz-zzzz"); bodyCode(t, w) != "invalid_code" {
		t.Errorf("with a wrong code: got %q, want invalid_code", bodyCode(t, w))
	}
}

// The status endpoint tells the page whether a code would be taken, so the
// prompt can offer the field only when it would work.
func TestStepUpStatus_ReportsUsableCodes(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "rec-status", "admin")

	read := func() map[string]interface{} {
		r := authedRequest("GET", "/api/v1/auth/step-up", nil, token)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/auth/step-up", middleware.AuthRequired(db, handler.StepUpStatus(db)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var body map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &body)
		return body
	}

	if n, _ := read()["recovery_ready"].(float64); n != 0 {
		t.Errorf("recovery_ready = %v before any batch, want 0", n)
	}
	agedCodes(t, db, admin.ID, token)
	if n, _ := read()["recovery_ready"].(float64); int(n) != auth.RecoveryCodeCount {
		t.Errorf("recovery_ready = %v, want %d", n, auth.RecoveryCodeCount)
	}
}

// Redeeming a code to sign in is itself the proof, so the window is open on
// arrival — without this the person who most needs the fallback would land
// holding a fresh batch that their brand-new session refuses.
func TestRecoverySignIn_ArrivesConfirmed(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "rec-signin", "admin")
	codes, err := auth.GenerateRecoveryCodes(db, admin.ID)
	if err != nil {
		t.Fatal(err)
	}

	r := authedRequest("POST", "/api/v1/auth/recovery",
		map[string]string{"username": "rec-signin", "code": codes[0]}, "")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/recovery", handler.RedeemRecoveryCode(db))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("recovery sign-in returned %d: %s", w.Code, w.Body.String())
	}

	var newToken string
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName {
			newToken = c.Value
		}
	}
	if newToken == "" {
		t.Fatal("recovery sign-in set no session cookie")
	}

	var sudoUntil string
	db.QueryRow(`SELECT COALESCE(sudo_until,'') FROM sessions WHERE token = ?`, auth.HashToken(newToken)).Scan(&sudoUntil)
	if sudoUntil == "" {
		t.Fatal("recovery sign-in left the step-up window shut")
	}
	until, err := time.Parse(time.RFC3339, sudoUntil)
	if err != nil || until.Before(time.Now().UTC()) {
		t.Errorf("sudo_until = %q, want a time in the future", sudoUntil)
	}
}
