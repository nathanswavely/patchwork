package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The per-patch trusted-contributor grant
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decisions 2 and 5). Same standing ADR 026 describes, on one unclaimed patch
// and nothing else.

// grantOnPatch gives a person the per-patch grant, the way approving their
// patch suggestion will.
func grantOnPatch(t *testing.T, db *database.DB, userID, nodeID, grantedBy string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO node_trusted_contributors (user_id, node_id, granted_by, granted_at, source)
		 VALUES (?, ?, ?, '2026-09-18T00:00:00.000Z', 'suggestion')`,
		userID, nodeID, grantedBy,
	); err != nil {
		t.Fatalf("grant per-patch trust: %v", err)
	}
}

func trustGrantCount(t *testing.T, db *database.DB, nodeID string) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM node_trusted_contributors WHERE node_id = ?`, nodeID).Scan(&n)
	return n
}

// The grant reaches exactly one patch: the event posts directly on A and
// queues for review on B, from the same person in the same breath.
func TestPerPatchTrustReachesOnePatch(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	siteAdmin, _ := createTestUser(t, db, "siteadmin", "admin")
	suggester, suggesterToken := createTestUser(t, db, "suggester", "member")

	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)
	nodeB := createTestNode(t, db, owner.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, nodeB)

	grantOnPatch(t, db, suggester.ID, nodeA, siteAdmin.ID)

	e, code := createEventVia(t, db, cfg, suggesterToken, eventBody(nodeA, "Zine Fair"))
	if code != http.StatusCreated || e.Status != "active" {
		t.Fatalf("granted patch: code=%d status=%q, want 201 active", code, e.Status)
	}

	e2, code := createEventVia(t, db, cfg, suggesterToken, eventBody(nodeB, "Basement Show"))
	if code != http.StatusCreated || e2.Status != "pending_review" {
		t.Fatalf("ungranted patch: code=%d status=%q, want 201 pending_review", code, e2.Status)
	}
}

