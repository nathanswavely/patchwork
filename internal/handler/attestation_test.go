package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/attest"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// issueAttestation drives the endpoint through its real gate chain: admin,
// then step-up (docs/adr/017).
func issueAttestation(t *testing.T, db *database.DB, nonce, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/admin/attestation", map[string]string{"nonce": nonce}, token)
	return serveSudoAdmin(db, "POST", "/api/v1/admin/attestation",
		handler.IssueAttestation(db, testConfig()), r)
}

// attestationAdmin makes an admin whose session already holds a step-up
// window, with a unique username so the per-admin rate limit — which is
// process-global — never leaks between tests.
func attestationAdmin(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	_, token := createTestUser(t, db, username, "admin")
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}
	return token
}

func attestationBlob(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Attestation string `json:"attestation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return body.Attestation
}

// servePublic mounts a handler with no gate at all, which is the point: the
// key is for somebody with no account here.
func servePublic(method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, h)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// The end-to-end shape of the feature: an admin signs the outsider's nonce,
// the outsider fetches the key from the domain, and the two agree.
func TestAttestationVerifiesAgainstTheServedKey(t *testing.T) {
	db := setupTestDB(t)
	token := attestationAdmin(t, db, "attest-admin-roundtrip")

	w := issueAttestation(t, db, "host-ticket-40912", token)
	if w.Code != http.StatusOK {
		t.Fatalf("issue returned %d, want 200: %s", w.Code, w.Body.String())
	}
	blob := attestationBlob(t, w)

	kw := servePublic("GET", attest.KeyPath, handler.AttestationKey(db, testConfig()),
		httptest.NewRequest("GET", attest.KeyPath, nil))
	if kw.Code != http.StatusOK {
		t.Fatalf("key endpoint returned %d: %s", kw.Code, kw.Body.String())
	}
	var key struct {
		Algorithm string `json:"algorithm"`
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(kw.Body.Bytes(), &key); err != nil {
		t.Fatalf("decode key document: %v", err)
	}
	if key.Algorithm != attest.Algorithm {
		t.Errorf("algorithm = %q, want %q", key.Algorithm, attest.Algorithm)
	}

	statement, err := attest.Verify(blob, key.PublicKey, time.Now())
	if err != nil {
		t.Fatalf("verify against the served key: %v", err)
	}
	if statement.Nonce != "host-ticket-40912" {
		t.Errorf("nonce = %q", statement.Nonce)
	}
	if statement.Domain != "quilt.example.com" {
		t.Errorf("domain = %q, want the configured instance domain", statement.Domain)
	}
}

// It signs with the instance service actor's key — the one ADR 024 already
// mints — rather than a second keypair nobody would know to look for.
func TestAttestationUsesTheInstanceActorKey(t *testing.T) {
	db := setupTestDB(t)
	token := attestationAdmin(t, db, "attest-admin-samekey")

	if err := ap.EnsureInstanceActor(db, "quilt.example.com"); err != nil {
		t.Fatalf("ensure instance actor: %v", err)
	}
	_, publicKey, err := ap.InstanceActorKeys(db)
	if err != nil {
		t.Fatalf("instance actor keys: %v", err)
	}

	blob := attestationBlob(t, issueAttestation(t, db, "same-key-please", token))
	if _, err := attest.Verify(blob, publicKey, time.Now()); err != nil {
		t.Fatalf("the blob does not check against the instance actor's key: %v", err)
	}
}

// The blob leaves the building and stands on its own. An admin cookie alone
// must not be enough to mint one.
func TestAttestationRejectsSessionWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "attest-admin-nostepup", "admin")

	w := issueAttestation(t, db, "no-step-up-here", token)

	if w.Code != http.StatusForbidden {
		t.Fatalf("issue without step-up returned %d, want 403: %s", w.Code, w.Body.String())
	}
	if code := bodyCode(t, w); code != "sudo_required" && code != "passkey_required" {
		t.Fatalf("code = %q, want the SPA's step-up prompt", code)
	}
	var rows int
	db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action = 'admin.attestation_issued'").Scan(&rows)
	if rows != 0 {
		t.Fatalf("%d audit rows written despite the 403 — the gate ran after the work", rows)
	}
}

// A member holding a step-up window is still a member.
func TestAttestationRefusesNonAdmin(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "attest-member", "member")
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := issueAttestation(t, db, "not-an-admin-here", token)

	if w.Code != http.StatusForbidden {
		t.Fatalf("member got %d, want 403: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "attestation") {
		t.Fatalf("the refusal leaked a blob: %s", w.Body.String())
	}
}

func TestAttestationRefusesAnonymous(t *testing.T) {
	db := setupTestDB(t)

	w := issueAttestation(t, db, "nobody-at-all-here", "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous got %d, want 401: %s", w.Code, w.Body.String())
	}
}

// The nonce reaches the audit log and the signed payload, so it has to be
// one printable bounded line before either.
func TestAttestationValidatesTheNonce(t *testing.T) {
	db := setupTestDB(t)
	token := attestationAdmin(t, db, "attest-admin-nonce")

	for _, nonce := range []string{
		"",
		"short",
		strings.Repeat("x", 65),
		"line-one\nline-two",
		"tabbed\there",
	} {
		w := issueAttestation(t, db, nonce, token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("nonce %q returned %d, want 400: %s", nonce, w.Code, w.Body.String())
		}
	}
}

// The audit row is the only thing that can answer "did anyone here sign this
// string", so it has to carry the string.
func TestAttestationIsAudited(t *testing.T) {
	db := setupTestDB(t)
	token := attestationAdmin(t, db, "attest-admin-audit")

	if w := issueAttestation(t, db, "audit-me-40912", token); w.Code != http.StatusOK {
		t.Fatalf("issue returned %d: %s", w.Code, w.Body.String())
	}

	var metadata string
	err := db.QueryRow(
		"SELECT metadata FROM audit_log WHERE action = 'admin.attestation_issued'",
	).Scan(&metadata)
	if err != nil {
		t.Fatalf("no admin.attestation_issued row: %v", err)
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatalf("decode metadata %q: %v", metadata, err)
	}
	if meta["nonce"] != "audit-me-40912" {
		t.Errorf("audit nonce = %q, want the nonce that was signed", meta["nonce"])
	}
	if meta["expires_at"] == "" {
		t.Error("audit row records no expiry")
	}
}

// ADR 023 refuses to publish the roster. Neither the response nor the signed
// payload may name the admin who signed.
func TestAttestationNamesNobody(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "recognizable-steward", "admin")
	token, err := auth.CreateSession(db, admin.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := issueAttestation(t, db, "names-nobody-please", token)
	if w.Code != http.StatusOK {
		t.Fatalf("issue returned %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	for _, leak := range []string{admin.ID, admin.Username, admin.DisplayName} {
		if strings.Contains(body, leak) {
			t.Errorf("the response body carries %q — the attestation is about the quilt, not the person", leak)
		}
	}

	statement, err := attest.Parse(attestationBlob(t, w))
	if err != nil {
		t.Fatalf("parse blob: %v", err)
	}
	raw, _ := json.Marshal(statement)
	for _, leak := range []string{admin.ID, admin.Username, admin.DisplayName} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the signed statement carries %q", leak)
		}
	}
}

// The key is the half a verifier needs and a verifier has no account, so it
// answers an unauthenticated request — and it answers it on a quilt with
// federation off, which is the whole reason it does not live at /ap/instance.
func TestAttestationKeyIsPublicAndCacheable(t *testing.T) {
	db := setupTestDB(t)
	// No federation startup pass has run against this database — exactly the
	// state of an instance with federation.enabled = false.
	if err := ap.EnsureInstanceActor(db, "quilt.example.com"); err != nil {
		t.Fatalf("ensure instance actor: %v", err)
	}

	w := servePublic("GET", attest.KeyPath, handler.AttestationKey(db, testConfig()),
		httptest.NewRequest("GET", attest.KeyPath, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("anonymous key fetch returned %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age") {
		t.Errorf("Cache-Control = %q, want a cacheable answer", cc)
	}
	var key struct {
		PublicKey string `json:"public_key"`
		Domain    string `json:"domain"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &key); err != nil {
		t.Fatalf("decode key document: %v", err)
	}
	if !strings.Contains(key.PublicKey, "BEGIN PUBLIC KEY") {
		t.Errorf("public_key is not a PEM public key: %q", key.PublicKey)
	}
	if strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal("the key endpoint served a private key")
	}
	if key.Domain != "quilt.example.com" {
		t.Errorf("domain = %q", key.Domain)
	}
}

// An instance upgrading into this feature signs before its next restart, so
// the sign path mints the actor rather than failing on a database that has
// never federated.
func TestAttestationMintsTheInstanceActorOnDemand(t *testing.T) {
	db := setupTestDB(t)
	token := attestationAdmin(t, db, "attest-admin-mint")

	var before int
	db.QueryRow("SELECT COUNT(*) FROM instance_actor").Scan(&before)
	if before != 0 {
		t.Fatalf("fixture already has %d instance_actor rows", before)
	}

	if w := issueAttestation(t, db, "mint-on-demand-01", token); w.Code != http.StatusOK {
		t.Fatalf("issue returned %d: %s", w.Code, w.Body.String())
	}

	var after int
	db.QueryRow("SELECT COUNT(*) FROM instance_actor").Scan(&after)
	if after != 1 {
		t.Fatalf("instance_actor rows = %d, want 1", after)
	}
}
