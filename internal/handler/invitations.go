package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Membership invitations (docs/adr/098).
//
// An invite-only patch had no door: JoinNode refuses on invite_only, and
// instance invite links (docs/adr/001) create accounts without admitting
// anyone to a patch. An admin now invites by username and the person
// accepts — consent stays with the person, which is why there is no
// add-member route and why UpdateMember cannot approve an invited row.
//
// 'invited' is not membership anywhere. It is outside the ladder the way a
// pending request is (docs/adr/088): no role, no thread, absent from every
// count, listing, audience and electorate. Every membership query pins
// status = 'active', so an invited row is invisible to all of them; the two
// surfaces that do show it — the admin's Invited list and the invitee's own
// Accept/Decline control — read it by name.
//
// Every admin-side route checks the room the way the noticeboard does
// (docs/adr/081): an active admin of this patch, never a follower, never an
// instance admin holding no role here — a 404, because who has been asked
// into a room is not an outsider's to learn.

// invitedPerson is one row of the admin's Invited list.
type invitedPerson struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	// InvitedAt is the row's joined_at: the moment the invitation was made.
	// Accepting resets it, so joined_at means what its name says once the
	// row is active.
	InvitedAt string `json:"invited_at"`
}

// invitedPeople lists the outstanding invitations on a patch, oldest first.
// Callers have already established the viewer is an active admin of it.
func invitedPeople(db *database.DB, nodeID string) []invitedPerson {
	rows, err := db.Query(`SELECT m.id, m.user_id, `+usernameExpr("u")+`, `+displayNameExpr("u")+`,
			COALESCE(u.avatar_url, ''), m.joined_at
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.node_id = ? AND m.status = 'invited'
		ORDER BY m.id ASC`, nodeID)
	if err != nil {
		return []invitedPerson{}
	}
	defer rows.Close()
	out := []invitedPerson{}
	for rows.Next() {
		var p invitedPerson
		if err := rows.Scan(&p.ID, &p.UserID, &p.Username, &p.DisplayName, &p.AvatarURL, &p.InvitedAt); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

// invitationRoom resolves the slug and checks the caller is an active admin
// of the patch. One 404 for every failure, the noticeboard's rule.
func invitationRoom(db *database.DB, w http.ResponseWriter, user *model.User, slug string) (string, bool) {
	nodeID := NodeIDFromSlug(db, slug)
	if nodeID == "" || user == nil || !userHasNodeRole(db, user.ID, nodeID, "admin") {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return "", false
	}
	return nodeID, true
}

// InviteMember handles POST /api/v1/nodes/{slug}/invitations.
func InviteMember(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := invitationRoom(db, w, user, r.PathValue("slug"))
		if !ok {
			return
		}

		// A patch that has moved starts no new relationships (docs/adr/090),
		// and an invitation is the admin's side of starting one.
		if moved := nodeMovedTo(db, nodeID); moved != "" {
			writeMovedAway(w, moved)
			return
		}

		var req struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		// People paste handles with the @ on; the column stores it without.
		username := strings.TrimPrefix(strings.TrimSpace(req.Username), "@")
		if username == "" {
			http.Error(w, `{"error":"username is required"}`, http.StatusBadRequest)
			return
		}

		var targetID string
		err := db.QueryRow(
			"SELECT id FROM users WHERE username = ? AND suspended_at IS NULL AND deleted_at IS NULL",
			username,
		).Scan(&targetID)
		if err != nil {
			http.Error(w, `{"error":"no account with that username"}`, http.StatusNotFound)
			return
		}

		var nodeSlug, nodeName string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlug, &nodeName)

		// One row per person per patch, so what an invitation does depends
		// on what is already there. Only a 'left' row is re-invited; every
		// other state is a relationship or a decision this route must not
		// quietly overwrite.
		var memID, status string
		err = db.QueryRow(
			"SELECT id, status FROM memberships WHERE user_id = ? AND node_id = ?",
			targetID, nodeID,
		).Scan(&memID, &status)
		switch {
		case err == nil && status == "active":
			http.Error(w, `{"error":"already in this patch"}`, http.StatusConflict)
			return
		case err == nil && status == "pending":
			// They asked first. The answer is Approve, in the pending queue,
			// not an invitation crossing their request in the post.
			http.Error(w, `{"error":"they have already asked to join: approve their request instead"}`, http.StatusConflict)
			return
		case err == nil && status == "invited":
			http.Error(w, `{"error":"already invited"}`, http.StatusConflict)
			return
		case err == nil && status == "banned":
			// An invitation is not a quiet un-ban. Reinstating is a
			// decision and is logged as one (UpdateMember).
			http.Error(w, `{"error":"removed from this patch: reinstate them first"}`, http.StatusConflict)
			return
		case err == nil:
			// 'left': re-invite on the same row. join_message goes, as it
			// does everywhere a request ends — whatever they wrote was for a
			// request this invitation is not.
			_, err = db.Exec(
				"UPDATE memberships SET role = 'member', status = 'invited', join_message = NULL, joined_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
				memID,
			)
		default:
			memID = auth.NewUUIDv7()
			_, err = db.Exec(
				"INSERT INTO memberships (id, user_id, node_id, role, status) VALUES (?, ?, ?, 'member', 'invited')",
				memID, targetID, nodeID,
			)
		}
		if err != nil {
			http.Error(w, `{"error":"failed to invite"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEventJSON(db, user.ID, "membership.invite", "membership", memID,
			map[string]any{"target_user_id": targetID}, clientIP(r))

		notify(notifications.Event{
			Type:     notifications.MembershipInvited,
			NodeID:   nodeID,
			NodeSlug: nodeSlug,
			NodeName: nodeName,
			ActorID:  user.ID,
			TargetID: targetID,
			Title:    "You're invited to join " + nodeName,
			Link:     weblink.Patch(nodeSlug),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "invited", "membership_id": memID, "user_id": targetID})
	}
}

// acceptInvitedRow turns the caller's own invited row into an active
// membership: the one write that makes a member out of an invitation, shared
// by AcceptInvitation and by JoinNode, so a person pressing the ordinary
// button is not refused for having been asked first. Callers have already
// established that memID is the caller's row and that it is 'invited'.
func acceptInvitedRow(db *database.DB, r *http.Request, user *model.User, memID, nodeID string) error {
	_, err := db.Exec(
		"UPDATE memberships SET status = 'active', role = 'member', join_message = NULL, joined_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ? AND status = 'invited'",
		memID,
	)
	if err != nil {
		return err
	}
	auth.LogAuditEvent(db, user.ID, "membership.accept_invite", "membership", memID, "{}", clientIP(r))

	// The admins hear what they hear for any other join: the person is in.
	var nodeSlug, nodeName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlug, &nodeName)
	notify(notifications.Event{
		Type:     notifications.MembershipJoined,
		NodeID:   nodeID,
		NodeSlug: nodeSlug,
		NodeName: nodeName,
		ActorID:  user.ID,
		Title:    "New member joined " + nodeName,
		Link:     weblink.PatchMembers(nodeSlug),
	})
	return nil
}

// ownInvitation finds the caller's invited row on a patch. Scoped to the
// caller: there is no admin path through accept or decline, because the
// answer is the invitee's to give.
func ownInvitation(db *database.DB, w http.ResponseWriter, user *model.User, slug string) (memID, nodeID string, ok bool) {
	nodeID = NodeIDFromSlug(db, slug)
	if nodeID == "" {
		http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
		return "", "", false
	}
	err := db.QueryRow(
		"SELECT id FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'invited'",
		user.ID, nodeID,
	).Scan(&memID)
	if err != nil {
		http.Error(w, `{"error":"no invitation to answer"}`, http.StatusBadRequest)
		return "", "", false
	}
	return memID, nodeID, true
}

// AcceptInvitation handles POST /api/v1/nodes/{slug}/invitations/accept.
func AcceptInvitation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		memID, nodeID, ok := ownInvitation(db, w, user, r.PathValue("slug"))
		if !ok {
			return
		}
		// Accepting is joining, and a patch that has moved takes no joins
		// (docs/adr/090) — the invitation may predate the move.
		if moved := nodeMovedTo(db, nodeID); moved != "" {
			writeMovedAway(w, moved)
			return
		}
		if err := acceptInvitedRow(db, r, user, memID, nodeID); err != nil {
			http.Error(w, `{"error":"failed to accept invitation"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "active", "membership_id": memID})
	}
}

// DeclineInvitation handles POST /api/v1/nodes/{slug}/invitations/decline.
//
// The row is deleted rather than marked: an invitation nobody took is not a
// record of anything. A declined person was never in the patch, so unlike
// leaving there is no departure to keep, and unlike withdrawing there is
// no request of theirs to close — the audit row is the whole trace.
// Nobody is notified; the admin's Invited list simply gets shorter.
func DeclineInvitation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		memID, _, ok := ownInvitation(db, w, user, r.PathValue("slug"))
		if !ok {
			return
		}
		if _, err := db.Exec("DELETE FROM memberships WHERE id = ? AND status = 'invited'", memID); err != nil {
			http.Error(w, `{"error":"failed to decline invitation"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "membership.decline_invite", "membership", memID, "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "declined"})
	}
}

// RescindInvitation handles DELETE /api/v1/nodes/{slug}/invitations/{userId}.
// An admin takes back an invitation nobody has answered. Deleted like a
// decline, for the same reason; refused for any row that is not 'invited',
// so it can never become a quieter way to remove a member.
func RescindInvitation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := invitationRoom(db, w, user, r.PathValue("slug"))
		if !ok {
			return
		}
		targetID := r.PathValue("userId")
		var memID string
		err := db.QueryRow(
			"SELECT id FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'invited'",
			targetID, nodeID,
		).Scan(&memID)
		if err != nil {
			http.Error(w, `{"error":"no invitation to rescind"}`, http.StatusNotFound)
			return
		}
		if _, err := db.Exec("DELETE FROM memberships WHERE id = ? AND status = 'invited'", memID); err != nil {
			http.Error(w, `{"error":"failed to rescind invitation"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEventJSON(db, user.ID, "membership.rescind_invite", "membership", memID,
			map[string]any{"target_user_id": targetID}, clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "rescinded"})
	}
}

// ListMyInvitations handles GET /api/v1/users/me/invitations: the patches
// the caller has been asked into and has not answered.
//
// Its own endpoint rather than rows on GET /me/nodes, because that list is
// what every client surface counts as "my patches" — the onboarding redirect,
// the event form's host picker, the unlock panel — and an invitation is not
// one. Serving it there would have made every consumer defend itself
// against its own server again (docs/adr/088). The relationship row reads
// this by name to offer Accept and Decline.
func ListMyInvitations(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		rows, err := db.Query(`SELECT m.id, m.node_id, n.slug, n.name, m.joined_at
			FROM memberships m JOIN nodes n ON n.id = m.node_id
			WHERE m.user_id = ? AND m.status = 'invited'
			  AND n.status = 'active' AND n.removed_at IS NULL
			ORDER BY m.id ASC`, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to list invitations"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type invitation struct {
			ID       string `json:"id"`
			NodeID   string `json:"node_id"`
			NodeSlug string `json:"node_slug"`
			NodeName string `json:"node_name"`
			// Status is always 'invited'; sent so a client reading this
			// beside me/nodes can tell the two kinds of row apart by the
			// same field.
			Status    string `json:"status"`
			InvitedAt string `json:"invited_at"`
		}
		items := []invitation{}
		for rows.Next() {
			var it invitation
			if err := rows.Scan(&it.ID, &it.NodeID, &it.NodeSlug, &it.NodeName, &it.InvitedAt); err != nil {
				continue
			}
			it.Status = "invited"
			items = append(items, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}
