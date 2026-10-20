package handler_test

// A contact is a place to reach someone, not only an address
// (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md, amended
// 2026-10-19): a link to message them on, usually Instagram, sits beside
// the email. A dm is rendered short, copied out, recorded against the
// link, and never carries the claim token.

import (
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func (f *introFixture) setContactURL(t *testing.T, tok, u string) (map[string]any, int) {
	t.Helper()
	r := authedRequest("PUT", "/api/v1/nodes/the-venue/contact", map[string]any{"url": u}, tok)
	w := serveMux(t, f.db, "PUT", "/api/v1/nodes/{slug}/contact", handler.SetNodeContact(f.db), r)
	return decodeJSON(t, w), w.Code
}

func TestIntroDM_ALinkIsAPlaceToReachThem(t *testing.T) {
	f := setupIntro(t)

	// A bare handle is refused rather than guessed into a network.
	if _, code := f.setContactURL(t, f.adminTok, "@thevenue"); code != 400 {
		t.Fatalf("a bare handle was accepted: %d", code)
	}
	resp, code := f.setContactURL(t, f.trustedTok, "https://instagram.com/thevenue")
	if code != 200 || resp["contact_url"] != "https://instagram.com/thevenue" {
		t.Fatalf("set url: %d %v", code, resp)
	}
	// Saving the link alone leaves the address alone, and the other way
	// round, so the two fields save separately.
	f.setContact(t, f.adminTok, "info@venue.test")
	var email, u string
	_ = f.db.QueryRow(`SELECT contact_email, contact_url FROM nodes WHERE id = ?`, f.nodeID).Scan(&email, &u)
	if email != "info@venue.test" || u != "https://instagram.com/thevenue" {
		t.Fatalf("fields did not save independently: %q %q", email, u)
	}
}

func TestIntroDM_PreviewAndSend(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(true)
	prev := handler.IntroSendMail
	handler.IntroSendMail = func(_ config.SMTP, _ []string, _ []byte) error { t.Fatal("a dm must never reach the relay"); return nil }
	t.Cleanup(func() { handler.IntroSendMail = prev })

	f.setContactURL(t, f.adminTok, "https://instagram.com/thevenue")

	// Only a link on file: the preview opens on the dm.
	resp, code := f.preview(t, cfg, nil)
	if code != 200 || resp["channel"] != "dm" {
		t.Fatalf("preview: %d %v", code, resp)
	}
	body, _ := resp["body"].(string)
	if resp["subject"] != "" {
		t.Errorf("a dm has no subject: %q", resp["subject"])
	}
	if !strings.HasPrefix(body, "Hi, I'm admin") || !strings.Contains(body, "https://quilt.test/patches/the-venue/claim") {
		t.Errorf("dm opener not rendered:\n%s", body)
	}
	if strings.Contains(body, "Thanks,") {
		t.Errorf("a dm has no sign-off:\n%s", body)
	}
	if resp["claim_shortcut"] != false {
		t.Errorf("a dm never offers the claim shortcut")
	}
	if resp["destination"] != "https://instagram.com/thevenue" {
		t.Errorf("destination: %v", resp["destination"])
	}

	// A dm is copied out, never relayed.
	if _, code := f.send(t, cfg, f.adminTok, map[string]any{"channel": "dm", "body": body, "situation": "none", "delivery": "smtp"}); code != 400 {
		t.Fatalf("a dm went to the relay: %d", code)
	}
	sent, code := f.send(t, cfg, f.adminTok, map[string]any{"channel": "dm", "body": body, "situation": "none", "delivery": "copied"})
	if code != 201 || sent["channel"] != "dm" || sent["claim_link"] != false {
		t.Fatalf("dm send: %d %v", code, sent)
	}
	var ch, dest, tok string
	_ = f.db.QueryRow(`SELECT channel, email, COALESCE(claim_token,'') FROM intro_sends WHERE node_id = ?`, f.nodeID).Scan(&ch, &dest, &tok)
	if ch != "dm" || dest != "https://instagram.com/thevenue" || tok != "" {
		t.Fatalf("row: %s %s token=%q", ch, dest, tok)
	}
	// Once per destination still holds.
	if _, code := f.send(t, cfg, f.adminTok, map[string]any{"channel": "dm", "body": body, "situation": "none", "delivery": "copied"}); code != 409 {
		t.Fatalf("second dm not refused: %d", code)
	}
}

func TestIntroDM_EmailIsTheDefaultWhenBothAreOnFile(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(false)
	f.setContactURL(t, f.adminTok, "https://instagram.com/thevenue")
	f.setContact(t, f.adminTok, "info@venue.test")

	resp, _ := f.preview(t, cfg, nil)
	if resp["channel"] != "email" {
		t.Fatalf("default channel: %v", resp["channel"])
	}
	if resp["claim_shortcut"] != true {
		t.Errorf("email to the verification domain should offer the shortcut")
	}
	// The admin can ask for the other one.
	resp, _ = f.preview(t, cfg, map[string]any{"channel": "dm"})
	if resp["channel"] != "dm" || resp["claim_shortcut"] != false {
		t.Fatalf("dm on request: %v", resp)
	}
	if _, code := f.preview(t, cfg, map[string]any{"channel": "carrier-pigeon"}); code != 400 {
		t.Errorf("unknown channel accepted: %d", code)
	}

	// The state read names the default and offers the patch's links.
	r := authedRequest("GET", "/api/v1/nodes/the-venue/intro", nil, f.adminTok)
	w := serveMux(t, f.db, "GET", "/api/v1/nodes/{slug}/intro", middleware.AdminRequired(f.db, handler.IntroState(f.db, cfg)), r)
	st := decodeJSON(t, w)
	if st["default_channel"] != "email" || st["contact_url"] != "https://instagram.com/thevenue" {
		t.Fatalf("state: %v", st)
	}
}

func TestIntroDM_TemplateHasItsOwnOpener(t *testing.T) {
	f := setupIntro(t)
	cfg := outreachCfg(false)
	f.setContactURL(t, f.adminTok, "https://instagram.com/thevenue")

	r := authedRequest("PATCH", "/api/v1/admin/outreach", map[string]any{
		"template": map[string]any{"dm_opener": "Yo from {quilt_name}."},
	}, f.adminTok)
	w := serveMux(t, f.db, "PATCH", "/api/v1/admin/outreach", middleware.AdminRequired(f.db, handler.AdminUpdateOutreach(f.db)), r)
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	resp, _ := f.preview(t, cfg, nil)
	body, _ := resp["body"].(string)
	if !strings.HasPrefix(body, "Yo from Test Quilt.") {
		t.Errorf("custom dm opener not used:\n%s", body)
	}
	// The email opener was not touched by a dm-only override.
	f.setContact(t, f.adminTok, "info@venue.test")
	resp, _ = f.preview(t, cfg, map[string]any{"channel": "email"})
	if body, _ := resp["body"].(string); !strings.HasPrefix(body, "Hi,") {
		t.Errorf("email opener should still be the shipped one:\n%s", body)
	}
}
