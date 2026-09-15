package handler_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// A vote that carries has to say so, and has to say which of the two things
// happened: the change is in effect, or it is waiting on an admin. The Formal
// template ships `amendment_auto_apply: false`, so the second is the ordinary
// case there — and it told nobody at all, least of all the one person who
// could act.

const formalNoAutoApply = `{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":168,` +
	`"amendment_threshold":"majority","amendment_auto_apply":false,"min_voting_tenure_days":0}`

const formalAutoApply = `{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":168,` +
	`"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`

// carryFixture is a patch with one admin and one member, a governance repo,
// and an amendment whose window has already closed with both of them in
// favour. Returns the proposal id and the governance data dir.
func carryFixture(t *testing.T, db *database.DB, slug, rules string) (proposalID, dataDir, adminToken, adminID, memberID string) {
	t.Helper()
	admin, token := createTestUser(t, db, slug+"_admin", "member")
	member, _ := createTestUser(t, db, slug+"_member", "member")
	nodeID := createTestNode(t, db, admin.ID, slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	dataDir = setupGovernanceForNode(t, nodeID)
	setNodeRules(t, db, nodeID, rules)

	proposalID = auth.NewUUIDv7()
	branch := "amendment-" + proposalID[:8]
	proposed := "# Community Standards\n\nMembership is approval-required.\n"
	sha, err := governance.CreateBranch(dataDir, nodeID, branch, "community-standards.md",
		proposed, "Nell", "nell@example.test", "Proposed amendment")
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}

	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type, duration_hours,
		 voting_ends_at, created_at, updated_at, target_doc, proposed_branch, proposed_body, git_sha, voting_terms)
		 VALUES (?, ?, ?, ?, '', 'open', 'voting', 'amendment', 1, ?, ?, ?, 'community-standards.md', ?, ?, ?, ?)`,
		proposalID, nodeID, admin.ID, "Require approval to join", past, now, now, branch, proposed, sha, rules,
	); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, admin.ID)
	db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, member.ID)

	return proposalID, dataDir, token, admin.ID, member.ID
}

func TestProposalCarries_SaysItIsWaitingOnAnAdmin(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	proposalID, _, _, adminID, memberID := carryFixture(t, db, "carrywait", formalNoAutoApply)

	handler.SweepProposals(db)

	var status, state string
	db.QueryRow("SELECT status, COALESCE(state,'') FROM proposals WHERE id = ?", proposalID).Scan(&status, &state)
	if status != "approved" || state != "approved" {
		t.Fatalf("expected approved/approved, got %s/%s", status, state)
	}

	// Both of them hear that it carried — and the admin, who is the only one
	// who can finish it, is among them.
	for _, uid := range []string{adminID, memberID} {
		if n := countNotifications(t, db, uid, notifications.ProposalApproved, 1); n != 1 {
			t.Errorf("ProposalApproved notifications for %s = %d, want 1", uid, n)
		}
	}

	// And it does not claim the rule changed, because it hasn't.
	var title, body string
	db.QueryRow("SELECT title, COALESCE(body,'') FROM notifications WHERE user_id = ? AND type = ?",
		adminID, string(notifications.ProposalApproved)).Scan(&title, &body)
	if !strings.Contains(body, "not in effect yet") || !strings.Contains(body, "admin") {
		t.Errorf("notice body does not say it is waiting on an admin: %q", body)
	}
	if n := countNotifications(t, db, adminID, notifications.ProposalApplied, 0); n != 0 {
		t.Errorf("ProposalApplied notifications = %d, want 0 — nothing was applied", n)
	}
}

func TestProposalCarries_SaysInEffectWhenAutoApplyIsOn(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	proposalID, dataDir, _, adminID, memberID := carryFixture(t, db, "carryeffect", formalAutoApply)

	handler.SweepProposals(db)

	var state string
	db.QueryRow("SELECT COALESCE(state,'') FROM proposals WHERE id = ?", proposalID).Scan(&state)
	if state != "in_effect" {
		t.Fatalf("expected state=in_effect, got %s", state)
	}
	doc, err := governance.GetDocument(dataDir, nodeIDOfProposal(t, db, proposalID), "community-standards.md")
	if err != nil {
		t.Fatalf("read document: %v", err)
	}
	if !strings.Contains(doc, "approval-required") {
		t.Errorf("document was not amended: %q", doc)
	}

	for _, uid := range []string{adminID, memberID} {
		if n := countNotifications(t, db, uid, notifications.ProposalApplied, 1); n != 1 {
			t.Errorf("ProposalApplied notifications for %s = %d, want 1", uid, n)
		}
	}
	var body string
	db.QueryRow("SELECT COALESCE(body,'') FROM notifications WHERE user_id = ? AND type = ?",
		memberID, string(notifications.ProposalApplied)).Scan(&body)
	if !strings.Contains(body, "in effect") {
		t.Errorf("notice body does not say the change is in effect: %q", body)
	}
	if n := countNotifications(t, db, adminID, notifications.ProposalApproved, 0); n != 0 {
		t.Errorf("ProposalApproved notifications = %d, want 0 — this one is already in effect", n)
	}
}

// Three paths resolve a proposal: the hourly sweep, a read after the window,
// and a sole voter's decisive ballot. Whichever arrives first settles it, and
// the rest say nothing — a decision announced twice teaches people to read
// neither notice.
func TestProposalCarries_TellsEachPersonOnce(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	proposalID, _, adminToken, adminID, _ := carryFixture(t, db, "carryonce", formalNoAutoApply)

	handler.SweepProposals(db)
	handler.SweepProposals(db)

	// ...and a read of the page after the sweep, which is the other path.
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	if w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r); w.Code != http.StatusOK {
		t.Fatalf("read proposal: %d", w.Code)
	}

	// countNotifications waits for at least `want`, so ask for two: getting
	// one back is the assertion.
	if n := countNotifications(t, db, adminID, notifications.ProposalApproved, 2); n != 1 {
		t.Errorf("ProposalApproved notifications = %d, want exactly 1", n)
	}
}

// --- F-063: an approved amendment survives a repo rebuilt from the rows ---

// The restore every operator actually has is the SQLite file (docs/adr/084).
// The repos are not in it, `Repair` rebuilds `main` from the canonical rows,
// and a pending amendment's branch is not a row — so the decision the members
// took could never be enacted, and the button that was meant to enact it
// answered "branch amendment-01a0a26e not found: reference not found".
func TestApplyAmendment_WorksAfterTheRepoIsRebuiltFromTheRows(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "rebuild_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Small Press", "small-press", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	dataDir := setupGovernanceForNode(t, nodeID)
	setNodeRules(t, db, nodeID, formalNoAutoApply)

	// The canonical row the rebuild reads. Its title is what the mirror's
	// filename derives from, so it lands as community-standards.md.
	adopted := "# Community Standards\n\nAs adopted.\n"
	if _, err := db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, created_by) VALUES (?, ?, 'Community Standards', ?, ?)`,
		auth.NewUUIDv7(), nodeID, adopted, admin.ID,
	); err != nil {
		t.Fatalf("insert governance doc: %v", err)
	}

	proposalID := auth.NewUUIDv7()
	branch := "amendment-" + proposalID[:8]
	proposed := "# Community Standards\n\nMembership is approval-required.\n"
	if _, err := governance.CreateBranch(dataDir, nodeID, branch, "community-standards.md",
		proposed, "Nell", "nell@example.test", "Proposed amendment"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type, duration_hours,
		 created_at, updated_at, target_doc, proposed_branch, proposed_body)
		 VALUES (?, ?, ?, 'Require approval to join', '', 'approved', 'approved', 'amendment', 168, ?, ?,
		 'community-standards.md', ?, ?)`,
		proposalID, nodeID, admin.ID, now, now, branch, proposed,
	); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}

	// The restore: the repo never reached this machine, and the boot pass
	// rebuilds it from the rows.
	if err := os.RemoveAll(governance.NodeRepoPath(dataDir, nodeID)); err != nil {
		t.Fatalf("remove repo: %v", err)
	}
	if _, err := governance.Repair(db, dataDir, governance.RepairOptions{}); err != nil {
		t.Fatalf("repair: %v", err)
	}

	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/apply", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/apply", handler.ApplyProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("apply after rebuild: %d %s", w.Code, w.Body.String())
	}

	doc, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("read document: %v", err)
	}
	if doc != proposed {
		t.Errorf("document = %q, want the proposed text", doc)
	}
	var state string
	db.QueryRow("SELECT COALESCE(state,'') FROM proposals WHERE id = ?", proposalID).Scan(&state)
	if state != "in_effect" {
		t.Errorf("state = %q, want in_effect", state)
	}
}

// When applying genuinely cannot work, what reaches the admin is a sentence
// about their patch, not a sentence about git.
func TestApplyFailure_SaysSomethingAPersonCanActOn(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "applyfail_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Broken Repo", "broken-repo", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	dataDir := setupGovernanceForNode(t, nodeID)
	setNodeRules(t, db, nodeID, formalNoAutoApply)

	proposalID := auth.NewUUIDv7()
	branch := "amendment-" + proposalID[:8]
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type, duration_hours,
		 created_at, updated_at, target_doc, proposed_branch, proposed_body)
		 VALUES (?, ?, ?, 'Require approval to join', '', 'approved', 'approved', 'amendment', 168, ?, ?,
		 'community-standards.md', ?, 'proposed text')`,
		proposalID, nodeID, admin.ID, now, now, branch,
	); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}

	// No repo and no repair: nothing here can work, which is the case the
	// message has to handle without handing over a go-git string.
	if err := os.RemoveAll(governance.NodeRepoPath(dataDir, nodeID)); err != nil {
		t.Fatalf("remove repo: %v", err)
	}

	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/apply", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/apply", handler.ApplyProposal(db), r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, leak := range []string{"reference not found", "open repo", "repository does not exist"} {
		if strings.Contains(body, leak) {
			t.Errorf("the raw error reached the person: %q", body)
		}
	}
	if !strings.Contains(body, "nothing has changed yet") || !strings.Contains(body, "repair") {
		t.Errorf("message says nothing a person can act on: %q", body)
	}
}

func nodeIDOfProposal(t *testing.T, db *database.DB, proposalID string) string {
	t.Helper()
	var nodeID string
	if err := db.QueryRow("SELECT node_id FROM proposals WHERE id = ?", proposalID).Scan(&nodeID); err != nil {
		t.Fatalf("node for proposal: %v", err)
	}
	return nodeID
}
