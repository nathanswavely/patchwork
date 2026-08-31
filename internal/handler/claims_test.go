package handler_test

// Claim lifecycle tests (docs/adr/030): concurrent claims, per-user limit,
// withdraw, expiry, and all four verification methods — dns and meta_tag
// against injected fakes, email as a full round-trip through a captured
// sender, admin via the review endpoint.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

func claimCfg(smtp bool) *config.Config {
	cfg := &config.Config{}
	if smtp {
		cfg.SMTP = config.SMTP{Host: "smtp.test", From: "quilt@test"}
	}
	return cfg
}

// makeClaimable turns a node into an unclaimed patch with a verified domain.
func makeClaimable(t *testing.T, db *database.DB, nodeID, domain string) {
	t.Helper()
	if _, err := db.Exec("UPDATE nodes SET status = 'unclaimed', verification_domain = ? WHERE id = ?", domain, nodeID); err != nil {
		t.Fatalf("make claimable: %v", err)
	}
}

func openClaim(t *testing.T, db *database.DB, cfg *config.Config, token, slug string, body map[string]interface{}) (map[string]interface{}, int) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/claim", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/claim", handler.RequestClaim(db, cfg), r)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp, w.Code
}

func nodeState(t *testing.T, db *database.DB, nodeID string) (status, ownerID string) {
	t.Helper()
	if err := db.QueryRow("SELECT status, owner_id FROM nodes WHERE id = ?", nodeID).Scan(&status, &ownerID); err != nil {
		t.Fatalf("node state: %v", err)
	}
	return status, ownerID
}

func claimStatus(t *testing.T, db *database.DB, claimID string) string {
	t.Helper()
	var s string
	if err := db.QueryRow("SELECT status FROM claim_requests WHERE id = ?", claimID).Scan(&s); err != nil {
		t.Fatalf("claim status: %v", err)
	}
	return s
}

// claimSetupExpiresAt reads a claim's setup window, empty if unset.
func claimSetupExpiresAt(t *testing.T, db *database.DB, claimID string) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow("SELECT setup_expires_at FROM claim_requests WHERE id = ?", claimID).Scan(&s); err != nil {
		t.Fatalf("claim setup_expires_at: %v", err)
	}
	if !s.Valid {
		return ""
	}
	return s.String
}

// --- Concurrency + per-user rules ---

func TestClaimsRunConcurrently(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, bobToken := createTestUser(t, db, "bob", "member")

	nodeID := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeClaimable(t, db, nodeID, "westart.example")

	if _, code := openClaim(t, db, cfg, aliceToken, "spark-hall", map[string]interface{}{"method": "admin", "evidence": "I run it"}); code != http.StatusCreated {
		t.Fatalf("alice claim: got %d", code)
	}
	// Bob's claim must not be blocked by Alice's — a claim is an assertion,
	// not a reservation.
	if _, code := openClaim(t, db, cfg, bobToken, "spark-hall", map[string]interface{}{"method": "admin", "evidence": "no, I run it"}); code != http.StatusCreated {
		t.Fatalf("bob claim blocked by alice's: got %d", code)
	}
	// But Alice can't open a second one.
	if _, code := openClaim(t, db, cfg, aliceToken, "spark-hall", map[string]interface{}{"method": "admin"}); code != http.StatusConflict {
		t.Fatalf("alice duplicate claim: got %d, want 409", code)
	}
}

func TestSelfServiceMethodsNeedVerifiedDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "No Domain", "no-domain", "open")
	makeClaimable(t, db, nodeID, "") // unclaimed, but nothing vetted

	for _, method := range []string{"dns", "meta_tag", "email"} {
		if _, code := openClaim(t, db, cfg, aliceToken, "no-domain", map[string]interface{}{"method": method, "email": "a@b.c"}); code != http.StatusBadRequest {
			t.Fatalf("method %s without domain: got %d, want 400", method, code)
		}
	}
	if _, code := openClaim(t, db, cfg, aliceToken, "no-domain", map[string]interface{}{"method": "admin", "evidence": "e"}); code != http.StatusCreated {
		t.Fatalf("admin method without domain: got %d, want 201", code)
	}
}

// --- DNS ---

func TestClaimDNSVerify(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	alice, aliceToken := createTestUser(t, db, "alice", "member")
	_, bobToken := createTestUser(t, db, "bob", "member")

	nodeID := createTestNode(t, db, owner.ID, "DNS Venue", "dns-venue", "open")
	makeClaimable(t, db, nodeID, "dnsvenue.example")

	resp, code := openClaim(t, db, cfg, aliceToken, "dns-venue", map[string]interface{}{"method": "dns"})
	if code != http.StatusCreated {
		t.Fatalf("open dns claim: got %d", code)
	}
	claimID := resp["id"].(string)
	record := resp["record_value"].(string)

	bobResp, _ := openClaim(t, db, cfg, bobToken, "dns-venue", map[string]interface{}{"method": "admin", "evidence": "mine"})
	bobClaimID := bobResp["id"].(string)

	// Wrong TXT records: verification fails, claim stays pending.
	orig := handler.ClaimLookupTXT
	t.Cleanup(func() { handler.ClaimLookupTXT = orig })
	handler.ClaimLookupTXT = func(domain string) ([]string, error) {
		return []string{"unrelated=nope"}, nil
	}
	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/verify", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/verify", handler.VerifyClaim(db), r)
	var vr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &vr)
	if vr["verified"] != false {
		t.Fatal("verification passed with wrong TXT record")
	}

	// Correct record on the verification domain.
	handler.ClaimLookupTXT = func(domain string) ([]string, error) {
		if domain != "dnsvenue.example" {
			t.Fatalf("lookup on %q, want the verification domain", domain)
		}
		return []string{"  " + record + "  "}, nil
	}
	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/verify", nil, aliceToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/verify", handler.VerifyClaim(db), r)
	json.Unmarshal(w.Body.Bytes(), &vr)
	if vr["verified"] != true {
		t.Fatalf("dns verification failed: %s", w.Body.String())
	}
	if vr["setup_required"] != true {
		t.Fatalf("verify response missing setup_required: %s", w.Body.String())
	}

	// Verification approves the claim and opens a setup window — it does not
	// activate the patch (docs/adr/039). Every visitor still sees "unclaimed"
	// until setup is submitted.
	status, ownerID := nodeState(t, db, nodeID)
	if status != "unclaimed" || ownerID != owner.ID {
		t.Fatalf("node after verify: status=%s owner=%s, want still unclaimed under the original owner", status, ownerID)
	}
	if s := claimStatus(t, db, claimID); s != "approved" {
		t.Fatalf("winning claim status: %s", s)
	}
	if claimSetupExpiresAt(t, db, claimID) == "" {
		t.Fatal("approved claim has no setup_expires_at")
	}
	var claimant string
	db.QueryRow("SELECT user_id FROM claim_requests WHERE id = ?", claimID).Scan(&claimant)
	if claimant != alice.ID {
		t.Fatalf("approved claim belongs to %s, want alice", claimant)
	}
	// First proof wins: the competing claim is auto-rejected.
	if s := claimStatus(t, db, bobClaimID); s != "rejected" {
		t.Fatalf("sibling claim status: %s, want rejected", s)
	}
}

