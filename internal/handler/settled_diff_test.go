package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A settled proposal's Changes tab diffs against the text it was made
// against. Once a change applies the live document *is* the proposed body,
// so `current_doc_content` alone made an applied rules change read "No
// changes to governance rules." The payload carries the prior text for a
// settled proposal, read from the charter's history.

func TestSettledProposal_DiffsAgainstTheTextBeforeIt(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "sd_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Settled Diff", "settled-diff", "open")
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	dataDir := setupGovernanceForNode(t, nodeID)
	setNodeRules(t, db, nodeID,
		`{"decision_method":"admin","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`)

	create := func(title, doc, proposed string) string {
		t.Helper()
		body := map[string]interface{}{
			"title":          title,
			"proposal_type":  "amendment",
			"target_doc":     doc,
			"proposed_body":  proposed,
			"duration_hours": 48,
		}
		r := authedRequest("POST", "/api/v1/nodes/settled-diff/proposals", body, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %q: expected 201, got %d: %s", title, w.Code, w.Body.String())
		}
		return decodeJSON(t, w)["id"].(string)
	}
	get := func(id string) map[string]interface{} {
		t.Helper()
		// As the admin: a charter's text is members-only by default
		// (docs/adr/036), and an anonymous read would get none of it.
		r := authedRequest("GET", "/api/v1/proposals/"+id, nil, adminToken)
		w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("get %s: expected 200, got %d: %s", id, w.Code, w.Body.String())
		}
		return decodeJSON(t, w)
	}

	t.Run("an applied rules change", func(t *testing.T) {
		before, err := governance.GetDocument(dataDir, nodeID, "governance-rules.json")
		if err != nil {
			t.Fatalf("read rules: %v", err)
		}
		proposed := `{"decision_method":"admin","membership_policy":"invite_only","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`
		id := create("Invite only", "governance-rules.json", proposed)

		p := get(id)
		if p["state"] != "in_effect" {
			t.Fatalf("fixture: a direct change should be born in effect, got %v", p["state"])
		}
		prior, ok := p["prior_doc_content"].(string)
		if !ok {
			t.Fatalf("an applied change must carry prior_doc_content, got %#v", p["prior_doc_content"])
		}
		if prior != before {
			t.Errorf("prior_doc_content should be the rules before the change\n got: %s\nwant: %s", prior, before)
		}
		if prior == p["proposed_body"] {
			t.Error("prior_doc_content equals the proposed body: the diff would show no change")
		}
		if _, ok := p["prior_doc_unavailable"]; ok {
			t.Error("prior_doc_unavailable set on a change whose history is intact")
		}
	})

	t.Run("an applied charter amendment", func(t *testing.T) {
		before, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
		if err != nil {
			t.Fatalf("read charter: %v", err)
		}
		id := create("Rewrite the standards", "community-standards.md", "# Standards\n\nRewritten.")

		p := get(id)
		if p["current_doc_content"] != "# Standards\n\nRewritten." {
			t.Fatalf("fixture: the live charter should now be the proposed body, got %v", p["current_doc_content"])
		}
		if p["prior_doc_content"] != before {
			t.Errorf("prior_doc_content should be the charter before the amendment, got %v", p["prior_doc_content"])
		}
	})

	t.Run("a pending proposal still compares with the live text", func(t *testing.T) {
		member, memberToken := createTestUser(t, db, "sd_member", "member")
		createTestMembership(t, db, member.ID, nodeID, "member", "active")
		body := map[string]interface{}{
			"title":          "A member asks",
			"proposal_type":  "amendment",
			"target_doc":     "community-standards.md",
			"proposed_body":  "# Standards\n\nAsked for.",
			"duration_hours": 48,
		}
		r := authedRequest("POST", "/api/v1/nodes/settled-diff/proposals", body, memberToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
		if w.Code != http.StatusCreated {
			t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		id := decodeJSON(t, w)["id"].(string)

		p := get(id)
		if p["status"] != "open" {
			t.Fatalf("fixture: a member's request should wait on the admin, got status %v", p["status"])
		}
		if _, ok := p["prior_doc_content"]; ok {
			t.Error("a pending proposal must not carry prior_doc_content: its diff is against what applying would change now")
		}
		if _, ok := p["prior_doc_unavailable"]; ok {
			t.Error("a pending proposal must not carry prior_doc_unavailable")
		}

		// Withdrawn, and then the charter moves. The record keeps the diff it
		// was raised with, not one against a text it never saw.
		asProposed, _ := governance.GetDocument(dataDir, nodeID, "community-standards.md")
		if _, err := db.Exec("UPDATE proposals SET status = 'withdrawn', state = 'withdrawn' WHERE id = ?", id); err != nil {
			t.Fatalf("withdraw: %v", err)
		}
		if _, err := governance.DirectEdit(dataDir, nodeID, "community-standards.md", "# Standards\n\nMoved on.", "t", "t@example.com", "move"); err != nil {
			t.Fatalf("move the charter: %v", err)
		}
		p = get(id)
		if p["prior_doc_content"] != asProposed {
			t.Errorf("a withdrawn proposal should diff against the text it was raised against, got %v", p["prior_doc_content"])
		}
	})

	t.Run("a commit the repo does not hold is said, not papered over", func(t *testing.T) {
		id := create("Lost history", "community-standards.md", "# Standards\n\nAgain.")
		// A repo rebuilt from rows (docs/adr/084) holds none of the old commits.
		if _, err := db.Exec("UPDATE proposals SET git_sha = ? WHERE id = ?", "0123456789abcdef0123456789abcdef01234567", id); err != nil {
			t.Fatalf("point git_sha away: %v", err)
		}
		p := get(id)
		if p["prior_doc_unavailable"] != true {
			t.Errorf("expected prior_doc_unavailable, got %#v", p["prior_doc_unavailable"])
		}
		if _, ok := p["prior_doc_content"]; ok {
			t.Error("prior_doc_content must be absent when it cannot be read; the live text is the wrong answer")
		}
	})
}
