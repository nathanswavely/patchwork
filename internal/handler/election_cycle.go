package handler

import (
	"encoding/json"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The recurring cycle (docs/adr/051): "every cycle after is scheduled from
// when the last one seated the council."
//
// Nothing stores a next-election date. The seats already carry their term
// ends, so being due is derivable: a council whose term runs out inside the
// time a whole contest takes is a council that needs one starting now. That
// keeps one fact in one place — the alternative, a `next_election_at` beside
// `term_ends_at`, is two descriptions of the same thing waiting to disagree,
// which is the failure this run of ADRs keeps finding.
//
// It is also what makes staggering free. Seats due at different dates simply
// come due at different times; nothing here assumes they share one.

// ScheduleDueElections opens a contest for every patch whose council is close
// enough to the end of its term that the election must start now to seat a
// successor on time.
func ScheduleDueElections(db *database.DB) {
	rows, err := db.Query(`SELECT id, slug, COALESCE(governance_config,'{}')
	                       FROM nodes WHERE status = 'active' AND removed_at IS NULL`)
	if err != nil {
		return
	}
	type nodeRow struct{ id, slug, gc string }
	var nodes []nodeRow
	for rows.Next() {
		var n nodeRow
		if rows.Scan(&n.id, &n.slug, &n.gc) == nil {
			nodes = append(nodes, n)
		}
	}
	rows.Close()

	for _, n := range nodes {
		var gc model.GovernanceConfig
		if json.Unmarshal([]byte(n.gc), &gc) != nil {
			continue
		}
		// Patchwork only runs the calendar for patches that elect here. Where
		// the venue is elsewhere the community keeps its own calendar and
		// records the result (docs/adr/052).
		if gc.LeadershipModel != "elected" || gc.LeadershipVenue == "elsewhere" {
			continue
		}
		// A patch that sets no term length has a council serving until the
		// next election, and never schedules one. That is a real position —
		// elected once, then stable — and the honest thing is to leave it
		// alone rather than invent a cadence it never asked for.
		if gc.AdminTermMonths <= 0 {
			continue
		}
		scheduleFor(db, n.id, n.slug, gc)
	}
}

func scheduleFor(db *database.DB, nodeID, slug string, gc model.GovernanceConfig) {
	// One contest at a time. A second concurrent election for the same council
	// would split the electorate between two slates deciding one thing.
	var open int
	db.QueryRow(`SELECT COUNT(*) FROM proposals
	             WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&open)
	if open > 0 {
		return
	}

	lead := electionLeadHours(gc)
	dueBy := time.Now().UTC().Add(time.Duration(lead) * time.Hour).Format("2006-01-02")

	// Seats whose term ends within the time a contest takes. A term already
	// past counts — that council is overdue, and holdover has been carrying it.
	var due int
	db.QueryRow(`SELECT COUNT(*) FROM seats
	             WHERE node_id = ? AND term_ends_at IS NOT NULL AND term_ends_at <= ?`,
		nodeID, dueBy).Scan(&due)
	if due == 0 {
		return
	}

	// An election that settled nothing does not get retried immediately. The
	// council is overdue either way, and reopening the same contest the day it
	// failed is the alert that cries wolf (docs/adr/047's reasoning about
	// notices people learn to ignore). One full contest length is the breather
	// — the same span the patch just failed to use.
	var lastFailedAt string
	db.QueryRow(`SELECT COALESCE(MAX(updated_at),'') FROM proposals
	             WHERE node_id = ? AND seats_contested > 0 AND status = 'rejected'`, nodeID).Scan(&lastFailedAt)
	if lastFailedAt != "" {
		if when, err := time.Parse("2006-01-02T15:04:05.000Z", lastFailedAt); err == nil {
			if time.Since(when) < time.Duration(lead)*time.Hour {
				return
			}
		}
	}

	if id := openElectionFor(db, nodeID, gc, due); id != "" {
		log.Printf("election: %s is due (%d seat(s) at term end), opened %s", slug, due, id)
	}
}
