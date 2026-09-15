package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The page must say the tenure the gate is running, not the one the patch
// configured (docs/adr/098).
//
// The frozen terms carry `min_voting_tenure_days`, and the vote gate
// enforces the effective number, which on a patch younger than its own bar
// is none. A page reading the frozen number recites a rule nothing is
// running: five simulated members read "voting requires 30 days'
// membership" under a button that took their vote, and one pressed it and
// could not tell whether it had counted.

func proposalPayload(t *testing.T, rig tenureRig, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/proposals/"+rig.proposalID, nil, token)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(rig.db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("get proposal: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

func TestProposalPayload_YoungPatchSaysNoTenure(t *testing.T) {
	rig := newTenureRig(t, "pay-young")
	got := proposalPayload(t, rig, rig.memberToken)

	if days, _ := got["tenure_days"].(float64); days != 0 {
		t.Errorf("tenure_days = %v, want 0 — the patch is younger than its own bar", got["tenure_days"])
	}
	if when, _ := got["vote_eligible_at"].(string); when != "" {
		t.Errorf("vote_eligible_at = %q, want empty — nothing is holding this member back", when)
	}
	if can, _ := got["can_vote"].(bool); !can {
		t.Error("can_vote = false, but the gate admits this member")
	}
	// The frozen terms still carry what the patch configured: this is the
	// number the vote is judged by if the patch outlives its own bar, and
	// docs/adr/047 says it does not move. The payload has both on purpose.
	terms, _ := got["voting_terms"].(map[string]interface{})
	if terms == nil || terms["min_voting_tenure_days"].(float64) != 30 {
		t.Errorf("voting_terms lost the configured tenure: %v", got["voting_terms"])
	}
}

func TestProposalPayload_OlderPatchNamesTheDay(t *testing.T) {
	rig := newTenureRig(t, "pay-old")
	predateNode(t, rig.db, rig.nodeID)
	got := proposalPayload(t, rig, rig.memberToken)

	if days, _ := got["tenure_days"].(float64); days != 30 {
		t.Errorf("tenure_days = %v, want 30 — this patch is old enough to run its rule", got["tenure_days"])
	}
	if can, _ := got["can_vote"].(bool); can {
		t.Error("can_vote = true, but this member is inside the tenure window")
	}
	when, _ := got["vote_eligible_at"].(string)
	want := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
	if when != want {
		t.Errorf("vote_eligible_at = %q, want %q — the day the member may first vote", when, want)
	}
}

// Somebody who may already vote is not told the rule: the line exists to
// answer a refusal, and there is none.
func TestProposalPayload_TenuredMemberIsNotToldTheDate(t *testing.T) {
	rig := newTenureRig(t, "pay-tenured")
	predateNode(t, rig.db, rig.nodeID)
	rig.db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE node_id = ?`, rig.nodeID)

	got := proposalPayload(t, rig, rig.memberToken)
	if when, _ := got["vote_eligible_at"].(string); when != "" {
		t.Errorf("vote_eligible_at = %q, want empty — this member has been here for years", when)
	}
	if can, _ := got["can_vote"].(bool); !can {
		t.Error("can_vote = false for a member of four years")
	}
}

// A follower is not in the room, so tenure is not what is stopping them and
// a date would be a promise the patch never made.
func TestProposalPayload_FollowerGetsNoDate(t *testing.T) {
	rig := newTenureRig(t, "pay-follower")
	predateNode(t, rig.db, rig.nodeID)
	follower, followerToken := createTestUser(t, rig.db, "pay-follower_watcher", "member")
	createTestMembership(t, rig.db, follower.ID, rig.nodeID, "follower", "active")

	got := proposalPayload(t, rig, followerToken)
	if when, _ := got["vote_eligible_at"].(string); when != "" {
		t.Errorf("vote_eligible_at = %q, want empty for a follower", when)
	}
	if can, _ := got["can_vote"].(bool); can {
		t.Error("can_vote = true for a follower")
	}
}
