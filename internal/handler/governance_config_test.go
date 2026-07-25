package handler_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func mustExec(t *testing.T, db *database.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func nodeGovernanceConfigMap(t *testing.T, db *database.DB, nodeID string) map[string]interface{} {
	t.Helper()
	var gcJSON string
	if err := db.QueryRow("SELECT COALESCE(governance_config,'') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON); err != nil {
		t.Fatalf("read governance_config: %v", err)
	}
	var gc map[string]interface{}
	if err := json.Unmarshal([]byte(gcJSON), &gc); err != nil {
		t.Fatalf("parse governance_config %q: %v", gcJSON, err)
	}
	return gc
}

// TestMigration041CompletesLeadershipConfig replays migration 041's
// statements (idempotent UPDATEs) against rows shaped like the states it
// backfills: the pre-041 column default, a synced config with legitimate
// omitempty absences, and a NULL config.
func TestMigration041CompletesLeadershipConfig(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "m41user", "member")

	// A patch dealt migration 013's column default: decision keys only.
	legacy := createTestNode(t, db, user.ID, "Legacy Config", "legacy-config", "open")
	mustExec(t, db, `UPDATE nodes SET governance_config = '{"decision_method":"supermajority","quorum_percent":10,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0}' WHERE id = ?`, legacy)

	// A patch synced from a minimal-template repo: leadership_model present,
	// inactivity_days legitimately absent (omitempty zero). Must not change.
	synced := createTestNode(t, db, user.ID, "Synced Config", "synced-config", "invite_only")
	mustExec(t, db, `UPDATE nodes SET governance_config = '{"decision_method":"admin","quorum_percent":0,"default_vote_duration_hours":0,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0,"leadership_model":"maintainer","succession_method":"founder_designate","max_admins":1}' WHERE id = ?`, synced)

	// No config at all.
	nullRow := createTestNode(t, db, user.ID, "Null Config", "null-config", "open")
	mustExec(t, db, `UPDATE nodes SET governance_config = NULL WHERE id = ?`, nullRow)

	sqlBytes, err := fs.ReadFile(patchwork.MigrationsFS, "migrations/041_governance_config_leadership.sql")
	if err != nil {
		t.Fatalf("read migration 041: %v", err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatalf("replay migration 041: %v", err)
	}

	gc := nodeGovernanceConfigMap(t, db, legacy)
	if gc["leadership_model"] != "maintainer" {
		t.Errorf("legacy: expected leadership_model=maintainer, got %v", gc["leadership_model"])
	}
	if gc["succession_method"] != "admin_nominate" {
		t.Errorf("legacy: expected succession_method=admin_nominate, got %v", gc["succession_method"])
	}
	if gc["max_admins"] != float64(3) || gc["inactivity_days"] != float64(90) {
		t.Errorf("legacy: expected max_admins=3 inactivity_days=90, got %v / %v", gc["max_admins"], gc["inactivity_days"])
	}
	// The decision keys the row already had survive untouched.
	if gc["decision_method"] != "supermajority" || gc["quorum_percent"] != float64(10) {
		t.Errorf("legacy: decision keys clobbered: %v", gc)
	}

	gc = nodeGovernanceConfigMap(t, db, synced)
	if gc["max_admins"] != float64(1) || gc["succession_method"] != "founder_designate" {
		t.Errorf("synced: leadership keys clobbered: %v", gc)
	}
	if _, present := gc["inactivity_days"]; present {
		t.Errorf("synced: inactivity_days injected into an already-synced config: %v", gc)
	}

	gc = nodeGovernanceConfigMap(t, db, nullRow)
	if gc["leadership_model"] != "maintainer" || gc["decision_method"] != "majority" {
		t.Errorf("null: expected full default config, got %v", gc)
	}
}

// TestCreateNodeWritesCompleteGovernanceConfig guards the creation path:
// the row must not be left with the (incomplete) column default.
func TestCreateNodeWritesCompleteGovernanceConfig(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "gccreator", "member")

	r := authedRequest("POST", "/api/v1/nodes", map[string]string{"name": "Config Node"}, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var node map[string]interface{}
	json.NewDecoder(w.Body).Decode(&node)
	gc := nodeGovernanceConfigMap(t, db, node["id"].(string))
	if gc["leadership_model"] != "maintainer" {
		t.Errorf("expected leadership_model=maintainer, got %v (config: %v)", gc["leadership_model"], gc)
	}
	if gc["succession_method"] == nil || gc["max_admins"] == nil {
		t.Errorf("expected complete leadership block, got %v", gc)
	}
}

// TestSyncConfigToDBLeavesMembershipChoicesAlone: the template's rules fill
// the governance_config cache, but membership_policy stays the creator's
// form choice even when the template disagrees (minimal says invite_only).
func TestSyncConfigToDBLeavesMembershipChoicesAlone(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "gcsyncer", "member")
	nodeID := createTestNode(t, db, user.ID, "Sync Node", "sync-node", "open")

	dataDir := t.TempDir()
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("InitInstanceRepo: %v", err)
	}
	if err := governance.ForkForNode(dataDir, nodeID, "minimal"); err != nil {
		t.Fatalf("ForkForNode: %v", err)
	}
	if err := governance.SyncConfigToDB(db, dataDir, nodeID); err != nil {
		t.Fatalf("SyncConfigToDB: %v", err)
	}

	var policy string
	db.QueryRow("SELECT membership_policy FROM nodes WHERE id = ?", nodeID).Scan(&policy)
	if policy != "open" {
		t.Errorf("expected membership_policy=open (creator's choice), got %q", policy)
	}
	gc := nodeGovernanceConfigMap(t, db, nodeID)
	if gc["leadership_model"] != "maintainer" || gc["max_admins"] != float64(1) {
		t.Errorf("expected minimal-template config (maintainer, max_admins 1), got %v", gc)
	}
	if gc["decision_method"] != "admin" {
		t.Errorf("expected minimal-template decision_method=admin, got %v", gc["decision_method"])
	}
}
