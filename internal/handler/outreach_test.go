package handler_test

// Intro emails to the owners of unclaimed patches
// (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md): who may
// enter the address, who may send, what the preview renders, that a send is
// recorded once and a resend is explicit, and the claim shortcut as a full
// round trip through a captured relay.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func outreachCfg(smtp bool) *config.Config {
	cfg := &config.Config{}
	cfg.Instance.Name = "Test Quilt"
	cfg.Instance.Domain = "quilt.test"
	if smtp {
		cfg.SMTP = config.SMTP{Host: "smtp.test", From: "quilt@quilt.test"}
	}
	return cfg
}

// introFixture is one unclaimed patch with a verified domain, an instance
// admin, a trusted contributor whose grant reaches the patch, and a
// bystander.
type introFixture struct {
	db                                        *database.DB
	nodeID                                    string
	adminTok, trustedTok, memberTok, ownerTok string
	ownerID                                   string
}

func setupIntro(t *testing.T) *introFixture {
	t.Helper()
	db := setupTestDB(t)
	admin, adminTok := createTestUser(t, db, "admin", "admin")
	trusted, trustedTok := createTestUser(t, db, "trusted", "member")
	_, memberTok := createTestUser(t, db, "member", "member")
	owner, ownerTok := createTestUser(t, db, "owner", "member")
	nodeID := createTestNode(t, db, admin.ID, "The Venue", "the-venue", "open")
	makeClaimable(t, db, nodeID, "venue.test")
	if _, err := db.Exec(`UPDATE nodes SET website = '' WHERE id = ?`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO node_trusted_contributors (node_id, user_id, granted_by, granted_at, source) VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 'admin')`, nodeID, trusted.ID, admin.ID); err != nil {
		t.Fatalf("grant per-patch trust: %v", err)
	}
	return &introFixture{db: db, nodeID: nodeID, adminTok: adminTok, trustedTok: trustedTok, memberTok: memberTok, ownerTok: ownerTok, ownerID: owner.ID}
}

func (f *introFixture) setContact(t *testing.T, tok, email string) int {
	t.Helper()
	r := authedRequest("PUT", "/api/v1/nodes/the-venue/contact", map[string]any{"email": email}, tok)
	w := serveMux(t, f.db, "PUT", "/api/v1/nodes/{slug}/contact", handler.SetNodeContact(f.db), r)
	return w.Code
}

func (f *introFixture) preview(t *testing.T, cfg *config.Config, body map[string]any) (map[string]any, int) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/the-venue/intro/preview", body, f.adminTok)
	w := serveMux(t, f.db, "POST", "/api/v1/nodes/{slug}/intro/preview", middleware.AdminRequired(f.db, handler.IntroPreview(f.db, cfg)), r)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp, w.Code
}

func (f *introFixture) send(t *testing.T, cfg *config.Config, tok string, body map[string]any) (map[string]any, int) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/the-venue/intro/send", body, tok)
	w := serveMux(t, f.db, "POST", "/api/v1/nodes/{slug}/intro/send", middleware.AdminRequired(f.db, handler.IntroSend(f.db, cfg)), r)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp, w.Code
}

func TestIntro_WhoMayEnterTheAddress(t *testing.T) {
	f := setupIntro(t)

	if code := f.setContact(t, f.memberTok, "info@venue.test"); code != 403 {
		t.Fatalf("a bystander entered the address: %d", code)
	}
	if code := f.setContact(t, f.trustedTok, "Info@Venue.Test"); code != 200 {
		t.Fatalf("a trusted contributor whose grant reaches the patch could not enter it: %d", code)
	}
	var stored string
	_ = f.db.QueryRow(`SELECT contact_email FROM nodes WHERE id = ?`, f.nodeID).Scan(&stored)
	if stored != "info@venue.test" {
		t.Fatalf("address not normalized: %q", stored)
	}
	if code := f.setContact(t, f.adminTok, "not an address"); code != 400 {
		t.Fatalf("a malformed address was accepted: %d", code)
	}
	if code := f.setContact(t, f.adminTok, ""); code != 200 {
		t.Fatalf("clearing failed: %d", code)
	}
	_ = f.db.QueryRow(`SELECT contact_email FROM nodes WHERE id = ?`, f.nodeID).Scan(&stored)
	if stored != "" {
		t.Fatalf("address not cleared: %q", stored)
	}
}

func TestIntro_PreviewRendersTheWholeMessage(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(false)
	f.setContact(t, f.adminTok, "info@venue.test")

	resp, code := f.preview(t, cfg, nil)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}
	// No website on the patch: nothing to read, and the probe says so.
	if resp["situation"] != "none" {
		t.Fatalf("situation: %v", resp["situation"])
	}
	body, _ := resp["body"].(string)
	for _, want := range []string{"The Venue", "https://quilt.test/patches/the-venue", "https://quilt.test/patches/the-venue/claim", "Test Quilt", "admin"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "{") {
		t.Errorf("an unfilled placeholder survived:\n%s", body)
	}
	if resp["claim_shortcut"] != true {
		t.Errorf("an address at the verification domain should offer the shortcut: %v", resp["claim_shortcut"])
	}
	if resp["smtp"] != false {
		t.Errorf("smtp should read as unavailable")
	}

	// The admin may overrule the probe.
	resp, _ = f.preview(t, cfg, map[string]any{"situation": "wordpress"})
	if resp["situation"] != "wordpress" {
		t.Fatalf("override ignored: %v", resp["situation"])
	}
	if body, _ := resp["body"].(string); !strings.Contains(body, "Events Calendar plugin") {
		t.Errorf("wordpress paragraph not rendered")
	}
	if _, code := f.preview(t, cfg, map[string]any{"situation": "attachable"}); code != 400 {
		t.Errorf("attachable is advice, not a paragraph: %d", code)
	}
}

func TestIntro_SendIsRecordedOnceAndResendIsExplicit(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(false)

	msg := map[string]any{"subject": "Hello", "body": "Hi there.", "situation": "none", "delivery": "copied"}

	// Nobody but the instance admin sends.
	if _, code := f.send(t, cfg, f.trustedTok, msg); code != 403 {
		t.Fatalf("a trusted contributor sent: %d", code)
	}
	// Nothing to send to yet.
	if _, code := f.send(t, cfg, f.adminTok, msg); code != 400 {
		t.Fatalf("send with no address: %d", code)
	}
	f.setContact(t, f.adminTok, "hello@gmail.test")

	// Without a relay, the relay path is refused and the copied path works.
	smtpMsg := map[string]any{"subject": "Hello", "body": "Hi there.", "situation": "none", "delivery": "smtp"}
	if _, code := f.send(t, cfg, f.adminTok, smtpMsg); code != 400 {
		t.Fatalf("smtp delivery without a relay: %d", code)
	}
	resp, code := f.send(t, cfg, f.adminTok, msg)
	if code != 201 {
		t.Fatalf("copied send: %d %v", code, resp)
	}
	if resp["claim_link"] != false {
		t.Errorf("an address off the verification domain must not get a claim token")
	}

	// Once per address. Twice is a decision.
	if _, code := f.send(t, cfg, f.adminTok, msg); code != 409 {
		t.Fatalf("a second send was not refused: %d", code)
	}
	again := map[string]any{"subject": "Hello", "body": "Hi again.", "situation": "none", "delivery": "copied", "resend": true}
	if _, code := f.send(t, cfg, f.adminTok, again); code != 201 {
		t.Fatalf("explicit resend refused: %d", code)
	}
	var n int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM intro_sends WHERE node_id = ?`, f.nodeID).Scan(&n)
	if n != 2 {
		t.Fatalf("sends recorded: %d", n)
	}

	// Switched off, nothing sends.
	_, _ = f.db.Exec(`INSERT INTO instance_settings (key, value) VALUES ('outreach_enabled', 'false')`)
	if _, code := f.send(t, cfg, f.adminTok, again); code != 403 {
		t.Fatalf("send while disabled: %d", code)
	}
}

