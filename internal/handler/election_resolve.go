package handler

import (
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Closing an election and seating the council (docs/adr/051).
//
// Holdover is the rule for every way this can fail: no candidates, quorum
// unmet, or nobody approved leaves the sitting council exactly where it is.
// "Directors serve until their successors are elected and qualified" is
// boilerplate in real bylaws for a reason — an election that settles nothing
// must never be able to empty a patch.

// resolveElection closes a finished election. Reports whether it resolved.
func resolveElection(db *database.DB, proposalID string) bool {
	var nodeID, votingEnds string
	var seats int
	err := db.QueryRow(
		`SELECT node_id, COALESCE(voting_ends_at,''), seats_contested
		 FROM proposals WHERE id = ? AND status = 'open' AND seats_contested > 0`, proposalID,
	).Scan(&nodeID, &votingEnds, &seats)
	if err != nil || votingEnds == "" {
		return false
	}
	ends, perr := time.Parse("2006-01-02T15:04:05.000Z", votingEnds)
	if perr != nil || time.Now().UTC().Before(ends) {
		return false
	}

	var slug, nodeName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&slug, &nodeName)

	tally := tallyElection(db, proposalID)
	if len(tally) == 0 {
		closeElectionUnsettled(db, proposalID, nodeID, slug, nodeName,
			"No candidates stood. The council continues until a successor is elected.")
		return true
	}

	// Judged by the terms it opened with (docs/adr/047) — for an election that
	// is the photograph taken when nominations closed, not when they opened.
	gc := votingTerms(db, proposalID, nodeID)

	// Quorum counts people who cast a ballot, over the electorate that could
	// have. An approval ballot with nobody on it leaves no rows, so it cannot
	// be told apart from not voting — a real limit of the model, and one that
	// errs toward "not enough people took part" rather than toward seating a
	// council on silence.
	var voted int
	db.QueryRow(`SELECT COUNT(DISTINCT voter_id) FROM election_ballots WHERE proposal_id = ?`, proposalID).Scan(&voted)
	eligible, _ := eligibleVoters(db, nodeID, gc)
	if gc.QuorumPercent > 0 && (eligible == 0 || (voted*100/eligible) < gc.QuorumPercent) {
		closeElectionUnsettled(db, proposalID, nodeID, slug, nodeName,
			"Not enough people voted. The council continues until a successor is elected.")
		return true
	}

	// Only candidates somebody approved can take a seat. An election nobody
	// voted in seats nobody, rather than handing the council to whoever
	// happened to stand.
	var winners []electionTallyRow
	for _, t := range tally {
		if t.Approvals == 0 || len(winners) == seats {
			break
		}
		winners = append(winners, t)
	}
	if len(winners) == 0 {
		closeElectionUnsettled(db, proposalID, nodeID, slug, nodeName,
			"No candidate was approved by anyone. The council continues until a successor is elected.")
		return true
	}

	seatWinners(db, nodeID, slug, nodeName, proposalID, winners, electionTermEnd(gc))

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET status = 'approved', state = 'in_effect', applied_at = ?, updated_at = ?
	         WHERE id = ?`, now, now, proposalID)
	auth.LogAuditEvent(db, "", "election.resolved", "proposal", proposalID,
		`{"node_id":"`+nodeID+`"}`, "")
	log.Printf("election: %s seated %d of %d seat(s)", slug, len(winners), seats)
	return true
}

// closeElectionUnsettled ends an election that decided nothing. The council is
// untouched — that is what holdover means — and the record says so rather than
// reading as though a council had been rejected.
func closeElectionUnsettled(db *database.DB, proposalID, nodeID, slug, nodeName, why string) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET status = 'rejected', state = 'rejected', updated_at = ? WHERE id = ?`, now, proposalID)
	auth.LogAuditEvent(db, "", "election.unsettled", "proposal", proposalID,
		`{"node_id":"`+nodeID+`"}`, "")
	notify(notifications.Event{
		Type: notifications.ProposalRejected, NodeID: nodeID, NodeSlug: slug, NodeName: nodeName,
		EntityID: proposalID,
		Title:    "The election in " + nodeName + " settled nothing",
		Body:     why,
		Link:     weblink.Proposal(slug, proposalID),
	})
	log.Printf("election: %s unsettled", slug)
}

