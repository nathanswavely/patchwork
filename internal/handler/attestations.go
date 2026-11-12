package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Decisions can happen elsewhere, and be recorded here (docs/adr/052).
//
// An attestation records what a community decided at a venue Patchwork was
// not. Its claim — "the membership decided this, elsewhere" — is much larger
// than a direct change's "an admin decided this", so it is its own thing and
// never worded like one.
//
// Two rules carry the whole design:
//
//   - It is offered only where the patch has declared that venue. Without
//     that gate, "the community approved this" becomes a button and the vote
//     machinery is decorative — docs/adr/049's disease with the polarity
//     reversed.
//   - The record may name anyone; the effect lands only on members. A record
//     of what a meeting decided is the community's own statement about
//     itself. Holding admin is a relationship inside the platform, and that
//     needs the person to have arrived.

// leadershipVenue reports where a patch chooses its admins. Empty or absent
// reads as "patchwork", so every existing patch keeps conducting its own
// leadership changes without being asked.
func leadershipVenue(db *database.DB, nodeID string) string {
	var gcJSON string
	if err := db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON); err != nil {
		return "patchwork"
	}
	var gc struct {
		LeadershipVenue string `json:"leadership_venue"`
	}
	if err := json.Unmarshal([]byte(gcJSON), &gc); err != nil || gc.LeadershipVenue == "" {
		return "patchwork"
	}
	return gc.LeadershipVenue
}

// leadershipDecidedElsewhere is the gate every attestation path checks.
func leadershipDecidedElsewhere(db *database.DB, nodeID string) bool {
	return leadershipVenue(db, nodeID) == "elsewhere"
}

// Which side of a decision a recorded name is on (migration
// 20261111T225617). Only a seated name carries a role; a stood-down name is
// the record saying who the meeting removed, which the effect already brings
// about by leaving them off the council (applyAttestation). It existed before
// only as a clause in the free-text summary, where a meeting that "took
// Clyde Ferris out of the chair" was not something the record could show
// (F-078).
const (
	nameSeated    = "seated"
	nameStoodDown = "stood_down"
)

// liveCorrectionSQL is "a correction of a.id that still counts". A struck
// correction corrects nothing, so the record it pointed at is in force again,
// and a fresh correction may be made of it.
const liveCorrectionSQL = `SELECT s.id FROM attestations s WHERE s.supersedes_id = a.id AND s.struck_at IS NULL`

type attestationName struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name"`
	// Realized says whether this name carries a role. False is an unrealized
	// name: recorded, shown, counted nowhere, holding nothing.
	Realized bool `json:"realized"`
}

type attestationView struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"`
	DecidedAt string `json:"decided_at"`
	// TermEndsAt is when the council this record seated stops serving —
	// present only on an elected patch, which is the only model with terms
	// (docs/adr/051). Empty everywhere else.
	TermEndsAt   string            `json:"term_ends_at,omitempty"`
	Summary      string            `json:"summary"`
	RecordedBy   string            `json:"recorded_by"`
	RecorderName string            `json:"recorder_name"`
	CreatedAt    string            `json:"created_at"`
	SupersedesID string            `json:"supersedes_id,omitempty"`
	SupersededBy string            `json:"superseded_by,omitempty"`
	Names        []attestationName `json:"names"`
	// StoodDown is who the decision removed. Always present, empty on every
	// record made before the field existed.
	StoodDown []attestationName `json:"stood_down"`
	// EnteredInError, StruckAt and StruckByName say an admin crossed this
	// record out. It stays listed, and it counts for nothing.
	EnteredInError bool   `json:"entered_in_error"`
	StruckAt       string `json:"struck_at,omitempty"`
	StruckByName   string `json:"struck_by_name,omitempty"`
}

