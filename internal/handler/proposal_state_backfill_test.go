package handler_test

import (
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Migration 043 heals proposals whose `state` never followed their `status`:
// withdraw, vote resolution, and amendment auto-apply all used to move only
// `status`, and the proposal page renders from `state`. setupTestDB starts
// from an empty table, so there is nothing for the migration to have repaired
// — these tests seed the stale rows and replay the migration's own SQL, the
// same shape as the 042 link-repair tests.
func replayStateBackfill(t *testing.T, db *database.DB) {
	t.Helper()
	sql, err := patchwork.MigrationsFS.ReadFile("migrations/043_proposal_state_backfill.sql")
	if err != nil {
		t.Fatalf("read migration 043: %v", err)
	}
	if _, err := db.Exec(string(sql)); err != nil {
		t.Fatalf("run migration 043: %v", err)
	}
}

// seedStaleProposal writes a proposal directly, so it can carry the
// status/state pairing the old write paths left behind.
func seedStaleProposal(t *testing.T, db *database.DB, nodeID, authorID, status, state string, appliedAt any) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, applied_at, proposal_type, duration_hours)
		 VALUES (?, ?, ?, 'Stale Proposal', 'Body', ?, ?, ?, 'action', 72)`,
		id, nodeID, authorID, status, state, appliedAt,
	)
	if err != nil {
		t.Fatalf("seed proposal: %v", err)
	}
	return id
}

func stateOf(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var state string
	if err := db.QueryRow("SELECT COALESCE(state,'') FROM proposals WHERE id = ?", id).Scan(&state); err != nil {
		t.Fatalf("read state: %v", err)
	}
	return state
}

func TestMigration043_TerminalStatusPullsStateAcross(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "backfilluser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")

	withdrawn := seedStaleProposal(t, db, nodeID, user.ID, "withdrawn", "voting", nil)
	rejected := seedStaleProposal(t, db, nodeID, user.ID, "rejected", "voting", nil)
	// Still open: nothing to repair, and the row must not be dragged anywhere.
	open := seedStaleProposal(t, db, nodeID, user.ID, "open", "voting", nil)

	replayStateBackfill(t, db)

	if got := stateOf(t, db, withdrawn); got != "withdrawn" {
		t.Errorf("withdrawn proposal: got state %q, want %q", got, "withdrawn")
	}
	if got := stateOf(t, db, rejected); got != "rejected" {
		t.Errorf("rejected proposal: got state %q, want %q", got, "rejected")
	}
	if got := stateOf(t, db, open); got != "voting" {
		t.Errorf("open proposal should be untouched: got state %q, want %q", got, "voting")
	}
}

// 'approved' is a resting state — the community decided, an admin still makes
// it official. A row already stamped applied_at had that second step happen
// (auto-apply merged the branch), so it belongs in_effect instead.
func TestMigration043_ApprovedSplitsOnAppliedAt(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "approveduser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")

	decided := seedStaleProposal(t, db, nodeID, user.ID, "approved", "voting", nil)
	applied := seedStaleProposal(t, db, nodeID, user.ID, "approved", "voting", "2026-07-20T12:00:00.000Z")

	replayStateBackfill(t, db)

	if got := stateOf(t, db, decided); got != "approved" {
		t.Errorf("approved, never applied: got state %q, want %q", got, "approved")
	}
	if got := stateOf(t, db, applied); got != "in_effect" {
		t.Errorf("approved and applied: got state %q, want %q", got, "in_effect")
	}
}

// Rows that took the correct path must not be rewritten — in particular an
// in_effect proposal must not be knocked back to 'approved'.
func TestMigration043_LeavesCorrectRowsAlone(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "correctuser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")

	inEffect := seedStaleProposal(t, db, nodeID, user.ID, "approved", "in_effect", "2026-07-20T12:00:00.000Z")
	restingApproved := seedStaleProposal(t, db, nodeID, user.ID, "approved", "approved", nil)
	alreadyWithdrawn := seedStaleProposal(t, db, nodeID, user.ID, "withdrawn", "withdrawn", nil)

	replayStateBackfill(t, db)

	if got := stateOf(t, db, inEffect); got != "in_effect" {
		t.Errorf("in_effect must survive: got state %q", got)
	}
	if got := stateOf(t, db, restingApproved); got != "approved" {
		t.Errorf("approved must survive: got state %q", got)
	}
	if got := stateOf(t, db, alreadyWithdrawn); got != "withdrawn" {
		t.Errorf("withdrawn must survive: got state %q", got)
	}
}

// Withdrawal can happen before voting opens, leaving a stale 'draft' or
// 'discussion' rather than 'voting' — the guard is on the state not being
// terminal, so those are repaired too.
func TestMigration043_RepairsPreVotingStates(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "draftuser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")

	fromDraft := seedStaleProposal(t, db, nodeID, user.ID, "withdrawn", "draft", nil)
	fromDiscussion := seedStaleProposal(t, db, nodeID, user.ID, "withdrawn", "discussion", nil)

	replayStateBackfill(t, db)

	if got := stateOf(t, db, fromDraft); got != "withdrawn" {
		t.Errorf("withdrawn from draft: got state %q, want %q", got, "withdrawn")
	}
	if got := stateOf(t, db, fromDiscussion); got != "withdrawn" {
		t.Errorf("withdrawn from discussion: got state %q, want %q", got, "withdrawn")
	}
}

// Deployments run a migration once, but the statements should be safe anyway.
func TestMigration043_IsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "idemstateuser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")

	withdrawn := seedStaleProposal(t, db, nodeID, user.ID, "withdrawn", "voting", nil)
	applied := seedStaleProposal(t, db, nodeID, user.ID, "approved", "voting", "2026-07-20T12:00:00.000Z")

	replayStateBackfill(t, db)
	replayStateBackfill(t, db)

	if got := stateOf(t, db, withdrawn); got != "withdrawn" {
		t.Errorf("withdrawn after two runs: got state %q", got)
	}
	if got := stateOf(t, db, applied); got != "in_effect" {
		t.Errorf("applied after two runs: got state %q", got)
	}
}
