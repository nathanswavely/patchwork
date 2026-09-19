package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Succession follows the leadership model (docs/adr/051). This file carries
// the `maintainer` mechanic: one person runs the patch and names who inherits
// it. No seats, no terms, no ballot — those belong to the other two models.
//
// The designation is only meaningful because it fires: a successor nobody can
// ever succeed to would be a seventh field in the family docs/adr/049 and 050
// catalogued, stored and rendered and read by nothing. Its trigger is the
// maintainer leaving, which is also what lets them leave at all.

// roleSinceNow is the SET clause every write that changes a membership's role
// carries with it (migration 072).
//
// `memberships.role_since` is when this person got the role they hold now, and
// it is a floor the inactivity sweep measures absence from (docs/adr/051).
// Without it the floor was `joined_at`, so somebody promoted to fill an
// absence was already older than the vacate threshold on the day they were
// appointed: vacated on the next pass, replaced by the next member down, and
// round again — 1,348 successions in a simulated year.
//
// It is a constant spliced into the SQL rather than a bound parameter because
// the statements it joins are a mix of positional-argument shapes, and a
// literal `strftime` keeps the timestamp in the same format and the same clock
// as every other column written by the same statement.
//
// Paths that write `joined_at = now` in the same statement — a follower
// upgrading, an invitation being accepted — deliberately do not carry it:
// NULL reads as `joined_at`, which is the same instant.
const roleSinceNow = "role_since = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')"

// leadershipModel reads the node's cached governance config. Empty when the
// config is absent or unparseable, which reads as "not maintainer" everywhere
// this is used — the conservative direction, since every caller is deciding
// whether to permit a power transfer.
func leadershipModel(db *database.DB, nodeID string) string {
	var gcJSON string
	if err := db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON); err != nil {
		return ""
	}
	var gc model.GovernanceConfig
	if err := json.Unmarshal([]byte(gcJSON), &gc); err != nil {
		return ""
	}
	return gc.LeadershipModel
}

// designatedSuccessor returns the node's named successor, but only if the
// designation is still good: the person must be an active admin or member of
// this patch. Leaving voids a designation without touching the row, so the
// membership is re-checked here rather than trusted from when it was written.
// A follower is never eligible — admin is a rung on the member ladder.
func designatedSuccessor(db *database.DB, nodeID string) string {
	var successorID string
	db.QueryRow("SELECT COALESCE(designated_successor_id,'') FROM nodes WHERE id = ?", nodeID).Scan(&successorID)
	if successorID == "" {
		return ""
	}
	var role string
	err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
		successorID, nodeID,
	).Scan(&role)
	if err != nil || (role != "admin" && role != "member") {
		return ""
	}
	return successorID
}

// nullIfEmpty keeps an empty foreign key out of the column as NULL rather than
// "", which no users row will ever match and which the ON DELETE clause cannot
// clean up.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// validateNomination checks a proposal that is about a person rather than a
// thing — the meritocratic mechanic, where "existing admins nominate from
// active members and the community ratifies." Returns an error message, empty
// when the nomination is good.
//
// The three conditions are the template's own sentence read literally:
// existing *admins* nominate (so the author holds admin), from active
// *members* (so the nominee is one, and a follower never is), and the
// community *ratifies* (so this is a real vote and not a promotion wearing a
// proposal's clothes).
func validateNomination(db *database.DB, nodeID, authorID, nomineeID string) string {
	if leadershipDecidedElsewhere(db, nodeID) {
		return "this patch chooses its admins elsewhere: record that decision instead of nominating"
	}
	switch leadershipModel(db, nodeID) {
	case "meritocratic":
		// The mechanic this was written for. Conditions below.
	case "elected":
		// A mid-term vacancy borrows meritocratic's mechanic, because the
		// template already says so: "the council may appoint a replacement
		// from active members. The appointment must be ratified by the
		// community within 14 days" (docs/adr/051). What is different is
		// that the appointment lands in a chair — so with no vacant chair
		// there is nothing to nominate *into*, and the honest answer is the
		// date of the next contest (docs/adr/100). Refused here, at
		// creation, so the "approved but unseatable" case at ratification
		// stays the rarity it should be.
		if vacantSeat(db, nodeID) == "" {
			return electedPromotionDenial(db, nodeID)
		}
	default:
		return "nominating an admin is the meritocratic model's mechanic; this patch fills admin seats another way"
	}
	if !userHasNodeRole(db, authorID, nodeID, "admin") {
		return "only an admin of this patch can nominate someone for admin"
	}
	var role string
	err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
		nomineeID, nodeID,
	).Scan(&role)
	if err != nil {
		return "a nominee must be an active member of this patch"
	}
	if role == "admin" {
		return "that person is already an admin of this patch"
	}
	if role != "member" {
		return "a nominee must be an active member of this patch"
	}
	return ""
}

