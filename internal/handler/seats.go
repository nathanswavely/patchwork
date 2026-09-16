package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// A seat is a chair you can count (docs/adr/100).
//
// The council's size is its seats, and a seat is added and removed
// explicitly. Neither act puts anybody in power or takes anyone out of it —
// filling a seat is nomination and ratification (docs/adr/051), and removal
// can only ever touch an empty chair — so the furniture is administrative
// while sitting in it is not.
//
// There is no second number: the seats table is the council's size, which is
// what makes "the next contest contests the seats that exist" true rather than
// "however many admins happen to hold the role". max_admins is not that
// number and never becomes it (docs/adr/051 retracted its enforcement, and
// migration 041 backfilled 3 into nearly every patch).

// Seat fill routes. One chair, one answer — which is the whole point of
// carrying them (F-067).
//
// A member looked at a council page that said, all at once, "the community
// elects admins for fixed terms", "a vacant seat is filled by nomination", and
// "next seat comes up Sep 1, 2027", and could not tell which sentence was
// about the two empty chairs in front of him. All three were true, of
// different chairs at different times. So the server decides per seat which
// one applies and the page states only that.
const (
	// seatFillContest — a contest is running right now and this chair is in
	// it. It outranks the others for that chair: while the community is
	// deciding a seat, that is what is happening to it. A chair the contest
	// did not put up reads as it always did, because nothing is happening to
	// it (docs/adr/103).
	seatFillContest = "contest_open"
	// seatFillNomination — vacant, and fillable today: an admin puts a name
	// forward and the members ratify (docs/adr/100, docs/adr/051).
	seatFillNomination = "nomination"
	// seatFillElection — vacant, and *not* fillable today, because this
	// patch has no admins and a nomination is raised by one (docs/adr/108).
	// The contest is the way back, so the row says when it opens instead of
	// describing a person who does not exist here. A member read the
	// nomination sentence on a council with nobody in it and said so: "don't
	// tell me a vacant seat is filled by a nomination an admin raises. We
	// have no admins."
	seatFillElection = "contest_fills"
	// seatFillScheduled — held, and contested at the election the calendar
	// opens on ContestOpens. Where that date has arrived, ContestDue says so
	// and holdover is carrying the holder until a successor is elected.
	seatFillScheduled = "contest_scheduled"
	// seatFillNone — held, with no calendar: no term end, or a patch that
	// sets no term length. Elected once, then stable, which is a real
	// position rather than an omission.
	seatFillNone = "none"
)

