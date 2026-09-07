package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// profileMembership is one visible membership on a public profile.
type profileMembership struct {
	NodeID   string `json:"node_id"`
	NodeSlug string `json:"node_slug"`
	NodeName string `json:"node_name"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

// GetUserProfile handles GET /api/v1/users/{username} — the public profile.
// Identity fields plus visible member/admin memberships in public patches.
// Follower relationships and hidden memberships never appear here, for any
// viewer (docs/adr/006).
//
// A deleted account is a 404 here even though its row is still present
// (docs/adr/086). The tombstone exists to keep the community's record whole,
// not to keep serving a page about somebody who left.
//
// One part does depend on who is looking: the shared contact items
// (docs/adr/083). A visitor sees exactly the items this person shares into a
// patch the visitor is also an active member or admin of — which is what they
// could already have read by walking into that room, so the profile is a
// window onto that audience and never a wider one. The page never says which
// patch an item came through: the granting membership may be private or
// hidden, and citing it would disclose what docs/adr/006 keeps off this page.
//
// Because the response now varies by caller it carries Vary: Cookie. Under
// multi-quilt CORS the public GET answers Access-Control-Allow-Origin: *, so
// a browser sends no credentials cross-origin and a remote quilt receives the
// anonymous view — a shared item never appears in a merged view even for
// someone in the room, which docs/adr/083 records as intended.
func GetUserProfile(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		viewer := middleware.UserFromContext(r.Context())

		var (
			u         model.User
			linksJSON string
		)
		err := db.QueryRow(
			`SELECT id, username, display_name, bio, avatar_url, COALESCE(links,'[]'), COALESCE(moved_to,''), created_at
			 FROM users WHERE username = ? AND suspended_at IS NULL AND deleted_at IS NULL AND id != ?`,
			username, model.SystemUserID,
		).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &linksJSON, &u.MovedTo, &u.CreatedAt)
		if err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		links := []model.NodeLink{}
		json.Unmarshal([]byte(linksJSON), &links)

		rows, err := db.Query(
			`SELECT m.node_id, n.slug, n.name, m.role, m.joined_at
			 FROM memberships m JOIN nodes n ON m.node_id = n.id
			 WHERE m.user_id = ? AND m.status = 'active' AND m.visible = 1
			   AND m.role IN ('member', 'admin')
			   AND n.visibility = 'public' AND n.status = 'active'
			 ORDER BY CASE m.role WHEN 'admin' THEN 0 ELSE 1 END, n.name`,
			u.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to load profile"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		memberships := []profileMembership{}
		for rows.Next() {
			var m profileMembership
			if err := rows.Scan(&m.NodeID, &m.NodeSlug, &m.NodeName, &m.Role, &m.JoinedAt); err != nil {
				continue
			}
			memberships = append(memberships, m)
		}

		contact := []model.ContactItem{}
		if viewer != nil {
			contact = sharedContactItemsFor(db, u.ID, viewer.ID)
		}

		// The body differs by caller, so no shared cache may reuse it.
		w.Header().Set("Vary", "Cookie")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           u.ID,
			"username":     u.Username,
			"display_name": u.DisplayName,
			"bio":          u.Bio,
			"avatar_url":   u.AvatarURL,
			"links":        links,
			// Where this person says they have gone (docs/adr/090). Public,
			// like the rest of the profile: a pointer nobody can read is not
			// a pointer.
			"moved_to":    u.MovedTo,
			"created_at":  u.CreatedAt,
			"memberships": memberships,
			"contact":     contact,
		})
	}
}

// UpdateMyMembership handles PATCH /api/v1/users/me/memberships/{nodeId}
// — the two switches a member owns on their own membership, never patch
// admins: `visible`, the one membership-visibility switch (docs/adr/006),
// (docs/adr/006). Contact sharing moved off this endpoint with
// docs/adr/083: it is per item and per patch now, at
// PUT /api/v1/nodes/{slug}/contact-shares.
func UpdateMyMembership(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := r.PathValue("nodeId")

		var req struct {
			Visible *bool `json:"visible"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Visible == nil {
			http.Error(w, `{"error":"body must include visible: true|false"}`, http.StatusBadRequest)
			return
		}

		var (
			memID, role string
			visible     bool
		)
		if err := db.QueryRow(
			"SELECT id, role, visible FROM memberships WHERE user_id = ? AND node_id = ?",
			user.ID, nodeID,
		).Scan(&memID, &role, &visible); err != nil {
			http.Error(w, `{"error":"membership not found"}`, http.StatusNotFound)
			return
		}

		if req.Visible != nil {
			visible = *req.Visible
			if _, err := db.Exec("UPDATE memberships SET visible = ? WHERE id = ?", visible, memID); err != nil {
				http.Error(w, `{"error":"failed to update visibility"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "membership.visibility", "membership", memID, "{}", clientIP(r))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"node_id": nodeID,
			"visible": visible,
		})
	}
}
