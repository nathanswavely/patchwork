package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// JoinNode handles POST /api/v1/nodes/{slug}/join.
func JoinNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Check if this is a follow request.
		var reqBody struct {
			Role string `json:"role"`
		}
		// Try to parse body for role; default to member.
		json.NewDecoder(r.Body).Decode(&reqBody)
		isFollow := reqBody.Role == "follower"

		// Followers can follow any public patch regardless of membership policy.
		if isFollow {
			var vis string
			db.QueryRow("SELECT visibility FROM nodes WHERE id = ?", nodeID).Scan(&vis)
			if vis != "public" {
				http.Error(w, `{"error":"can only follow public patches"}`, http.StatusForbidden)
				return
			}
		}

		// Look up membership policy and node status.
		var membershipPolicy, nodeStatus string
		db.QueryRow("SELECT membership_policy, status FROM nodes WHERE id = ?", nodeID).Scan(&membershipPolicy, &nodeStatus)

		// Unclaimed patches only accept followers, not members.
		if nodeStatus == "unclaimed" && !isFollow {
			http.Error(w, `{"error":"this patch hasn't been claimed yet — you can follow it"}`, http.StatusForbidden)
			return
		}

		if !isFollow && membershipPolicy == "invite_only" {
			http.Error(w, `{"error":"this node is invite only"}`, http.StatusForbidden)
			return
		}

		// Check for existing membership.
		var existingID, existingStatus, existingRole string
		err := db.QueryRow("SELECT id, status, role FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&existingID, &existingStatus, &existingRole)
		if err == nil {
			// Membership row exists.
			if existingStatus == "banned" {
				http.Error(w, `{"error":"You have been removed from this community"}`, http.StatusForbidden)
				return
			}
			if existingStatus == "active" {
				if isFollow && existingRole == "follower" {
					http.Error(w, `{"error":"already following"}`, http.StatusConflict)
					return
				}
				if isFollow && existingRole != "follower" {
					// Already a member or admin — following is a downgrade, ignore.
					http.Error(w, `{"error":"already a member"}`, http.StatusConflict)
					return
				}
				if !isFollow && existingRole == "follower" {
					// Follower upgrading to member — this is the "Become Member" flow.
					newRole := "member"
					newStatus := "active"
					auditAction := "membership.join"
					if !isFollow && membershipPolicy == "approval_required" {
						newStatus = "pending"
						auditAction = "membership.request"
					}
					_, err = db.Exec(
						"UPDATE memberships SET role = ?, status = ?, joined_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
						newRole, newStatus, existingID,
					)
					if err != nil {
						http.Error(w, `{"error":"failed to upgrade membership"}`, http.StatusInternalServerError)
						return
					}
					auth.LogAuditEvent(db, user.ID, auditAction, "membership", existingID, `{"from":"follower"}`, clientIP(r))

					var nodeSlugN, nodeNameN string
					db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
					notify(notifications.Event{
						Type:     notifications.MembershipJoined,
						NodeID:   nodeID,
						NodeSlug: nodeSlugN,
						NodeName: nodeNameN,
						ActorID:  user.ID,
						Title:    "New member joined " + nodeNameN,
						Link:     "/patches/" + nodeSlugN + "/members",
					})

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(map[string]string{"status": newStatus, "membership_id": existingID})
					return
				}
				// Already an active member/admin trying to join again.
				http.Error(w, `{"error":"already a member"}`, http.StatusConflict)
				return
			}
			if existingStatus == "pending" {
				http.Error(w, `{"error":"membership request already pending"}`, http.StatusConflict)
				return
			}
			// Status is "left" — reactivate with the requested role.
			newRole := "member"
			newStatus := "active"
			auditAction := "membership.join"
			if isFollow {
				newRole = "follower"
				auditAction = "membership.follow"
			} else if membershipPolicy == "approval_required" {
				newStatus = "pending"
				auditAction = "membership.request"
			}
			_, err = db.Exec(
				"UPDATE memberships SET status = ?, role = ?, joined_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
				newStatus, newRole, existingID,
			)
			if err != nil {
				http.Error(w, `{"error":"failed to rejoin node"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, auditAction, "membership", existingID, "{}", clientIP(r))

			// Notify admins about the join/request (not for follows).
			if !isFollow {
				var nodeSlugN, nodeNameN string
				db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
				if newStatus == "active" {
					notify(notifications.Event{
						Type:     notifications.MembershipJoined,
						NodeID:   nodeID,
						NodeSlug: nodeSlugN,
						NodeName: nodeNameN,
						ActorID:  user.ID,
						Title:    "New member joined " + nodeNameN,
						Link:     "/patches/" + nodeSlugN + "/members",
					})
				} else if newStatus == "pending" {
					notify(notifications.Event{
						Type:     notifications.MembershipRequest,
						NodeID:   nodeID,
						NodeSlug: nodeSlugN,
						NodeName: nodeNameN,
						ActorID:  user.ID,
						Title:    "Membership request for " + nodeNameN,
						Link:     "/patches/" + nodeSlugN + "/members?status=pending",
					})
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": newStatus, "membership_id": existingID})
			return
		}

		// No existing membership — create new one.
		role := "member"
		newStatus := "active"
		auditAction := "membership.join"

		if isFollow {
			role = "follower"
			auditAction = "membership.follow"
		} else if membershipPolicy == "approval_required" {
			newStatus = "pending"
			auditAction = "membership.request"
		}

		id := auth.NewUUIDv7()
		_, err = db.Exec(
			`INSERT INTO memberships (id, user_id, node_id, role, status) VALUES (?, ?, ?, ?, ?)`,
			id, user.ID, nodeID, role, newStatus,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to join node"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, auditAction, "membership", id, "{}", clientIP(r))

		// Notify admins about member join/request (not for follows).
		if !isFollow {
			var nodeSlugN, nodeNameN string
			db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
			if newStatus == "active" {
				notify(notifications.Event{
					Type:     notifications.MembershipJoined,
					NodeID:   nodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					Title:    "New member joined " + nodeNameN,
					Link:     "/patches/" + nodeSlugN + "/members",
				})
			} else if newStatus == "pending" {
				notify(notifications.Event{
					Type:     notifications.MembershipRequest,
					NodeID:   nodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					Title:    "Membership request for " + nodeNameN,
					Link:     "/patches/" + nodeSlugN + "/members?status=pending",
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": newStatus, "membership_id": id})
	}
}

// LeaveNode handles POST /api/v1/nodes/{slug}/leave.
func LeaveNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Check if user is an active member.
		var memberRole string
		err := db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'", user.ID, nodeID).Scan(&memberRole)
		if err != nil {
			http.Error(w, `{"error":"not a member"}`, http.StatusBadRequest)
			return
		}

		// Cannot leave if you're the only admin.
		if memberRole == "admin" {
			var adminCount int
			db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
			if adminCount <= 1 {
				http.Error(w, `{"error":"cannot leave as the only admin"}`, http.StatusConflict)
				return
			}
		}

		_, err = db.Exec("UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?", user.ID, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to leave node"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "membership.leave", "membership", nodeID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ListMembers handles GET /api/v1/nodes/{slug}/members.
func ListMembers(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		after, limit := parsePaginationParams(r)

		// Default to active members only. Allow ?status=pending for node admins.
		statusFilter := r.URL.Query().Get("status")
		if statusFilter == "" {
			statusFilter = "active"
		}

		user := middleware.UserFromContext(r.Context())

		// Only allow pending/banned status filter for node admins or site admins.
		if statusFilter == "pending" || statusFilter == "banned" {
			if user == nil || (user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin")) {
				// Non-admins just see active members.
				statusFilter = "active"
			}
		}

		// The patch's admins and members see the full list, including hidden
		// memberships and followers. Everyone else gets the public view:
		// visible member/admin rows only — hidden memberships and follower
		// relationships are never public (docs/adr/006).
		insider := false
		if user != nil {
			if user.Role == "admin" {
				insider = true
			} else {
				var role string
				db.QueryRow(
					"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN ('member','admin')",
					user.ID, nodeID,
				).Scan(&role)
				insider = role != ""
			}
		}

		query := `SELECT m.id, m.user_id, m.node_id, m.role, m.status, m.joined_at, u.username, u.display_name, u.avatar_url
			FROM memberships m JOIN users u ON m.user_id = u.id
			WHERE m.node_id = ? AND m.status = ?`
		args := []interface{}{nodeID, statusFilter}
		if !insider {
			query += " AND m.visible = 1 AND m.role IN ('member','admin')"
		}

		if after != "" {
			query += " AND m.id > ?"
			args = append(args, after)
		}
		query += " ORDER BY m.id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list members"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type memberResponse struct {
			ID          string `json:"id"`
			UserID      string `json:"user_id"`
			NodeID      string `json:"node_id"`
			Role        string `json:"role"`
			Status      string `json:"status"`
			JoinedAt    string `json:"joined_at"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			AvatarURL   string `json:"avatar_url"`
		}
		var members []memberResponse
		for rows.Next() {
			var m memberResponse
			if err := rows.Scan(&m.ID, &m.UserID, &m.NodeID, &m.Role, &m.Status, &m.JoinedAt, &m.Username, &m.DisplayName, &m.AvatarURL); err != nil {
				continue
			}
			members = append(members, m)
		}

		var nextCursor string
		if len(members) > limit {
			nextCursor = members[limit-1].ID
			members = members[:limit]
		}
		if members == nil {
			members = []memberResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       members,
			"next_cursor": nextCursor,
		})
	}
}