// seatView is a seat as every surface reads it: the chair, who is in it, when
// its term ends, and what happens to it next. A vacant seat carries no holder
// and still carries a term — the clock belongs to the seat, not the person
// (docs/adr/051).
type seatView struct {
	ID          string `json:"id"`
	HolderID    string `json:"holder_id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	TermEndsAt  string `json:"term_ends_at,omitempty"`
	// Vacant is stated rather than left to the client to infer from an absent
	// holder, because it is the fact the rest of the row turns on.
	Vacant bool `json:"vacant"`
	// Fill is which of the seatFill* routes applies to this chair now.
	Fill string `json:"fill"`
	// ContestOpens is when the calendar opens the contest for this chair,
	// derived from this seat's own term end (docs/adr/051 put the clock on
	// the seat so staggered chairs come up separately). Empty unless Fill is
	// seatFillScheduled.
	ContestOpens string `json:"contest_opens,omitempty"`
	// ContestDue is true when that day has arrived and the sweep has not run
	// yet — the council is overdue and holdover is carrying it.
	ContestDue bool `json:"contest_due,omitempty"`
	// ContestID is the open election deciding this chair, when Fill is
	// seatFillContest, so the row can link to it.
	ContestID string `json:"contest_id,omitempty"`
}

// seatsOf lists a patch's council in the order the chairs were made, which is
// the order a settled contest refills them in, with each chair's own answer to
// "what happens to this one".
func seatsOf(db *database.DB, nodeID string, gc model.GovernanceConfig) []seatView {
	out := []seatView{}
	// contestedIn runs alongside: which contest claimed each chair, if any. It
	// is not part of the payload — what a reader needs is the route, and
	// ContestID already carries the link.
	var contestedIn []string
	rows, err := db.Query(`
		SELECT s.id, COALESCE(s.holder_id,''), COALESCE(`+usernameExpr("u")+`,''),
		       COALESCE(`+displayNameExpr("u")+`,''), COALESCE(s.term_ends_at,''),
		       COALESCE(s.contested_in,'')
		FROM seats s LEFT JOIN users u ON u.id = s.holder_id
		WHERE s.node_id = ? ORDER BY s.created_at ASC`, nodeID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s seatView
		var in string
		if rows.Scan(&s.ID, &s.HolderID, &s.Username, &s.DisplayName, &s.TermEndsAt, &in) != nil {
			continue
		}
		out = append(out, s)
		contestedIn = append(contestedIn, in)
	}
	rows.Close()

	contestID := openContestID(db, nodeID)
	today := time.Now().UTC().Format("2006-01-02")
	// What is holding the calendar back, so a chair's date is the day
	// something happens rather than the day the arithmetic first said "now"
	// (docs/adr/108).
	notBefore := breatherUntil(db, nodeID, gc)
	// Whether anybody is left to raise a nomination at all.
	var admins int
	db.QueryRow(`SELECT COUNT(*) FROM memberships
	             WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&admins)
	nextOpens := contestOpensFor(gc, nextTermEnd(db, nodeID), notBefore)
	for i := range out {
		out[i].Vacant = out[i].HolderID == ""
		switch {
		// Only the chairs the contest put up (docs/adr/103). A staggered
		// council running a one-seat contest has two chairs that nobody is
		// voting on, and telling their holders an election is deciding them
		// is the same false sentence F-067 was about, one layer down.
		case contestID != "" && contestedIn[i] == contestID:
			out[i].Fill = seatFillContest
			out[i].ContestID = contestID
		case out[i].Vacant && admins > 0:
			out[i].Fill = seatFillNomination
		case out[i].Vacant:
			// Nobody to nominate, so the contest fills it. Its own stale term
			// end says nothing useful — it belonged to whoever sat here last
			// — so the date is the council's next contest (docs/adr/108).
			if nextOpens == "" {
				out[i].Fill = seatFillNone
				continue
			}
			out[i].Fill = seatFillElection
			out[i].ContestOpens = nextOpens
			out[i].ContestDue = nextOpens <= today
		default:
			opens := contestOpensFor(gc, out[i].TermEndsAt, notBefore)
			if opens == "" {
				out[i].Fill = seatFillNone
				continue
			}
			out[i].Fill = seatFillScheduled
			out[i].ContestOpens = opens
			out[i].ContestDue = opens <= today
		}
	}
	return out
}

