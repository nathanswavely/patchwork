package handler

import (
	"log"
	"sort"
	"strconv"
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
//
// Holdover is also a sentence, and the sentence has to be true (docs/adr/106).
// "The council continues until a successor is elected" describes a council
// that exists. Said to a patch with no admins at all it is a comfortable lie,
// and five simulated members read it five times over a year while their co-op
// had nobody in charge. One of them stopped reading the page because of it:
// "Reading a comforting sentence I know to be false, five times in a row, is
// what made me stop trusting the rest of the page." So this file asks the
// council what it is before it says what happened to it.

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

	// The chairs this contest is for (docs/adr/103). Everything below touches
	// these and nothing else.
	chairs := contestedSeats(db, nodeID, proposalID, seats)

	tally := tallyElection(db, proposalID)
	if len(tally) == 0 {
		closeElectionUnsettled(db, proposalID, nodeID, slug, nodeName, "No candidates stood.")
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
		closeElectionUnsettled(db, proposalID, nodeID, slug, nodeName, "Not enough people voted.")
		return true
	}

	// Only candidates somebody approved can take a seat. An election nobody
	// voted in seats nobody, rather than handing the council to whoever
	// happened to stand.
	var winners []electionTallyRow
	for _, t := range tally {
		if t.Approvals == 0 || len(winners) == len(chairs) {
			break
		}
		winners = append(winners, t)
	}
	if len(winners) == 0 {
		closeElectionUnsettled(db, proposalID, nodeID, slug, nodeName, "No candidate was approved by anyone.")
		return true
	}

	seatWinners(db, nodeID, slug, nodeName, proposalID, winners, chairs, electionTermEnd(gc))

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
//
// `why` is the half of the sentence about the contest; holdoverLine is the
// half about the council, and it is the half that can be false (docs/adr/106).
func closeElectionUnsettled(db *database.DB, proposalID, nodeID, slug, nodeName, why string) {
	why = why + " " + holdoverLine(db, nodeID)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	// `status` stays inside the schema's CHECK (open/approved/rejected/
	// withdrawn) and `state` carries the truth — the same split docs/adr/097
	// made for a lapse, for the same reason: a fifth status would be a
	// migration for a word. Without a state of its own this wrote 'rejected'
	// and the banner read "This proposal did not pass. 0 approved, 0
	// rejected." over a contest nobody voted in, while the notice and the
	// record beside it correctly said it settled nothing. A simulated
	// candidate read that as the community turning him down.
	db.Exec(`UPDATE proposals SET status = 'rejected', state = 'unsettled', updated_at = ? WHERE id = ?`, now, proposalID)
	// The chairs go back to being ordinary chairs. Holdover means nothing
	// happened to them, and a chair still marked as being decided would show
	// a contest on the governance page that has already closed.
	db.Exec(`UPDATE seats SET contested_in = NULL WHERE contested_in = ?`, proposalID)
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

// holdoverLine says what an unsettled contest leaves behind, and it is the
// one sentence here that can be wrong.
//
// With somebody still in a chair, holdover is the reassuring and true thing:
// the sitting council carries on. With every chair empty there is no council
// to carry on, and saying there is tells a patch in trouble that it is fine.
// docs/adr/102 made an empty council a state the product accepts; this is the
// product admitting to it at the moment it happens, in the notification, the
// first time rather than the fifth.
func holdoverLine(db *database.DB, nodeID string) string {
	var held, total int
	db.QueryRow(`SELECT COUNT(holder_id), COUNT(*) FROM seats WHERE node_id = ?`, nodeID).Scan(&held, &total)
	if held > 0 {
		return "The council continues until a successor is elected."
	}
	// No chairs at all is not this file's problem to explain — a patch with
	// no seats has no contest either — but it must not claim empty ones.
	if total == 0 {
		return "Nobody was elected."
	}
	seats := "seats are"
	if total == 1 {
		seats = "seat is"
	}
	return "Nobody was elected, so all " + strconv.Itoa(total) + " " + seats +
		" still empty and this patch has no admins."
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

// contestedSeats is which chairs this contest decides, oldest chair first —
// the order winners are seated in.
//
// The set is frozen when the contest opens (docs/adr/103). A contest that
// opened before `seats.contested_in` existed names none, so it is
// reconstructed the way the calendar picked it: the chairs whose terms end
// soonest, as many as the contest counts. That is a reading of history, not a
// guess about intent — `scheduleFor` chose exactly that set, in exactly that
// order, and a contest in flight when a server upgrades has to resolve
// somehow.
func contestedSeats(db *database.DB, nodeID, proposalID string, seats int) []string {
	chairs := seatsMarked(db, proposalID)
	if len(chairs) > 0 {
		return chairs
	}
	if seats <= 0 {
		return nil
	}
	rows, err := db.Query(`SELECT id FROM seats WHERE node_id = ?
	                       ORDER BY COALESCE(term_ends_at,'9999-12-31') ASC, created_at ASC
	                       LIMIT ?`, nodeID, seats)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			chairs = append(chairs, id)
		}
	}
	return chairs
}

// seatsMarked lists the chairs a contest claimed when it opened.
func seatsMarked(db *database.DB, proposalID string) []string {
	rows, err := db.Query(`SELECT id FROM seats WHERE contested_in = ? ORDER BY created_at ASC`, proposalID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// seatWinners fills the chairs the contest was for. Winners take those chairs
// and the admin role; whoever held one of them and was not returned steps
// down, which is the other half of what an election decides.
//
// **Only those chairs.** This used to refill the council in created order and
// step down every admin it had not seated, which is right for a contest
// covering the whole council and wrong for every other kind. A council with
// one chair ending in March and two running to the following year put its
// March chair up, and the resolution seated the winner in the *oldest* chair,
// emptied the other two, and removed two admins nobody had voted out
// (docs/adr/103). Staggering is what docs/adr/051 put the clock on the seat
// for, and SetSeatTerm is the box that makes it; the contest had to learn
// which chairs it was for.
//
// The last-admin floor still holds. Every winner becomes an admin before
// anyone steps down, so a resolved election cannot empty a patch.
func seatWinners(db *database.DB, nodeID, slug, nodeName, proposalID string, winners []electionTallyRow, chairs []string, termEnds string) {
	// Who sat in a contested chair going in. Read before anything is written:
	// these are the people the contest is deciding about, and they are the
	// only people it may unseat.
	incumbent := map[string]bool{}
	for _, seatID := range chairs {
		var holder string
		db.QueryRow(`SELECT COALESCE(holder_id,'') FROM seats WHERE id = ?`, seatID).Scan(&holder)
		if holder != "" {
			incumbent[holder] = true
		}
	}

	// Reuse the contested chairs rather than making new ones: a seat outlives
	// its holder (docs/adr/051), so an election refills the chairs it was for
	// rather than replacing the furniture.
	seated := map[string]bool{}
	for i, wnr := range winners {
		if i >= len(chairs) {
			break // more winners than chairs is not something a tally can produce
		}
		seated[wnr.UserID] = true
		db.Exec(`UPDATE seats SET holder_id = ?, term_ends_at = ?, contested_in = NULL WHERE id = ?`,
			wnr.UserID, nullIfEmpty(termEnds), chairs[i])

		var role string
		db.QueryRow(`SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'`,
			wnr.UserID, nodeID).Scan(&role)
		if role == "admin" {
			continue
		}
		if _, err := db.Exec(`UPDATE memberships SET role = 'admin', `+roleSinceNow+`
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

	// A contested chair beyond the ones just filled is vacant: it was put to
	// the electorate and nobody won it. The chair stays for the next contest.
	for i := len(winners); i < len(chairs); i++ {
		db.Exec(`UPDATE seats SET holder_id = NULL, contested_in = NULL WHERE id = ?`, chairs[i])
	}

	// Step down the incumbents the electorate did not return — and only them.
	// An admin holding an uncontested chair was not on this ballot and keeps
	// both chair and role.
	var toStepDown []string
	for uid := range incumbent {
		if seated[uid] {
			continue
		}
		// Somebody who also holds a chair this contest was not for stays: the
		// role follows the chairs, and they still have one.
		var stillSeated int
		db.QueryRow(`SELECT COUNT(*) FROM seats WHERE node_id = ? AND holder_id = ?`, nodeID, uid).Scan(&stillSeated)
		if stillSeated == 0 {
			toStepDown = append(toStepDown, uid)
		}
	}
	sort.Strings(toStepDown) // a stable order, so the last-admin floor is not a coin toss

	for _, uid := range toStepDown {
		var admins int
		db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&admins)
		if admins <= 1 {
			break
		}
		if _, err := db.Exec(`UPDATE memberships SET role = 'member', `+roleSinceNow+`
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
	// Only on a patch that is still here. `ScheduleDueElections` has always
	// filtered this way and this sweep never did, so an archived patch went
	// on holding the contest it was carrying when it was archived: opening
	// voting on the calendar, notifying its members, and seating a council
	// nobody can go and look at. A simulated collective created a duplicate
	// patch by accident, archived it, and a fortnight later its ghost
	// election had closed nominations and opened a ballot.
	//
	// The same condition `SweepProposals` uses, and for the same reason.
	rows, err := db.Query(`SELECT p.id FROM proposals p
	                       JOIN nodes n ON n.id = p.node_id
	                       WHERE p.status = 'open' AND p.seats_contested > 0
	                         AND n.status IN ('active', 'unclaimed') AND n.removed_at IS NULL`)
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

	// And open the ones that have come due. Runs after resolution so a council
	// seated on this pass is not immediately found overdue on the same one.
	ScheduleDueElections(db)
}
