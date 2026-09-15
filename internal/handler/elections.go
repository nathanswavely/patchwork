package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Elections (docs/adr/051): the contest Patchwork runs itself, as opposed to
// one held elsewhere and recorded here (docs/adr/052).
//
// An election is a proposal that carries candidates. It is the one proposal
// that is not born voting — it opens for nominations first, so that the slate
// is not whoever opened it — and that exception is what docs/adr/048 was
// amended for. The window is governable (`nomination_days`), which is the
// whole reason it is allowed to exist where `draft` and `discussion` were not.

// leadershipModelOf reads the model out of a raw governance_config blob, which
// is how the rules-change path compares before against after.
func leadershipModelOf(gcJSON string) string {
	if gcJSON == "" {
		return ""
	}
	var gc model.GovernanceConfig
	if json.Unmarshal([]byte(gcJSON), &gc) != nil {
		return ""
	}
	return gc.LeadershipModel
}

// electionNominating reports whether a proposal is an election still taking
// nominations. Voting has not opened, so no ballot may be cast yet.
func electionNominating(nominationsCloseAt string) bool {
	if nominationsCloseAt == "" {
		return false
	}
	closes, err := time.Parse("2006-01-02T15:04:05.000Z", nominationsCloseAt)
	if err != nil {
		return false
	}
	return time.Now().UTC().Before(closes)
}

// electedHere reads a patch's rules and reports whether Patchwork runs its
// leadership: `elected`, and not decided elsewhere (docs/adr/052).
func electedHere(db *database.DB, nodeID string) (model.GovernanceConfig, bool) {
	var gcJSON string
	db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
	var gc model.GovernanceConfig
	if json.Unmarshal([]byte(gcJSON), &gc) != nil {
		return gc, false
	}
	return gc, gc.LeadershipModel == "elected" && gc.LeadershipVenue != "elsewhere"
}

// SeatFounder gives a patch born elected its first seat (docs/adr/098): the
// founding admin holds it for one term from today, and the calendar opens
// the first real election a lead time before that term ends, the way
// ScheduleDueElections opens every one after.
//
// No election opens at birth. docs/adr/051's "adoption starts an election"
// is written for a community that already exists — a council holding over
// through a contest its members can judge. A founder alone has nobody to
// elect from: the contest would be one seat, opened the day the page was
// made, attributed to the founder, unstoppable, and one the founder could
// not vote in. And since seats were only ever created by a *resolved*
// election, that contest settling nothing left no seat behind, nothing ever
// came due, and the calendar never ran again. A seat with a term is what the
// calendar needs; a contest is not.
//
// Silent on every other model and where the venue is elsewhere. Idempotent:
// a patch that already has seats keeps them.
func SeatFounder(db *database.DB, nodeID, founderID, ip string) {
	gc, ok := electedHere(db, nodeID)
	if !ok {
		return
	}
	var seats int
	db.QueryRow(`SELECT COUNT(*) FROM seats WHERE node_id = ?`, nodeID).Scan(&seats)
	if seats > 0 {
		return
	}
	id := auth.NewUUIDv7()
	termEnds := electionTermEnd(gc)
	if _, err := db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		id, nodeID, founderID, nullIfEmpty(termEnds)); err != nil {
		log.Printf("election: seat founder of %s: %v", nodeID, err)
		return
	}
	auth.LogAuditEvent(db, founderID, "seat.founded", "seat", id,
		`{"node_id":"`+nodeID+`","term_ends_at":`+jsonStringOrNull(termEnds)+`}`, ip)
}

