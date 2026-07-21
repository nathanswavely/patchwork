package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func TestCreateProposal(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "padmin1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Prop Node", "prop-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]interface{}{
		"title":          "Test Proposal",
		"body":           "This is a test proposal",
		"proposal_type":  "action",
		"duration_hours": 48,
	}
	r := authedRequest("POST", "/api/v1/nodes/prop-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["title"] != "Test Proposal" {
		t.Errorf("expected title=Test Proposal, got %v", result["title"])
	}
	if result["proposal_type"] != "action" {
		t.Errorf("expected proposal_type=action, got %v", result["proposal_type"])
	}
	if result["voting_ends_at"] == nil || result["voting_ends_at"] == "" {
		t.Error("expected voting_ends_at to be set")
	}
}

func TestListProposals(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "padmin2", "member")
	nodeID := createTestNode(t, db, admin.ID, "List Prop", "list-prop", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create a proposal.
	body := map[string]interface{}{"title": "Proposal 1", "proposal_type": "other", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/list-prop/proposals", body, adminToken)
	serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

	// List proposals.
	r = authedRequest("GET", "/api/v1/nodes/list-prop/proposals", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 1 {
		t.Errorf("expected 1 proposal, got %d", len(items))
	}
	first := items[0].(map[string]interface{})
	if first["author_name"] == nil || first["author_name"] == "" {
		t.Error("expected author_name in list response")
	}
}

func TestGetProposalWithTally(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "padmin3", "member")
	voter, voterToken := createTestUser(t, db, "voter3", "member")
	nodeID := createTestNode(t, db, admin.ID, "Tally Node", "tally-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	// Create proposal.
	body := map[string]interface{}{"title": "Tally Test", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/tally-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// Vote approve from admin.
	voteBody := map[string]string{"value": "approve"}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, adminToken)
	serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)

	// Vote reject from voter.
	voteBody = map[string]string{"value": "reject"}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, voterToken)
	serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)

	// Get proposal.
	r = httptest.NewRequest("GET", "/api/v1/proposals/"+proposalID, nil)
	w = servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["approve_count"].(float64) != 1 {
		t.Errorf("expected approve_count=1, got %v", result["approve_count"])
	}
	if result["reject_count"].(float64) != 1 {
		t.Errorf("expected reject_count=1, got %v", result["reject_count"])
	}
	voters, ok := result["voters"].([]interface{})
	if !ok || len(voters) != 2 {
		t.Errorf("expected 2 voters, got %v", len(voters))
	}
}

func TestVoteAndChangeVote(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "padmin4", "member")
	nodeID := createTestNode(t, db, admin.ID, "Vote Node", "vote-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create proposal.
	body := map[string]interface{}{"title": "Vote Test", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/vote-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// Vote approve.
	voteBody := map[string]string{"value": "approve"}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Change vote to reject.
	voteBody = map[string]string{"value": "reject"}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on vote change, got %d: %s", w.Code, w.Body.String())
	}

	// Verify only one vote exists with value=reject.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ?", proposalID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 vote, got %d", count)
	}
	var voteValue string
	db.QueryRow("SELECT value FROM votes WHERE proposal_id = ? AND user_id = ?", proposalID, admin.ID).Scan(&voteValue)
	if voteValue != "reject" {
		t.Errorf("expected vote=reject, got %s", voteValue)
	}
}