// And it is worth nothing on an active patch, which is the whole of ADR 026's
// "must not reach into active patches". A grant row pointing at a patch that
// has since gone active decides nothing, because every gate asks the status
// first.
func TestPerPatchTrustIsWorthlessOnActivePatch(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	siteAdmin, _ := createTestUser(t, db, "siteadmin", "admin")
	suggester, suggesterToken := createTestUser(t, db, "suggester", "member")

	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")
	grantOnPatch(t, db, suggester.ID, activeID, siteAdmin.ID)

	e, code := createEventVia(t, db, cfg, suggesterToken, eventBody(activeID, "Open Mic"))
	if code != http.StatusCreated || e.Status != "pending_review" {
		t.Fatalf("granted row on an active patch: code=%d status=%q, want 201 pending_review", code, e.Status)
	}

	// Same at the CSV door, which is the batch version of the same act.
	r := authedRequest("POST", "/api/v1/nodes/gallery-row/events/bulk", map[string]any{
		"events": []map[string]any{{"title": "Season", "starts_at": "2027-02-01T20:00:00Z"}},
	}, suggesterToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("bulk upload on an active patch: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// Event sources follow the grant now (decision 5), and follow it patch by
// patch: the person who keeps A's calendar may attach and list its feeds, and
// is a stranger at B.
func TestPerPatchTrustReachesEventSources(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	siteAdmin, _ := createTestUser(t, db, "siteadmin", "admin")
	suggester, suggesterToken := createTestUser(t, db, "suggester", "member")

	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)
	nodeB := createTestNode(t, db, owner.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, nodeB)
	grantOnPatch(t, db, suggester.ID, nodeA, siteAdmin.ID)

	r := authedRequest("POST", "/api/v1/nodes/spark-hall/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/cal.ics"}, suggesterToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("attach on the granted patch: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	r = authedRequest("GET", "/api/v1/nodes/spark-hall/event-sources", nil, suggesterToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-sources", handler.ListEventSources(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list on the granted patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listed struct {
		Items []struct {
			URL     string `json:"url"`
			AddedBy string `json:"added_by"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Items) != 1 || listed.Items[0].AddedBy != suggester.ID {
		t.Errorf("listed sources: %+v, want one attached by the suggester", listed.Items)
	}

	r = authedRequest("POST", "/api/v1/nodes/selvage/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/cal.ics"}, suggesterToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("attach on the ungranted patch: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	r = authedRequest("GET", "/api/v1/nodes/selvage/event-sources", nil, suggesterToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-sources", handler.ListEventSources(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("list on the ungranted patch: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// Claim setup ends the grant, in the act that gives the calendar an owner.
func TestSetupClaimClearsPerPatchTrust(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, aliceToken := createTestUser(t, db, "alice", "member")
	siteAdmin, adminToken := createTestUser(t, db, "siteadmin", "admin")
	suggester, suggesterToken := createTestUser(t, db, "suggester", "member")

	oldDir := governance.GetDataDir()
	governance.SetDataDir(t.TempDir())
	t.Cleanup(func() { governance.SetDataDir(oldDir) })

	nodeID := createTestNode(t, db, owner.ID, "Setup Venue", "setup-venue", "open")
	makeClaimable(t, db, nodeID, "")
	grantOnPatch(t, db, suggester.ID, nodeID, siteAdmin.ID)

	// A trust request naming this patch, waiting on an admin who will never
	// need to answer it.
	requestID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO trust_requests (id, user_id, scope, created_at) VALUES (?, ?, 'patches', '2026-09-18T00:00:00.000Z')`,
		requestID, suggester.ID,
	); err != nil {
		t.Fatalf("seed trust request: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO trust_request_nodes (request_id, node_id) VALUES (?, ?)`, requestID, nodeID,
	); err != nil {
		t.Fatalf("seed trust request node: %v", err)
	}

	claimID := approveAdminClaim(t, db, cfg, "setup-venue", aliceToken, adminToken)
	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: got %d %s", w.Code, w.Body.String())
	}

	if n := trustGrantCount(t, db, nodeID); n != 0 {
		t.Errorf("per-patch grants after the claim: %d, want 0", n)
	}
	var named int
	db.QueryRow(`SELECT COUNT(*) FROM trust_request_nodes WHERE node_id = ?`, nodeID).Scan(&named)
	if named != 0 {
		t.Errorf("trust request still names the claimed patch: %d rows", named)
	}
	// The request row itself survives — resolving it as moot is the request
	// handler's job, not the claim's.
	var requests int
	db.QueryRow(`SELECT COUNT(*) FROM trust_requests WHERE id = ?`, requestID).Scan(&requests)
	if requests != 1 {
		t.Errorf("trust request rows: %d, want the request itself left standing", requests)
	}

	// And the standing is gone where it counts: the next event queues.
	e, code := createEventVia(t, db, submissionsCfg(true), suggesterToken, eventBody(nodeID, "After The Claim"))
	if code != http.StatusCreated || e.Status != "pending_review" {
		t.Fatalf("event after the claim: code=%d status=%q, want 201 pending_review", code, e.Status)
	}
}

// viewer_trusted is what the event form reads to decide between "post" and
// "submit for review", so it has to answer for the viewer and the patch
// together (docs/adr/2026-09-18..., Consequences).
func TestGetNodeViewerTrusted(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	siteAdmin, siteAdminToken := createTestUser(t, db, "siteadmin", "admin")
	suggester, suggesterToken := createTestUser(t, db, "suggester", "member")
	quiltWide, quiltWideToken := createTestUser(t, db, "trusty", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	makeTrusted(t, db, quiltWide.ID)

	nodeA := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeA)
	nodeB := createTestNode(t, db, owner.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, nodeB)
	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")
	grantOnPatch(t, db, suggester.ID, nodeA, siteAdmin.ID)
	grantOnPatch(t, db, suggester.ID, activeID, siteAdmin.ID)

	viewerTrusted := func(slug, token string) bool {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/"+slug, nil, token)
		w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("get %s: %d %s", slug, w.Code, w.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s: %v", slug, err)
		}
		v, present := resp["viewer_trusted"]
		if !present {
			t.Fatalf("%s: viewer_trusted absent; an absent key is not an answer", slug)
		}
		b, ok := v.(bool)
		if !ok {
			t.Fatalf("%s: viewer_trusted is %T, want bool", slug, v)
		}
		return b
	}

	cases := []struct {
		name  string
		slug  string
		token string
		want  bool
	}{
		{"per-patch grant on the patch it names", "spark-hall", suggesterToken, true},
		{"per-patch grant elsewhere", "selvage", suggesterToken, false},
		{"per-patch grant on a patch that is active", "gallery-row", suggesterToken, false},
		{"quilt-wide grant on an unclaimed patch", "spark-hall", quiltWideToken, true},
		{"quilt-wide grant on an active patch", "gallery-row", quiltWideToken, false},
		{"instance admin holding no grant", "spark-hall", siteAdminToken, false},
		{"a stranger", "spark-hall", strangerToken, false},
		{"nobody signed in", "spark-hall", "", false},
	}
	for _, c := range cases {
		if got := viewerTrusted(c.slug, c.token); got != c.want {
			t.Errorf("%s: viewer_trusted = %v, want %v", c.name, got, c.want)
		}
	}
}