// ratifyNomination promotes the subject of an approved nomination. Called from
// resolution, because the community ratifying *is* the decision — leaving an
// admin to "apply" it afterwards would hand admins a veto over the ratification
// they asked the community for.
//
// Membership is re-checked here: a nominee who left the patch between
// nomination and ratification is not dragged back into it, and one who was
// promoted by some other path in the meantime is left alone.
func ratifyNomination(db *database.DB, proposalID, nodeID, nomineeID string) {
	if nomineeID == "" {
		return
	}
	lm := leadershipModel(db, nodeID)
	if lm != "meritocratic" && lm != "elected" {
		return
	}
	var role string
	if err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
		nomineeID, nodeID,
	).Scan(&role); err != nil || role != "member" {
		return
	}

	// On an elected patch the ratification seats them, so it needs a chair.
	// The chair's own term end carries over untouched: someone appointed to a
	// mid-term vacancy serves out the remainder, not a fresh term
	// (docs/adr/051), which is what stops a council resetting its own clocks
	// by appointing allies.
	//
	// No chair, no promotion. The proposal approved something that is no
	// longer possible — somebody else took the last seat, or an admin removed
	// it, between the vote opening and closing — and quietly promoting anyway
	// would put an admin on this patch in no seat at all, which is the state
	// docs/adr/100 exists to end. Creation refuses a nomination with no
	// vacancy, so this is the narrow race and not the ordinary path.
	seatID := ""
	if lm == "elected" {
		seatID = vacantSeat(db, nodeID)
		if seatID == "" {
			reportUnseatableRatification(db, proposalID, nodeID, nomineeID)
			return
		}
	}

	if _, err := db.Exec(
		"UPDATE memberships SET role = 'admin', "+roleSinceNow+" WHERE user_id = ? AND node_id = ? AND status = 'active'",
		nomineeID, nodeID,
	); err != nil {
		return
	}

	// The chair, not a new one, and not a new term. term_ends_at is left
	// exactly as the seat carried it (docs/adr/051).
	if seatID != "" {
		// The promotion above already landed, so a seat that does not record
		// its holder leaves an admin sitting in no chair — the state
		// docs/adr/100 exists to end, and one nothing else would notice.
		// `seat.filled` is only written where the seat was in fact filled.
		if _, err := db.Exec("UPDATE seats SET holder_id = ? WHERE id = ?", nomineeID, seatID); err != nil {
			applyIncomplete(db, proposalID, "", "seat handover to "+nomineeID, err)
		} else {
			auth.LogAuditEvent(db, "", "seat.filled", "seat", seatID,
				`{"node_id":"`+nodeID+`","holder_id":"`+nomineeID+`","proposal_id":"`+proposalID+`"}`, "")
		}
	}

	// No actor: ratification is the electorate and the clock, not a person, so
	// the notice reaches everyone including whoever raised the nomination.
	auth.LogAuditEvent(db, "", "membership.ratified", "membership", nomineeID,
		`{"node_id":"`+nodeID+`","proposal_id":"`+proposalID+`"}`, "")

	var slug, nodeName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&slug, &nodeName)
	notify(notifications.Event{
		Type:     notifications.MembershipRoleChanged,
		NodeID:   nodeID,
		NodeSlug: slug,
		NodeName: nodeName,
		TargetID: nomineeID,
		EntityID: proposalID,
		Title:    "You are now an admin of " + nodeName,
		Body:     "The community ratified your nomination.",
		Link:     weblink.Patch(slug),
	})
}

// reportUnseatableRatification says plainly that a ratification could not be
// carried out (docs/adr/100): the community approved somebody for a seat and
// by the time the vote closed there was no seat to put them in.
//
// The patch's admins hear it, because adding a chair is their act and nobody
// else's. Logged as well as notified: this is the one outcome where an
// approved proposal changes nothing, and a record that does not say so is
// worse than the outcome.
func reportUnseatableRatification(db *database.DB, proposalID, nodeID, nomineeID string) {
	var slug, nodeName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&slug, &nodeName)
	var who string
	db.QueryRow("SELECT "+displayNameExpr("u")+" FROM users u WHERE u.id = ?", nomineeID).Scan(&who)
	if who == "" {
		who = "the nominee"
	}

	log.Printf("election: %s ratified %s with no vacant seat; nobody promoted", slug, nomineeID)
	auth.LogAuditEvent(db, "", "membership.ratification_unseated", "membership", nomineeID,
		`{"node_id":"`+nodeID+`","proposal_id":"`+proposalID+`"}`, "")

	notify(notifications.Event{
		Type:     notifications.GovernanceSeatUnavailable,
		NodeID:   nodeID,
		NodeSlug: slug,
		NodeName: nodeName,
		EntityID: proposalID,
		Title:    "A ratified nomination had no seat in " + nodeName,
		Body:     "The members approved " + who + " for the council, and every seat was held by the time the vote closed. Nothing changed. Add a seat and nominate again, or wait for the next contest.",
		Link:     weblink.PatchGovernance(slug),
	})
}