func TestVoteAfterWindowExpires(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "padmin5", "member")
	nodeID := createTestNode(t, db, admin.ID, "Expired Node", "expired-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create proposal with already-expired voting window.
	proposalID := auth.NewUUIDv7()
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'open', 'other', 1, ?, ?, ?)`,
		proposalID, nodeID, admin.ID, "Expired Proposal", "", pastTime, now, now,
	)
	if err != nil {
		t.Fatalf("insert expired proposal: %v", err)
	}

	// Try to vote — should fail.
	voteBody := map[string]string{"value": "approve"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for expired voting, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWithdrawByAuthor(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "author6", "member")
	nodeID := createTestNode(t, db, author.ID, "Withdraw Node", "withdraw-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")

	// Create proposal.
	body := map[string]interface{}{"title": "To Withdraw", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/withdraw-node/proposals", body, authorToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// Withdraw.
	r = authedRequest("DELETE", "/api/v1/proposals/"+proposalID, nil, authorToken)
	w = serveMux(t, db, "DELETE", "/api/v1/proposals/{id}", handler.WithdrawProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "withdrawn" {
		t.Errorf("expected status=withdrawn, got %v", result["status"])
	}
}

func TestWithdrawByNonAuthor(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "author7", "member")
	_, otherToken := createTestUser(t, db, "other7", "member")
	nodeID := createTestNode(t, db, author.ID, "NonWith Node", "nonwith-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")

	// Create proposal.
	body := map[string]interface{}{"title": "Cannot Withdraw", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/nonwith-node/proposals", body, authorToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// Try to withdraw as non-author, non-admin.
	r = authedRequest("DELETE", "/api/v1/proposals/"+proposalID, nil, otherToken)
	w = serveMux(t, db, "DELETE", "/api/v1/proposals/{id}", handler.WithdrawProposal(db), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoteResolutionOnExpiredProposal(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "padmin8", "member")
	voter, voterToken := createTestUser(t, db, "voter8", "member")
	nodeID := createTestNode(t, db, admin.ID, "Resolve Node", "resolve-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	// Create proposal with past voting_ends_at.
	proposalID := auth.NewUUIDv7()
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'open', 'other', 1, ?, ?, ?)`,
		proposalID, nodeID, admin.ID, "Resolve Test", "", pastTime, now, now,
	)
	if err != nil {
		t.Fatalf("insert proposal: %v", err)
	}

	// Add votes directly.
	_, _ = db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, admin.ID)
	_, _ = db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, voter.ID)

	// Trigger resolution by getting the proposal.
	r := httptest.NewRequest("GET", "/api/v1/proposals/"+proposalID, nil)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "approved" {
		t.Errorf("expected status=passed, got %v", result["status"])
	}

	// Verify in DB.
	var dbStatus string
	db.QueryRow("SELECT status FROM proposals WHERE id = ?", proposalID).Scan(&dbStatus)
	if dbStatus != "approved" {
		t.Errorf("expected DB status=approved, got %s", dbStatus)
	}

	// Suppress unused variable warnings.
	_ = adminToken
	_ = voterToken
}

// --- Amendment and Governance Config Tests ---

// setupGovernanceForNode initializes a governance repo for a test node.
func setupGovernanceForNode(t *testing.T, nodeID string) string {
	t.Helper()
	dataDir := t.TempDir()
	governance.SetDataDir(dataDir)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}
	if err := governance.ForkForNode(dataDir, nodeID, "casual"); err != nil {
		t.Fatalf("fork for node: %v", err)
	}
	t.Cleanup(func() { governance.SetDataDir("") })
	return dataDir
}

func TestCreateAmendmentProposal(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amend_admin1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Amendment Node", "amendment-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Set up governance repo.
	setupGovernanceForNode(t, nodeID)

	// Set governance config on the node.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":168,"amendment_threshold":"supermajority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0}`,
		nodeID)

	body := map[string]interface{}{
		"title":          "Amend Community Standards",
		"body":           "Updating the community standards",
		"proposal_type":  "amendment",
		"target_doc":     "community-standards.md",
		"proposed_body":  "# Updated Community Standards\n\nNew content here.",
		"duration_hours": 72,
	}
	r := authedRequest("POST", "/api/v1/nodes/amendment-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["target_doc"] != "community-standards.md" {
		t.Errorf("expected target_doc=community-standards.md, got %v", result["target_doc"])
	}
	if result["proposed_branch"] == nil || result["proposed_branch"] == "" {
		t.Error("expected proposed_branch to be set")
	}
	if result["git_sha"] == nil || result["git_sha"] == "" {
		t.Error("expected git_sha to be set")
	}
	if result["proposal_type"] != "amendment" {
		t.Errorf("expected proposal_type=amendment, got %v", result["proposal_type"])
	}
}