// --- Meta tag ---

// rewriteTransport sends every request to the test server, whatever host the
// client asked for.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestClaimMetaTagVerify(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Meta Venue", "meta-venue", "open")
	makeClaimable(t, db, nodeID, "metavenue.example")

	resp, code := openClaim(t, db, cfg, aliceToken, "meta-venue", map[string]interface{}{"method": "meta_tag"})
	if code != http.StatusCreated {
		t.Fatalf("open meta claim: got %d", code)
	}
	claimID := resp["id"].(string)
	metaContent := resp["meta_content"].(string)

	page := ""
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, page)
	}))
	t.Cleanup(site.Close)
	target, _ := url.Parse(site.URL)

	origClient := handler.ClaimHTTPClient
	t.Cleanup(func() { handler.ClaimHTTPClient = origClient })
	handler.ClaimHTTPClient = &http.Client{Transport: rewriteTransport{target: target}}

	// Page without the tag: fails.
	page = "<html><head><title>hi</title></head></html>"
	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/verify", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/verify", handler.VerifyClaim(db), r)
	var vr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &vr)
	if vr["verified"] != false {
		t.Fatal("verification passed without the meta tag")
	}

	// Page with the tag: succeeds — approves the claim and opens setup,
	// but the patch stays unclaimed until setup is submitted (docs/adr/039).
	page = fmt.Sprintf(`<html><head><meta name="patchwork-verify" content="%s"></head></html>`, metaContent)
	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/verify", nil, aliceToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/verify", handler.VerifyClaim(db), r)
	json.Unmarshal(w.Body.Bytes(), &vr)
	if vr["verified"] != true {
		t.Fatalf("meta_tag verification failed: %s", w.Body.String())
	}
	if vr["setup_required"] != true {
		t.Fatalf("verify response missing setup_required: %s", w.Body.String())
	}
	status, ownerID := nodeState(t, db, nodeID)
	if status != "unclaimed" || ownerID != owner.ID {
		t.Fatalf("node after verify: status=%s owner=%s, want still unclaimed", status, ownerID)
	}
	if s := claimStatus(t, db, claimID); s != "approved" {
		t.Fatalf("claim after verify: %s, want approved", s)
	}
	if claimSetupExpiresAt(t, db, claimID) == "" {
		t.Fatal("approved claim has no setup_expires_at")
	}
}

// The default meta_tag fetch client must refuse non-public addresses: it
// fetches a page on the claimed domain, a URL someone outside the
// instance influences (SSRF).
func TestClaimHTTPClientRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	// srv listens on 127.0.0.1 — the guard must refuse it.
	resp, err := handler.ClaimHTTPClient.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected ssrf guard refusal for loopback fetch, got success")
	}
	if !strings.Contains(err.Error(), "ssrf guard") {
		t.Fatalf("expected ssrf guard error, got %v", err)
	}
}

// --- Email ---

func TestClaimEmailRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	alice, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Mail Venue", "mail-venue", "open")
	makeClaimable(t, db, nodeID, "mailvenue.example")

	var sentTo string
	var sentMsg string
	origSend := handler.ClaimSendMail
	t.Cleanup(func() { handler.ClaimSendMail = origSend })
	handler.ClaimSendMail = func(smtp config.SMTP, to []string, msg []byte) error {
		sentTo = to[0]
		sentMsg = string(msg)
		return nil
	}

	// Wrong domain is refused before anything sends.
	if _, code := openClaim(t, db, cfg, aliceToken, "mail-venue", map[string]interface{}{"method": "email", "email": "alice@gmail.com"}); code != http.StatusBadRequest {
		t.Fatalf("off-domain email: got %d, want 400", code)
	}

	if _, code := openClaim(t, db, cfg, aliceToken, "mail-venue", map[string]interface{}{"method": "email", "email": "Booking@MailVenue.example"}); code != http.StatusCreated {
		t.Fatalf("open email claim: got %d", code)
	}
	if sentTo != "booking@mailvenue.example" {
		t.Fatalf("mail sent to %q", sentTo)
	}
	tokenMatch := regexp.MustCompile(`token=([0-9a-f]+)`).FindStringSubmatch(sentMsg)
	if tokenMatch == nil {
		t.Fatalf("no token link in mail body: %s", sentMsg)
	}
	token := tokenMatch[1]

	// The GET is read-only info for the landing page.
	r := authedRequest("GET", "/api/v1/claims/verify-email?token="+token, nil, "")
	w := servePublicMux(t, "GET", "/api/v1/claims/verify-email", handler.EmailClaimInfo(db), r)
	var info map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &info)
	if info["node_name"] != "Mail Venue" || info["expired"] != false {
		t.Fatalf("email claim info: %s", w.Body.String())
	}
	if status, _ := nodeState(t, db, nodeID); status != "unclaimed" {
		t.Fatal("GET completed the claim — it must be read-only")
	}

	// The POST completes it, no session needed. It approves the claim and
	// opens setup — the patch itself stays unclaimed (docs/adr/039).
	r = authedRequest("POST", "/api/v1/claims/verify-email", map[string]interface{}{"token": token}, "")
	w = servePublicMux(t, "POST", "/api/v1/claims/verify-email", handler.CompleteEmailClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("complete email claim: %d %s", w.Code, w.Body.String())
	}
	var completeResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &completeResp)
	if completeResp["setup_required"] != true {
		t.Fatalf("complete-email response missing setup_required: %s", w.Body.String())
	}
	status, ownerID := nodeState(t, db, nodeID)
	if status != "unclaimed" || ownerID != owner.ID {
		t.Fatalf("node after email claim: status=%s owner=%s, want still unclaimed", status, ownerID)
	}
	var emailClaimID string
	db.QueryRow("SELECT id FROM claim_requests WHERE node_id = ? AND method = 'email'", nodeID).Scan(&emailClaimID)
	if s := claimStatus(t, db, emailClaimID); s != "approved" {
		t.Fatalf("claim after email complete: %s, want approved", s)
	}
	var emailClaimant string
	db.QueryRow("SELECT user_id FROM claim_requests WHERE id = ?", emailClaimID).Scan(&emailClaimant)
	if emailClaimant != alice.ID {
		t.Fatalf("approved claim belongs to %s, want alice", emailClaimant)
	}
	if claimSetupExpiresAt(t, db, emailClaimID) == "" {
		t.Fatal("approved email claim has no setup_expires_at")
	}

	// Token is single-use: the claim is no longer pending, so replay dies.
	r = authedRequest("POST", "/api/v1/claims/verify-email", map[string]interface{}{"token": token}, "")
	w = servePublicMux(t, "POST", "/api/v1/claims/verify-email", handler.CompleteEmailClaim(db), r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("token replay: got %d, want 404", w.Code)
	}
}

