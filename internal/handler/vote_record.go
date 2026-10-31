package handler

import (
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// couldHaveVoted reports whether a viewer who cast no ballot on a closed
// proposal was someone who could have cast one (F-070).
//
// A settled vote box was three counts, and the only way to find your own
// ballot was the voter list behind "Show voters". The page now says how you
// voted, and where you did not, it says so only when it is true that you
// could have: somebody who joined after the vote closed, or whose tenure had
// not run by then, or who is the subject the vote recused, did not "not
// vote". Nobody keeps a snapshot of the electorate at closing, so this
// reconstructs it from what is stored: an active admin or member today,
// under this vote's own terms (docs/adr/047), whose tenure had run by the
// time voting closed. Tenure is the patch's effective bar now, which is never
// less than it was then, so the answer errs towards saying nothing.
func couldHaveVoted(db *database.DB, viewerID, nodeID string, gc model.GovernanceConfig, recused, closedAt string) bool {
	if viewerID == "" || closedAt == "" || viewerID == recused {
		return false
	}
	days := effectiveTenureDays(db, nodeID, gc)
	var one int
	return db.QueryRow(
		`SELECT 1 FROM memberships WHERE node_id = ? AND user_id = ? AND `+electorateMembership("")+`
		 AND strftime('%Y-%m-%dT%H:%M:%fZ', joined_at, ?) <= strftime('%Y-%m-%dT%H:%M:%fZ', ?)`,
		nodeID, viewerID, fmt.Sprintf("+%d days", days), closedAt,
	).Scan(&one) == nil
}

// voteClosedAt is when a settled proposal stopped taking ballots, as near as
// the row records it: when it was applied, or else the earlier of its
// deadline and its last write, since a vote can close before its deadline
// (a consensus block, a sole voter) and the row is written as it closes.
func voteClosedAt(p model.Proposal, appliedAt string) string {
	if appliedAt != "" {
		return appliedAt
	}
	if p.VotingEndsAt != nil && *p.VotingEndsAt != "" && (p.UpdatedAt == "" || *p.VotingEndsAt < p.UpdatedAt) {
		return *p.VotingEndsAt
	}
	return p.UpdatedAt
}