// ListAttestations handles GET /api/v1/nodes/{slug}/attestations.
// Public: an attestation's whole value is that it can be read and checked by
// the people who were in the room.
func ListAttestations(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Attestations record what a patch decided at a venue Patchwork was
		// not, so they are part of its deliberation and follow the record
		// setting (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
		// They were a public read under docs/adr/052 and they name people —
		// an attestation's whole payload is who a meeting seated.
		//
		// 200 with an empty list and the setting beside it, matching the
		// proposals listing: "decided nothing elsewhere" and "withheld" want
		// opposite copy.
		if !canReadGovernanceRecord(db, r, nodeID) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"items":                    []interface{}{},
				"public_governance_record": "nobody",
			})
			return
		}
		rows, err := db.Query(
			`SELECT a.id, a.node_id, a.kind, a.decided_at, COALESCE(a.term_ends_at,''), a.summary, a.recorded_by,
			        `+displayNameExpr("u")+`, a.created_at,
			        COALESCE(a.supersedes_id,''),
			        COALESCE((`+liveCorrectionSQL+` LIMIT 1), ''),
			        COALESCE(a.struck_at,''),
			        CASE WHEN a.struck_by IS NULL THEN '' ELSE `+displayNameExpr("sb")+` END
			 FROM attestations a
			 LEFT JOIN users u ON u.id = a.recorded_by
			 LEFT JOIN users sb ON sb.id = a.struck_by
			 WHERE a.node_id = ?
			 ORDER BY a.decided_at DESC, a.created_at DESC`, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list attestations"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []attestationView{}
		for rows.Next() {
			var a attestationView
			if rows.Scan(&a.ID, &a.NodeID, &a.Kind, &a.DecidedAt, &a.TermEndsAt, &a.Summary, &a.RecordedBy,
				&a.RecorderName, &a.CreatedAt, &a.SupersedesID, &a.SupersededBy,
				&a.StruckAt, &a.StruckByName) != nil {
				continue
			}
			a.EnteredInError = a.StruckAt != ""
			a.Names, a.StoodDown = attestationNames(db, a.ID)
			items = append(items, a)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}

// attestationNames returns a record's names split by side: who it put in
// place, and who it removed.
func attestationNames(db *database.DB, attestationID string) (seated, stoodDown []attestationName) {
	seated, stoodDown = []attestationName{}, []attestationName{}
	rows, err := db.Query(
		`SELECT n.id, COALESCE(n.user_id,''), `+usernameExpr("u")+`, n.display_name, n.effect
		 FROM attestation_names n
		 LEFT JOIN users u ON u.id = n.user_id
		 WHERE n.attestation_id = ?
		 ORDER BY n.position ASC, n.id ASC`, attestationID)
	if err != nil {
		return seated, stoodDown
	}
	defer rows.Close()
	for rows.Next() {
		var n attestationName
		var effect string
		if rows.Scan(&n.ID, &n.UserID, &n.Username, &n.DisplayName, &effect) != nil {
			continue
		}
		n.Realized = n.UserID != ""
		if effect == nameStoodDown {
			stoodDown = append(stoodDown, n)
		} else {
			seated = append(seated, n)
		}
	}
	return seated, stoodDown
}

// CreateAttestation handles POST /api/v1/nodes/{slug}/attestations.
func CreateAttestation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only an admin of this patch can record a decision"}`, http.StatusForbidden)
			return
		}
		if !leadershipDecidedElsewhere(db, nodeID) {
			http.Error(w, `{"error":"this patch chooses its admins in Patchwork; there is nothing to record"}`, http.StatusConflict)
			return
		}

		var req struct {
			DecidedAt    string         `json:"decided_at"`
			TermEndsAt   string         `json:"term_ends_at"`
			Summary      string         `json:"summary"`
			SupersedesID string         `json:"supersedes_id"`
			Names        []recordedName `json:"names"`
			// Who the decision removed (F-078). Recorded, never acted on in
			// its own right: anyone not in Names steps down already.
			StoodDown []recordedName `json:"stood_down"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.DecidedAt) == "" {
			http.Error(w, `{"error":"decided_at is required: a record of what happened has to say when"}`, http.StatusBadRequest)
			return
		}

		// Only an elected patch has terms (docs/adr/051). Storing a date a
		// maintainer or meritocratic patch will never read is the field-nobody-
		// reads failure docs/adr/049 was written about, so it is refused rather
		// than quietly dropped.
		req.TermEndsAt = strings.TrimSpace(req.TermEndsAt)
		if req.TermEndsAt != "" && leadershipModel(db, nodeID) != "elected" {
			http.Error(w, `{"error":"only a patch with elected leadership has terms; this one fills admin seats another way"}`, http.StatusConflict)
			return
		}

		// A correction names the record it corrects, and that record has to
		// belong to this patch — otherwise the supersede chain could be made
		// to point anywhere.
		if req.SupersedesID != "" {
			var ownerNode, struckAt string
			if err := db.QueryRow("SELECT node_id, COALESCE(struck_at,'') FROM attestations WHERE id = ?", req.SupersedesID).Scan(&ownerNode, &struckAt); err != nil || ownerNode != nodeID {
				http.Error(w, `{"error":"the record being corrected does not belong to this patch"}`, http.StatusBadRequest)
				return
			}
			// A struck record counts for nothing, so a correction of it would
			// mend a line that has already been crossed out.
			if struckAt != "" {
				http.Error(w, `{"error":"that record was entered in error and counts for nothing; record the decision instead of correcting it"}`, http.StatusConflict)
				return
			}
			// A struck correction does not count as one, so the record it
			// pointed at may be corrected again.
			var already string
			_ = db.QueryRow("SELECT COALESCE(id,'') FROM attestations WHERE supersedes_id = ? AND struck_at IS NULL", req.SupersedesID).Scan(&already)
			if already != "" {
				http.Error(w, `{"error":"that record has already been corrected; correct the correction instead"}`, http.StatusConflict)
				return
			}
		}

		// Nobody is on both sides of one decision. A person named as put in
		// place and as removed would leave the record contradicting itself,
		// and applyAttestation would quietly believe the first half.
		seatedIDs := map[string]bool{}
		for _, n := range req.Names {
			if n.UserID != "" {
				seatedIDs[n.UserID] = true
			}
		}
		for _, n := range req.StoodDown {
			if n.UserID != "" && seatedIDs[n.UserID] {
				http.Error(w, `{"error":"the same person cannot be both put in place and stood down by one decision"}`, http.StatusBadRequest)
				return
			}
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO attestations (id, node_id, kind, decided_at, term_ends_at, summary, recorded_by, supersedes_id)
			 VALUES (?, ?, 'leadership', ?, ?, ?, ?, ?)`,
			id, nodeID, strings.TrimSpace(req.DecidedAt), nullIfEmpty(req.TermEndsAt),
			strings.TrimSpace(req.Summary), user.ID, nullIfEmpty(req.SupersedesID),
		); err != nil {
			http.Error(w, `{"error":"failed to record the decision"}`, http.StatusInternalServerError)
			return
		}

		insertAttestationNames(db, id, nodeID, nameSeated, req.Names, 0)
		insertAttestationNames(db, id, nodeID, nameStoodDown, req.StoodDown, len(req.Names))

		applyAttestation(db, nodeID, slug, id)

		auth.LogAuditEvent(db, user.ID, "attestation.record", "node", nodeID,
			`{"attestation_id":"`+id+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}

