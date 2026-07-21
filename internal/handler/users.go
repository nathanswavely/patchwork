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
// Everyone (including anonymous visitors) gets the same view: identity
// fields plus visible member/admin memberships in public patches. Follower
// relationships and hidden memberships never appear here, for any viewer
// (docs/adr/006).
func GetUserProfile(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")

		var (
			u         model.User
			linksJSON string
		)
		err := db.QueryRow(
			`SELECT id, username, display_name, bio, avatar_url, COALESCE(links,'[]'), created_at
			 FROM users WHERE username = ? AND suspended_at IS NULL AND id != ?`,
			username, model.SystemUserID,
		).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &linksJSON, &u.CreatedAt)
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           u.ID,
			"username":     u.Username,
			"display_name": u.DisplayName,
			"bio":          u.Bio,
			"avatar_url":   u.AvatarURL,
			"links":        links,
			"created_at":   u.CreatedAt,
			"memberships":  memberships,
		})
	}
}

// UpdateMyMembershipVisibility handles
// PATCH /api/v1/users/me/memberships/{nodeId} — flip the one
// membership-visibility switch (docs/adr/006). Owned by the member, never
// by patch admins.
func UpdateMyMembershipVisibility(db *database.DB) http.HandlerFunc {
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

		var memID string
		if err := db.QueryRow(
			"SELECT id FROM memberships WHERE user_id = ? AND node_id = ?",
			user.ID, nodeID,
		).Scan(&memID); err != nil {
			http.Error(w, `{"error":"membership not found"}`, http.StatusNotFound)
			return
		}

		visible := 0
		if *req.Visible {
			visible = 1
		}
		if _, err := db.Exec("UPDATE memberships SET visible = ? WHERE id = ?", visible, memID); err != nil {
			http.Error(w, `{"error":"failed to update visibility"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "membership.visibility", "membership", memID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"node_id": nodeID,
			"visible": *req.Visible,
		})
	}
}
