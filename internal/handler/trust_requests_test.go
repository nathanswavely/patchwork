package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// The trust request
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decision 7: "a trust request is answered, never merely seen").

func withNotifier(t *testing.T, db *database.DB) {
	t.Helper()
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
}

func postTrustRequest(t *testing.T, db *database.DB, token string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/users/me/trust-request", body, token)
	return serveMux(t, db, "POST", "/api/v1/users/me/trust-request", handler.CreateTrustRequest(db), r)
}

func getTrustRequest(t *testing.T, db *database.DB, token string) map[string]any {
	t.Helper()
	r := authedRequest("GET", "/api/v1/users/me/trust-request", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/users/me/trust-request", handler.GetMyTrustRequest(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("get trust request: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode trust request: %v", err)
	}
	return resp
}

func listTrustRequests(t *testing.T, db *database.DB, adminToken string) []any {
	t.Helper()
	r := authedRequest("GET", "/api/v1/admin/trust-requests", nil, adminToken)
	w := serveAdminMux(t, db, "GET", "/api/v1/admin/trust-requests", handler.ListTrustRequests(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list trust requests: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []any `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Items
}

func decideTrustRequest(t *testing.T, db *database.DB, adminToken, id string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/admin/trust-requests/"+id, body, adminToken)
	return serveAdminMux(t, db, "PATCH", "/api/v1/admin/trust-requests/{id}", handler.DecideTrustRequest(db), r)
}

// requestNodeIDs pulls the node ids out of a GET payload's request object.
func requestNodeIDs(t *testing.T, payload map[string]any) []string {
	t.Helper()
	req, ok := payload["request"].(map[string]any)
	if !ok {
		t.Fatalf("payload carries no request: %+v", payload)
	}
	nodes, _ := req["nodes"].([]any)
	ids := []string{}
	for _, n := range nodes {
		m, _ := n.(map[string]any)
		ids = append(ids, m["id"].(string))
	}
	return ids
}

// An ask is one row, naming the patches it names, in the order the asker
// named them — and there is only ever one of them open.
func TestTrustRequestCreated(t *testing.T) {
	db := setupTestDB(t)
	withNotifier(t, db)
	owner, _ := createTestUser(t, db, "owner", "member")
	siteAdmin, _ := createTestUser(t, db, "siteadmin", "admin")
	asker, askerToken := createTestUser(t, db, "asker", "member")

	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)
	nodeB := createTestNode(t, db, owner.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, nodeB)

	// Named youngest-first, so the order that comes back cannot be the order
	// the ids happen to sort in.
	w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{nodeB, nodeA, nodeB}, "message": "I keep both calendars",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	payload := getTrustRequest(t, db, askerToken)
	req := payload["request"].(map[string]any)
	if req["status"] != "pending" || req["scope"] != "patches" {
		t.Errorf("status/scope: %v/%v, want pending/patches", req["status"], req["scope"])
	}
	if req["message"] != "I keep both calendars" {
		t.Errorf("message: %v", req["message"])
	}
	if got := requestNodeIDs(t, payload); len(got) != 2 || got[0] != nodeB || got[1] != nodeA {
		t.Errorf("named patches: %v, want [%s %s] — deduped, in the order asked", got, nodeB, nodeA)
	}
	if payload["can_ask_again_at"] != nil {
		t.Errorf("can_ask_again_at on a pending request: %v, want null", payload["can_ask_again_at"])
	}

	// The site admin who can answer it hears about it.
	if n := countNotifications(t, db, siteAdmin.ID, notifications.AdminTrustRequest, 1); n != 1 {
		t.Errorf("site admin notifications: %d, want 1", n)
	}

	// A second ask, while the first is waiting, is refused.
	if w := postTrustRequest(t, db, askerToken, map[string]any{"scope": "all"}); w.Code != http.StatusConflict {
		t.Errorf("second ask: %d %s, want 409", w.Code, w.Body.String())
	}
	_ = asker
}

// Somebody who already holds the quilt-wide grant is asking for what they
// have, and an active patch is never askable.
func TestTrustRequestRefusals(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	createTestUser(t, db, "siteadmin", "admin")
	trusted, trustedToken := createTestUser(t, db, "trusty", "member")
	makeTrusted(t, db, trusted.ID)
	_, askerToken := createTestUser(t, db, "asker", "member")

	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")

	if w := postTrustRequest(t, db, trustedToken, map[string]any{"scope": "all"}); w.Code != http.StatusForbidden {
		t.Errorf("already trusted: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{activeID},
	}); w.Code != http.StatusBadRequest {
		t.Errorf("active patch: %d %s, want 400", w.Code, w.Body.String())
	}
	if w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{},
	}); w.Code != http.StatusBadRequest {
		t.Errorf("no patches named: %d %s, want 400", w.Code, w.Body.String())
	}
}

// The admin answers at the scope they judge right, which may be wider than
// the ask — and the audit line says both, or nobody can tell a partial yes
// from an admin giving exactly what was asked for.
func TestTrustRequestApprovedWiderThanAsked(t *testing.T) {
	db := setupTestDB(t)
	withNotifier(t, db)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")
	asker, askerToken := createTestUser(t, db, "asker", "member")

	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)
	nodeB := createTestNode(t, db, owner.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, nodeB)

	if w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{nodeA},
	}); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	items := listTrustRequests(t, db, adminToken)
	if len(items) != 1 {
		t.Fatalf("queue: %d items, want 1", len(items))
	}
	item := items[0].(map[string]any)
	requestID := item["id"].(string)
	user := item["user"].(map[string]any)
	if user["username"] != "asker" || user["display_name"] != "asker" {
		t.Errorf("queued asker: %+v", user)
	}

	if w := decideTrustRequest(t, db, adminToken, requestID, map[string]any{
		"action": "approve", "scope": "patches", "node_ids": []string{nodeA, nodeB},
	}); w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}

	for _, id := range []string{nodeA, nodeB} {
		var source string
		if err := db.QueryRow(
			`SELECT source FROM node_trusted_contributors WHERE user_id = ? AND node_id = ?`, asker.ID, id,
		).Scan(&source); err != nil {
			t.Errorf("grant on %s missing: %v", id, err)
		} else if source != "request" {
			t.Errorf("grant source on %s: %q, want request", id, source)
		}
	}

	// The requester hears, and the request states what was granted.
	if n := countNotifications(t, db, asker.ID, notifications.TrustRequestApproved, 1); n != 1 {
		t.Errorf("approval notifications: %d, want 1", n)
	}
	payload := getTrustRequest(t, db, askerToken)
	req := payload["request"].(map[string]any)
	if req["status"] != "approved" {
		t.Errorf("status: %v, want approved", req["status"])
	}
	granted, _ := req["granted_scope"].([]any)
	if len(granted) != 2 {
		t.Fatalf("granted_scope: %+v, want two patches", req["granted_scope"])
	}
	if granted[0].(map[string]any)["slug"] != "spark-hall" {
		t.Errorf("granted_scope[0]: %+v", granted[0])
	}

	// Requested and granted are two facts in one audit line.
	var metadata string
	if err := db.QueryRow(
		`SELECT metadata FROM audit_log WHERE action = 'trust.granted' AND entity_id = ?`, asker.ID,
	).Scan(&metadata); err != nil {
		t.Fatalf("audit line: %v", err)
	}
	var meta struct {
		Scope     string   `json:"scope"`
		Requested []string `json:"requested"`
		Granted   []string `json:"granted"`
		RequestID string   `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatalf("decode audit metadata %q: %v", metadata, err)
	}
	if meta.Scope != "patches" || meta.RequestID != requestID {
		t.Errorf("audit scope/request: %q/%q", meta.Scope, meta.RequestID)
	}
	if len(meta.Requested) != 1 || meta.Requested[0] != nodeA {
		t.Errorf("audit requested: %v, want just %s", meta.Requested, nodeA)
	}
	if len(meta.Granted) != 2 {
		t.Errorf("audit granted: %v, want both patches", meta.Granted)
	}
}

// Approved at quilt-wide scope, the grant is the column ADR 026 put it on.
func TestTrustRequestApprovedQuiltWide(t *testing.T) {
	db := setupTestDB(t)
	withNotifier(t, db)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")
	asker, askerToken := createTestUser(t, db, "asker", "member")
	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)

	if w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{nodeA},
	}); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	requestID := listTrustRequests(t, db, adminToken)[0].(map[string]any)["id"].(string)

	if w := decideTrustRequest(t, db, adminToken, requestID, map[string]any{
		"action": "approve", "scope": "all",
	}); w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}

	var trusted bool
	db.QueryRow(`SELECT trusted_contributor FROM users WHERE id = ?`, asker.ID).Scan(&trusted)
	if !trusted {
		t.Error("users.trusted_contributor not set by a quilt-wide approval")
	}
	if n := countNotifications(t, db, asker.ID, notifications.TrustRequestApproved, 1); n != 1 {
		t.Errorf("approval notifications: %d, want 1", n)
	}
	payload := getTrustRequest(t, db, askerToken)
	if got := payload["request"].(map[string]any)["granted_scope"]; got != "all" {
		t.Errorf("granted_scope: %v, want \"all\"", got)
	}
}