func TestClaimEmailExpiryAndResendLimit(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Slow Venue", "slow-venue", "open")
	makeClaimable(t, db, nodeID, "slowvenue.example")

	sends := 0
	origSend := handler.ClaimSendMail
	t.Cleanup(func() { handler.ClaimSendMail = origSend })
	handler.ClaimSendMail = func(smtp config.SMTP, to []string, msg []byte) error {
		sends++
		return nil
	}

	resp, code := openClaim(t, db, cfg, aliceToken, "slow-venue", map[string]interface{}{"method": "email", "email": "a@slowvenue.example"})
	if code != http.StatusCreated {
		t.Fatalf("open email claim: got %d", code)
	}
	claimID := resp["id"].(string)

	// Expire the token; completing must fail but the claim stays pending.
	var token string
	db.QueryRow("SELECT verification_token FROM claim_requests WHERE id = ?", claimID).Scan(&token)
	past := time.Now().Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec("UPDATE claim_requests SET email_token_expires_at = ? WHERE id = ?", past, claimID)

	r := authedRequest("POST", "/api/v1/claims/verify-email", map[string]interface{}{"token": token}, "")
	w := servePublicMux(t, "POST", "/api/v1/claims/verify-email", handler.CompleteEmailClaim(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expired token: got %d, want 400", w.Code)
	}
	if s := claimStatus(t, db, claimID); s != "pending" {
		t.Fatalf("claim after expired link: %s, want pending", s)
	}

	// Resend refreshes the expiry (2 more allowed in the window)...
	for i := 0; i < 2; i++ {
		r = authedRequest("POST", "/api/v1/claims/"+claimID+"/resend-email", nil, aliceToken)
		w = serveMux(t, db, "POST", "/api/v1/claims/{id}/resend-email", handler.ResendClaimEmail(db, cfg), r)
		if w.Code != http.StatusOK {
			t.Fatalf("resend %d: got %d %s", i, w.Code, w.Body.String())
		}
	}
	// ...and the fourth send in 24h is refused.
	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/resend-email", nil, aliceToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/resend-email", handler.ResendClaimEmail(db, cfg), r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("resend over limit: got %d, want 429", w.Code)
	}

	// The refreshed link works now.
	r = authedRequest("POST", "/api/v1/claims/verify-email", map[string]interface{}{"token": token}, "")
	w = servePublicMux(t, "POST", "/api/v1/claims/verify-email", handler.CompleteEmailClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("refreshed token: got %d %s", w.Code, w.Body.String())
	}
}

// --- Withdraw + reopen (the reported bug) ---

func TestWithdrawThenChooseAnotherMethod(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, bobToken := createTestUser(t, db, "bob", "member")

	nodeID := createTestNode(t, db, owner.ID, "Regret Venue", "regret-venue", "open")
	makeClaimable(t, db, nodeID, "regret.example")

	resp, code := openClaim(t, db, cfg, aliceToken, "regret-venue", map[string]interface{}{"method": "meta_tag"})
	if code != http.StatusCreated {
		t.Fatalf("open claim: got %d", code)
	}
	claimID := resp["id"].(string)

	// Bob can't withdraw Alice's claim.
	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/withdraw", nil, bobToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/withdraw", handler.WithdrawClaim(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign withdraw: got %d, want 403", w.Code)
	}

	// Alice withdraws...
	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/withdraw", nil, aliceToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/withdraw", handler.WithdrawClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("withdraw: got %d %s", w.Code, w.Body.String())
	}
	if s := claimStatus(t, db, claimID); s != "withdrawn" {
		t.Fatalf("claim after withdraw: %s", s)
	}

	// ...MyClaim no longer reports an open claim...
	r = authedRequest("GET", "/api/v1/nodes/regret-venue/claims/mine", nil, aliceToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/claims/mine", handler.MyClaim(db, cfg), r)
	var mine map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &mine)
	if mine["claim"] != nil {
		t.Fatalf("open claim survived withdrawal: %s", w.Body.String())
	}

	// ...and she can open a claim with a different method.
	if _, code := openClaim(t, db, cfg, aliceToken, "regret-venue", map[string]interface{}{"method": "admin", "evidence": "actually mine"}); code != http.StatusCreated {
		t.Fatalf("re-claim after withdraw: got %d", code)
	}
	_ = nodeID
}

