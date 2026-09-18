package handler_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// A merge cannot be undone by SQLite. So the writes that follow one have two
// jobs: land together, or say out loud that they didn't. Before this they did
// neither — their errors were dropped, and a charter could be amended in git
// while the proposal row still showed an open vote over it, with nothing in
// the log or the audit trail to find it by.

// The ordinary ending: one settle writes the commit and the state the commit
// put the patch in, so the row never holds one without the other.
func TestAutoApply_SettlesShaAndStateTogether(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	proposalID, _, _, _, _ := carryFixture(t, db, "settletogether", formalAutoApply)

	// The fixture stamps the branch's own commit here, and a fast-forward
	// merge produces that same commit — so clear it first, and whatever comes
	// back can only have been written by the settle.
	if _, err := db.Exec("UPDATE proposals SET git_sha = NULL WHERE id = ?", proposalID); err != nil {
		t.Fatalf("clear git_sha: %v", err)
	}

	handler.SweepProposals(db)

	var status, state, sha, appliedAt, appliedBy string
	db.QueryRow(
		`SELECT status, COALESCE(state,''), COALESCE(git_sha,''), COALESCE(applied_at,''), COALESCE(applied_by,'')
		 FROM proposals WHERE id = ?`, proposalID,
	).Scan(&status, &state, &sha, &appliedAt, &appliedBy)

	if status != "approved" || state != "in_effect" {
		t.Fatalf("status/state = %s/%s, want approved/in_effect", status, state)
	}
	if !looksLikeCommit(sha) {
		t.Errorf("git_sha = %q, want the merge commit", sha)
	}
	if appliedAt == "" {
		t.Errorf("applied_at is empty; the row says in_effect without saying when")
	}
	// Nobody applied this one: the clock and the electorate did.
	if appliedBy != "" {
		t.Errorf("applied_by = %q, want empty — no person applied an auto-applied amendment", appliedBy)
	}
	if n := countApplyIncomplete(t, db, proposalID); n != 0 {
		t.Errorf("apply_incomplete audit entries = %d, want 0 on the happy path", n)
	}
}

// The failure this issue is about, induced where it actually happens: the
// merge lands, and the write that records it is refused. Patchwork cannot put
// the commit back, so what it owes the operator is a record — and the row it
// could not finish must not be left half-written.
func TestAutoApply_RecordsDivergenceWhenTheSettleFails(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	proposalID, dataDir, _, _, _ := carryFixture(t, db, "settlefails", formalAutoApply)
	nodeID := nodeIDOfProposal(t, db, proposalID)

	if _, err := db.Exec("UPDATE proposals SET git_sha = NULL WHERE id = ?", proposalID); err != nil {
		t.Fatalf("clear git_sha: %v", err)
	}

	// Refuse exactly the post-merge write, and nothing before it: the
	// compare-and-swap that settles the vote sets state 'approved' and is
	// left alone.
	if _, err := db.Exec(`CREATE TRIGGER refuse_in_effect BEFORE UPDATE ON proposals
	                      WHEN NEW.state = 'in_effect'
	                      BEGIN SELECT RAISE(ABORT, 'induced write failure'); END`); err != nil {
		t.Fatalf("install trigger: %v", err)
	}
	t.Cleanup(func() { db.Exec("DROP TRIGGER IF EXISTS refuse_in_effect") })

	handler.SweepProposals(db)

	// The merge happened. That is the whole problem, and the test asserts it
	// so the divergence being recorded is a real one.
	doc, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("read document: %v", err)
	}
	if !strings.Contains(doc, "approval-required") {
		t.Fatalf("the amendment did not merge, so this test is not testing anything: %q", doc)
	}

	// The settle is all-or-nothing: no sha landed without the state.
	var state, sha string
	db.QueryRow("SELECT COALESCE(state,''), COALESCE(git_sha,'') FROM proposals WHERE id = ?", proposalID).Scan(&state, &sha)
	if state == "in_effect" {
		t.Fatalf("state = in_effect; the trigger should have refused that write")
	}
	if sha != "" {
		t.Errorf("git_sha = %q, want it still empty — half of a failed settle landed", sha)
	}

	// And an operator can find it.
	var metadata string
	if err := db.QueryRow(
		"SELECT metadata FROM audit_log WHERE action = 'proposal.apply_incomplete' AND entity_id = ?", proposalID,
	).Scan(&metadata); err != nil {
		t.Fatalf("no proposal.apply_incomplete audit entry for the failed settle: %v", err)
	}
	var detail struct {
		Step   string `json:"step"`
		GitSHA string `json:"git_sha"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(metadata), &detail); err != nil {
		t.Fatalf("audit metadata is not JSON: %q (%v)", metadata, err)
	}
	if detail.Step != "settle" {
		t.Errorf("audit step = %q, want settle", detail.Step)
	}
	// The commit the row does not carry is in the audit entry instead, which
	// is the only place left to find what the charter now says.
	if !looksLikeCommit(detail.GitSHA) {
		t.Errorf("audit git_sha = %q, want the merge commit", detail.GitSHA)
	}
	if detail.Error == "" {
		t.Errorf("audit entry records no cause")
	}
}

// looksLikeCommit is the whole check a sha needs here: the test has no other
// way to know the merge commit, since a fast-forward makes it the branch's
// own and the fixture already knew that one.
func looksLikeCommit(sha string) bool {
	return len(sha) == 40 && strings.TrimLeft(sha, "0123456789abcdef") == ""
}

func countApplyIncomplete(t *testing.T, db *database.DB, proposalID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE action = 'proposal.apply_incomplete' AND entity_id = ?", proposalID,
	).Scan(&n); err != nil {
		t.Fatalf("count apply_incomplete: %v", err)
	}
	return n
}
