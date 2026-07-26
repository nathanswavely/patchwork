package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Changing a patch's rules notified nobody: governance.rules_changed was
// declared in the registry and never fired. The one governance event that
// changes what every future vote means was the only silent one.
func TestRulesChange_NotifiesThePatch(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	admin, adminToken := createTestUser(t, db, "rc_admin", "member")
	member, _ := createTestUser(t, db, "rc_member", "member")
	follower, _ := createTestUser(t, db, "rc_follower", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rules Notice", "rules-notice", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	setupGovernanceForNode(t, nodeID)

	// Admin-decides rules, so the admin's change applies on submission — the
	// direct-change path (docs/adr/041).
	setNodeRules(t, db, nodeID,
		`{"decision_method":"admin","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`)

	// A vote already running, which the change will not reach.
	openBefore := createProposalVia(t, db, "rules-notice", adminToken, "Running before the change")

	body := map[string]interface{}{
		"title":          "Require thirty days before voting",
		"proposal_type":  "amendment",
		"target_doc":     "governance-rules.json",
		"proposed_body":  `{"decision_method":"admin","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":30}`,
		"duration_hours": 48,
	}
	r := authedRequest("POST", "/api/v1/nodes/rules-notice/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("rules change: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// The member hears about it.
	if n := countNotifications(t, db, member.ID, notifications.GovernanceRulesChanged, 1); n != 1 {
		t.Errorf("member notifications = %d, want 1", n)
	}

	// The follower does not: this type is AudienceAllMembers, which is admins
	// and members and never followers — the same set as everywhere else the
	// word member is load-bearing (CONTEXT.md, "Member count").
	if n := countNotifications(t, db, follower.ID, notifications.GovernanceRulesChanged, 0); n != 0 {
		t.Errorf("follower notifications = %d, want 0", n)
	}

	// And the notice says the running vote keeps its own terms.
	var noticeBody string
	db.QueryRow(
		"SELECT COALESCE(body,'') FROM notifications WHERE user_id = ? AND type = ?",
		member.ID, string(notifications.GovernanceRulesChanged),
	).Scan(&noticeBody)
	if noticeBody == "" || !strings.Contains(noticeBody, "already open") {
		t.Errorf("notice body does not mention the running vote: %q", noticeBody)
	}

	// The vote that was already running kept its terms, which is what the
	// notice is warning about.
	var terms string
	db.QueryRow("SELECT COALESCE(voting_terms,'') FROM proposals WHERE id = ?", openBefore).Scan(&terms)
	if terms == "" || strings.Contains(terms, `"min_voting_tenure_days":30`) {
		t.Errorf("the running vote should still hold the old terms, got %q", terms)
	}
}

// A rules sync that changes no rule is not a rules change, and must not mail
// the patch — amendments to prose documents run through the same path.
func TestRulesChange_SilentWhenNothingMoved(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	admin, adminToken := createTestUser(t, db, "rc2_admin", "member")
	member, _ := createTestUser(t, db, "rc2_member", "member")
	nodeID := createTestNode(t, db, admin.ID, "Prose Only", "prose-only", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	setupGovernanceForNode(t, nodeID)
	setNodeRules(t, db, nodeID,
		`{"decision_method":"admin","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`)

	// An amendment to a prose charter, not to the rules.
	body := map[string]interface{}{
		"title":          "Reword the standards",
		"proposal_type":  "amendment",
		"target_doc":     "community-standards.md",
		"proposed_body":  "# Standards\n\nReworded.",
		"duration_hours": 48,
	}
	r := authedRequest("POST", "/api/v1/nodes/prose-only/proposals", body, adminToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r); w.Code != http.StatusCreated {
		t.Fatalf("prose amendment: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if n := countNotifications(t, db, member.ID, notifications.GovernanceRulesChanged, 0); n != 0 {
		t.Errorf("rules-changed notifications = %d, want 0 — no rule moved", n)
	}
}
