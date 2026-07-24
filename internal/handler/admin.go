package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// ListUsers handles GET /api/v1/admin/users.
func ListUsers(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)
		search := r.URL.Query().Get("search")

		query := `SELECT id, email, username, display_name, bio, avatar_url, role, trusted_contributor, suspended_at, created_at, updated_at FROM users`
		var conditions []string
		var args []interface{}

		if search != "" {
			conditions = append(conditions, "(username LIKE ? OR display_name LIKE ? OR email LIKE ?)")
			s := "%" + search + "%"
			args = append(args, s, s, s)
		}
		if after != "" {
			conditions = append(conditions, "id > ?")
			args = append(args, after)
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}
		query += " ORDER BY id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var users []model.User
		for rows.Next() {
			var u model.User
			var email *string
			if err := rows.Scan(&u.ID, &email, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.Role, &u.TrustedContributor, &u.SuspendedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
				continue
			}
			if email != nil {
				u.Email = *email
			}
			users = append(users, u)
		}

		var nextCursor string
		if len(users) > limit {
			nextCursor = users[limit-1].ID
			users = users[:limit]
		}
		if users == nil {
			users = []model.User{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       users,
			"next_cursor": nextCursor,
		})
	}
}

// UpdateUser handles PATCH /api/v1/admin/users/{id}.
func UpdateUser(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		targetID := r.PathValue("id")
		if targetID == "" {
			http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Role        *string `json:"role"`
			SuspendedAt *string `json:"suspended_at"`
			// The trusted-contributor grant (docs/adr/026): given and
			// revoked explicitly here, never earned automatically.
			TrustedContributor *bool `json:"trusted_contributor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		var setClauses []string
		var args []interface{}

		if req.Role != nil {
			if *req.Role != "member" && *req.Role != "admin" {
				http.Error(w, `{"error":"role must be member or admin"}`, http.StatusBadRequest)
				return
			}
			// Promotion to instance admin is one of the three step-up actions
			// (docs/adr/017): it hands someone the wipe button, so it needs a
			// fresh assertion rather than whatever cookie is lying around.
			// Demotion does not — taking privilege away is not the dangerous
			// direction, and requiring ceremony to revoke an account you have
			// just lost trust in is the wrong tradeoff.
			if *req.Role == "admin" && !middleware.SudoSatisfied(db, r) {
				var currentRole string
				db.QueryRow("SELECT role FROM users WHERE id = ?", targetID).Scan(&currentRole)
				if currentRole != "admin" {
					middleware.WriteSudoRequired(db, w, r)
					return
				}
			}
			setClauses = append(setClauses, "role = ?")
			args = append(args, *req.Role)
		}

		if req.TrustedContributor != nil {
			setClauses = append(setClauses, "trusted_contributor = ?")
			args = append(args, *req.TrustedContributor)
		}

		if req.SuspendedAt != nil {
			if *req.SuspendedAt == "" {
				// Unsuspend: set to null.
				setClauses = append(setClauses, "suspended_at = NULL")
			} else {
				// Suspend: set to now.
				setClauses = append(setClauses, "suspended_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
			}
		}

		if len(setClauses) == 0 {
			http.Error(w, `{"error":"no valid fields to update"}`, http.StatusBadRequest)
			return
		}

		setClauses = append(setClauses, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
		args = append(args, targetID)

		result, err := db.Exec(
			fmt.Sprintf("UPDATE users SET %s WHERE id = ?", strings.Join(setClauses, ", ")),
			args...,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update user"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.user_update", "user", targetID, "{}", clientIP(r))

		// Send notification if suspended/unsuspended.
		if req.SuspendedAt != nil {
			if *req.SuspendedAt == "" {
				CreateNotification(db, targetID, "account.unsuspended", "Account Restored",
					"Your account suspension has been lifted.", "/settings")
			} else {
				// Revoke live sessions so the suspension takes effect now
				// rather than whenever their cookie happens to expire.
				if err := auth.DestroyUserSessions(db, targetID); err != nil {
					log.Printf("admin: revoke sessions for suspended user %s: %v", targetID, err)
				}
				CreateNotification(db, targetID, "account.suspended", "Account Suspended",
					"Your account has been suspended due to a policy violation.", "/settings")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// auditEntryWithUser is an audit log entry enriched with user display info.
type auditEntryWithUser struct {
	model.AuditEntry
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// AuditLog handles GET /api/v1/admin/audit-log.
func AuditLog(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)
		action := r.URL.Query().Get("action")
		entityType := r.URL.Query().Get("entity_type")
		userID := r.URL.Query().Get("user_id")
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")

		query := `SELECT a.id, a.user_id, a.action, a.entity_type, a.entity_id, a.metadata, a.ip_address, a.created_at,
			COALESCE(u.username, '') AS username, COALESCE(u.display_name, '') AS display_name
			FROM audit_log a LEFT JOIN users u ON a.user_id = u.id`
		var conditions []string
		var args []interface{}

		if action != "" {
			conditions = append(conditions, "a.action = ?")
			args = append(args, action)
		}
		if entityType != "" {
			conditions = append(conditions, "a.entity_type = ?")
			args = append(args, entityType)
		}
		if userID != "" {
			conditions = append(conditions, "a.user_id = ?")
			args = append(args, userID)
		}
		if from != "" {
			conditions = append(conditions, "a.created_at >= ?")
			args = append(args, from)
		}
		if to != "" {
			conditions = append(conditions, "a.created_at <= ?")
			args = append(args, to)
		}
		if after != "" {
			conditions = append(conditions, "a.id < ?")
			args = append(args, after)
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}
		query += " ORDER BY a.id DESC LIMIT ?"
		args = append(args, limit+1)

		dbRows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list audit log"}`, http.StatusInternalServerError)
			return
		}
		defer dbRows.Close()

		var entries []auditEntryWithUser
		for dbRows.Next() {
			var e auditEntryWithUser
			var uid *string
			if err := dbRows.Scan(&e.ID, &uid, &e.Action, &e.EntityType, &e.EntityID, &e.Metadata, &e.IPAddress, &e.CreatedAt, &e.Username, &e.DisplayName); err != nil {
				continue
			}
			if uid != nil {
				e.UserID = *uid
			}
			entries = append(entries, e)
		}

		var nextCursor string
		if len(entries) > limit {
			nextCursor = entries[limit-1].ID
			entries = entries[:limit]
		}
		if entries == nil {
			entries = []auditEntryWithUser{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       entries,
			"next_cursor": nextCursor,
		})
	}
}