func TestMyClaimSurvivesReload(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Reload Venue", "reload-venue", "open")
	makeClaimable(t, db, nodeID, "reload.example")

	resp, _ := openClaim(t, db, cfg, aliceToken, "reload-venue", map[string]interface{}{"method": "dns"})
	wantRecord := resp["record_value"].(string)

	// A fresh page load can recover the claim and its instructions.
	r := authedRequest("GET", "/api/v1/nodes/reload-venue/claims/mine", nil, aliceToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/claims/mine", handler.MyClaim(db, cfg), r)
	var mine struct {
		Claim   map[string]interface{} `json:"claim"`
		Methods map[string]bool        `json:"methods"`
	}
	json.Unmarshal(w.Body.Bytes(), &mine)
	if mine.Claim == nil || mine.Claim["record_value"] != wantRecord {
		t.Fatalf("claims/mine lost the claim or its instructions: %s", w.Body.String())
	}
	if !mine.Methods["dns"] || !mine.Methods["meta_tag"] || !mine.Methods["admin"] {
		t.Fatalf("methods map wrong: %v", mine.Methods)
	}
	// No SMTP on this instance — email must read as unavailable.
	if mine.Methods["email"] {
		t.Fatal("email method offered without SMTP")
	}
}

// --- Admin review ---

func TestAdminReviewClaim(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	alice, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Review Venue", "review-venue", "open")
	makeClaimable(t, db, nodeID, "")

	resp, _ := openClaim(t, db, cfg, aliceToken, "review-venue", map[string]interface{}{"method": "admin", "evidence": "I book every show"})
	claimID := resp["id"].(string)

	// Reject first.
	r := authedRequest("PATCH", "/api/v1/admin/claims/"+claimID, map[string]interface{}{"action": "reject", "note": "not enough"}, adminToken)
	w := serveAdminMux(t, db, "PATCH", "/api/v1/admin/claims/{id}", handler.ReviewClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("reject: got %d", w.Code)
	}
	if s := claimStatus(t, db, claimID); s != "rejected" {
		t.Fatalf("after reject: %s", s)
	}

	// New claim, approve: opens the claimant's setup window — it does not
	// transfer ownership by itself (docs/adr/039).
	resp, _ = openClaim(t, db, cfg, aliceToken, "review-venue", map[string]interface{}{"method": "admin", "evidence": "here are the deeds"})
	claimID = resp["id"].(string)
	r = authedRequest("PATCH", "/api/v1/admin/claims/"+claimID, map[string]interface{}{"action": "approve"}, adminToken)
	w = serveAdminMux(t, db, "PATCH", "/api/v1/admin/claims/{id}", handler.ReviewClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	var reviewResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &reviewResp)
	if reviewResp["setup_required"] != true {
		t.Fatalf("review-approve response missing setup_required: %s", w.Body.String())
	}
	status, ownerID := nodeState(t, db, nodeID)
	if status != "unclaimed" || ownerID != owner.ID {
		t.Fatalf("node after approve: status=%s owner=%s, want still unclaimed", status, ownerID)
	}
	if s := claimStatus(t, db, claimID); s != "approved" {
		t.Fatalf("claim after review-approve: %s, want approved", s)
	}
	if claimSetupExpiresAt(t, db, claimID) == "" {
		t.Fatal("reviewed claim has no setup_expires_at")
	}
	var reviewedClaimant string
	db.QueryRow("SELECT user_id FROM claim_requests WHERE id = ?", claimID).Scan(&reviewedClaimant)
	if reviewedClaimant != alice.ID {
		t.Fatalf("approved claim belongs to %s, want alice", reviewedClaimant)
	}

	// The claimant completes setup like any other claim.
	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: got %d %s", w.Code, w.Body.String())
	}
	status, ownerID = nodeState(t, db, nodeID)
	if status != "active" || ownerID != alice.ID {
		t.Fatalf("node after setup: status=%s owner=%s", status, ownerID)
	}
}

// --- Patch setup (docs/adr/039) ---

// approveAdminClaim opens an admin-method claim for claimantToken and has
// adminToken approve it, returning the claim ID with status 'approved' and
// setup_expires_at set — the shared starting point for every setup test.
func approveAdminClaim(t *testing.T, db *database.DB, cfg *config.Config, slug, claimantToken, adminToken string) string {
	t.Helper()
	resp, code := openClaim(t, db, cfg, claimantToken, slug, map[string]interface{}{"method": "admin", "evidence": "it's mine"})
	if code != http.StatusCreated {
		t.Fatalf("open claim: got %d", code)
	}
	claimID := resp["id"].(string)

	r := authedRequest("PATCH", "/api/v1/admin/claims/"+claimID, map[string]interface{}{"action": "approve"}, adminToken)
	w := serveAdminMux(t, db, "PATCH", "/api/v1/admin/claims/{id}", handler.ReviewClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	return claimID
}

func TestSetupClaimHappyPath(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	alice, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })

	nodeID := createTestNode(t, db, owner.ID, "Setup Venue", "setup-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "setup-venue", aliceToken, adminToken)

	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" || resp["slug"] != "setup-venue" {
		t.Fatalf("setup response: %s", w.Body.String())
	}

	status, ownerID := nodeState(t, db, nodeID)
	if status != "active" || ownerID != alice.ID {
		t.Fatalf("node after setup: status=%s owner=%s", status, ownerID)
	}

	var memRole, memStatus string
	if err := db.QueryRow("SELECT role, status FROM memberships WHERE user_id = ? AND node_id = ?", alice.ID, nodeID).Scan(&memRole, &memStatus); err != nil {
		t.Fatalf("claimant membership missing: %v", err)
	}
	if memRole != "admin" || memStatus != "active" {
		t.Fatalf("claimant membership: role=%s status=%s", memRole, memStatus)
	}

	var kind, docBody string
	if err := db.QueryRow("SELECT kind, body FROM governance_docs WHERE node_id = ?", nodeID).Scan(&kind, &docBody); err != nil {
		t.Fatalf("lining row missing after setup: %v", err)
	}
	if kind != "lining" || docBody != governance.CurrentLiningBody() {
		t.Fatalf("lining after setup: kind=%s, body matches current=%v", kind, docBody == governance.CurrentLiningBody())
	}

	// The lining is adopted out loud on setup, git mirror included — the
	// create-path bug this task also fixes.
	gitBody, err := governance.GetDocument(tmp, nodeID, "community-standards.md")
	if err != nil || gitBody != docBody {
		t.Errorf("git mirror missing or mismatched at setup: err=%v", err)
	}

	// The claim itself stays 'approved' — consumed is distinguished by the
	// node being active, not by a claim status change (docs/adr/039).
	if s := claimStatus(t, db, claimID); s != "approved" {
		t.Fatalf("claim after setup: %s, want still approved", s)
	}
}

func TestSetupClaimWithTemplate(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })

	nodeID := createTestNode(t, db, owner.ID, "Formal Venue", "formal-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "formal-venue", aliceToken, adminToken)

	// Setup reuses the creation form, template picker included (docs/adr/039).
	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", map[string]interface{}{"template": "formal"}, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup with template: got %d %s", w.Code, w.Body.String())
	}

	if _, err := governance.GetDocument(tmp, nodeID, "charter.md"); err != nil {
		t.Errorf("formal template file missing from forked repo: %v", err)
	}
}

func TestSetupClaimUnknownTemplateRejected(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Bad Template Venue", "bad-template-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "bad-template-venue", aliceToken, adminToken)

	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", map[string]interface{}{"template": "not-a-template"}, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown template: got %d, want 400: %s", w.Code, w.Body.String())
	}
	if status, _ := nodeState(t, db, nodeID); status != "unclaimed" {
		t.Fatal("node activated despite rejected template")
	}
}

func TestSetupClaimForbiddenForOtherUser(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, bobToken := createTestUser(t, db, "bob", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Guarded Venue", "guarded-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "guarded-venue", aliceToken, adminToken)

	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, bobToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign setup: got %d, want 403", w.Code)
	}
	if status, _ := nodeState(t, db, nodeID); status != "unclaimed" {
		t.Fatal("node activated by a foreign setup attempt")
	}
}