func TestIntro_ClaimShortcutRoundTrip(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(true)
	f.setContact(t, f.adminTok, "booking@venue.test")

	var sent []byte
	prev := handler.IntroSendMail
	handler.IntroSendMail = func(_ config.SMTP, to []string, msg []byte) error {
		if len(to) != 1 || to[0] != "booking@venue.test" {
			t.Errorf("sent to %v", to)
		}
		sent = msg
		return nil
	}
	t.Cleanup(func() { handler.IntroSendMail = prev })

	resp, code := f.preview(t, cfg, nil)
	if code != 200 {
		t.Fatalf("preview: %d", code)
	}
	resp, code = f.send(t, cfg, f.adminTok, map[string]any{
		"subject": resp["subject"], "body": resp["body"], "situation": resp["situation"], "delivery": "smtp",
	})
	if code != 201 {
		t.Fatalf("send: %d %v", code, resp)
	}
	if resp["claim_link"] != true {
		t.Fatalf("an address at the verification domain should carry the shortcut")
	}
	m := regexp.MustCompile(`/patches/the-venue/claim\?intro=([0-9a-f]+)`).FindStringSubmatch(string(sent))
	if m == nil {
		t.Fatalf("no tokened claim link in the message:\n%s", sent)
	}
	if !strings.Contains(string(sent), "Content-Type: text/plain") {
		t.Errorf("an intro is plain text")
	}
	token := m[1]

	// Whoever follows the link, signed in as themselves, holds an approved
	// claim at once: the token was the email leg.
	claim, code := openClaim(t, f.db, cfg, f.ownerTok, "the-venue", map[string]any{"intro_token": token})
	if code != 201 || claim["status"] != "approved" {
		t.Fatalf("claim by intro token: %d %v", code, claim)
	}
	var status, email string
	_ = f.db.QueryRow(`SELECT status, email FROM claim_requests WHERE node_id = ? AND user_id = ?`, f.nodeID, f.ownerID).Scan(&status, &email)
	if status != "approved" || email != "booking@venue.test" {
		t.Fatalf("claim row: %s %s", status, email)
	}
	// The patch is still unclaimed until setup (docs/adr/039): approval is
	// a right to enter setup, and the token changed nothing about that.
	if st, _ := nodeState(t, f.db, f.nodeID); st != "unclaimed" {
		t.Fatalf("node went %s on approval", st)
	}

	// Spent before the claim was approved, so it can approve at most one.
	var usedAt string
	_ = f.db.QueryRow(`SELECT COALESCE(claim_token_used_at,'') FROM intro_sends WHERE claim_token = ?`, token).Scan(&usedAt)
	if usedAt == "" {
		t.Fatal("token not marked used")
	}
	// Even once the approved claim is out of the way (withdrawn, so the
	// patch is claimable again), the same link opens nothing for the next
	// person who follows it.
	_, _ = f.db.Exec(`UPDATE claim_requests SET status = 'withdrawn' WHERE node_id = ?`, f.nodeID)
	_, memberTok := createTestUser(t, f.db, "second", "member")
	if _, code := openClaim(t, f.db, cfg, memberTok, "the-venue", map[string]any{"intro_token": token}); code != 400 {
		t.Fatalf("a used token opened a claim: %d", code)
	}
}