// SetSuccessor handles PUT /api/v1/nodes/{slug}/successor.
func SetSuccessor(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only an admin of this patch can name a successor"}`, http.StatusForbidden)
			return
		}

		// Designation is the maintainer model's mechanic and nobody else's. A
		// meritocratic patch fills a seat by nomination and ratification; an
		// elected one runs a cycle. Storing a value on those would be writing
		// a field that nothing will ever act on, which is the whole failure
		// docs/adr/049 was written about.
		if lm := leadershipModel(db, nodeID); lm != "maintainer" {
			http.Error(w, `{"error":"naming a successor is the maintainer model's mechanic; this patch fills admin seats another way"}`, http.StatusConflict)
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.UserID == "" {
			http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
			return
		}
		if req.UserID == user.ID {
			http.Error(w, `{"error":"you cannot name yourself as your own successor"}`, http.StatusBadRequest)
			return
		}

		// The successor has to already be in the patch. Naming an outsider
		// would hand a stranger the keys on the maintainer's way out, and
		// admin is a rung people climb rather than a role handed sideways.
		var role string
		err := db.QueryRow(
			"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
			req.UserID, nodeID,
		).Scan(&role)
		if err != nil || (role != "admin" && role != "member") {
			http.Error(w, `{"error":"a successor must be an active member of this patch"}`, http.StatusBadRequest)
			return
		}

		if _, err := db.Exec("UPDATE nodes SET designated_successor_id = ? WHERE id = ?", req.UserID, nodeID); err != nil {
			http.Error(w, `{"error":"failed to name a successor"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.successor_set", "node", nodeID,
			`{"successor_id":"`+req.UserID+`"}`, clientIP(r))

		// Being named is worth knowing about before it takes effect — the
		// point of naming a successor early is that they are not surprised by
		// it later.
		var nodeName string
		db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeName)
		notify(notifications.Event{
			Type:     notifications.MembershipRoleChanged,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: nodeName,
			ActorID:  user.ID,
			TargetID: req.UserID,
			Title:    "You are named successor for " + nodeName,
			Body:     "If the current admin steps away, this patch passes to you.",
			Link:     weblink.Patch(slug),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"successor_id": req.UserID})
	}
}

// ClearSuccessor handles DELETE /api/v1/nodes/{slug}/successor.
func ClearSuccessor(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only an admin of this patch can clear a successor"}`, http.StatusForbidden)
			return
		}

		// Deliberately not gated on the leadership model. Clearing is the
		// safe direction, and a patch that changed models with a designation
		// still on the row has to be able to take it off.
		if _, err := db.Exec("UPDATE nodes SET designated_successor_id = NULL WHERE id = ?", nodeID); err != nil {
			http.Error(w, `{"error":"failed to clear the successor"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.successor_cleared", "node", nodeID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"successor_id": ""})
	}
}

// succeedOnDeparture promotes a maintainer patch's designated successor so the
// departing sole admin can leave. Reports whether succession happened; false
// means the caller's last-admin floor still applies.
//
// docs/adr/012 says leaving is a member right, and the floor has always made
// that untrue for the one person who cannot be replaced. Designation is how a
// maintainer earns their own exit: name who takes over, and the door opens.
// Without a designation the floor holds, because the alternative is a patch
// with nobody who can administer it.
func succeedOnDeparture(db *database.DB, r *http.Request, nodeID, slug, departingUserID string) bool {
	if leadershipModel(db, nodeID) != "maintainer" {
		return false
	}
	successorID := designatedSuccessor(db, nodeID)
	if successorID == "" || successorID == departingUserID {
		return false
	}

	if _, err := db.Exec(
		"UPDATE memberships SET role = 'admin', "+roleSinceNow+" WHERE user_id = ? AND node_id = ? AND status = 'active'",
		successorID, nodeID,
	); err != nil {
		return false
	}
	// The designation is spent. Leaving it in place would name the new
	// maintainer as their own successor, and re-naming is theirs to do.
	db.Exec("UPDATE nodes SET designated_successor_id = NULL WHERE id = ?", nodeID)

	auth.LogAuditEvent(db, departingUserID, "node.succession", "node", nodeID,
		`{"successor_id":"`+successorID+`"}`, clientIP(r))

	var nodeName string
	db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeName)
	notify(notifications.Event{
		Type:     notifications.MembershipRoleChanged,
		NodeID:   nodeID,
		NodeSlug: slug,
		NodeName: nodeName,
		ActorID:  departingUserID,
		TargetID: successorID,
		Title:    "You now run " + nodeName,
		Body:     "The previous admin has left and named you as their successor.",
		Link:     weblink.Patch(slug),
	})
	return true
}