// ListMyMemberships handles GET /api/v1/me/nodes.
func ListMyMemberships(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		after, limit := parsePaginationParams(r)

		query := `SELECT m.id, m.user_id, m.node_id, m.role, m.status, m.visible, m.joined_at,
			n.name, n.slug, n.description, n.visibility, n.membership_policy
			FROM memberships m JOIN nodes n ON m.node_id = n.id
			WHERE m.user_id = ? AND m.status IN ('active', 'pending') AND n.status IN ('active','unclaimed')`
		args := []interface{}{user.ID}

		if after != "" {
			query += " AND m.id > ?"
			args = append(args, after)
		}
		query += " ORDER BY m.id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list memberships"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type membershipResponse struct {
			ID               string `json:"id"`
			UserID           string `json:"user_id"`
			NodeID           string `json:"node_id"`
			Role             string `json:"role"`
			Status           string `json:"status"`
			Visible          bool   `json:"visible"`
			JoinedAt         string `json:"joined_at"`
			NodeName         string `json:"node_name"`
			NodeSlug         string `json:"node_slug"`
			NodeDescription  string `json:"node_description"`
			NodeVisibility   string `json:"node_visibility"`
			MembershipPolicy string `json:"membership_policy"`
		}
		var memberships []membershipResponse
		for rows.Next() {
			var m membershipResponse
			if err := rows.Scan(&m.ID, &m.UserID, &m.NodeID, &m.Role, &m.Status, &m.Visible, &m.JoinedAt,
				&m.NodeName, &m.NodeSlug, &m.NodeDescription, &m.NodeVisibility, &m.MembershipPolicy); err != nil {
				continue
			}
			memberships = append(memberships, m)
		}

		var nextCursor string
		if len(memberships) > limit {
			nextCursor = memberships[limit-1].ID
			memberships = memberships[:limit]
		}
		if memberships == nil {
			memberships = []membershipResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       memberships,
			"next_cursor": nextCursor,
		})
	}
}