func TestIntro_ChangingTheAddressRevokesTheToken(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(true)
	f.setContact(t, f.adminTok, "booking@venue.test")
	prev := handler.IntroSendMail
	handler.IntroSendMail = func(_ config.SMTP, _ []string, _ []byte) error { return nil }
	t.Cleanup(func() { handler.IntroSendMail = prev })

	resp, _ := f.preview(t, cfg, nil)
	f.send(t, cfg, f.adminTok, map[string]any{"subject": resp["subject"], "body": resp["body"], "situation": resp["situation"], "delivery": "smtp"})
	var token string
	_ = f.db.QueryRow(`SELECT claim_token FROM intro_sends WHERE node_id = ?`, f.nodeID).Scan(&token)
	if token == "" {
		t.Fatal("no token minted")
	}
	f.setContact(t, f.adminTok, "someone-else@venue.test")
	if _, code := openClaim(t, f.db, cfg, f.ownerTok, "the-venue", map[string]any{"intro_token": token}); code != 400 {
		t.Fatalf("a token for a replaced address still worked: %d", code)
	}
}

func TestIntro_AdminBacklogAndTemplate(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(false)
	f.setContact(t, f.adminTok, "hello@gmail.test")
	f.send(t, cfg, f.adminTok, map[string]any{"subject": "s", "body": "b", "situation": "none", "delivery": "copied"})

	r := authedRequest("GET", "/api/v1/admin/outreach", nil, f.adminTok)
	w := serveMux(t, f.db, "GET", "/api/v1/admin/outreach", middleware.AdminRequired(f.db, handler.AdminOutreach(f.db, cfg)), r)
	if w.Code != 200 {
		t.Fatalf("list: %d", w.Code)
	}
	var list map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	patches, _ := list["patches"].([]any)
	if len(patches) != 1 {
		t.Fatalf("patches: %v", list["patches"])
	}
	p := patches[0].(map[string]any)
	if p["slug"] != "the-venue" || p["sends"].(float64) != 1 || p["contact_email"] != "hello@gmail.test" {
		t.Fatalf("row: %v", p)
	}
	tpl := list["template"].(map[string]any)
	if tpl["customized"] != false {
		t.Fatalf("shipped template should read as default")
	}

	// Replace one part; the rest stay shipped.
	r = authedRequest("PATCH", "/api/v1/admin/outreach", map[string]any{
		"template": map[string]any{"opener": "Hello from {quilt_name}."},
		"enabled":  false,
	}, f.adminTok)
	w = serveMux(t, f.db, "PATCH", "/api/v1/admin/outreach", middleware.AdminRequired(f.db, handler.AdminUpdateOutreach(f.db)), r)
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	resp, _ := f.preview(t, cfg, nil)
	body, _ := resp["body"].(string)
	if !strings.HasPrefix(body, "Hello from Test Quilt.") {
		t.Errorf("custom opener not used:\n%s", body)
	}
	if !strings.Contains(body, "Thanks,") {
		t.Errorf("a part the override left empty should fall back to the shipped text")
	}
	if resp["enabled"] != false {
		t.Errorf("switch not honoured in preview: %v", resp["enabled"])
	}

	r = authedRequest("DELETE", "/api/v1/admin/outreach/template", nil, f.adminTok)
	w = serveMux(t, f.db, "DELETE", "/api/v1/admin/outreach/template", middleware.AdminRequired(f.db, handler.AdminResetOutreachTemplate(f.db)), r)
	if w.Code != 200 {
		t.Fatalf("reset: %d", w.Code)
	}
	resp, _ = f.preview(t, cfg, nil)
	if body, _ := resp["body"].(string); strings.HasPrefix(body, "Hello from") {
		t.Errorf("reset did not restore the shipped opener")
	}
}
