package handler

import (
	"encoding/json"
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
		"UPDATE memberships SET role = 'admin' WHERE user_id = ? AND node_id = ? AND status = 'active'",
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
