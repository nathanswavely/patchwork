package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Personal export (docs/adr/012, affordance 1): everything about the person
// asking and nothing about anybody else.

func exportCfg() *config.Config {
	return &config.Config{Instance: config.Instance{Domain: "quilt.example"}}
}

// requestExport runs GET /api/v1/users/me/export through the auth middleware.
func requestExport(t *testing.T, db *database.DB, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("GET", "/api/v1/users/me/export", nil, token)
	return serveMux(t, db, "GET", "/api/v1/users/me/export",
		handler.PersonalExport(db, exportCfg()), r)
}

func exportSection(t *testing.T, doc map[string]interface{}, key string) []interface{} {
	t.Helper()
	items, ok := doc[key].([]interface{})
	if !ok {
		t.Fatalf("export has no %q array: %T", key, doc[key])
	}
	return items
}

func TestPersonalExport_RequiresAuth(t *testing.T) {
	db := setupTestDB(t)

	w := requestExport(t, db, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated export: got %d, want 401", w.Code)
	}
}

func TestPersonalExport_CarriesHiddenMembershipsAndOwnContent(t *testing.T) {
	db := setupTestDB(t)
	weaver, token := createTestUser(t, db, "weaver", "member")
	_, err := db.Exec(
		`UPDATE users SET email = ?, bio = ?, contact_phone = ? WHERE id = ?`,
		"weaver@example.com", "Sews at night", "+1 717 555 0100", weaver.ID)
	if err != nil {
		t.Fatal(err)
	}

	shown := createTestNode(t, db, weaver.ID, "Gallery Row", "gallery-row", "open")
	hidden := createTestNode(t, db, weaver.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, weaver.ID, shown, "admin", "active")
	hiddenMembership := createTestMembership(t, db, weaver.ID, hidden, "member", "active")
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE id = ?`, hiddenMembership); err != nil {
		t.Fatal(err)
	}

	proposalID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state)
		 VALUES (?, ?, ?, 'Buy a PA', 'The old one hums.', 'open', 'voting')`,
		proposalID, shown, weaver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
		auth.NewUUIDv7(), proposalID, weaver.ID); err != nil {
		t.Fatal(err)
	}
	noticeID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO notices (id, node_id, author_id, title, body) VALUES (?, ?, ?, 'PA is broken', 'Again.')`,
		noticeID, shown, weaver.ID); err != nil {
		t.Fatal(err)
	}

	w := requestExport(t, db, token)
	if w.Code != http.StatusOK {
		t.Fatalf("export: got %d: %s", w.Code, w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	disp := w.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disp, "attachment; filename=") || !strings.Contains(disp, "patchwork-weaver-") {
		t.Errorf("Content-Disposition = %q", disp)
	}
	if !strings.HasSuffix(strings.TrimSuffix(disp, `"`), ".json") {
		t.Errorf("export is not named as JSON: %q", disp)
	}

	doc := decodeJSON(t, w)
	if doc["exported_at"] == nil || doc["exported_at"] == "" {
		t.Error("export has no exported_at")
	}
	instance, _ := doc["instance"].(map[string]interface{})
	if instance == nil || instance["domain"] != "quilt.example" {
		t.Errorf("instance block = %v", doc["instance"])
	}

	profile, _ := doc["user"].(map[string]interface{})
	if profile == nil {
		t.Fatal("export has no user block")
	}
	if profile["email"] != "weaver@example.com" || profile["bio"] != "Sews at night" {
		t.Errorf("profile = %v", profile)
	}
	card, _ := profile["contact_card"].(map[string]interface{})
	if card == nil || card["phone"] != "+1 717 555 0100" {
		t.Errorf("contact card = %v", profile["contact_card"])
	}

	// The hidden membership is the person's own, so it travels — ADR 012
	// says the visibility flags are theirs.
	memberships := exportSection(t, doc, "memberships")
	if len(memberships) != 2 {
		t.Fatalf("expected 2 memberships, got %d", len(memberships))
	}
	var sawHidden bool
	for _, m := range memberships {
		row := m.(map[string]interface{})
		if row["node_slug"] == "the-selvage" {
			sawHidden = true
			if visible, _ := row["visible"].(float64); visible != 0 {
				t.Errorf("hidden membership exported as visible: %v", row["visible"])
			}
		}
	}
	if !sawHidden {
		t.Error("the hidden membership is missing from the export")
	}

	if n := len(exportSection(t, doc, "proposals_authored")); n != 1 {
		t.Errorf("proposals_authored = %d, want 1", n)
	}
	if n := len(exportSection(t, doc, "votes")); n != 1 {
		t.Errorf("votes = %d, want 1", n)
	}
	if n := len(exportSection(t, doc, "notices")); n != 1 {
		t.Errorf("notices = %d, want 1", n)
	}

	// A section with nothing in it is still an empty array, so a reader
	// never has to tell "no rows" from "key absent".
	for _, key := range []string{"remote_follows", "claims_filed", "notification_preferences"} {
		if _, ok := doc[key].([]interface{}); !ok {
			t.Errorf("%s is %T, want an (empty) array", key, doc[key])
		}
	}

	// Downloading your own record is a self-service action the audit log
	// carries, the way feed_secret.generate is.
	var audited int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'user.export' AND user_id = ?`,
		weaver.ID).Scan(&audited)
	if audited != 1 {
		t.Errorf("audit rows for user.export = %d, want 1", audited)
	}
}

func TestPersonalExport_ExcludesOtherPeoplesData(t *testing.T) {
	db := setupTestDB(t)
	weaver, token := createTestUser(t, db, "weaver", "member")
	stranger, _ := createTestUser(t, db, "stranger", "member")
	if _, err := db.Exec(`UPDATE users SET email = ? WHERE id = ?`,
		"stranger@example.com", stranger.ID); err != nil {
		t.Fatal(err)
	}

	node := createTestNode(t, db, weaver.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, weaver.ID, node, "admin", "active")
	strangerMembership := createTestMembership(t, db, stranger.ID, node, "member", "active")
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE id = ?`, strangerMembership); err != nil {
		t.Fatal(err)
	}

	// The stranger authors a proposal, votes, and posts a notice in the same
	// patch the exporter administers. None of it is the exporter's.
	proposalID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state)
		 VALUES (?, ?, ?, 'Stranger proposal', '', 'open', 'voting')`,
		proposalID, node, stranger.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'reject')`,
		auth.NewUUIDv7(), proposalID, stranger.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO notices (id, node_id, author_id, title, body) VALUES (?, ?, ?, 'Stranger notice', '')`,
		auth.NewUUIDv7(), node, stranger.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO notifications (id, user_id, type, title) VALUES (?, ?, 'proposal', 'For the stranger')`,
		auth.NewUUIDv7(), stranger.ID); err != nil {
		t.Fatal(err)
	}

	w := requestExport(t, db, token)
	if w.Code != http.StatusOK {
		t.Fatalf("export: got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	for _, needle := range []string{
		"stranger@example.com",
		"Stranger proposal",
		"Stranger notice",
		"For the stranger",
		strangerMembership,
	} {
		if strings.Contains(body, needle) {
			t.Errorf("export leaks another person's data: %q", needle)
		}
	}

	doc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if n := len(exportSection(t, doc, "memberships")); n != 1 {
		t.Errorf("memberships = %d, want only the exporter's 1", n)
	}
	if n := len(exportSection(t, doc, "proposals_authored")); n != 0 {
		t.Errorf("proposals_authored = %d, want 0", n)
	}
	if n := len(exportSection(t, doc, "votes")); n != 0 {
		t.Errorf("votes = %d, want 0", n)
	}
	if n := len(exportSection(t, doc, "notifications")); n != 0 {
		t.Errorf("notifications = %d, want 0", n)
	}
}