// A decline carries the admin's reason to the person, and spends the asking
// for a while — but not forever: a person is not a word.
func TestTrustRequestDeclinedAndCooldown(t *testing.T) {
	db := setupTestDB(t)
	withNotifier(t, db)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")
	asker, askerToken := createTestUser(t, db, "asker", "member")
	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)

	if w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{nodeA},
	}); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	requestID := listTrustRequests(t, db, adminToken)[0].(map[string]any)["id"].(string)

	if w := decideTrustRequest(t, db, adminToken, requestID, map[string]any{
		"action": "decline", "note": "Let's talk at the next meeting.",
	}); w.Code != http.StatusOK {
		t.Fatalf("decline: %d %s", w.Code, w.Body.String())
	}

	if n := countNotifications(t, db, asker.ID, notifications.TrustRequestDeclined, 1); n != 1 {
		t.Fatalf("decline notifications: %d, want 1", n)
	}
	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = ?`,
		asker.ID, string(notifications.TrustRequestDeclined)).Scan(&body)
	if body != "Let's talk at the next meeting." {
		t.Errorf("decline notification body: %q, want the admin's note", body)
	}

	payload := getTrustRequest(t, db, askerToken)
	again, _ := payload["can_ask_again_at"].(string)
	if again == "" {
		t.Fatalf("can_ask_again_at after a decline: %+v, want an instant", payload["can_ask_again_at"])
	}
	w := postTrustRequest(t, db, askerToken, map[string]any{"scope": "all"})
	if w.Code != http.StatusConflict {
		t.Fatalf("asking again inside the cooldown: %d %s, want 409", w.Code, w.Body.String())
	}
	var refusal map[string]any
	json.Unmarshal(w.Body.Bytes(), &refusal)
	if refusal["can_ask_again_at"] != again {
		t.Errorf("refusal carries %v, want %v", refusal["can_ask_again_at"], again)
	}

	// Thirty-one days later the door is open again.
	past := time.Now().UTC().Add(-31 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec(`UPDATE trust_requests SET decided_at = ? WHERE id = ?`, past, requestID); err != nil {
		t.Fatalf("age the decision: %v", err)
	}
	if w := postTrustRequest(t, db, askerToken, map[string]any{"scope": "all"}); w.Code != http.StatusCreated {
		t.Errorf("asking again after the cooldown: %d %s, want 201", w.Code, w.Body.String())
	}
	if payload := getTrustRequest(t, db, askerToken); payload["can_ask_again_at"] != nil {
		t.Errorf("can_ask_again_at on the new pending request: %v, want null", payload["can_ask_again_at"])
	}
}

// A request whose patches were all claimed answers itself: moot, told
// quietly, and off the admin's queue, because there is nothing left to grant.
func TestTrustRequestGoesMoot(t *testing.T) {
	db := setupTestDB(t)
	withNotifier(t, db)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")
	asker, askerToken := createTestUser(t, db, "asker", "member")

	claimed := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, claimed)
	dropped := createTestNode(t, db, owner.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, dropped)

	if w := postTrustRequest(t, db, askerToken, map[string]any{
		"scope": "patches", "node_ids": []string{claimed, dropped},
	}); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	requestID := listTrustRequests(t, db, adminToken)[0].(map[string]any)["id"].(string)

	// One patch is claimed the way activateClaimedNode does it — the rows
	// naming it are deleted — and the other simply goes active.
	if _, err := db.Exec(`DELETE FROM trust_request_nodes WHERE node_id = ?`, dropped); err != nil {
		t.Fatalf("drop the claimed patch off the request: %v", err)
	}
	if _, err := db.Exec(`UPDATE nodes SET status = 'active' WHERE id IN (?, ?)`, claimed, dropped); err != nil {
		t.Fatalf("activate: %v", err)
	}

	if items := listTrustRequests(t, db, adminToken); len(items) != 0 {
		t.Errorf("queue after every named patch was claimed: %d items, want none", len(items))
	}
	var status string
	db.QueryRow(`SELECT status FROM trust_requests WHERE id = ?`, requestID).Scan(&status)
	if status != "moot" {
		t.Errorf("status: %q, want moot", status)
	}
	if n := countNotifications(t, db, asker.ID, notifications.TrustRequestMoot, 1); n != 1 {
		t.Errorf("moot notifications: %d, want 1", n)
	}
	if n := countNotifications(t, db, asker.ID, notifications.TrustRequestDeclined, 0); n != 0 {
		t.Errorf("moot must not read as a decline: %d decline notifications", n)
	}

	// The sweep runs on every read; it must not send the news twice.
	getTrustRequest(t, db, askerToken)
	listTrustRequests(t, db, adminToken)
	if n := countNotifications(t, db, asker.ID, notifications.TrustRequestMoot, 2); n != 1 {
		t.Errorf("moot notifications after three sweeps: %d, want 1", n)
	}
}

// The grant given outright, with no request behind it — and the admin screen
// showing what is held.
func TestAdminTrustedPatchesRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")
	target, _ := createTestUser(t, db, "keeper", "member")

	listing := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, listing)
	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")

	grant := func(nodeID string) *httptest.ResponseRecorder {
		r := authedRequest("POST", "/api/v1/admin/users/"+target.ID+"/trusted-patches",
			map[string]string{"node_id": nodeID}, adminToken)
		return serveAdminMux(t, db, "POST", "/api/v1/admin/users/{id}/trusted-patches",
			handler.GrantTrustedPatch(db), r)
	}
	if w := grant(activeID); w.Code != http.StatusBadRequest {
		t.Errorf("granting on an active patch: %d %s, want 400", w.Code, w.Body.String())
	}
	if w := grant(listing); w.Code != http.StatusCreated {
		t.Fatalf("grant: %d %s", w.Code, w.Body.String())
	}
	if w := grant(listing); w.Code != http.StatusCreated {
		t.Errorf("granting twice: %d %s, want 201 and no duplicate row", w.Code, w.Body.String())
	}
	if n := trustGrantCount(t, db, listing); n != 1 {
		t.Errorf("grant rows: %d, want 1", n)
	}
	var source string
	db.QueryRow(`SELECT source FROM node_trusted_contributors WHERE user_id = ? AND node_id = ?`,
		target.ID, listing).Scan(&source)
	if source != "admin" {
		t.Errorf("source: %q, want admin", source)
	}

	// The admin listing carries what is held, so the screen that grants can
	// also show and revoke.
	trustedNodes := func() []any {
		r := authedRequest("GET", "/api/v1/admin/users", nil, adminToken)
		w := serveAdminMux(t, db, "GET", "/api/v1/admin/users", handler.ListUsers(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("list users: %d %s", w.Code, w.Body.String())
		}
		var resp struct {
			Items []map[string]any `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		for _, item := range resp.Items {
			if item["id"] == target.ID {
				nodes, ok := item["trusted_nodes"].([]any)
				if !ok {
					t.Fatalf("trusted_nodes absent on the listed user: %+v", item)
				}
				return nodes
			}
		}
		t.Fatalf("target user not listed")
		return nil
	}
	nodes := trustedNodes()
	if len(nodes) != 1 || nodes[0].(map[string]any)["slug"] != "spark-hall" {
		t.Fatalf("trusted_nodes: %+v, want the one listing", nodes)
	}

	r := authedRequest("DELETE", "/api/v1/admin/users/"+target.ID+"/trusted-patches/"+listing, nil, adminToken)
	w := serveAdminMux(t, db, "DELETE", "/api/v1/admin/users/{id}/trusted-patches/{nodeId}",
		handler.RevokeTrustedPatch(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if n := trustGrantCount(t, db, listing); n != 0 {
		t.Errorf("grant rows after revoking: %d, want 0", n)
	}
	if nodes := trustedNodes(); len(nodes) != 0 {
		t.Errorf("trusted_nodes after revoking: %+v, want none", nodes)
	}
	if countRows(db, `SELECT COUNT(*) FROM audit_log WHERE action = 'trust.revoked'`) != 1 {
		t.Error("revoking was not audited")
	}
}

// The listing endpoint answers "which patches may be asked about" in one
// query (decision 7: active patches are never askable).
func TestListNodesStatusFilter(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")

	listing := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, listing)
	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")

	slugs := func(query string) []string {
		r := httptest.NewRequest("GET", "/api/v1/nodes"+query, nil)
		w := servePublicMux(t, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("list nodes%s: %d %s", query, w.Code, w.Body.String())
		}
		var resp struct {
			Items []struct {
				Slug   string `json:"slug"`
				Status string `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		out := []string{}
		for _, item := range resp.Items {
			if item.Status == "" {
				t.Errorf("%s carries no status", item.Slug)
			}
			out = append(out, item.Slug)
		}
		return out
	}

	if got := slugs(""); len(got) != 2 {
		t.Errorf("unfiltered: %v, want both patches", got)
	}
	if got := slugs("?status=unclaimed"); len(got) != 1 || got[0] != "spark-hall" {
		t.Errorf("status=unclaimed: %v, want just the listing", got)
	}
	if got := slugs("?status=active"); len(got) != 1 || got[0] != "gallery-row" {
		t.Errorf("status=active: %v, want just the active patch", got)
	}

	r := httptest.NewRequest("GET", "/api/v1/nodes?status=pending_review", nil)
	if w := servePublicMux(t, "GET", "/api/v1/nodes", handler.ListNodes(db), r); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown status: %d, want 400 rather than a quietly unfiltered list", w.Code)
	}
}

// A grant names a living account. A made-up id must not reach the FK and
// come back as a 500, and a tombstone (docs/adr/086) is not an account
// anything may be granted to.
func TestAdminTrustedPatchesRefuseUnknownUser(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")
	listing := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, listing)

	r := authedRequest("POST", "/api/v1/admin/users/no-such-user/trusted-patches",
		map[string]string{"node_id": listing}, adminToken)
	w := serveAdminMux(t, db, "POST", "/api/v1/admin/users/{id}/trusted-patches",
		handler.GrantTrustedPatch(db), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("granting to an unknown user: %d %s, want 404", w.Code, w.Body.String())
	}

	r = authedRequest("DELETE", "/api/v1/admin/users/no-such-user/trusted-patches/"+listing, nil, adminToken)
	w = serveAdminMux(t, db, "DELETE", "/api/v1/admin/users/{id}/trusted-patches/{nodeId}",
		handler.RevokeTrustedPatch(db), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("revoking from an unknown user: %d %s, want 404", w.Code, w.Body.String())
	}
}
