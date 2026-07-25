package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// TestCreateNode_SyncsTemplateRules covers the creation half of docs/adr/041:
// the template's rules file must land in the governance_config cache at
// birth — with the creation form's membership choices absorbed into the
// rules file first, not clobbered by the template's values.
func TestCreateNode_SyncsTemplateRules(t *testing.T) {
	db := setupTestDB(t)

	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })
	if err := governance.InitInstanceRepo(tmp); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}

	_, token := createTestUser(t, db, "syncadmin", "member")

	// Minimal template (admin-decides, invite_only in the template file)
	// but the form picks open membership — the form must win.
	r := authedRequest("POST", "/api/v1/nodes", map[string]string{
		"name":              "Sync Node",
		"template":          "minimal",
		"membership_policy": "open",
	}, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create node: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var node map[string]interface{}
	json.NewDecoder(w.Body).Decode(&node)
	nodeID := node["id"].(string)

	var gcJSON, membershipPolicy string
	db.QueryRow(`SELECT COALESCE(governance_config,''), membership_policy FROM nodes WHERE id = ?`, nodeID).
		Scan(&gcJSON, &membershipPolicy)
	if gcJSON == "" || gcJSON == "{}" {
		t.Fatalf("governance_config not synced at creation")
	}
	var gc map[string]interface{}
	json.Unmarshal([]byte(gcJSON), &gc)
	if gc["decision_method"] != "admin" {
		t.Errorf("expected decision_method=admin from minimal template, got %v", gc["decision_method"])
	}
	if membershipPolicy != "open" {
		t.Errorf("form's membership_policy clobbered: got %q", membershipPolicy)
	}

	// The rules file absorbed the form's choice — git and DB agree.
	rules, err := governance.ReadRules(tmp, nodeID)
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	if rules.MembershipPolicy != "open" {
		t.Errorf("rules file not absorbed: membership_policy %q", rules.MembershipPolicy)
	}
	if rules.DecisionMethod != "admin" {
		t.Errorf("rules file lost template decision method: %q", rules.DecisionMethod)
	}
}

// TestBackfillGovernanceConfig covers the startup half of docs/adr/041:
// nodes created while CreateNode forked without syncing get their cache
// filled from git, with the DB's live membership settings absorbed rather
// than overwritten; nodes with a populated cache are left alone.
func TestBackfillGovernanceConfig(t *testing.T) {
	db := setupTestDB(t)

	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })
	if err := governance.InitInstanceRepo(tmp); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}

	owner, _ := createTestUser(t, db, "backfillowner", "member")

	// A bug-era node: minimal-template repo, empty cache, live policy that
	// differs from the template file's invite_only.
	staleID := createTestNode(t, db, owner.ID, "Stale Node", "stale-node", "open")
	if err := governance.ForkForNode(tmp, staleID, "minimal"); err != nil {
		t.Fatalf("fork: %v", err)
	}

	// A node whose cache is already populated — must not be touched.
	freshID := createTestNode(t, db, owner.ID, "Fresh Node", "fresh-node", "open")
	if err := governance.ForkForNode(tmp, freshID, "minimal"); err != nil {
		t.Fatalf("fork: %v", err)
	}
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"consensus","quorum_percent":50}`, freshID)

	n, err := handler.BackfillGovernanceConfig(db)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node synced, got %d", n)
	}

	var gcJSON, membershipPolicy string
	db.QueryRow(`SELECT COALESCE(governance_config,''), membership_policy FROM nodes WHERE id = ?`, staleID).
		Scan(&gcJSON, &membershipPolicy)
	var gc map[string]interface{}
	json.Unmarshal([]byte(gcJSON), &gc)
	if gc["decision_method"] != "admin" {
		t.Errorf("expected decision_method=admin after backfill, got %v", gc["decision_method"])
	}
	if membershipPolicy != "open" {
		t.Errorf("live membership_policy clobbered by backfill: got %q", membershipPolicy)
	}
	rules, err := governance.ReadRules(tmp, staleID)
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	if rules.MembershipPolicy != "open" {
		t.Errorf("rules file did not absorb live membership_policy: %q", rules.MembershipPolicy)
	}

	db.QueryRow(`SELECT COALESCE(governance_config,'') FROM nodes WHERE id = ?`, freshID).Scan(&gcJSON)
	json.Unmarshal([]byte(gcJSON), &gc)
	if gc["decision_method"] != "consensus" {
		t.Errorf("populated cache was overwritten: got %v", gc["decision_method"])
	}
}