func TestSetupClaimAfterExpiry(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Lapsed Venue", "lapsed-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "lapsed-venue", aliceToken, adminToken)

	past := time.Now().Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec("UPDATE claim_requests SET setup_expires_at = ? WHERE id = ?", past, claimID)

	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusGone {
		t.Fatalf("expired setup window: got %d, want 410: %s", w.Code, w.Body.String())
	}
	if s := claimStatus(t, db, claimID); s != "expired" {
		t.Fatalf("claim after expired setup attempt: %s, want expired", s)
	}
	if status, _ := nodeState(t, db, nodeID); status != "unclaimed" {
		t.Fatal("node activated after setup window expired")
	}
}

func TestSetupClaimSecondAttemptConflicts(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Twice Venue", "twice-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "twice-venue", aliceToken, adminToken)

	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("first setup: got %d %s", w.Code, w.Body.String())
	}

	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusConflict {
		t.Fatalf("second setup attempt: got %d, want 409: %s", w.Code, w.Body.String())
	}
	_ = nodeID
}

func TestRequestClaimBlockedByApprovedClaim(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, bobToken := createTestUser(t, db, "bob", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Contested Venue", "contested-venue", "open")
	makeClaimable(t, db, nodeID, "")
	approveAdminClaim(t, db, cfg, "contested-venue", aliceToken, adminToken)

	// Someone else can't open a fresh claim while alice's setup window is
	// still open — a claim is single-use, but it isn't nothing either
	// (docs/adr/039).
	if _, code := openClaim(t, db, cfg, bobToken, "contested-venue", map[string]interface{}{"method": "admin", "evidence": "actually me"}); code != http.StatusConflict {
		t.Fatalf("claim while another is approved: got %d, want 409", code)
	}
}

func TestRequestClaimNotBlockedByExpiredApprovedClaim(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, bobToken := createTestUser(t, db, "bob", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Lapsed Open Venue", "lapsed-open-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "lapsed-open-venue", aliceToken, adminToken)

	past := time.Now().Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec("UPDATE claim_requests SET setup_expires_at = ? WHERE id = ?", past, claimID)

	if _, code := openClaim(t, db, cfg, bobToken, "lapsed-open-venue", map[string]interface{}{"method": "admin", "evidence": "now it's me"}); code != http.StatusCreated {
		t.Fatalf("claim after the prior one expired: got %d, want 201", code)
	}
	if s := claimStatus(t, db, claimID); s != "expired" {
		t.Fatalf("lapsed claim not lazily expired: %s", s)
	}
}

func TestMyClaimReturnsApprovedClaim(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Approved Reload Venue", "approved-reload-venue", "open")
	makeClaimable(t, db, nodeID, "")
	approveAdminClaim(t, db, cfg, "approved-reload-venue", aliceToken, adminToken)

	r := authedRequest("GET", "/api/v1/nodes/approved-reload-venue/claims/mine", nil, aliceToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/claims/mine", handler.MyClaim(db, cfg), r)
	var mine struct {
		Claim map[string]interface{} `json:"claim"`
	}
	json.Unmarshal(w.Body.Bytes(), &mine)
	if mine.Claim == nil {
		t.Fatalf("no claim returned: %s", w.Body.String())
	}
	if mine.Claim["status"] != "approved" {
		t.Fatalf("claim status = %v, want approved", mine.Claim["status"])
	}
	if s, _ := mine.Claim["setup_expires_at"].(string); s == "" {
		t.Fatalf("claims/mine missing setup_expires_at: %s", w.Body.String())
	}
}

func TestAdminAssignOwnerOpensSetupWindow(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	assignee, assigneeToken := createTestUser(t, db, "assignee", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Assigned Venue", "assigned-venue", "open")
	makeClaimable(t, db, nodeID, "")

	r := authedRequest("POST", "/api/v1/admin/nodes/assigned-venue/assign", map[string]string{"user_id": assignee.ID}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/admin/nodes/{slug}/assign", handler.AdminAssignOwner(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("assign: got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["setup_required"] != true {
		t.Fatalf("assign response missing setup_required: %s", w.Body.String())
	}

	// Consent can't be assigned: the patch stays unclaimed, and what exists
	// is a claim (approved, no proof required) rather than an activation.
	status, ownerID := nodeState(t, db, nodeID)
	if status != "unclaimed" || ownerID != owner.ID {
		t.Fatalf("node after assign: status=%s owner=%s, want still unclaimed", status, ownerID)
	}
	var method, evidence, claimStat string
	var setupExpiresAt sql.NullString
	if err := db.QueryRow("SELECT method, evidence, status, setup_expires_at FROM claim_requests WHERE node_id = ? AND user_id = ?", nodeID, assignee.ID).
		Scan(&method, &evidence, &claimStat, &setupExpiresAt); err != nil {
		t.Fatalf("assigned claim missing: %v", err)
	}
	if method != "admin" || evidence != "Assigned by instance admin" || claimStat != "approved" || !setupExpiresAt.Valid {
		t.Fatalf("assigned claim: method=%s evidence=%s status=%s setup_expires_at.Valid=%v", method, evidence, claimStat, setupExpiresAt.Valid)
	}

	// The assignee still has to complete setup — assignment isn't a shortcut
	// past the creation moment.
	var claimID string
	db.QueryRow("SELECT id FROM claim_requests WHERE node_id = ? AND user_id = ?", nodeID, assignee.ID).Scan(&claimID)
	r = authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, assigneeToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("assignee setup: got %d %s", w.Code, w.Body.String())
	}
	status, ownerID = nodeState(t, db, nodeID)
	if status != "active" || ownerID != assignee.ID {
		t.Fatalf("node after assignee setup: status=%s owner=%s", status, ownerID)
	}
}

// --- Expiry sweep ---

func TestExpireStaleClaims(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Stale Venue", "stale-venue", "open")
	makeClaimable(t, db, nodeID, "stale.example")

	resp, _ := openClaim(t, db, cfg, aliceToken, "stale-venue", map[string]interface{}{"method": "dns"})
	claimID := resp["id"].(string)

	// Fresh claims survive the sweep.
	notifications.ExpireStaleClaims(db)
	if s := claimStatus(t, db, claimID); s != "pending" {
		t.Fatalf("fresh claim swept: %s", s)
	}

	// Backdate past 30 days: swept to expired.
	old := time.Now().Add(-31 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec("UPDATE claim_requests SET created_at = ? WHERE id = ?", old, claimID)
	notifications.ExpireStaleClaims(db)
	if s := claimStatus(t, db, claimID); s != "expired" {
		t.Fatalf("stale claim not expired: %s", s)
	}

	// An expired claim doesn't block a fresh one.
	if _, code := openClaim(t, db, cfg, aliceToken, "stale-venue", map[string]interface{}{"method": "dns"}); code != http.StatusCreated {
		t.Fatalf("re-claim after expiry: got %d", code)
	}
}

// --- Verification domain provenance ---

func TestAdminCreateDerivesVerificationDomain(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	// A real website derives an anchor (scheme, www, path stripped).
	r := authedRequest("POST", "/api/v1/admin/unclaimed", map[string]interface{}{"name": "Real Venue", "website": "https://www.RealVenue.example/about"}, adminToken)
	w := serveAdminMux(t, db, "POST", "/api/v1/admin/unclaimed", handler.CreateUnclaimedPatch(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", w.Code, w.Body.String())
	}
	var vd string
	db.QueryRow("SELECT COALESCE(verification_domain,'') FROM nodes WHERE slug = 'real-venue'").Scan(&vd)
	if vd != "realvenue.example" {
		t.Fatalf("derived domain: %q", vd)
	}

	// A shared platform never becomes an anchor.
	r = authedRequest("POST", "/api/v1/admin/unclaimed", map[string]interface{}{"name": "FB Band", "website": "https://facebook.com/fbband"}, adminToken)
	w = serveAdminMux(t, db, "POST", "/api/v1/admin/unclaimed", handler.CreateUnclaimedPatch(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create fb: got %d", w.Code)
	}
	db.QueryRow("SELECT COALESCE(verification_domain,'') FROM nodes WHERE slug = 'fb-band'").Scan(&vd)
	if vd != "" {
		t.Fatalf("facebook derived an anchor: %q", vd)
	}

	// Explicitly naming a shared platform is refused outright.
	r = authedRequest("POST", "/api/v1/admin/unclaimed", map[string]interface{}{"name": "Sneaky", "verification_domain": "gmail.com"}, adminToken)
	w = serveAdminMux(t, db, "POST", "/api/v1/admin/unclaimed", handler.CreateUnclaimedPatch(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("explicit gmail.com: got %d, want 400", w.Code)
	}
}

func TestCommunitySubmissionCarriesNoAnchor(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true, AutoApprove: true}}
	_, memberToken := createTestUser(t, db, "randomperson", "member")
	trusted, trustedToken := createTestUser(t, db, "helper", "member")
	makeTrusted(t, db, trusted.ID)

	// Ordinary submitter: website accepted, but no verification domain —
	// even with auto-approve, self-service claiming stays closed.
	r := authedRequest("POST", "/api/v1/submissions", map[string]interface{}{"name": "Fake Real Venue", "website": "https://attacker.example"}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}
	var vd string
	db.QueryRow("SELECT COALESCE(verification_domain,'') FROM nodes WHERE slug = 'fake-real-venue'").Scan(&vd)
	if vd != "" {
		t.Fatalf("untrusted submission derived an anchor: %q", vd)
	}

	// Trusted contributor: their website vouches.
	r = authedRequest("POST", "/api/v1/submissions", map[string]interface{}{"name": "Vouched Venue", "website": "https://vouched.example"}, trustedToken)
	w = serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("trusted submit: got %d", w.Code)
	}
	db.QueryRow("SELECT COALESCE(verification_domain,'') FROM nodes WHERE slug = 'vouched-venue'").Scan(&vd)
	if vd != "vouched.example" {
		t.Fatalf("trusted submission anchor: %q", vd)
	}
}

func TestReviewSubmissionSetsAnchor(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true}}
	_, memberToken := createTestUser(t, db, "randomperson", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	r := authedRequest("POST", "/api/v1/submissions", map[string]interface{}{"name": "Pending Venue", "website": "https://pending.example"}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	var sub map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &sub)
	nodeID := sub["id"].(string)

	// The review queue suggests the derived domain to the admin.
	r = authedRequest("GET", "/api/v1/admin/submissions", nil, adminToken)
	w = serveAdminMux(t, db, "GET", "/api/v1/admin/submissions", handler.ListSubmissions(db), r)
	if !bodyContains(w.Body.Bytes(), `"suggested_verification_domain":"pending.example"`) {
		t.Fatalf("no suggestion in review queue: %s", w.Body.String())
	}

	// Approving with a vetted domain applies it.
	r = authedRequest("PATCH", "/api/v1/admin/submissions/"+nodeID, map[string]interface{}{"action": "approve", "verification_domain": "pending.example"}, adminToken)
	w = serveAdminMux(t, db, "PATCH", "/api/v1/admin/submissions/{id}", handler.ReviewSubmission(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	var vd, status string
	db.QueryRow("SELECT COALESCE(verification_domain,''), status FROM nodes WHERE id = ?", nodeID).Scan(&vd, &status)
	if status != "unclaimed" || vd != "pending.example" {
		t.Fatalf("after approve: status=%s vd=%q", status, vd)
	}
}

// --- Community submission tags (docs/adr/021) ---

func TestSubmitPatchWithValidTagsPersists(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true}}
	_, memberToken := createTestUser(t, db, "tagsubmitter", "member")
	createTestTag(t, db, "music")
	createTestTag(t, db, "craft")

	r := authedRequest("POST", "/api/v1/submissions", map[string]interface{}{
		"name": "Tagged Venue",
		"tags": []string{"craft", "music"},
	}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}

	var nodeID string
	if err := db.QueryRow("SELECT id FROM nodes WHERE slug = 'tagged-venue'").Scan(&nodeID); err != nil {
		t.Fatalf("find node: %v", err)
	}
	rows, err := db.Query(
		`SELECT t.name FROM node_tags nt JOIN tags t ON nt.tag_id = t.id
		 WHERE nt.node_id = ? ORDER BY nt.position`, nodeID,
	)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		got = append(got, name)
	}
	if len(got) != 2 || got[0] != "craft" || got[1] != "music" {
		t.Fatalf("stored tags = %v, want [craft music] in submitted priority order", got)
	}
}

func TestSubmitPatchUnknownTagRejected(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true}}
	_, memberToken := createTestUser(t, db, "badtagsubmitter", "member")
	createTestTag(t, db, "music")

	r := authedRequest("POST", "/api/v1/submissions", map[string]interface{}{
		"name": "Bad Tag Venue",
		"tags": []string{"music", "not-a-real-tag"},
	}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("submit with unknown tag: got %d, want 400: %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.Bytes(), `"error":"unknown tag: not-a-real-tag"`) {
		t.Fatalf("unexpected error body: %s", w.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE slug = 'bad-tag-venue'").Scan(&count)
	if count != 0 {
		t.Fatalf("node was created despite rejected tag")
	}
}

func TestSubmitPatchWithoutTagsSucceeds(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true}}
	_, memberToken := createTestUser(t, db, "notagsubmitter", "member")

	// Tags omitted entirely.
	r := authedRequest("POST", "/api/v1/submissions", map[string]interface{}{"name": "No Tags Venue"}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit without tags: got %d %s", w.Code, w.Body.String())
	}

	// Tags present but empty.
	r = authedRequest("POST", "/api/v1/submissions", map[string]interface{}{"name": "Empty Tags Venue", "tags": []string{}}, memberToken)
	w = serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit with empty tags: got %d %s", w.Code, w.Body.String())
	}
}