// UpdateMember handles PATCH /api/v1/nodes/{slug}/members/{userId}.
func UpdateMember(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		targetUserID := r.PathValue("userId")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Check that the caller has admin role on the node, or is a site admin.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		// Look up the target membership.
		var memID, currentRole, currentStatus string
		err := db.QueryRow(
			"SELECT id, role, status FROM memberships WHERE user_id = ? AND node_id = ?",
			targetUserID, nodeID,
		).Scan(&memID, &currentRole, &currentStatus)
		if err != nil {
			http.Error(w, `{"error":"membership not found"}`, http.StatusNotFound)
			return
		}

		var req struct {
			Role   *string `json:"role"`
			Status *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Handle status changes (approve/reject/ban/reinstate).
		if req.Status != nil {
			switch *req.Status {
			case "active":
				// Approve a pending member.
				if currentStatus != "pending" {
					http.Error(w, `{"error":"can only approve pending members"}`, http.StatusBadRequest)
					return
				}
				_, err = db.Exec("UPDATE memberships SET status = 'active' WHERE id = ?", memID)
				if err != nil {
					http.Error(w, `{"error":"failed to approve member"}`, http.StatusInternalServerError)
					return
				}
				auth.LogAuditEvent(db, user.ID, "membership.approve", "membership", memID,
					fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))

				// Notify the approved user.
				var nodeSlugN, nodeNameN string
				db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
				notify(notifications.Event{
					Type:     notifications.MembershipApproved,
					NodeID:   nodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					TargetID: targetUserID,
					Title:    "Your membership in " + nodeNameN + " was approved",
					Link:     "/patches/" + nodeSlugN,
				})

			case "banned":
				// Ban an active member or follower.
				if currentStatus != "active" {
					http.Error(w, `{"error":"can only ban active members"}`, http.StatusBadRequest)
					return
				}
				// Cannot ban yourself.
				if targetUserID == user.ID {
					http.Error(w, `{"error":"cannot ban yourself"}`, http.StatusBadRequest)
					return
				}
				// Cannot ban the last admin.
				if currentRole == "admin" {
					var adminCount int
					db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
					if adminCount <= 1 {
						http.Error(w, `{"error":"cannot ban the last admin"}`, http.StatusConflict)
						return
					}
				}
				_, err = db.Exec("UPDATE memberships SET status = 'banned' WHERE id = ?", memID)
				if err != nil {
					http.Error(w, `{"error":"failed to ban member"}`, http.StatusInternalServerError)
					return
				}
				auth.LogAuditEvent(db, user.ID, "membership.ban", "membership", memID,
					fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))

				var banSlug, banName string
				db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&banSlug, &banName)
				notify(notifications.Event{
					Type:     notifications.MembershipBanned,
					NodeID:   nodeID,
					NodeSlug: banSlug,
					NodeName: banName,
					ActorID:  user.ID,
					TargetID: targetUserID,
					Title:    "You have been removed from " + banName,
					Body:     "A patch admin has removed you from this community.",
					Link:     "/patches/" + banSlug,
				})

			case "left":
				// Reject a pending member OR reinstate a banned member.
				if currentStatus == "pending" {
					_, err = db.Exec("UPDATE memberships SET status = 'left' WHERE id = ?", memID)
					if err != nil {
						http.Error(w, `{"error":"failed to reject member"}`, http.StatusInternalServerError)
						return
					}
					auth.LogAuditEvent(db, user.ID, "membership.reject", "membership", memID,
						fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))
				} else if currentStatus == "banned" {
					_, err = db.Exec("UPDATE memberships SET status = 'left' WHERE id = ?", memID)
					if err != nil {
						http.Error(w, `{"error":"failed to reinstate member"}`, http.StatusInternalServerError)
						return
					}
					auth.LogAuditEvent(db, user.ID, "membership.reinstate", "membership", memID,
						fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))

					var reinstateSlug, reinstateName string
					db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&reinstateSlug, &reinstateName)
					notify(notifications.Event{
						Type:     notifications.MembershipReinstated,
						NodeID:   nodeID,
						NodeSlug: reinstateSlug,
						NodeName: reinstateName,
						ActorID:  user.ID,
						TargetID: targetUserID,
						Title:    "You have been reinstated in " + reinstateName,
						Body:     "You can now rejoin this community.",
						Link:     "/patches/" + reinstateSlug,
					})
				} else {
					http.Error(w, `{"error":"can only reject pending or reinstate banned members"}`, http.StatusBadRequest)
					return
				}

			default:
				http.Error(w, `{"error":"invalid status value"}`, http.StatusBadRequest)
				return
			}
		}

		// Handle role changes.
		if req.Role != nil {
			newRole := *req.Role
			if newRole != "member" && newRole != "follower" && newRole != "admin" {
				http.Error(w, `{"error":"invalid role value"}`, http.StatusBadRequest)
				return
			}

			// Cannot demote the last admin.
			if currentRole == "admin" && newRole != "admin" {
				var adminCount int
				db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
				if adminCount <= 1 {
					http.Error(w, `{"error":"cannot demote the last admin"}`, http.StatusConflict)
					return
				}
			}

			_, err = db.Exec("UPDATE memberships SET role = ? WHERE id = ?", newRole, memID)
			if err != nil {
				http.Error(w, `{"error":"failed to update role"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "membership.role_change", "membership", memID,
				fmt.Sprintf(`{"target_user_id":"%s","old_role":"%s","new_role":"%s"}`, targetUserID, currentRole, newRole), clientIP(r))
		}

		// Return the updated membership.
		var updatedRole, updatedStatus, joinedAt string
		db.QueryRow("SELECT role, status, joined_at FROM memberships WHERE id = ?", memID).Scan(&updatedRole, &updatedStatus, &joinedAt)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":        memID,
			"user_id":   targetUserID,
			"node_id":   nodeID,
			"role":      updatedRole,
			"status":    updatedStatus,
			"joined_at": joinedAt,
		})
	}
}