// openContestID is the election this council is running, or empty. Dates are
// ISO, so a string compare is a date compare.
func openContestID(db *database.DB, nodeID string) string {
	var id string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND status = 'open' AND seats_contested > 0
	             ORDER BY created_at DESC LIMIT 1`, nodeID).Scan(&id)
	return id
}

// vacantSeat returns the id of a seat nobody holds and nobody is voting on,
// oldest chair first, or empty when there is none to hand out.
//
// A chair in a running contest is not vacant in the sense this asks about
// (docs/adr/103). It is empty *because* the members are deciding who sits in
// it, and every caller here is a way of putting somebody in a chair without
// an election — a ratified nomination, an interim appointment. Seating one of
// those into the chair being voted on would settle the contest before it
// closed, and the ballot would then unseat them.
func vacantSeat(db *database.DB, nodeID string) string {
	var id string
	db.QueryRow(`SELECT id FROM seats WHERE node_id = ? AND holder_id IS NULL
	             AND (contested_in IS NULL
	                  OR contested_in NOT IN (SELECT id FROM proposals WHERE status = 'open'))
	             ORDER BY created_at ASC LIMIT 1`, nodeID).Scan(&id)
	return id
}

// seatCount is how many chairs the council has.
func seatCount(db *database.DB, nodeID string) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM seats WHERE node_id = ?", nodeID).Scan(&n)
	return n
}

// seatRoom resolves the patch and checks the caller may arrange its
// furniture: an active admin of this patch, and nobody else. An instance
// admin holding no role here is not one — they curate the quilt and do not
// override a patch's choices (CONTEXT.md).
//
// The refusal is a 403, not the noticeboard's 404 (docs/adr/081). That rule
// hides a room whose very existence is private; a council's seats are on the
// public governance page, so there is nothing here to hide and "node not
// found" would deny a patch the caller is looking at. What is private is the
// button, and 403 is how you say that.
//
// Returns the node id and true, or writes the error and returns false.
func seatRoom(db *database.DB, w http.ResponseWriter, r *http.Request) (string, bool) {
	user := middleware.UserFromContext(r.Context())
	nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
	if nodeID == "" {
		http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
		return "", false
	}
	if user == nil || !userHasNodeRole(db, user.ID, nodeID, "admin") {
		http.Error(w, `{"error":"only this patch's admins can change its council seats"}`, http.StatusForbidden)
		return "", false
	}

	// Where the council is decided elsewhere, the attestation supplies it
	// (docs/adr/052) — seats there would be a second home for what the record
	// already says, and the next attestation would overwrite them anyway.
	if leadershipDecidedElsewhere(db, nodeID) {
		http.Error(w, `{"error":"this patch chooses its council elsewhere: record that decision instead"}`, http.StatusConflict)
		return "", false
	}
	// Only `elected` has seats. A maintainer designates a successor and a
	// meritocratic patch ratifies a nomination; neither has a term for a chair
	// to hold, so a seat there would carry an occupant and nothing else, which
	// is what the membership row already is (docs/adr/051).
	if leadershipModel(db, nodeID) != "elected" {
		http.Error(w, `{"error":"seats belong to the elected model; this patch makes admins another way"}`, http.StatusConflict)
		return "", false
	}
	return nodeID, true
}

// electedPromotionDenial is what the role dropdown says instead of making an
// admin on an elected patch (docs/adr/100). Two answers, because there are
// two situations and telling someone "you cannot" without telling them what
// can is how Sam ended up raising a consensus vote on himself.
//
// The date is derived, never stored: it is the same arithmetic scheduleFor
// does — the earliest seat's term end, less the length of a whole contest.
func electedPromotionDenial(db *database.DB, nodeID string) string {
	if vacantSeat(db, nodeID) != "" {
		return "this patch elects its council: a seat is vacant, so nominate them for it and the members ratify"
	}
	total := seatCount(db, nodeID)
	full := "this patch elects its council and all " + strconv.Itoa(total) + " seat" + plural(total) + " " +
		isAre(total) + " held"
	gc, _ := electedHere(db, nodeID)
	opens := nextContestOpens(db, nodeID, gc)
	if opens == "" {
		return full + ": no contest is scheduled, so add a seat first"
	}
	if opensAt, err := time.Parse("2006-01-02", opens); err == nil && !time.Now().UTC().Before(opensAt) {
		return full + ": the next contest is due now"
	}
	return full + ": the next contest opens " + opens
}

// isAre agrees the verb with a counted noun, so the copy does not have to.
func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// AddSeat handles POST /api/v1/nodes/{slug}/seats. One more vacant chair.
//
// The new seat inherits the council's next term end, so an aligned council
// stays aligned and the chair is contested at the patch's next scheduled
// contest rather than at a date of its own. A council with no dated seat
// starts one from today. An admin who adds five seats has added five
// contests — what they cannot do is put anybody in them (docs/adr/100).
func AddSeat(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := seatRoom(db, w, r)
		if !ok {
			return
		}

		gc, _ := electedHere(db, nodeID)
		termEnds := nextTermEnd(db, nodeID)
		if termEnds == "" {
			termEnds = electionTermEnd(gc)
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, NULL, ?)`,
			id, nodeID, nullIfEmpty(termEnds),
		); err != nil {
			http.Error(w, `{"error":"failed to add a seat"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "seat.add", "seat", id,
			`{"node_id":"`+nodeID+`","term_ends_at":`+jsonStringOrNull(termEnds)+`}`, clientIP(r))

		total := seatCount(db, nodeID)
		announceCouncil(db, nodeID, user.ID,
			"A seat was added to the council",
			"The council now has "+strconv.Itoa(total)+" seat"+plural(total)+". The new seat is vacant until somebody is elected or nominated into it.")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(seatView{ID: id, TermEndsAt: termEnds})
	}
}

// RemoveSeat handles DELETE /api/v1/nodes/{slug}/seats/{id}.
//
// Only an empty chair. Fusing "dissolve a seat" with "remove a person from a
// seat" would let a patch shrink itself permanently as a side effect of a
// grudge, and leave the record unable to say which argument won
// (docs/adr/051).
func RemoveSeat(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := seatRoom(db, w, r)
		if !ok {
			return
		}
		seatID := r.PathValue("id")

		var holderID, holderName, contestedIn string
		err := db.QueryRow(
			`SELECT COALESCE(s.holder_id,''), COALESCE(`+displayNameExpr("u")+`,''), COALESCE(s.contested_in,'')
			 FROM seats s LEFT JOIN users u ON u.id = s.holder_id
			 WHERE s.id = ? AND s.node_id = ?`, seatID, nodeID,
		).Scan(&holderID, &holderName, &contestedIn)
		if err != nil {
			http.Error(w, `{"error":"seat not found"}`, http.StatusNotFound)
			return
		}
		// A chair people are voting on is not furniture to move. It is empty
		// precisely because the contest is deciding who sits in it, and
		// dissolving it mid-ballot would throw away a vote already cast
		// (docs/adr/103).
		if contestedIn != "" && contestedIn == openContestID(db, nodeID) {
			http.Error(w, `{"error":"this seat is in the contest running now: it can be removed once that settles"}`, http.StatusConflict)
			return
		}
		if holderID != "" {
			who := holderName
			if who == "" {
				who = "somebody"
			}
			http.Error(w, `{"error":"`+who+` holds that seat: a seat can only be removed while it is vacant"}`, http.StatusConflict)
			return
		}

		if _, err := db.Exec("DELETE FROM seats WHERE id = ? AND node_id = ?", seatID, nodeID); err != nil {
			http.Error(w, `{"error":"failed to remove the seat"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "seat.remove", "seat", seatID,
			`{"node_id":"`+nodeID+`"}`, clientIP(r))

		total := seatCount(db, nodeID)
		announceCouncil(db, nodeID, user.ID,
			"A vacant seat was removed from the council",
			"The council now has "+strconv.Itoa(total)+" seat"+plural(total)+".")

		w.WriteHeader(http.StatusNoContent)
	}
}

