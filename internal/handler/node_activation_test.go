package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// When a patch joined the quilt (docs/adr/076). A patch joins when a
// community arrives — someone creates it, or a claim completes through setup
// — never when a directory row is written.

func nodeActivatedAt(t *testing.T, db *database.DB, nodeID string) *string {
	t.Helper()
	var at *string
	if err := db.QueryRow("SELECT activated_at FROM nodes WHERE id = ?", nodeID).Scan(&at); err != nil {
		t.Fatalf("read activated_at: %v", err)
	}
	return at
}

func TestCreatingAPatchIsJoiningTheQuilt(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "founder", "member")

	r := authedRequest("POST", "/api/v1/nodes", map[string]string{"name": "New Patch"}, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", w.Code, w.Body.String())
	}

	var node map[string]interface{}
	json.NewDecoder(w.Body).Decode(&node)
	at := nodeActivatedAt(t, db, node["id"].(string))
	if at == nil || *at == "" {
		t.Fatal("a patch created here is active from its first moment, so it must carry an arrival")
	}
}

func TestAnUnclaimedListingHasNotJoined(t *testing.T) {
	// A directory row is a listing nobody has claimed. No community has
	// arrived, so there is no arrival to date — and the migration's backfill
	// must not have invented one.
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "listowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Unclaimed Venue", "unclaimed-venue", "open")
	makeClaimable(t, db, nodeID, "")

	if at := nodeActivatedAt(t, db, nodeID); at != nil {
		t.Fatalf("unclaimed listing carries an arrival date %q", *at)
	}
}

func TestCompletingAClaimIsJoiningTheQuilt(t *testing.T) {
	db := setupTestDB(t)
	cfg := claimCfg(false)
	owner, _ := createTestUser(t, db, "claimowner", "member")
	_, aliceToken := createTestUser(t, db, "claimalice", "member")
	_, adminToken := createTestUser(t, db, "claimadmin", "admin")

	oldDir := governance.GetDataDir()
	governance.SetDataDir(t.TempDir())
	t.Cleanup(func() { governance.SetDataDir(oldDir) })

	nodeID := createTestNode(t, db, owner.ID, "Claimed Venue", "claimed-venue", "open")
	makeClaimable(t, db, nodeID, "")
	claimID := approveAdminClaim(t, db, cfg, "claimed-venue", aliceToken, adminToken)

	if at := nodeActivatedAt(t, db, nodeID); at != nil {
		t.Fatalf("an approved claim is only the right to enter setup, not an arrival; got %q", *at)
	}

	r := authedRequest("POST", "/api/v1/claims/"+claimID+"/setup", nil, aliceToken)
	w := serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup: got %d %s", w.Code, w.Body.String())
	}

	if at := nodeActivatedAt(t, db, nodeID); at == nil || *at == "" {
		t.Fatal("submitting setup is the moment the community arrives; it must be dated")
	}
}

func TestAnArrivalOutlivesALaterEdit(t *testing.T) {
	// The reason the column exists at all. Completing a claim also writes
	// updated_at, so updated_at reads like an arrival date exactly once —
	// until the first edit moves it. Nothing on the edit path may touch
	// activated_at.
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "editor", "member")
	nodeID := createTestNode(t, db, user.ID, "Edited Patch", "edited-patch", "open")
	mustExec(t, db, `UPDATE nodes SET activated_at = '2020-01-01T00:00:00.000Z' WHERE id = ?`, nodeID)
	mustExec(t, db,
		`INSERT INTO memberships (id, user_id, node_id, role, status) VALUES (?, ?, ?, 'admin', 'active')`,
		auth.NewUUIDv7(), user.ID, nodeID)

	r := authedRequest("PATCH", "/api/v1/nodes/edited-patch", map[string]string{
		"description": "renamed and rewritten",
	}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d %s", w.Code, w.Body.String())
	}

	at := nodeActivatedAt(t, db, nodeID)
	if at == nil || *at != "2020-01-01T00:00:00.000Z" {
		t.Fatalf("an edit moved the arrival date: %v", at)
	}
	var updatedAt string
	db.QueryRow("SELECT updated_at FROM nodes WHERE id = ?", nodeID).Scan(&updatedAt)
	if updatedAt == "2020-01-01T00:00:00.000Z" {
		t.Fatal("updated_at did not move, so this test proves nothing")
	}
}

func TestTreeCarriesTheArrivalToTheList(t *testing.T) {
	// The cards list's "Recently added" order (docs/adr/074) reads this off
	// the tree, so the column existing is not enough — it has to reach the
	// payload. It did not on the first pass: the query and the scan were
	// right and the value was simply never copied onto the emitted node,
	// which no other test would have noticed.
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "treeowner", "member")

	joined := createTestNode(t, db, owner.ID, "Joined Patch", "joined-patch", "open")
	mustExec(t, db, `UPDATE nodes SET activated_at = '2026-03-03T00:00:00.000Z' WHERE id = ?`, joined)
	listing := createTestNode(t, db, owner.ID, "Listing", "listing", "open")
	makeClaimable(t, db, listing, "")

	r := authedRequest("GET", "/api/v1/nodes/tree", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("tree: got %d %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tree struct {
			Children []struct {
				Slug        string  `json:"slug"`
				ActivatedAt *string `json:"activated_at"`
				CreatedAt   string  `json:"created_at"`
			} `json:"children"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode tree: %v", err)
	}

	seen := map[string]*string{}
	created := map[string]string{}
	for _, c := range resp.Tree.Children {
		seen[c.Slug] = c.ActivatedAt
		created[c.Slug] = c.CreatedAt
	}
	if at := seen["joined-patch"]; at == nil || *at != "2026-03-03T00:00:00.000Z" {
		t.Errorf("arrival missing from the tree payload: %v", at)
	}
	// The listing has no arrival and still needs a date the cards list can
	// order by (docs/adr/074, amended): created_at is when it appeared.
	if created["listing"] == "" {
		t.Error("the tree sent no created_at, so Recently added cannot order a listing")
	}
	if at, ok := seen["listing"]; !ok {
		t.Error("the unclaimed listing left the tree entirely")
	} else if at != nil {
		t.Errorf("unclaimed listing reported an arrival: %q", *at)
	}
}
