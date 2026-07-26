package handler

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// newRulesNoticeTestDB is setupTestDB's twin for the internal package, which
// cannot see the handler_test helpers.
func newRulesNoticeTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "patchwork-notice-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(tmpFile.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// The counting query reads proposals by node and status only, so a bare
	// owner and node are all the foreign keys it needs.
	if _, err := db.Exec(`INSERT INTO users (id, username, display_name) VALUES ('owner-1','owner','Owner')`); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status)
		 VALUES ('node-1','owner-1','Notice Node','notice-node','','leaf','public','open','active')`,
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	return db
}

func insertTestProposal(t *testing.T, db *database.DB, id, nodeID, status, terms string) {
	t.Helper()
	var termsArg interface{}
	if terms != "" {
		termsArg = terms
	}
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, status, voting_terms) VALUES (?, ?, 'owner-1', ?, ?, ?)`,
		id, nodeID, "Proposal "+id, status, termsArg,
	); err != nil {
		t.Fatalf("insert proposal %s: %v", id, err)
	}
}

// A rules change reaches votes that have not opened yet, and does not reach
// the ones already running (docs/adr/047). The notice has to say which.
func TestRulesChangeBody_NamesTheVotesItDoesNotReach(t *testing.T) {
	newConfig := `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`
	oldConfig := `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":0}`

	db := newRulesNoticeTestDB(t)
	nodeID := "node-1"

	t.Run("nothing open is the routine type", func(t *testing.T) {
		typ, body := rulesChangeNotice(db, nodeID, newConfig, "")
		if typ != notifications.GovernanceRulesChanged {
			t.Errorf("type = %q, want the routine rules-changed type", typ)
		}
		if strings.Contains(body, "already open") {
			t.Errorf("body mentions open votes when there are none: %q", body)
		}
	})

	t.Run("one open vote under old terms escalates, and reads singular", func(t *testing.T) {
		insertTestProposal(t, db, "p1", nodeID, "open", oldConfig)
		typ, body := rulesChangeNotice(db, nodeID, newConfig, "")
		if typ != notifications.GovernanceRulesChangedMidVote {
			t.Errorf("type = %q, want the mid-vote type", typ)
		}
		if !strings.Contains(body, "One vote is already open") {
			t.Errorf("expected singular phrasing, got %q", body)
		}
	})

	t.Run("more than one is counted", func(t *testing.T) {
		insertTestProposal(t, db, "p2", nodeID, "open", oldConfig)
		typ, body := rulesChangeNotice(db, nodeID, newConfig, "")
		if typ != notifications.GovernanceRulesChangedMidVote {
			t.Errorf("type = %q, want the mid-vote type", typ)
		}
		if !strings.Contains(body, "2 votes are already open") {
			t.Errorf("expected a count of 2, got %q", body)
		}
	})

	t.Run("a vote already matching the new terms is not counted", func(t *testing.T) {
		insertTestProposal(t, db, "p3", nodeID, "open", newConfig)
		if got := proposalsUnderOldTerms(db, nodeID, newConfig, ""); got != 2 {
			t.Errorf("diverging count = %d, want 2 — p3 opened under the new terms", got)
		}
	})

	t.Run("a resolved vote is not counted", func(t *testing.T) {
		insertTestProposal(t, db, "p4", nodeID, "approved", oldConfig)
		if got := proposalsUnderOldTerms(db, nodeID, newConfig, ""); got != 2 {
			t.Errorf("diverging count = %d, want 2 — p4 is resolved and decides nothing", got)
		}
	})

	t.Run("a vote with no terms of its own is not counted", func(t *testing.T) {
		// It follows the live config, so there is nothing for it to diverge
		// from — the seeder and fixtures insert proposals this way.
		insertTestProposal(t, db, "p5", nodeID, "open", "")
		if got := proposalsUnderOldTerms(db, nodeID, newConfig, ""); got != 2 {
			t.Errorf("diverging count = %d, want 2 — p5 has no photograph", got)
		}
	})
}
