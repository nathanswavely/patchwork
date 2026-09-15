package handler

import (
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The turnout line has to name a number of ballots, not a percentage: "4
// needed" is a sentence somebody can act on, and "50 % needed" is the one
// five simulated members read and did nothing about. So the quorum bar is
// rounded up to the next whole ballot.
func TestVotesNeededForQuorum(t *testing.T) {
	cases := []struct{ quorum, eligible, want int }{
		{0, 8, 0},
		{50, 8, 4},
		{50, 7, 4},
		{25, 8, 2},
		{33, 3, 1},
		{100, 5, 5},
		{50, 0, 0},
	}
	for _, c := range cases {
		gc := model.GovernanceConfig{QuorumPercent: c.quorum}
		got := votesNeededForQuorum(gc, c.eligible)
		if got != c.want {
			t.Errorf("quorum %d%% of %d: got %d, want %d", c.quorum, c.eligible, got, c.want)
		}
		// And what it names must be enough: one fewer must not satisfy the
		// resolver's own test, or the notice asks for the wrong number.
		if c.want > 0 {
			if !quorumReached(gc, got, c.eligible) {
				t.Errorf("quorum %d%% of %d: %d ballots does not reach quorum", c.quorum, c.eligible, got)
			}
			if quorumReached(gc, got-1, c.eligible) {
				t.Errorf("quorum %d%% of %d: %d ballots already reached quorum, so %d is too many", c.quorum, c.eligible, got-1, got)
			}
		}
	}
}