func TestCreateAmendmentProposal_AdminFastTrackApplies(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amend_admin_ft", "member")
	nodeID := createTestNode(t, db, admin.ID, "FastTrack Node", "fasttrack-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	dataDir := setupGovernanceForNode(t, nodeID)

	// Casual-style rules: maintainer leadership + no quorum → admin fast-track.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"leadership_model":"maintainer","succession_policy":"longest_tenure","min_voting_tenure_days":0}`,
		nodeID)

	// DB-canonical lining row the fast-track must sync after merging (adr/011).
	db.Exec(`INSERT INTO governance_docs (id, node_id, title, body, version, created_by) VALUES (?, ?, 'Community Standards', 'old body', 1, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID)

	body := map[string]interface{}{
		"title":         "Fast-track amendment",
		"body":          "apply immediately",
		"proposal_type": "amendment",
		"target_doc":    "community-standards.md",
		"proposed_body": "# Standards\n\nFast-tracked content.",
	}
	r := authedRequest("POST", "/api/v1/nodes/fasttrack-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["state"] != "in_effect" {
		t.Errorf("expected state=in_effect, got %v", result["state"])
	}
	// The status write must survive the schema CHECK — 'passed' used to be
	// silently rejected, leaving fast-tracked amendments 'open' forever.
	if result["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", result["status"])
	}

	// The merge must have landed on the git main branch.
	content, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument after fast-track: %v", err)
	}
	if !strings.Contains(content, "Fast-tracked content") {
		t.Errorf("git main missing amended content, got: %q", content)
	}

	// The DB-canonical doc must be synced to the merged content.
	var docBody string
	db.QueryRow(`SELECT body FROM governance_docs WHERE node_id = ?`, nodeID).Scan(&docBody)
	if !strings.Contains(docBody, "Fast-tracked content") {
		t.Errorf("governance_docs not synced after fast-track, got: %q", docBody)
	}
}

func TestGetAmendmentProposal_IncludesCurrentContent(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amend_admin2", "member")
	nodeID := createTestNode(t, db, admin.ID, "AmendGet Node", "amendget-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	setupGovernanceForNode(t, nodeID)

	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":168,"amendment_threshold":"supermajority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0}`,
		nodeID)

	// Create the amendment proposal.
	body := map[string]interface{}{
		"title":          "Amend Standards Again",
		"body":           "More changes",
		"proposal_type":  "amendment",
		"target_doc":     "community-standards.md",
		"proposed_body":  "# Revised Standards\n\nAll new content.",
		"duration_hours": 48,
	}
	r := authedRequest("POST", "/api/v1/nodes/amendget-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create amendment: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// GET the proposal.
	r = httptest.NewRequest("GET", "/api/v1/proposals/"+proposalID, nil)
	w = servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("get amendment: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)

	// Should include current_doc_content from the governance repo.
	currentContent, ok := result["current_doc_content"].(string)
	if !ok || currentContent == "" {
		t.Error("expected current_doc_content to be a non-empty string")
	}
	// Should include proposed_body.
	proposedBody, ok := result["proposed_body"].(string)
	if !ok || proposedBody == "" {
		t.Error("expected proposed_body to be a non-empty string")
	}
	if proposedBody != "# Revised Standards\n\nAll new content." {
		t.Errorf("unexpected proposed_body: %v", proposedBody)
	}
}

func TestProposalResolution_QuorumNotMet(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "quorum_admin1", "member")
	_, _ = createTestUser(t, db, "quorum_m1", "member")
	m2, _ := createTestUser(t, db, "quorum_m2", "member")
	m3, _ := createTestUser(t, db, "quorum_m3", "member")
	nodeID := createTestNode(t, db, admin.ID, "Quorum Node", "quorum-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Look up m1 ID.
	var m1ID string
	db.QueryRow("SELECT id FROM users WHERE username = 'quorum_m1'").Scan(&m1ID)

	createTestMembership(t, db, m1ID, nodeID, "member", "active")
	createTestMembership(t, db, m2.ID, nodeID, "member", "active")
	createTestMembership(t, db, m3.ID, nodeID, "member", "active")

	// Set governance config with 25% quorum.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":168}`,
		nodeID)

	// Create a proposal with already-expired voting window and NO votes.
	proposalID := auth.NewUUIDv7()
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'open', 'action', 1, ?, ?, ?)`,
		proposalID, nodeID, admin.ID, "Quorum Test No Votes", "", pastTime, now, now,
	)
	if err != nil {
		t.Fatalf("insert proposal: %v", err)
	}

	// Trigger resolution by GET.
	r := httptest.NewRequest("GET", "/api/v1/proposals/"+proposalID, nil)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)

	// Status should remain "open" because quorum (25% of 4 = 1 vote needed) was not met (0 votes).
	if result["status"] != "open" {
		t.Errorf("expected status=open (quorum not met), got %v", result["status"])
	}
}

func TestProposalResolution_QuorumMet_MajorityPasses(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "qpass_admin", "member")
	m1, _ := createTestUser(t, db, "qpass_m1", "member")
	m2, _ := createTestUser(t, db, "qpass_m2", "member")
	m3, _ := createTestUser(t, db, "qpass_m3", "member")
	nodeID := createTestNode(t, db, admin.ID, "QPass Node", "qpass-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, m1.ID, nodeID, "member", "active")
	createTestMembership(t, db, m2.ID, nodeID, "member", "active")
	createTestMembership(t, db, m3.ID, nodeID, "member", "active")

	// 25% quorum => 1 vote needed out of 4 members.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":168}`,
		nodeID)

	// Create proposal with expired voting window.
	proposalID := auth.NewUUIDv7()
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'open', 'action', 1, ?, ?, ?)`,
		proposalID, nodeID, admin.ID, "Quorum Pass Test", "", pastTime, now, now,
	)
	if err != nil {
		t.Fatalf("insert proposal: %v", err)
	}

	// 2 of 4 members vote approve (50% > 25% quorum, majority passes).
	db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, m1.ID)
	db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, m2.ID)

	// Trigger resolution via GET.
	r := httptest.NewRequest("GET", "/api/v1/proposals/"+proposalID, nil)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", result["status"])
	}

	// Verify in DB.
	var dbStatus string
	db.QueryRow("SELECT status FROM proposals WHERE id = ?", proposalID).Scan(&dbStatus)
	if dbStatus != "approved" {
		t.Errorf("expected DB status=approved, got %s", dbStatus)
	}
}

func TestProposalResolution_AmendmentAutoApply(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "auto_admin", "member")
	voter, _ := createTestUser(t, db, "auto_voter", "member")
	nodeID := createTestNode(t, db, admin.ID, "AutoApply Node", "autoapply-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	dataDir := setupGovernanceForNode(t, nodeID)

	// Governance config: majority, 0% quorum (always met), auto_apply enabled.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":168,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`,
		nodeID)

	// Create amendment proposal with expired voting window.
	proposalID := auth.NewUUIDv7()
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	branchName := "amendment-" + proposalID[:8]

	// Create the git branch with the proposed content.
	proposedContent := "# Amended Community Standards\n\nThis is the auto-applied amendment."
	sha, err := governance.CreateBranch(dataDir, nodeID, branchName, "community-standards.md", proposedContent, "Test Admin", "admin@test.local", "Proposed amendment")
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}

	_, err = db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, target_doc, proposed_branch, proposed_body, git_sha) VALUES (?, ?, ?, ?, ?, 'open', 'amendment', 1, ?, ?, ?, ?, ?, ?, ?)`,
		proposalID, nodeID, admin.ID, "Auto Apply Amendment", "Testing auto apply", pastTime, now, now, "community-standards.md", branchName, proposedContent, sha,
	)
	if err != nil {
		t.Fatalf("insert amendment proposal: %v", err)
	}

	// Add approve votes (majority).
	db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, admin.ID)
	db.Exec("INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')", auth.NewUUIDv7(), proposalID, voter.ID)

	// Trigger resolution via GET.
	r := httptest.NewRequest("GET", "/api/v1/proposals/"+proposalID, nil)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", result["status"])
	}

	// Verify the governance doc was updated (branch merged).
	content, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("get document after merge: %v", err)
	}
	if content != proposedContent {
		t.Errorf("expected merged content=%q, got %q", proposedContent, content)
	}
}

func TestVoteOnProposal_TenureCheck(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "tenure_admin", "member")
	newMember, newMemberToken := createTestUser(t, db, "tenure_new", "member")
	nodeID := createTestNode(t, db, admin.ID, "Tenure Node", "tenure-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, newMember.ID, nodeID, "member", "active")

	// Set min_voting_tenure_days=30.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`,
		nodeID)

	// The member joined "just now" (default), so they should not be able to vote.

	// Create proposal.
	body := map[string]interface{}{"title": "Tenure Test", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/tenure-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create proposal: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// New member tries to vote.
	voteBody := map[string]string{"value": "approve"}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, newMemberToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for new member tenure check, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoteOnProposal_TenureCheck_OldMemberPasses(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "tenure_admin2", "member")
	oldMember, oldMemberToken := createTestUser(t, db, "tenure_old", "member")
	nodeID := createTestNode(t, db, admin.ID, "Tenure2 Node", "tenure2-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, oldMember.ID, nodeID, "member", "active")

	// Set min_voting_tenure_days=30.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`,
		nodeID)

	// Backdate the member's joined_at to 60 days ago.
	sixtyDaysAgo := time.Now().UTC().Add(-60 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec("UPDATE memberships SET joined_at = ? WHERE user_id = ? AND node_id = ?", sixtyDaysAgo, oldMember.ID, nodeID)

	// Create proposal.
	body := map[string]interface{}{"title": "Tenure Pass Test", "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/tenure2-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create proposal: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	createResult := decodeJSON(t, w)
	proposalID := createResult["id"].(string)

	// Old member votes -- should succeed.
	voteBody := map[string]string{"value": "approve"}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", voteBody, oldMemberToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for old member vote, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateProposal_BroadcastsActivity(t *testing.T) {
	db := setupTestDB(t)

	ap.SetDomain("broadcast.test.example.com")
	t.Cleanup(func() { ap.SetDomain("") })

	admin, adminToken := createTestUser(t, db, "bc_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Broadcast Node", "broadcast-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Add an ap_follower so BroadcastToFollowers has a target inbox.
	followerID := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted) VALUES (?, 'node', ?, 'https://remote.example.com/ap/users/remote1', 'https://remote.example.com/inbox', 1)`,
		followerID, nodeID,
	)
	if err != nil {
		t.Fatalf("insert ap_follower: %v", err)
	}

	// Create proposal via handler.
	body := map[string]interface{}{
		"title":          "Broadcast Test Proposal",
		"body":           "This proposal should trigger a broadcast.",
		"proposal_type":  "action",
		"duration_hours": 48,
	}
	r := authedRequest("POST", "/api/v1/nodes/broadcast-node/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for the goroutine to complete.
	time.Sleep(100 * time.Millisecond)

	// Check ap_outbox_queue for a pending entry.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue WHERE status = 'pending'").Scan(&count)
	if err != nil {
		t.Fatalf("query outbox queue: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least 1 pending entry in ap_outbox_queue, got %d", count)
	}

	// Verify the activity JSON contains the Create type and proposal title.
	var activityJSON string
	err = db.QueryRow("SELECT activity_json FROM ap_outbox_queue WHERE status = 'pending' LIMIT 1").Scan(&activityJSON)
	if err != nil {
		t.Fatalf("query activity_json: %v", err)
	}
	if !strings.Contains(activityJSON, `"type":"Create"`) {
		t.Errorf("expected activity_json to contain '\"type\":\"Create\"', got: %s", activityJSON)
	}
	if !strings.Contains(activityJSON, "Broadcast Test Proposal") {
		t.Errorf("expected activity_json to contain proposal title, got: %s", activityJSON)
	}
}
