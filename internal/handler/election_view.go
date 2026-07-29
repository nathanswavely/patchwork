package handler

import (
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// What the proposal page needs to render an election (docs/adr/051).

// electionPhase names where an election stands, so the page does not have to
// work it out from two dates and a status. Empty on every proposal that is not
// an election, which is what tells the page to render an ordinary vote.
//
//	nominating — anyone may stand; no ballot may be cast yet
//	voting     — the slate is fixed and the electorate is deciding
//	closed     — resolved, one way or the other
func electionPhase(seatsContested int, nominationsCloseAt, status string) string {
	if seatsContested <= 0 {
		return ""
	}
	if status != "open" {
		return "closed"
	}
	if electionNominating(nominationsCloseAt) {
		return "nominating"
	}
	return "voting"
}

type candidateView struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Approvals   int    `json:"approvals"`
	// ApprovedByMe is this viewer's own ballot, so the page can render the set
	// they currently hold rather than an empty form they have to rebuild.
	ApprovedByMe bool `json:"approved_by_me"`
}

// electionCandidates lists who is standing, with the approvals each has and
// whether this viewer approved them.
//
// The running count is public while voting is open, which is the same choice
// the ordinary proposal page already makes with its approve/reject tallies.
// Hiding it would be a different doctrine than the rest of governance follows,
// and not one that has been decided.
func electionCandidates(db *database.DB, proposalID, viewerID string) []candidateView {
	out := []candidateView{}
	rows, err := db.Query(`
		SELECT c.id, c.user_id, COALESCE(u.username,''), COALESCE(u.display_name, u.username, ''),
		       (SELECT COUNT(*) FROM election_ballots b
		          JOIN memberships m ON m.user_id = b.voter_id AND m.node_id = p.node_id
		          WHERE b.candidate_id = c.id AND `+countedBallot+`) AS approvals,
		       (SELECT COUNT(*) FROM election_ballots b2
		          WHERE b2.candidate_id = c.id AND b2.voter_id = ?) AS mine
		FROM election_candidates c
		JOIN proposals p ON p.id = c.proposal_id
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.proposal_id = ?
		ORDER BY approvals DESC, c.id ASC`, viewerID, proposalID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c candidateView
		var mine int
		if rows.Scan(&c.ID, &c.UserID, &c.Username, &c.DisplayName, &c.Approvals, &mine) != nil {
			continue
		}
		c.ApprovedByMe = mine > 0
		out = append(out, c)
	}
	return out
}