// isActivePatchPerson reports whether someone is an active admin or member.
// A follower is never eligible to hold a seat — admin is a rung on the member
// ladder, not a role handed sideways to an observer.
func isActivePatchPerson(db *database.DB, userID, nodeID string) bool {
	var role string
	err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
		userID, nodeID,
	).Scan(&role)
	return err == nil && (role == "admin" || role == "member")
}

// recordedName is one name as a form sends it: a person here, or a name
// typed for somebody who is not.
type recordedName struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
}

// insertAttestationNames writes one side of a record's names, positioned
// after any already written so the two sides keep the order they were typed
// in.
func insertAttestationNames(db *database.DB, attestationID, nodeID, effect string, names []recordedName, offset int) {
	for i, n := range names {
		display := strings.TrimSpace(n.DisplayName)
		linked := ""
		// A user id only counts if that person is actually in the patch.
		// The record may name anyone; being *named as a user* is what
		// carries the effect, so it takes the same membership the effect
		// does. A stood-down name carries no effect, and is held to the
		// same rule anyway so that a link on a record always means one
		// thing: this was a person here.
		if n.UserID != "" && isActivePatchPerson(db, n.UserID, nodeID) {
			linked = n.UserID
			if display == "" {
				_ = db.QueryRow("SELECT COALESCE(display_name, username) FROM users WHERE id = ?", n.UserID).Scan(&display)
			}
		}
		if display == "" {
			continue
		}
		if _, err := db.Exec(
			`INSERT INTO attestation_names (id, attestation_id, user_id, display_name, position, effect)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			auth.NewUUIDv7(), attestationID, nullIfEmpty(linked), display, offset+i, effect,
		); err != nil {
			log.Printf("attestation %s: recording the name %q: %v", attestationID, display, err)
		}
	}
}

// attestationInForce is the leadership record that currently says who holds
// admin here: the latest decision nothing live has corrected and nobody has
// struck. Empty when there is none.
func attestationInForce(db *database.DB, nodeID string) string {
	var id string
	_ = db.QueryRow(`SELECT a.id FROM attestations a
		WHERE a.node_id = ? AND a.kind = 'leadership' AND a.struck_at IS NULL
		  AND NOT EXISTS (`+liveCorrectionSQL+`)
		ORDER BY a.decided_at DESC, a.created_at DESC LIMIT 1`, nodeID).Scan(&id)
	return id
}

// StrikeAttestation handles POST /api/v1/nodes/{slug}/attestations/{id}/strike.
//
// "Entered in error" (F-078). A secretary who pressed Record twice had two
// identical records, the overview showing one and the Record both, and the
// only repair on offer was a third entry correcting the second. His minutes
// book has a rule for this, and it is the rule here: the line is crossed out
// and initialled, and stays readable.
//
// Nothing is deleted. The record keeps its names and its date, and gains who
// struck it and when. From then on it counts for nothing: it is never the
// record in force, a correction of it is refused, and a correction it made
// stops correcting, so the record it corrected is in force again.
//
// When the struck record was the one in force, the record now in force is
// applied, because who holds admin here is what that record says
// (docs/adr/052). A duplicate changes nobody; a wrong record is undone back
// to the decision before it. Where there is no decision before it, nobody's
// role moves: there is no record left to say who should hold it, and the
// way forward is to record the decision the meeting actually made.
//
// Admin-only and step-up gated, the way recording is (docs/adr/017):
// striking can move the admin role exactly as recording can.
func StrikeAttestation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only an admin of this patch can strike a record"}`, http.StatusForbidden)
			return
		}
		if !leadershipDecidedElsewhere(db, nodeID) {
			http.Error(w, `{"error":"this patch chooses its admins in Patchwork; its recorded decisions no longer move anybody"}`, http.StatusConflict)
			return
		}

		attID := r.PathValue("id")
		var ownerNode, struckAt string
		if err := db.QueryRow("SELECT node_id, COALESCE(struck_at,'') FROM attestations WHERE id = ?", attID).
			Scan(&ownerNode, &struckAt); err != nil || ownerNode != nodeID {
			http.Error(w, `{"error":"record not found on this patch"}`, http.StatusNotFound)
			return
		}
		if struckAt != "" {
			http.Error(w, `{"error":"that record is already marked entered in error"}`, http.StatusConflict)
			return
		}

		wasInForce := attestationInForce(db, nodeID) == attID
		if _, err := db.Exec(
			`UPDATE attestations SET struck_at = ?, struck_by = ? WHERE id = ? AND struck_at IS NULL`,
			time.Now().UTC().Format(time.RFC3339), user.ID, attID,
		); err != nil {
			http.Error(w, `{"error":"failed to strike the record"}`, http.StatusInternalServerError)
			return
		}

		reapplied := ""
		if wasInForce {
			if now := attestationInForce(db, nodeID); now != "" {
				applyAttestation(db, nodeID, slug, now)
				reapplied = now
			}
		}

		auth.LogAuditEvent(db, user.ID, "attestation.strike", "node", nodeID,
			`{"attestation_id":"`+attID+`","in_force":`+jsonStringOrNull(reapplied)+`}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": attID, "entered_in_error": true, "in_force": reapplied})
	}
}

// applyAttestation makes the current record true of the patch: everyone it
// names as a user holds admin, and anyone holding admin who is *not* named
// steps down.
//
// The second half is what makes this a record of an election rather than a
// list of promotions — an election decides who is on the council, which is
// also a decision about who is off it. The last admin is never removed, since
// no record may leave a patch with nobody able to administer it.
func applyAttestation(db *database.DB, nodeID, slug, attestationID string) {
	named := map[string]bool{}
	// Seated names only: a stood-down name is who came off, and being named
	// there must never be what keeps somebody on.
	rows, err := db.Query("SELECT user_id FROM attestation_names WHERE attestation_id = ? AND user_id IS NOT NULL AND effect = 'seated'", attestationID)
	if err != nil {
		return
	}
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil && uid != "" {
			named[uid] = true
		}
	}
	rows.Close()
	if len(named) == 0 {
		return
	}

	var nodeName string
	db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeName)

	// Promote everyone named who isn't already an admin.
	for uid := range named {
		var role string
		if db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'", uid, nodeID).Scan(&role) != nil {
			continue
		}
		if role == "admin" {
			continue
		}
		if _, err := db.Exec("UPDATE memberships SET role = 'admin', "+roleSinceNow+" WHERE user_id = ? AND node_id = ? AND status = 'active'", uid, nodeID); err != nil {
			continue
		}
		notify(notifications.Event{
			Type: notifications.MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug,
			NodeName: nodeName, TargetID: uid, EntityID: attestationID,
			Title: "You are now an admin of " + nodeName,
			Body:  "Recorded from a decision made outside Patchwork.",
			Link:  weblink.Patch(slug),
		})
	}

	// Step down anyone holding admin who this record does not name.
	sitting, err := db.Query("SELECT user_id FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID)
	if err != nil {
		return
	}
	var toDemote []string
	for sitting.Next() {
		var uid string
		if sitting.Scan(&uid) == nil && !named[uid] {
			toDemote = append(toDemote, uid)
		}
	}
	sitting.Close()

	for _, uid := range toDemote {
		var adminCount int
		db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
		if adminCount <= 1 {
			break
		}
		if _, err := db.Exec("UPDATE memberships SET role = 'member', "+roleSinceNow+" WHERE user_id = ? AND node_id = ? AND status = 'active'", uid, nodeID); err != nil {
			continue
		}
		notify(notifications.Event{
			Type: notifications.MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug,
			NodeName: nodeName, TargetID: uid, EntityID: attestationID,
			Title: "You are no longer an admin of " + nodeName,
			Body:  "A recorded decision named a different set of admins.",
			Link:  weblink.Patch(slug),
		})
	}
}

// LinkAttestationName handles PATCH /api/v1/nodes/{slug}/attestation-names/{id}.
// An unrealized name becomes a person: this is the moment someone the record
// already named takes up the role it said they held.
func LinkAttestationName(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		nameID := r.PathValue("id")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only an admin of this patch can link a name"}`, http.StatusForbidden)
			return
		}

		var attestationID, ownerNode, existing string
		err := db.QueryRow(
			`SELECT n.attestation_id, a.node_id, COALESCE(n.user_id,'')
			 FROM attestation_names n JOIN attestations a ON a.id = n.attestation_id
			 WHERE n.id = ?`, nameID,
		).Scan(&attestationID, &ownerNode, &existing)
		if err != nil || ownerNode != nodeID {
			http.Error(w, `{"error":"name not found on this patch"}`, http.StatusNotFound)
			return
		}
		if existing != "" {
			http.Error(w, `{"error":"that name is already linked to a person"}`, http.StatusConflict)
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
			return
		}
		if !isActivePatchPerson(db, req.UserID, nodeID) {
			http.Error(w, `{"error":"a recorded name can only be linked to an active member of this patch"}`, http.StatusBadRequest)
			return
		}

		if _, err := db.Exec("UPDATE attestation_names SET user_id = ? WHERE id = ?", req.UserID, nameID); err != nil {
			http.Error(w, `{"error":"failed to link the name"}`, http.StatusInternalServerError)
			return
		}

		// Only the record currently in force changes anything. Linking a name
		// on a superseded record fixes the history without reaching into who
		// runs the patch today.
		// A struck record is not in force either, so linking a name on one
		// fixes the book and moves nobody.
		if attestationInForce(db, nodeID) == attestationID {
			applyAttestation(db, nodeID, slug, attestationID)
		}

		auth.LogAuditEvent(db, user.ID, "attestation.link_name", "node", nodeID,
			`{"name_id":"`+nameID+`","user_id":"`+req.UserID+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"user_id": req.UserID})
	}
}