// Nothing in the file should help anybody get in. The person's own secrets
// are as absent as everybody else's.
func TestPersonalExport_CarriesNoCredentialMaterial(t *testing.T) {
	db := setupTestDB(t)
	weaver, token := createTestUser(t, db, "weaver", "member")

	if _, err := db.Exec(
		`UPDATE users SET private_key = 'PRIVATE-KEY-MATERIAL', public_key = 'PUBLIC-KEY-MATERIAL',
		 feed_secret_hash = 'FEED-SECRET-HASH', ap_id = 'https://quilt.example/ap/users/weaver'
		 WHERE id = ?`, weaver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO credentials (id, user_id, credential_id, public_key, sign_count, name)
		 VALUES (?, ?, 'CREDENTIAL-ID', 'CREDENTIAL-PUBLIC-KEY', 0, 'Yubikey')`,
		auth.NewUUIDv7(), weaver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO recovery_codes (id, user_id, code) VALUES (?, ?, 'RECOVERY-CODE-HASH')`,
		auth.NewUUIDv7(), weaver.ID); err != nil {
		t.Fatal(err)
	}

	w := requestExport(t, db, token)
	if w.Code != http.StatusOK {
		t.Fatalf("export: got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	for _, needle := range []string{
		"PRIVATE-KEY-MATERIAL",
		"PUBLIC-KEY-MATERIAL",
		"FEED-SECRET-HASH",
		"CREDENTIAL-ID",
		"CREDENTIAL-PUBLIC-KEY",
		"RECOVERY-CODE-HASH",
		token, // the session token that fetched it
	} {
		if strings.Contains(body, needle) {
			t.Errorf("export leaks credential material: %q", needle)
		}
	}

	// Not by name either: no session, credential, or recovery block at all.
	doc := decodeJSON(t, w)
	for _, key := range []string{"sessions", "credentials", "passkeys", "recovery_codes", "feed_secret"} {
		if _, present := doc[key]; present {
			t.Errorf("export has a %q section", key)
		}
	}
	profile, _ := doc["user"].(map[string]interface{})
	for _, key := range []string{"private_key", "public_key", "feed_secret_hash"} {
		if _, present := profile[key]; present {
			t.Errorf("profile carries %q", key)
		}
	}
}

// Every section's query binds only the requesting user's id, which is what
// makes "no other person's rows" a property of the shape rather than of each
// query being read carefully.
func TestPersonalExport_SectionsBindOnlyTheCaller(t *testing.T) {
	db := setupTestDB(t)
	weaver, token := createTestUser(t, db, "weaver", "member")

	w := requestExport(t, db, token)
	if w.Code != http.StatusOK {
		t.Fatalf("export: got %d: %s", w.Code, w.Body.String())
	}
	doc := decodeJSON(t, w)

	// Sanity: the document is not silently empty of sections.
	arrays := 0
	for _, v := range doc {
		if _, ok := v.([]interface{}); ok {
			arrays++
		}
	}
	if arrays < 20 {
		t.Errorf("export has %d sections, expected the full survey", arrays)
	}
	if doc["user"].(map[string]interface{})["id"] != weaver.ID {
		t.Error("export describes somebody other than the caller")
	}
}