// jsonStringOrNull renders a date for an audit payload: the string, or null
// where the patch sets no term.
func jsonStringOrNull(s string) string {
	if s == "" {
		return "null"
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// StartElectionOnAdoption opens a patch's first election when it adopts
// elected leadership in Patchwork (docs/adr/051: "adopting `elected` starts an
// election"). Idempotent, and silent where it does not apply.
//
// It does nothing where the venue is elsewhere (docs/adr/052) — there the
// first attestation supplies the council, and a cycle nobody votes in would
// collect quorum failures and teach people to ignore governance notices.
//
// A patch with no seats yet gets them here, before the contest: one per
// sitting admin, each with its term already ended (docs/adr/098). That is
// 051's holdover made literal — the council serves until a successor is
// elected, and an overdue seat is exactly what "until" looks like on the
// calendar. Without them a contest that settled nothing left no seat behind,
// so nothing was ever due again and the patch fell silent forever; with
// them, scheduleFor finds the council overdue and tries again one breather
// later. The contest names those chairs (docs/adr/103), so a settled contest
// refills them rather than adding to the council.
func StartElectionOnAdoption(db *database.DB, nodeID string) {
	gc, ok := electedHere(db, nodeID)
	if !ok {
		return
	}

	// One open election at a time. Adoption can be re-signalled by any later
	// rules edit that leaves the model alone, and a second concurrent contest
	// for the same council would split the electorate's attention between two
	// slates deciding one thing.
	var open int
	db.QueryRow(`SELECT COUNT(*) FROM proposals
	             WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&open)
	if open > 0 {
		return
	}

	// Which seats the contest fills. Seat count follows from how the patch
	// governs (docs/adr/051) rather than from a configured cap, and at adoption
	// the honest answer is every chair it already has: a patch adopting
	// elections puts its whole council to the electorate at once, and there is
	// no staggering yet to respect.
	if seatCount(db, nodeID) == 0 {
		seatSittingAdminsOverdue(db, nodeID)
	}
	chairs := seatIDs(db, nodeID)
	if len(chairs) == 0 {
		return
	}

	openElectionFor(db, nodeID, gc, chairs)
}

// seatIDs lists a council's chairs in the order they were made, which is the
// order a settled contest refills them in.
func seatIDs(db *database.DB, nodeID string) []string {
	rows, err := db.Query(`SELECT id FROM seats WHERE node_id = ? ORDER BY created_at ASC`, nodeID)
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

// seatSittingAdminsOverdue creates one seat per active admin with its term
// ending today, and returns how many it made. Audited per seat with no actor:
// nobody appointed anyone, the rules change found them sitting.
func seatSittingAdminsOverdue(db *database.DB, nodeID string) int {
	rows, err := db.Query(`SELECT user_id FROM memberships
	                       WHERE node_id = ? AND role = 'admin' AND status = 'active'
	                       ORDER BY joined_at ASC`, nodeID)
	if err != nil {
		return 0
	}
	var admins []string
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			admins = append(admins, uid)
		}
	}
	rows.Close()

	today := time.Now().UTC().Format("2006-01-02")
	made := 0
	for _, uid := range admins {
		id := auth.NewUUIDv7()
		if _, err := db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
			id, nodeID, uid, today); err != nil {
			log.Printf("election: holdover seat on %s: %v", nodeID, err)
			continue
		}
		auth.LogAuditEvent(db, "", "seat.holdover", "seat", id,
			`{"node_id":"`+nodeID+`","holder_id":"`+uid+`","term_ends_at":"`+today+`"}`, "")
		made++
	}
	return made
}

// electionLeadHours is how long a whole contest takes: the nomination window
// plus the voting window. A cycle is scheduled so that its *seating* lands one
// term after the last (docs/adr/051), which means opening it this far ahead of
// the term end.
func electionLeadHours(gc model.GovernanceConfig) int {
	nominationDays := gc.NominationDays
	if nominationDays <= 0 {
		nominationDays = 14
	}
	duration := gc.DefaultVoteDuration
	if duration <= 0 {
		duration = 72
	}
	return nominationDays*24 + duration
}

// openElectionFor creates the contest itself. Shared by adoption and by the
// recurring cycle, so both open the same thing.
//
// It takes the chairs, not a number (docs/adr/103). Which chairs a contest is
// for is frozen here, the way docs/adr/047 freezes a proposal's terms: a
// council can stagger, so "how many" does not say which, and a resolution that
// has to guess guesses at the whole council.
func openElectionFor(db *database.DB, nodeID string, gc model.GovernanceConfig, chairs []string) string {
	seats := len(chairs)
	if seats == 0 {
		return ""
	}
	nominationDays := gc.NominationDays
	if nominationDays <= 0 {
		nominationDays = 14
	}
	duration := gc.DefaultVoteDuration
	if duration <= 0 {
		duration = 72
	}

	now := time.Now().UTC()
	nominationsClose := now.AddDate(0, 0, nominationDays).Format("2006-01-02T15:04:05.000Z")
	created := now.Format("2006-01-02T15:04:05.000Z")

	var nodeName, slug string
	db.QueryRow("SELECT name, slug FROM nodes WHERE id = ?", nodeID).Scan(&nodeName, &slug)

	id := auth.NewUUIDv7()
	// voting_ends_at stays NULL until nominations close: the window runs from
	// when voting opens, not from when the nomination period did
	// (docs/adr/051's amendment to 048). voting_terms likewise — the terms must
	// be fixed when the vote starts.
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
		 duration_hours, created_at, updated_at, seats_contested, nominations_close_at)
		 VALUES (?, ?, ?, ?, ?, 'open', 'voting', 'membership', ?, ?, ?, ?, ?)`,
		id, nodeID, systemAuthorFor(db, nodeID),
		"Council election",
		// Phase-neutral on purpose. A body is written once and read forever,
		// so anything phase-specific here becomes a lie the moment the phase
		// moves — "Nominations are open" sat directly above an open ballot for
		// the whole voting window, and would have sat above the result after
		// that. The panel below it is the phase-aware surface and says which
		// stage this is, with its dates.
		"This patch elects its admins. Nominations open first, then the ballot.",
		duration, created, created, seats, nominationsClose,
	)
	if err != nil {
		log.Printf("election: start for %s: %v", slug, err)
		return ""
	}

	// The chairs this contest is for. Nothing else may be emptied by it.
	for _, seatID := range chairs {
		db.Exec(`UPDATE seats SET contested_in = ? WHERE id = ? AND node_id = ?`, id, seatID, nodeID)
	}

	notify(notifications.Event{
		Type: notifications.ProposalNew, NodeID: nodeID, NodeSlug: slug, NodeName: nodeName,
		EntityID: id,
		Title:    "Nominations are open in " + nodeName,
		Body:     "This patch elects its admins. Stand, or put someone forward.",
		Link:     weblink.Proposal(slug, id),
	})
	log.Printf("election: opened for %s, %d seat(s), nominations close %s", slug, seats, nominationsClose)
	return id
}

// systemAuthorFor picks an author for a proposal nobody raised. The calendar
// opened this one, not a person, so the longest-standing admin stands in as
// the record's author rather than inventing a synthetic user.
func systemAuthorFor(db *database.DB, nodeID string) string {
	var id string
	db.QueryRow(`SELECT user_id FROM memberships
	             WHERE node_id = ? AND role = 'admin' AND status = 'active'
	             ORDER BY joined_at ASC LIMIT 1`, nodeID).Scan(&id)
	if id == "" {
		db.QueryRow("SELECT owner_id FROM nodes WHERE id = ?", nodeID).Scan(&id)
	}
	return id
}

// AddCandidate handles POST /api/v1/proposals/{id}/candidates.
//
// Candidacy is a member act with no tenure condition — it falls out of
// docs/adr/044 rather than being designed: tenure asks whether someone has
// been here long enough to *decide*, and a candidate is being decided about.
// The bylaws say the same ("any member may nominate themselves or another
// member").
func AddCandidate(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var nodeID, status, nominationsClose string
		var seats int
		err := db.QueryRow(
			`SELECT node_id, status, COALESCE(nominations_close_at,''), seats_contested
			 FROM proposals WHERE id = ?`, proposalID,
		).Scan(&nodeID, &status, &nominationsClose, &seats)
		if err != nil || seats == 0 {
			http.Error(w, `{"error":"election not found"}`, http.StatusNotFound)
			return
		}
		if status != "open" || !electionNominating(nominationsClose) {
			http.Error(w, `{"error":"nominations have closed"}`, http.StatusConflict)
			return
		}
		// Nominating is a member act, like raising a proposal.
		if !mayPropose(db, user.ID, nodeID) {
			http.Error(w, `{"error":"must be a member of this patch to nominate"}`, http.StatusForbidden)
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		nominee := req.UserID
		if nominee == "" {
			nominee = user.ID // standing yourself
		}
		if !isActivePatchPerson(db, nominee, nodeID) {
			http.Error(w, `{"error":"a candidate must be an active member of this patch"}`, http.StatusBadRequest)
			return
		}

		if _, err := db.Exec(
			`INSERT OR IGNORE INTO election_candidates (id, proposal_id, user_id) VALUES (?, ?, ?)`,
			auth.NewUUIDv7(), proposalID, nominee,
		); err != nil {
			http.Error(w, `{"error":"failed to add the candidate"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "election.nominate", "proposal", proposalID,
			`{"candidate":"`+nominee+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"user_id": nominee})
	}
}

// OpenElectionVoting closes nominations and starts the vote. Called when the
// nomination window has passed.
//
// This is where docs/adr/047's photograph is taken for an election: the terms
// must be fixed when the vote starts, not when the nomination period opened,
// or a slate could be assembled under one set of rules and judged by another.
func OpenElectionVoting(db *database.DB, proposalID string) bool {
	var nodeID, nominationsClose, votingEnds string
	var duration int
	err := db.QueryRow(
		`SELECT node_id, COALESCE(nominations_close_at,''), COALESCE(voting_ends_at,''), duration_hours
		 FROM proposals WHERE id = ? AND status = 'open' AND seats_contested > 0`, proposalID,
	).Scan(&nodeID, &nominationsClose, &votingEnds, &duration)
	if err != nil || nominationsClose == "" || votingEnds != "" {
		return false // not an election, or voting already opened
	}
	if electionNominating(nominationsClose) {
		return false // still nominating
	}

	var gcJSON string
	db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
	ends := time.Now().UTC().Add(time.Duration(duration) * time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET voting_ends_at = ?, voting_terms = ?,
	         updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, ends, gcJSON, proposalID)

	var slug, nodeName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&slug, &nodeName)
	notify(notifications.Event{
		Type: notifications.ProposalVoting, NodeID: nodeID, NodeSlug: slug, NodeName: nodeName,
		EntityID: proposalID,
		Title:    "Voting is open in " + nodeName,
		Body:     "Nominations have closed. Approve as many candidates as you like.",
		Link:     weblink.Proposal(slug, proposalID),
	})
	return true
}

// CastElectionBallot handles PUT /api/v1/proposals/{id}/ballot.
//
// Approval voting (docs/adr/051): a ballot is the set of candidates one person
// approves, so it replaces wholesale rather than accumulating — changing your
// mind means sending the set you now hold, not undoing rows one at a time.
func CastElectionBallot(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var nodeID, status, nominationsClose, votingEnds string
		var seats int
		err := db.QueryRow(
			`SELECT node_id, status, COALESCE(nominations_close_at,''), COALESCE(voting_ends_at,''), seats_contested
			 FROM proposals WHERE id = ?`, proposalID,
		).Scan(&nodeID, &status, &nominationsClose, &votingEnds, &seats)
		if err != nil || seats == 0 {
			http.Error(w, `{"error":"election not found"}`, http.StatusNotFound)
			return
		}
		if status != "open" {
			http.Error(w, `{"error":"this election has closed"}`, http.StatusConflict)
			return
		}
		if electionNominating(nominationsClose) || votingEnds == "" {
			http.Error(w, `{"error":"nominations are still open; voting has not started"}`, http.StatusConflict)
			return
		}

		// The electorate is one set (docs/adr/044), judged by the terms this
		// vote opened with (docs/adr/047).
		gc := votingTerms(db, proposalID, nodeID)
		if denial := electorateDenial(db, user.ID, nodeID, gc); denial != "" {
			http.Error(w, `{"error":"`+denial+`"}`, http.StatusForbidden)
			return
		}

		var req struct {
			CandidateIDs []string `json:"candidate_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		db.Exec(`DELETE FROM election_ballots WHERE proposal_id = ? AND voter_id = ?`, proposalID, user.ID)
		for _, cid := range req.CandidateIDs {
			var belongs int
			db.QueryRow(`SELECT COUNT(*) FROM election_candidates WHERE id = ? AND proposal_id = ?`, cid, proposalID).Scan(&belongs)
			if belongs == 0 {
				continue
			}
			db.Exec(`INSERT OR IGNORE INTO election_ballots (id, proposal_id, voter_id, candidate_id)
			         VALUES (?, ?, ?, ?)`, auth.NewUUIDv7(), proposalID, user.ID, cid)
		}
		auth.LogAuditEvent(db, user.ID, "election.ballot", "proposal", proposalID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"approved": len(req.CandidateIDs)})
	}
}

type electionTallyRow struct {
	CandidateID string
	UserID      string
	Approvals   int
}

// tallyElection counts approvals per candidate, highest first. Ballots from
// people the electorate no longer counts are dropped, the same rule the
// ordinary tally uses (countedBallot) so the two can never disagree.
func tallyElection(db *database.DB, proposalID string) []electionTallyRow {
	rows, err := db.Query(`
		SELECT c.id, c.user_id, COUNT(b.id)
		FROM election_candidates c
		LEFT JOIN election_ballots b ON b.candidate_id = c.id
		LEFT JOIN proposals p ON p.id = c.proposal_id
		LEFT JOIN memberships m ON m.user_id = b.voter_id AND m.node_id = p.node_id
		WHERE c.proposal_id = ? AND (b.id IS NULL OR `+countedBallot+`)
		GROUP BY c.id, c.user_id`, proposalID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []electionTallyRow
	for rows.Next() {
		var t electionTallyRow
		if rows.Scan(&t.CandidateID, &t.UserID, &t.Approvals) == nil {
			out = append(out, t)
		}
	}
	// Highest approvals first; ties break on candidate id, which is UUIDv7 and
	// therefore the order they stood in. Deterministic beats arbitrary, and a
	// real tie for the last seat is a thing the patch should see rather than
	// have silently resolved — surfacing that is left to the UI.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Approvals != out[j].Approvals {
			return out[i].Approvals > out[j].Approvals
		}
		return out[i].CandidateID < out[j].CandidateID
	})
	return out
}