// AdminStats handles GET /api/v1/admin/stats.
func AdminStats(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var totalUsers, activeUsers30d, totalNodes, totalEvents int
		var openProposals, passedProposals, rejectedProposals int
		var pendingReports, recentSignups7d int

		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&totalUsers)
		db.QueryRow("SELECT COUNT(DISTINCT user_id) FROM sessions WHERE expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-30 days')").Scan(&activeUsers30d)
		db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'active' AND removed_at IS NULL").Scan(&totalNodes)
		db.QueryRow("SELECT COUNT(*) FROM events WHERE removed_at IS NULL").Scan(&totalEvents)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE status = 'open'").Scan(&openProposals)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE status = 'approved'").Scan(&passedProposals)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE status = 'rejected'").Scan(&rejectedProposals)
		db.QueryRow("SELECT COUNT(*) FROM content_reports WHERE status = 'pending'").Scan(&pendingReports)
		db.QueryRow("SELECT COUNT(*) FROM users WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-7 days')").Scan(&recentSignups7d)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{
			"total_users":        totalUsers,
			"active_users_30d":   activeUsers30d,
			"total_nodes":        totalNodes,
			"total_events":       totalEvents,
			"open_proposals":     openProposals,
			"passed_proposals":   passedProposals,
			"rejected_proposals": rejectedProposals,
			"pending_reports":    pendingReports,
			"recent_signups_7d":  recentSignups7d,
		})
	}
}
