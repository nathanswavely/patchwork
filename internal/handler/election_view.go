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

// liveElection describes the contest a patch is currently running, for the
// governance hub. Nil where there isn't one.
type liveElection struct {
	ID string `json:"id"`
	// Phase is 'nominating' or 'voting' — never 'closed', since a resolved
	// election is history and the hub is about what needs attention now.
	Phase              string `json:"phase"`
	Seats              int    `json:"seats"`
	NominationsCloseAt string `json:"nominations_close_at,omitempty"`
	VotingEndsAt       string `json:"voting_ends_at,omitempty"`
	Candidates         int    `json:"candidates"`
}

// currentElection returns the patch's open contest, if it has one.
//
// The hub had no surface for this at all. During a nomination window —
// typically a fortnight, and the only stretch when standing or putting someone
// forward is possible — the governance page of a patch whose whole leadership
// story is elections said nothing about the election. The needs-a-vote banner
// deliberately stays quiet then (nominations are not a ballot), so quiet was
// all there was.
func currentElection(db *database.DB, nodeID string) *liveElection {
	var e liveElection
	var status string
	err := db.QueryRow(
		`SELECT id, status, seats_contested, COALESCE(nominations_close_at,''), COALESCE(voting_ends_at,'')
		 FROM proposals
		 WHERE node_id = ? AND status = 'open' AND seats_contested > 0
		 ORDER BY created_at DESC LIMIT 1`, nodeID,
	).Scan(&e.ID, &status, &e.Seats, &e.NominationsCloseAt, &e.VotingEndsAt)
	if err != nil {
		return nil
	}
	e.Phase = electionPhase(e.Seats, e.NominationsCloseAt, status)
	if e.Phase == "" || e.Phase == "closed" {
		return nil
	}
	db.QueryRow("SELECT COUNT(*) FROM election_candidates WHERE proposal_id = ?", e.ID).Scan(&e.Candidates)
	return &e
}

// nextTermEnd is when this council next faces the electorate: the earliest
// term end among its seats.
//
// Earliest rather than latest because staggered seats come up separately
// (docs/adr/051 left staggering free by putting the date on the seat), so the
// next date is the one a member is asking about. Empty where the patch sets no
// term length — a council serving until the next election, which is a real
// position rather than an omission.
func nextTermEnd(db *database.DB, nodeID string) string {
	var end string
	db.QueryRow(
		`SELECT COALESCE(MIN(term_ends_at),'') FROM seats
		 WHERE node_id = ? AND term_ends_at IS NOT NULL AND term_ends_at != ''`, nodeID,
	).Scan(&end)
	return end
}
