package handler

import (
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
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

// electionTurnout is how many people took part in a contest and how many had
// to. It is the answer to the only question a failed election was never asked
// on its own page.
//
// An ordinary proposal has said this for as long as it has existed: "Quorum
// met (3 of 4 voted, 50% needed)", the voters named underneath, the terms it
// was judged by beside them. A contest said "Settled nothing." A founder
// whose patch had failed six of them in ten months found the number she
// needed on the *rules form*, two clicks away and phrased about a
// hypothetical proposal — "2 of the 4 people who can vote today must cast a
// ballot" — and worked out from it that her election had failed by one person
// not turning up. Her words: one kind of decision on this site explains
// itself completely, and the kind that decides who runs her press does not.
//
// One struct, computed one way, so the number a member reads is the number
// that decided. resolveElection asks this too rather than counting for
// itself.
type electionTurnout struct {
	Voted    int `json:"voted"`
	Eligible int `json:"eligible"`
	// Needed is the count of ballots quorum takes, not the percentage. "2
	// needed" is a sentence somebody can act on.
	Needed int  `json:"needed"`
	Met    bool `json:"met"`
}

// ballotsCast counts the people whose ballots this contest counts.
//
// Deliberately the same `countedBallot` predicate the candidate tally uses.
// It was not: the tally excluded ballots from people the electorate no longer
// counts, and quorum's own count did not, so the numerator could include a
// voter the denominator had already dropped — a departed member pushing a
// contest over quorum while contributing to nobody's approvals, and turnout
// able to exceed the electorate. Two populations, one fraction.
func ballotsCast(db *database.DB, proposalID string) int {
	var n int
	// Approvals and abstentions together: turning up and approving nobody is
	// taking part (F-092), which is what `votes.value = 'abstain'` has always
	// meant on an ordinary proposal. DISTINCT over the union because the two
	// are exclusive per voter by construction and a UNION of voter ids is
	// cheaper to read than a pair of counts that must not double-count.
	db.QueryRow(`
		SELECT COUNT(DISTINCT v.voter_id) FROM (
			SELECT b.voter_id, p.node_id FROM election_ballots b
			  JOIN proposals p ON p.id = b.proposal_id
			 WHERE b.proposal_id = ?
			UNION
			SELECT a.voter_id, p.node_id FROM election_abstentions a
			  JOIN proposals p ON p.id = a.proposal_id
			 WHERE a.proposal_id = ?
		) v
		JOIN memberships m ON m.user_id = v.voter_id AND m.node_id = v.node_id
		WHERE `+countedBallot, proposalID, proposalID).Scan(&n)
	return n
}

// abstained reports whether this viewer has taken part without approving
// anybody, so the panel can show the ballot they actually hold.
func abstained(db *database.DB, proposalID, viewerID string) bool {
	if viewerID == "" {
		return false
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM election_abstentions WHERE proposal_id = ? AND voter_id = ?`,
		proposalID, viewerID).Scan(&n)
	return n > 0
}

// electionTurnoutFor is electionTurnoutOf for the proposal payload: nil on
// every proposal that is not an election, so the page can test one field
// rather than re-deriving whether it is looking at a contest.
func electionTurnoutFor(db *database.DB, seatsContested int, proposalID, nodeID string, gc model.GovernanceConfig) *electionTurnout {
	if seatsContested <= 0 {
		return nil
	}
	t := electionTurnoutOf(db, proposalID, nodeID, gc)
	return &t
}

// electionTurnoutOf reads a contest's turnout against the terms it is judged
// by. gc is the frozen photograph (docs/adr/047), never the patch's live
// rules, for the same reason every other reader of a vote's terms is.
func electionTurnoutOf(db *database.DB, proposalID, nodeID string, gc model.GovernanceConfig) electionTurnout {
	voted := ballotsCast(db, proposalID)
	eligible, _ := eligibleVoters(db, nodeID, gc)
	return electionTurnout{
		Voted:    voted,
		Eligible: eligible,
		Needed:   votesNeededForQuorum(gc, eligible),
		Met:      quorumReached(gc, voted, eligible),
	}
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
	// Seated is the stored outcome for this candidate, written when the
	// contest resolved. The page used to work it out — top `seats_contested`
	// with at least one approval — which is a tally, and a tally moves after
	// the fact when a voter leaves the patch.
	Seated bool `json:"seated"`
	// Statement is why they are standing, in their own words (F-095). Empty
	// where they wrote none, which stays the common case on a small patch.
	Statement string `json:"statement,omitempty"`
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
		SELECT c.id, c.user_id, `+usernameExpr("u")+`, `+displayNameExpr("u")+`,
		       (SELECT COUNT(*) FROM election_ballots b
		          JOIN memberships m ON m.user_id = b.voter_id AND m.node_id = p.node_id
		          WHERE b.candidate_id = c.id AND `+countedBallot+`) AS approvals,
		       (SELECT COUNT(*) FROM election_ballots b2
		          WHERE b2.candidate_id = c.id AND b2.voter_id = ?) AS mine,
		       c.seated, c.statement
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
		var seated int
		if rows.Scan(&c.ID, &c.UserID, &c.Username, &c.DisplayName, &c.Approvals, &mine, &seated, &c.Statement) != nil {
			continue
		}
		c.ApprovedByMe = mine > 0
		c.Seated = seated == 1
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
//
// Read off the same chairs the calendar reads (docs/adr/108), so the date a
// member is given and the contest that opens are answers about one set.
func nextTermEnd(db *database.DB, nodeID string) string {
	seats := calendarSeats(db, nodeID)
	if len(seats) == 0 {
		return ""
	}
	return seats[0].termEnd // soonest first
}