func TestListSubmissionsIncludesTags(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true}}
	_, memberToken := createTestUser(t, db, "queuetagsubmitter", "member")
	_, adminToken := createTestUser(t, db, "queuetagadmin", "admin")
	createTestTag(t, db, "music")

	r := authedRequest("POST", "/api/v1/submissions", map[string]interface{}{
		"name": "Queue Tag Venue",
		"tags": []string{"music"},
	}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}

	r = authedRequest("GET", "/api/v1/admin/submissions", nil, adminToken)
	w = serveAdminMux(t, db, "GET", "/api/v1/admin/submissions", handler.ListSubmissions(db), r)
	if !bodyContains(w.Body.Bytes(), `"tags":["music"]`) {
		t.Fatalf("submitted tags missing from review queue: %s", w.Body.String())
	}
}

// --- Reviewer tag correction at approval time (issue #5) ---

// storedTagNames reads a node's tags in stored priority order.
func storedTagNames(t *testing.T, db *database.DB, nodeID string) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT t.name FROM node_tags nt JOIN tags t ON nt.tag_id = t.id
		 WHERE nt.node_id = ? ORDER BY nt.position`, nodeID,
	)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		got = append(got, name)
	}
	return got
}

// submitPendingWithTags creates a pending_review submission carrying tags and
// returns its node ID.
func submitPendingWithTags(t *testing.T, db *database.DB, token string, name string, tags []string) string {
	t.Helper()
	cfg := &config.Config{Submissions: config.Submissions{Enabled: true}}
	body := map[string]interface{}{"name": name}
	if tags != nil {
		body["tags"] = tags
	}
	r := authedRequest("POST", "/api/v1/submissions", body, token)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &sub)
	return sub["id"].(string)
}

func TestReviewSubmissionTagOverridePersists(t *testing.T) {
	db := setupTestDB(t)
	_, memberToken := createTestUser(t, db, "overridesubmitter", "member")
	_, adminToken := createTestUser(t, db, "overrideadmin", "admin")
	createTestTag(t, db, "music")
	createTestTag(t, db, "craft")
	createTestTag(t, db, "theater")

	nodeID := submitPendingWithTags(t, db, memberToken, "Override Venue", []string{"music"})

	// The reviewer corrects the tags on approval — the override wins, in the
	// reviewer's priority order.
	r := authedRequest("PATCH", "/api/v1/admin/submissions/"+nodeID, map[string]interface{}{
		"action": "approve",
		"tags":   []string{"theater", "craft"},
	}, adminToken)
	w := serveAdminMux(t, db, "PATCH", "/api/v1/admin/submissions/{id}", handler.ReviewSubmission(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}

	var status string
	db.QueryRow("SELECT status FROM nodes WHERE id = ?", nodeID).Scan(&status)
	if status != "unclaimed" {
		t.Fatalf("after approve: status=%s, want unclaimed", status)
	}
	got := storedTagNames(t, db, nodeID)
	if len(got) != 2 || got[0] != "theater" || got[1] != "craft" {
		t.Fatalf("stored tags = %v, want [theater craft] in reviewer's order", got)
	}
}

func TestReviewSubmissionUnknownTagRejected(t *testing.T) {
	db := setupTestDB(t)
	_, memberToken := createTestUser(t, db, "unknowntagsubmitter", "member")
	_, adminToken := createTestUser(t, db, "unknowntagadmin", "admin")
	createTestTag(t, db, "music")

	nodeID := submitPendingWithTags(t, db, memberToken, "Unknown Tag Venue", []string{"music"})

	r := authedRequest("PATCH", "/api/v1/admin/submissions/"+nodeID, map[string]interface{}{
		"action": "approve",
		"tags":   []string{"music", "not-a-real-tag"},
	}, adminToken)
	w := serveAdminMux(t, db, "PATCH", "/api/v1/admin/submissions/{id}", handler.ReviewSubmission(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("approve with unknown tag: got %d, want 400: %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.Bytes(), `"error":"unknown tag: not-a-real-tag"`) {
		t.Fatalf("unexpected error body: %s", w.Body.String())
	}

	// The bad override must not half-approve: still pending, submitted tags intact.
	var status string
	db.QueryRow("SELECT status FROM nodes WHERE id = ?", nodeID).Scan(&status)
	if status != "pending_review" {
		t.Fatalf("after rejected override: status=%s, want pending_review", status)
	}
	got := storedTagNames(t, db, nodeID)
	if len(got) != 1 || got[0] != "music" {
		t.Fatalf("submitted tags disturbed by rejected override: %v", got)
	}
}

func TestReviewSubmissionWithoutTagsKeepsSubmitted(t *testing.T) {
	db := setupTestDB(t)
	_, memberToken := createTestUser(t, db, "keeptagsubmitter", "member")
	_, adminToken := createTestUser(t, db, "keeptagadmin", "admin")
	createTestTag(t, db, "music")
	createTestTag(t, db, "craft")

	nodeID := submitPendingWithTags(t, db, memberToken, "Keep Tags Venue", []string{"craft", "music"})

	// No tags field at all — the submitter's picks survive approval.
	r := authedRequest("PATCH", "/api/v1/admin/submissions/"+nodeID, map[string]interface{}{"action": "approve"}, adminToken)
	w := serveAdminMux(t, db, "PATCH", "/api/v1/admin/submissions/{id}", handler.ReviewSubmission(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	got := storedTagNames(t, db, nodeID)
	if len(got) != 2 || got[0] != "craft" || got[1] != "music" {
		t.Fatalf("stored tags = %v, want submitted [craft music] kept", got)
	}
}

func TestReviewSubmissionEmptyTagsClears(t *testing.T) {
	db := setupTestDB(t)
	_, memberToken := createTestUser(t, db, "cleartagsubmitter", "member")
	_, adminToken := createTestUser(t, db, "cleartagadmin", "admin")
	createTestTag(t, db, "music")

	nodeID := submitPendingWithTags(t, db, memberToken, "Clear Tags Venue", []string{"music"})

	// An explicit empty array is a correction too: the reviewer removed them all.
	r := authedRequest("PATCH", "/api/v1/admin/submissions/"+nodeID, map[string]interface{}{
		"action": "approve",
		"tags":   []string{},
	}, adminToken)
	w := serveAdminMux(t, db, "PATCH", "/api/v1/admin/submissions/{id}", handler.ReviewSubmission(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	if got := storedTagNames(t, db, nodeID); len(got) != 0 {
		t.Fatalf("stored tags = %v, want none after empty override", got)
	}
}

// docs/adr/062: the fourth claim method proves a binding, not possession of
// a token we issued, and records the DID it proved.
func TestClaimDIDVerify(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "did-owner", "member")
	alice, aliceToken := createTestUser(t, db, "did-alice", "member")
	_ = alice

	nodeID := createTestNode(t, db, owner.ID, "Tellus", "tellus", "open")
	makeClaimable(t, db, nodeID, "tellus.example")

	resp, code := openClaim(t, db, cfg, aliceToken, "tellus", map[string]interface{}{"method": "did"})
	if code != http.StatusCreated {
		t.Fatalf("open did claim: got %d — %v", code, resp)
	}
	claimID := resp["id"].(string)

	origTXT := handler.ClaimLookupTXT
	origClient := handler.ClaimHTTPClient
	t.Cleanup(func() {
		handler.ClaimLookupTXT = origTXT
		handler.ClaimHTTPClient = origClient
	})

	verify := func() map[string]interface{} {
		t.Helper()
		r := authedRequest("POST", "/api/v1/claims/"+claimID+"/verify", nil, aliceToken)
		w := serveMux(t, db, "POST", "/api/v1/claims/{id}/verify", handler.VerifyClaim(db), r)
		var vr map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &vr)
		return vr
	}

	// A did:plc handle is refused, and the message says why rather than
	// sending someone hunting a DNS typo that isn't there.
	handler.ClaimLookupTXT = func(domain string) ([]string, error) {
		return []string{"did=did:plc:abc123"}, nil
	}
	vr := verify()
	if vr["verified"] != false {
		t.Fatal("did:plc must not verify")
	}
	if msg, _ := vr["error"].(string); !strings.Contains(msg, "did:web") {
		t.Errorf("error should name did:web, got: %q", msg)
	}
	if got := nodeDID(t, db, nodeID); got != "" {
		t.Errorf("a refused claim stored a DID: %q", got)
	}

	// The real thing: handle names the DID, document names the handle back.
	handler.ClaimLookupTXT = func(domain string) ([]string, error) {
		if domain != "_atproto.tellus.example" {
			t.Fatalf("lookup on %q, want the _atproto subdomain", domain)
		}
		return []string{"did=did:web:tellus.example"}, nil
	}
	handler.ClaimHTTPClient = stubDIDClient(map[string]string{
		"https://tellus.example/.well-known/did.json": `{"id":"did:web:tellus.example","alsoKnownAs":["at://tellus.example"]}`,
	})
	vr = verify()
	if vr["verified"] != true {
		t.Fatalf("did verification failed: %v", vr)
	}
	if got := nodeDID(t, db, nodeID); got != "did:web:tellus.example" {
		t.Errorf("nodes.did = %q, want the verified DID", got)
	}
}

func nodeDID(t *testing.T, db *database.DB, nodeID string) string {
	t.Helper()
	var did sql.NullString
	if err := db.QueryRow(`SELECT did FROM nodes WHERE id = ?`, nodeID).Scan(&did); err != nil {
		t.Fatalf("read did: %v", err)
	}
	return did.String
}

// stubDIDClient serves canned documents and 404s everything else, so a test
// never reaches the network.
func stubDIDClient(pages map[string]string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, ok := pages[r.URL.String()]
		if !ok {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The claim address goes through the same grammar as sign-in
// (internal/auth/email.go). Before that, the only thing standing between a
// malformed address and the mail sender was the domain comparison — which
// rejected most junk, but only because of where the last '@' landed.
// "bob..smith@" has its '@' in exactly the right place and is not an address.
func TestClaimEmailRejectsMalformedAddress(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Strict Venue", "strict-venue", "open")
	makeClaimable(t, db, nodeID, "strictvenue.example")

	sent := 0
	origSend := handler.ClaimSendMail
	t.Cleanup(func() { handler.ClaimSendMail = origSend })
	handler.ClaimSendMail = func(smtp config.SMTP, to []string, msg []byte) error {
		sent++
		return nil
	}

	for _, bad := range []string{
		"booking..desk@strictvenue.example",
		".booking@strictvenue.example",
		"booking.@strictvenue.example",
		"Booking Desk <booking@strictvenue.example>",
		`"booking@evil.example"@strictvenue.example`,
		"booking@strictvenue.example (Booking)",
		"@strictvenue.example",
		"strictvenue.example",
		"",
	} {
		_, code := openClaim(t, db, cfg, aliceToken, "strict-venue",
			map[string]interface{}{"method": "email", "email": bad})
		if code != http.StatusBadRequest {
			t.Errorf("email %q: got %d, want 400", bad, code)
		}
	}

	if sent != 0 {
		t.Errorf("%d verification mails sent for malformed addresses; want 0", sent)
	}

	// And nothing was recorded — a refused claim must not occupy the
	// one-open-claim-per-user slot.
	var open int
	db.QueryRow("SELECT COUNT(*) FROM claim_requests WHERE node_id = ?", nodeID).Scan(&open)
	if open != 0 {
		t.Errorf("%d claim rows created for malformed addresses; want 0", open)
	}
}

// Normalization still happens, and still happens before the domain anchor is
// compared: a correct address typed with stray case and whitespace is a
// correct address.
func TestClaimEmailNormalizesBeforeDomainCheck(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")

	nodeID := createTestNode(t, db, owner.ID, "Case Venue", "case-venue", "open")
	makeClaimable(t, db, nodeID, "casevenue.example")

	var sentTo string
	origSend := handler.ClaimSendMail
	t.Cleanup(func() { handler.ClaimSendMail = origSend })
	handler.ClaimSendMail = func(smtp config.SMTP, to []string, msg []byte) error {
		sentTo = to[0]
		return nil
	}

	if _, code := openClaim(t, db, cfg, aliceToken, "case-venue",
		map[string]interface{}{"method": "email", "email": "  Booking@CaseVenue.EXAMPLE  "}); code != http.StatusCreated {
		t.Fatalf("mixed-case address at the anchor domain: got %d, want 201", code)
	}
	if sentTo != "booking@casevenue.example" {
		t.Errorf("mail sent to %q, want %q", sentTo, "booking@casevenue.example")
	}

	var stored string
	db.QueryRow("SELECT email FROM claim_requests WHERE node_id = ?", nodeID).Scan(&stored)
	if stored != "booking@casevenue.example" {
		t.Errorf("claim_requests.email = %q, want %q", stored, "booking@casevenue.example")
	}
}
