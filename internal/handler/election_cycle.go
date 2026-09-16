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
//
// A chair nobody wins keeps its clock (docs/adr/108). Nothing moved an empty
// chair's `term_ends_at`, so it stayed permanently overdue and came due again
// on the pass after every breather, for ever: a co-op in the simulation ran
// six contests **exactly 56 days apart, five times running**, under a page
// saying "Each term runs 12 months", and a member worked the cadence out by
// counting gaps in a list. A contest that settles leaves its unfilled chairs
// on the council's own calendar instead, so they come up with the rest of it
// — and in the meantime a vacancy is filled by nomination any day
// (docs/adr/100), which is the route a permanently-overdue calendar was
// standing in for.

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

// calendarSeat is a chair with a term end the calendar reads.
type calendarSeat struct{ id, termEnd string }

// calendarSeats is the chairs whose clocks this calendar runs, soonest first:
// every chair carrying a term end, held or not.
//
// Vacant ones included, which is docs/adr/100's rule that the council's size
// is its seats and "the next contest contests the seats that exist" — an
// admin who adds five chairs has added five contests. What docs/adr/108
// changes is not which chairs are read but what date a vacant one carries:
// a chair nobody wins is put back on the council's calendar rather than left
// with the stale date of whoever sat in it last.
//
// scheduleFor and the dates the pages print both read this, so the contest
// that opens and the date a member is told are answers about the same chairs.
func calendarSeats(db *database.DB, nodeID string) []calendarSeat {
	rows, err := db.Query(`SELECT id, term_ends_at FROM seats
	                       WHERE node_id = ? AND term_ends_at IS NOT NULL AND term_ends_at != ''
	                       ORDER BY term_ends_at ASC, created_at ASC`, nodeID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []calendarSeat
	for rows.Next() {
		var s calendarSeat
		if rows.Scan(&s.id, &s.termEnd) == nil {
			out = append(out, s)
		}
	}
	return out
}

// breatherUntil is the day the calendar may next open a contest here, or
// empty where nothing is holding it back.
//
// An election that settled nothing is not retried immediately: reopening the
// same contest the day it failed is the alert that cries wolf (docs/adr/047's
// reasoning about notices people learn to ignore). One full contest length is
// the breather — the same span the patch just failed to use.
//
// It is a date rather than a hidden early return (docs/adr/108) because the
// pages have to print it. A patch inside its breather used to be told the
// next contest was "due now", every day, for four weeks: a member trying to
// find out when to come back got a date that had already passed and a page
// that then did nothing.
func breatherUntil(db *database.DB, nodeID string, gc model.GovernanceConfig) string {
	var lastFailedAt string
	db.QueryRow(`SELECT COALESCE(MAX(updated_at),'') FROM proposals
	             WHERE node_id = ? AND seats_contested > 0 AND status = 'rejected'`, nodeID).Scan(&lastFailedAt)
	if lastFailedAt == "" {
		return ""
	}
	when, err := parseStoredInstant(lastFailedAt)
	if err != nil {
		return ""
	}
	return when.Add(time.Duration(electionLeadHours(gc)) * time.Hour).Format("2006-01-02")
}

// nextContestOpens is the day the calendar next opens a contest on this
// council, derived from the same two facts scheduleFor derives dueness from:
// the earliest seat term end, and how long a whole contest takes. Nothing
// stores this date and nothing should — a stored one is a second description
// of the seats' own clocks, waiting to disagree with them.
//
// Empty where no contest is coming: a patch that does not elect here, one
// that sets no term length, or one with no dated seat. A date already past
// means the contest is due now and the next sweep opens it.
func nextContestOpens(db *database.DB, nodeID string, gc model.GovernanceConfig) string {
	return contestOpensFor(gc, nextTermEnd(db, nodeID), breatherUntil(db, nodeID, gc))
}

// contestOpensFor is the same arithmetic for one chair: that seat's own term
// end, less the time a whole contest takes.
//
// Per seat rather than per council, because staggering is a policy
// docs/adr/051 deliberately left free — two chairs with different term ends
// come up at different times, and one date printed for the whole council is
// wrong about at least one of them. The governance page states this per row
// now, which is what it takes for a member reading two vacant chairs to learn
// what happens to *those* chairs rather than three general facts about
// councils.
//
// `notBefore` is the patch's breather, if it is inside one: a contest whose
// arithmetic says "now" still cannot open until the breather runs out, and
// the later of the two is the day something will actually happen. Printing
// the earlier one is how a page told a member the contest was due now on
// each of twenty-eight consecutive days (docs/adr/108).
//
// Empty where no contest is coming: a patch that does not elect here, one
// that sets no term length, or a seat with no term end.
func contestOpensFor(gc model.GovernanceConfig, termEnd, notBefore string) string {
	if gc.LeadershipModel != "elected" || gc.LeadershipVenue == "elsewhere" || gc.AdminTermMonths <= 0 {
		return ""
	}
	if termEnd == "" {
		return ""
	}
	end, err := time.Parse("2006-01-02", termEnd)
	if err != nil {
		return ""
	}
	opens := end.Add(-time.Duration(electionLeadHours(gc)) * time.Hour).Format("2006-01-02")
	// Dates are ISO, so a string compare is a date compare.
	if notBefore > opens {
		return notBefore
	}
	return opens
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
	today := time.Now().UTC().Format("2006-01-02")
	dueBy := time.Now().UTC().Add(time.Duration(lead) * time.Hour).Format("2006-01-02")

	// The breather, read as a date so this and the pages agree about it.
	if until := breatherUntil(db, nodeID, gc); until > today {
		return
	}

	// Chairs whose term ends within the time a contest takes. A term already
	// past counts — that council is overdue, and holdover has been carrying it.
	//
	// These chairs, and only these: a staggered council puts up the chairs
	// whose terms have run out and leaves the rest alone (docs/adr/103).
	// Soonest term first, so a contest that fills fewer chairs than it puts up
	// fills the most overdue ones.
	var due []string
	for _, s := range calendarSeats(db, nodeID) {
		if s.termEnd <= dueBy {
			due = append(due, s.id)
		}
	}
	if len(due) == 0 {
		return
	}

	if id := openElectionFor(db, nodeID, gc, due); id != "" {
		log.Printf("election: %s is due (%d seat(s) at term end), opened %s", slug, len(due), id)
	}
}