// electionTermEnd is when the council this election seats stops serving. Empty
// where the patch sets no term length — a council that serves until the next
// election rather than one with no end at all.
func electionTermEnd(gc model.GovernanceConfig) string {
	if gc.AdminTermMonths <= 0 {
		return ""
	}
	return time.Now().UTC().AddDate(0, gc.AdminTermMonths, 0).Format("2006-01-02")
}

// seatWinners fills the council. Winners take seats and the admin role; admins
// the electorate did not return step down, which is the other half of what an
// election decides.
//
// The last-admin floor still holds. Every winner becomes an admin before
// anyone steps down, so a resolved election cannot empty a patch.
func seatWinners(db *database.DB, nodeID, slug, nodeName, proposalID string, winners []electionTallyRow, termEnds string) {
	seated := map[string]bool{}

	// Reuse existing seats before making new ones: a seat outlives its holder
	// (docs/adr/051), so an election refills the council's chairs rather than
	// replacing the furniture.
	var existing []string
	rows, err := db.Query(`SELECT id FROM seats WHERE node_id = ? ORDER BY created_at ASC`, nodeID)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				existing = append(existing, id)
			}
		}
		rows.Close()
	}

	for i, wnr := range winners {
		seated[wnr.UserID] = true
		if i < len(existing) {
			db.Exec(`UPDATE seats SET holder_id = ?, term_ends_at = ? WHERE id = ?`,
				wnr.UserID, nullIfEmpty(termEnds), existing[i])
		} else {
			db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
				auth.NewUUIDv7(), nodeID, wnr.UserID, nullIfEmpty(termEnds))
		}

		var role string
		db.QueryRow(`SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'`,
			wnr.UserID, nodeID).Scan(&role)
		if role == "admin" {
			continue
		}
		if _, err := db.Exec(`UPDATE memberships SET role = 'admin'
		                      WHERE user_id = ? AND node_id = ? AND status = 'active'`, wnr.UserID, nodeID); err != nil {
			continue
		}
		notify(notifications.Event{
			Type: notifications.MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: nodeName,
			TargetID: wnr.UserID, EntityID: proposalID,
			Title: "You were elected to the council of " + nodeName,
			Body:  "The election has closed and you hold a seat.",
			Link:  weblink.PatchGovernance(slug),
		})
	}

	// A seat beyond the ones just filled is vacant: its holder was not
	// returned, and the chair stays for the next contest.
	for i := len(winners); i < len(existing); i++ {
		db.Exec(`UPDATE seats SET holder_id = NULL WHERE id = ?`, existing[i])
	}

	sitting, err := db.Query(`SELECT user_id FROM memberships
	                          WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID)
	if err != nil {
		return
	}
	var toStepDown []string
	for sitting.Next() {
		var uid string
		if sitting.Scan(&uid) == nil && !seated[uid] {
			toStepDown = append(toStepDown, uid)
		}
	}
	sitting.Close()

	for _, uid := range toStepDown {
		var admins int
		db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&admins)
		if admins <= 1 {
			break
		}
		if _, err := db.Exec(`UPDATE memberships SET role = 'member'
		                      WHERE user_id = ? AND node_id = ? AND status = 'active'`, uid, nodeID); err != nil {
			continue
		}
		notify(notifications.Event{
			Type: notifications.MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: nodeName,
			TargetID: uid, EntityID: proposalID,
			Title: "Your seat on the council of " + nodeName + " has ended",
			Body:  "The election returned a different council. You are still a member.",
			Link:  weblink.PatchGovernance(slug),
		})
	}
}

// SweepElections moves every election that is due: nominations that have
// closed open their vote, and votes that have ended resolve. This is the
// calendar doing its job — nobody calls an election and nobody closes one
// (docs/adr/051).
func SweepElections(db *database.DB) {
	rows, err := db.Query(`SELECT id FROM proposals WHERE status = 'open' AND seats_contested > 0`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	for _, id := range ids {
		OpenElectionVoting(db, id)
		resolveElection(db, id)
	}
}
