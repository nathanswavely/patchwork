package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
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

// seatView is a seat as every surface reads it: the chair, who is in it, and
// when its term ends. A vacant seat carries no holder and still carries a
// term — the clock belongs to the seat, not the person (docs/adr/051).
type seatView struct {
	ID          string `json:"id"`
	HolderID    string `json:"holder_id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	TermEndsAt  string `json:"term_ends_at,omitempty"`
}

// seatsOf lists a patch's council in the order the chairs were made, which is
// the order seatWinners refills them in.
func seatsOf(db *database.DB, nodeID string) []seatView {
	out := []seatView{}
	rows, err := db.Query(`
		SELECT s.id, COALESCE(s.holder_id,''), COALESCE(`+usernameExpr("u")+`,''),
		       COALESCE(`+displayNameExpr("u")+`,''), COALESCE(s.term_ends_at,'')
		FROM seats s LEFT JOIN users u ON u.id = s.holder_id
		WHERE s.node_id = ? ORDER BY s.created_at ASC`, nodeID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s seatView
		if rows.Scan(&s.ID, &s.HolderID, &s.Username, &s.DisplayName, &s.TermEndsAt) != nil {
			continue
		}
		out = append(out, s)
	}
	return out
}

// vacantSeat returns the id of a seat nobody holds, oldest chair first, or
// empty when the council is full.
func vacantSeat(db *database.DB, nodeID string) string {
	var id string
	db.QueryRow(`SELECT id FROM seats WHERE node_id = ? AND holder_id IS NULL
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
		announceCouncilSize(db, nodeID, user.ID,
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

		var holderID, holderName string
		err := db.QueryRow(
			`SELECT COALESCE(s.holder_id,''), COALESCE(`+displayNameExpr("u")+`,'')
			 FROM seats s LEFT JOIN users u ON u.id = s.holder_id
			 WHERE s.id = ? AND s.node_id = ?`, seatID, nodeID,
		).Scan(&holderID, &holderName)
		if err != nil {
			http.Error(w, `{"error":"seat not found"}`, http.StatusNotFound)
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
		announceCouncilSize(db, nodeID, user.ID,
			"A vacant seat was removed from the council",
			"The council now has "+strconv.Itoa(total)+" seat"+plural(total)+".")

		w.WriteHeader(http.StatusNoContent)
	}
}

// announceCouncilSize tells the members the council changed size.
//
// governance.rules_changed, not a type of its own: how many chairs a council
// has is part of how the patch governs, it reaches every member in the
// governance category, and it is muted and mailed with the rest of that
// category rather than being one more switch to find.
func announceCouncilSize(db *database.DB, nodeID, actorID, title, body string) {
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

// plural is the "s" on a counted noun, so the copy does not have to branch.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
