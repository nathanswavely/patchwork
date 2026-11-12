package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Every surface says a supermajority is "at least two thirds". The tally
// compared a float against 0.667, so exactly two thirds failed: a 2-1 vote
// on a patch whose charter said it passes was recorded as rejected.
func TestThresholdMet_TwoThirdsIsTwoThirds(t *testing.T) {
	cases := []struct {
		threshold       string
		approve, reject int
		want            bool
	}{
		{"supermajority", 2, 1, true},
		{"supermajority", 4, 2, true},
		{"supermajority", 6, 3, true},
		{"supermajority", 200, 100, true},
		{"supermajority", 3, 0, true},
		{"supermajority", 1, 1, false},
		{"supermajority", 3, 2, false},
		{"supermajority", 6, 4, false},
		{"supermajority", 199, 100, false},
		{"supermajority", 0, 0, false},

		{"majority", 2, 1, true},
		{"majority", 1, 1, false},
		{"majority", 0, 0, false},

		{"consensus", 3, 0, true},
		{"consensus", 3, 1, false},
		{"consensus", 0, 0, false},
	}
	for _, c := range cases {
		if got := handler.ThresholdMetForTest(c.threshold, c.approve, c.reject); got != c.want {
			t.Errorf("%s %d-%d: got %v, want %v", c.threshold, c.approve, c.reject, got, c.want)
		}
	}
}

// The same edge end to end: three members vote 2-1 on a supermajority patch
// and the clock settles it as approved.
func TestSweepProposals_TwoOfThreeCarriesASupermajority(t *testing.T) {
	rig := newSweepRig(t, "swp-two-thirds", 0)
	rig.db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"supermajority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"supermajority","min_voting_tenure_days":0}`,
		rig.nodeID)
	for i, v := range []string{"approve", "approve", "reject"} {
		if code := castVote(t, rig.db, rig.proposalID, rig.tokens[i+1], v); code != http.StatusOK {
			t.Fatalf("vote %d: %d", i+1, code)
		}
	}
	rig.closeWindow(t)

	handler.SweepProposals(rig.db)

	if status, state := rig.statusAndState(t); status != "approved" || state != "approved" {
		t.Errorf("2-1 on a supermajority patch: status %q state %q, want approved/approved", status, state)
	}
}