// SetSeatTerm handles PATCH /api/v1/nodes/{slug}/seats/{id}.
//
// `admin_term_months` and `seats.term_ends_at` were readable on four surfaces
// and settable on none. A founder who wanted her co-op's terms to end in
// November — because the members voted for November — found every seat saying
// September 2027 and no box anywhere. This is the box, and it sits beside the
// chairs.
//
// **What it will not do is extend a sitting term.** docs/adr/051's integrity
// argument is that the clock belongs to the seat precisely so that "appointment
// can fill a gap but can never manufacture a mandate"; a council that can push
// its own term ends out has the same power in a plainer form, and can outrun
// its election calendar indefinitely without ever facing the electorate. So:
//
//   - a **vacant** chair takes any future date. Nobody holds a mandate in an
//     empty chair, so nothing is manufactured, and this is what first-cohort
//     staggering is made of (051: "spreading a first cohort is just setting
//     shorter initial dates on some of them").
//   - a **held** chair's date may only be brought *forward*, or set for the
//     first time on a chair that had none. Both directions make the seat
//     contestable sooner, never later. Nobody is removed by it — holdover
//     means the holder serves until a successor is elected (051) — so the
//     worst an admin can do to a colleague with this is call an election.
//
// Refused on a chair that is in the contest running now, because an election
// is judged by the terms it opened with (docs/adr/047) and its own calendar is
// one of them. Only that chair: a staggered council's other chairs are not on
// that ballot, and a founder setting next year's dates should not have to wait
// out a contest for a seat she is not touching (docs/adr/103).
func SetSeatTerm(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := seatRoom(db, w, r)
		if !ok {
			return
		}
		seatID := r.PathValue("id")

		var req struct {
			TermEndsAt string `json:"term_ends_at"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		newEnd, err := time.Parse("2006-01-02", req.TermEndsAt)
		if err != nil {
			http.Error(w, `{"error":"a term end is a date, as 2027-11-01"}`, http.StatusBadRequest)
			return
		}
		today := time.Now().UTC().Format("2006-01-02")
		if req.TermEndsAt < today {
			http.Error(w, `{"error":"a term end must be today or later: a date in the past would say a contest was already overdue when it was not"}`, http.StatusBadRequest)
			return
		}
		termEnds := newEnd.Format("2006-01-02")

		var holderID, holderName, currentEnd, contestedIn string
		if err := db.QueryRow(
			`SELECT COALESCE(s.holder_id,''), COALESCE(`+displayNameExpr("u")+`,''),
			        COALESCE(s.term_ends_at,''), COALESCE(s.contested_in,'')
			 FROM seats s LEFT JOIN users u ON u.id = s.holder_id
			 WHERE s.id = ? AND s.node_id = ?`, seatID, nodeID,
		).Scan(&holderID, &holderName, &currentEnd, &contestedIn); err != nil {
			http.Error(w, `{"error":"seat not found"}`, http.StatusNotFound)
			return
		}

		if contestedIn != "" && contestedIn == openContestID(db, nodeID) {
			http.Error(w, `{"error":"this seat is in the contest running now: its term can move once that settles"}`, http.StatusConflict)
			return
		}

		// The one refusal. A held chair's date may come forward and may be set
		// where there was none; it may never move out.
		if holderID != "" && currentEnd != "" && termEnds > currentEnd {
			who := holderName
			if who == "" {
				who = "Somebody"
			}
			http.Error(w, `{"error":"`+who+` holds this seat until `+readableDay(currentEnd)+
				`. A held seat's term can be brought forward but not pushed back: extending it would hand out a term nobody voted for. Add a seat, or set an earlier date."}`,
				http.StatusConflict)
			return
		}

		if _, err := db.Exec(`UPDATE seats SET term_ends_at = ? WHERE id = ? AND node_id = ?`,
			termEnds, seatID, nodeID); err != nil {
			http.Error(w, `{"error":"failed to set the term end"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "seat.term_set", "seat", seatID,
			`{"node_id":"`+nodeID+`","term_ends_at":`+jsonStringOrNull(termEnds)+
				`,"previous_term_ends_at":`+jsonStringOrNull(currentEnd)+`}`, clientIP(r))

		// Every member hears it: this is the patch's election calendar, and a
		// date moving is the difference between standing for a seat this year
		// and standing for it next year.
		announceCouncil(db, nodeID, user.ID,
			// announceCouncil appends " of <patch>", so the title has to read
			// as a phrase that takes it.
			"A term end moved on the council",
			"One seat's term now ends "+readableDay(termEnds)+". The election for that seat is scheduled from its term end.")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(seatView{ID: seatID, HolderID: holderID, TermEndsAt: termEnds, Vacant: holderID == ""})
	}
}

// announceCouncil tells the members the council's furniture changed — a chair
// added or dissolved, or an election date moved.
//
// governance.rules_changed, not a type of its own: how many chairs a council
// has is part of how the patch governs, it reaches every member in the
// governance category, and it is muted and mailed with the rest of that
// category rather than being one more switch to find.
func announceCouncil(db *database.DB, nodeID, actorID, title, body string) {
	var slug, nodeName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&slug, &nodeName)
	notify(notifications.Event{
		Type:     notifications.GovernanceRulesChanged,
		NodeID:   nodeID,
		NodeSlug: slug,
		NodeName: nodeName,
		ActorID:  actorID,
		Title:    title + " of " + nodeName,
		Body:     body,
		Link:     weblink.PatchGovernance(slug),
	})
}

// readableDay renders a stored calendar date for a person to read. The column
// is ISO and stays ISO; what a member gets in a notification is not.
func readableDay(iso string) string {
	d, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return d.Format("January 2, 2006")
}

// plural is the "s" on a counted noun, so the copy does not have to branch.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
