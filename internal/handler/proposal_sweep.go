package handler

import (
	"fmt"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// A window that closes settles something (docs/adr/097).
//
// A proposal's vote has a clock, and the clock is what ends it — not the
// next person to open the page. Elections had a sweep from the day they
// existed (docs/adr/051); ordinary proposals resolved only when somebody
// read them after the deadline, so a list said "voting" about a vote that
// had ended, and the resolved notice reached people at whatever hour a
// reader happened by. This is the sweep the proposals were missing, run
// from the same hourly loop as the election one.

// SweepProposals resolves every ordinary proposal whose voting window has
// closed. Elections are the other sweep's; advisory votes land back on the
// maintainer here exactly as they would on a read (docs/adr/092).
func SweepProposals(db *database.DB) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	rows, err := db.Query(`SELECT p.id FROM proposals p
	                       JOIN nodes n ON n.id = p.node_id
	                       WHERE p.status = 'open'
	                         AND COALESCE(p.state, 'voting') = 'voting'
	                         AND COALESCE(p.seats_contested, 0) = 0
	                         AND p.voting_ends_at IS NOT NULL AND p.voting_ends_at <= ?
	                         AND n.status IN ('active', 'unclaimed') AND n.removed_at IS NULL`, now)
	if err != nil {
		log.Printf("proposal sweep: %v", err)
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
		resolveProposal(db, id)
	}
}

// windowClosed says whether a voting window has run out. Both layouts the
// column has ever been written in are read, because the creation path and
// the sweep have not always agreed on one.
func windowClosed(votingEndsAt string) bool {
	if votingEndsAt == "" {
		return false
	}
	ends, err := time.Parse("2006-01-02T15:04:05.000Z", votingEndsAt)
	if err != nil {
		ends, err = time.Parse(time.RFC3339, votingEndsAt)
	}
	return err == nil && time.Now().UTC().After(ends)
}

// lapseProposal closes a vote whose window ran out under quorum. Nobody
// decided anything, so it is neither approved nor rejected in the way the
// word means: the status is the schema's terminal "no" (docs/adr/097
// keeps the CHECK constraint as it is), the state says what actually
// happened, and the record and the notice both say "not decided" rather
// than "failed". Idempotent: the WHERE refuses a proposal already moved.
func lapseProposal(db *database.DB, p model.Proposal, votes int) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(`UPDATE proposals SET status = 'rejected', state = 'lapsed', updated_at = ?
	                     WHERE id = ? AND status = 'open'`, now, p.ID)
	if err != nil {
		log.Printf("proposal %s: lapse failed: %v", p.ID, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return
	}

	var slug, name string
	db.QueryRow(`SELECT slug, name FROM nodes WHERE id = ?`, p.NodeID).Scan(&slug, &name)

	// No actor: the clock closed it, not a person.
	auth.LogAuditEvent(db, "", "proposal.lapsed", "proposal", p.ID,
		fmt.Sprintf(`{"node_id":%q,"votes":%d}`, p.NodeID, votes), "")
	notify(notifications.Event{
		Type:     notifications.ProposalRejected,
		NodeID:   p.NodeID,
		NodeSlug: slug,
		NodeName: name,
		EntityID: p.ID,
		Title:    "Not decided: " + p.Title,
		Body:     "Voting ended without reaching quorum. The proposal lapsed; nobody decided it either way.",
		Link:     weblink.Proposal(slug, p.ID),
	})
	log.Printf("proposal %s in %s lapsed: quorum unmet at close (%d votes)", p.ID, slug, votes)
}
