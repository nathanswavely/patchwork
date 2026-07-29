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

// StartElectionOnAdoption opens a patch's first election when it adopts
// elected leadership in Patchwork (docs/adr/051: "adopting `elected` starts an
// election"). Idempotent, and silent where it does not apply.
//
// It does nothing where the venue is elsewhere (docs/adr/052) — there the
// first attestation supplies the council, and a cycle nobody votes in would
// collect quorum failures and teach people to ignore governance notices.
func StartElectionOnAdoption(db *database.DB, nodeID string) {
	var gcJSON string
	db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
	var gc model.GovernanceConfig
	if json.Unmarshal([]byte(gcJSON), &gc) != nil {
		return
	}
	if gc.LeadershipModel != "elected" || gc.LeadershipVenue == "elsewhere" {
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

	// How many seats the contest fills. Seat count follows from how the patch
	// governs (docs/adr/051) rather than from a configured cap, and at adoption
	// the honest number is the council it already has.
	seats := 0
	db.QueryRow(`SELECT COUNT(*) FROM seats WHERE node_id = ?`, nodeID).Scan(&seats)
	if seats == 0 {
		db.QueryRow(`SELECT COUNT(*) FROM memberships
		             WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&seats)
	}
	if seats == 0 {
		return
	}

	openElectionFor(db, nodeID, gc, seats)
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
func openElectionFor(db *database.DB, nodeID string, gc model.GovernanceConfig, seats int) string {
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
		"Nominations are open. Any member may stand, or put someone forward.",
		duration, created, created, seats, nominationsClose,
	)
	if err != nil {
		log.Printf("election: start for %s: %v", slug, err)
		return ""
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
