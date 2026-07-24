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
)

// CreateNotification inserts a notification for the given user.
// This is a helper used by other handlers (report resolution, suspension, etc.).
func CreateNotification(db *database.DB, userID, notifType, title, body, link string) {
	id := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO notifications (id, user_id, type, title, body, link) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, notifType, title, body, link,
	)
}

// ListNotifications handles GET /api/v1/notifications.
func ListNotifications(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		after, limit := parsePaginationParams(r)
		unread := r.URL.Query().Get("unread")

		query := `SELECT id, user_id, type, title, body, link, read_at, created_at FROM notifications`
		var conditions []string
		var args []interface{}

		conditions = append(conditions, "user_id = ?")
		args = append(args, user.ID)

		if unread == "true" {
			conditions = append(conditions, "read_at IS NULL")
		}

		if category := r.URL.Query().Get("category"); category != "" {
			// Map category name to type prefix: "proposals" → "proposal.%"
			prefix := strings.TrimSuffix(category, "s") + ".%"
			conditions = append(conditions, "type LIKE ?")
			args = append(args, prefix)
		}

		if after != "" {
			conditions = append(conditions, "id < ?")
			args = append(args, after)
		}

		query += " WHERE " + strings.Join(conditions, " AND ")
		query += " ORDER BY id DESC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list notifications"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var notifications []model.Notification
		for rows.Next() {
			var n model.Notification
			if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Link, &n.ReadAt, &n.CreatedAt); err != nil {
				continue
			}
			notifications = append(notifications, n)
		}

		var nextCursor string
		if len(notifications) > limit {
			nextCursor = notifications[limit-1].ID
			notifications = notifications[:limit]
		}
		if notifications == nil {
			notifications = []model.Notification{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       notifications,
			"next_cursor": nextCursor,
		})
	}
}

// NotificationCount handles GET /api/v1/notifications/count.
func NotificationCount(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var count int
		db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL", user.ID).Scan(&count)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"unread": count})
	}
}

// MarkNotificationRead handles PATCH /api/v1/notifications/{id}/read.
func MarkNotificationRead(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		notifID := r.PathValue("id")
		if notifID == "" {
			http.Error(w, `{"error":"notification id required"}`, http.StatusBadRequest)
			return
		}

		result, err := db.Exec(
			`UPDATE notifications SET read_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ? AND user_id = ? AND read_at IS NULL`,
			notifID, user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to mark notification read"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"notification not found or already read"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// MarkAllNotificationsRead handles POST /api/v1/notifications/read-all.
func MarkAllNotificationsRead(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		result, err := db.Exec(
			`UPDATE notifications SET read_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE user_id = ? AND read_at IS NULL`,
			user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to mark all notifications read"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"updated": rows,
		})
	}
}

// GetNotificationPreferences handles GET /api/v1/notifications/preferences.
// Returns the user's merged preferences grouped by category, plus available channels.
func GetNotificationPreferences(db *database.DB, notifier *notifications.Notifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		channels := notifier.AvailableChannels()
		prefs := notifications.GetUserPreferences(db, user.ID, channels)

		// Group by category for the UI.
		type TypePref struct {
			Type    string          `json:"type"`
			Label   string          `json:"label"`
			Channels map[string]bool `json:"channels"`
		}
		type CategoryGroup struct {
			ID          string     `json:"id"`
			Label       string     `json:"label"`
			Description string     `json:"description"`
			Types       []TypePref `json:"types"`
		}

		// Build lookup from flat prefs list.
		prefMap := make(map[string]map[string]bool) // type -> channel -> enabled
		for _, p := range prefs {
			if prefMap[string(p.Type)] == nil {
				prefMap[string(p.Type)] = make(map[string]bool)
			}
			prefMap[string(p.Type)][p.Channel] = p.Enabled
		}

		var groups []CategoryGroup
		for _, cat := range notifications.AllCategories() {
			types := notifications.TypesForCategory(cat.ID)
			var typePrefsList []TypePref
			for _, t := range types {
				meta := notifications.TypeRegistry[t]
				chMap := make(map[string]bool)
				for _, ch := range channels {
					if m, ok := prefMap[string(t)]; ok {
						chMap[ch] = m[ch]
					} else {
						chMap[ch] = notifications.DefaultEnabled(t, ch)
					}
				}
				typePrefsList = append(typePrefsList, TypePref{
					Type:     string(t),
					Label:    meta.Label,
					Channels: chMap,
				})
			}
			groups = append(groups, CategoryGroup{
				ID:          string(cat.ID),
				Label:       cat.Label,
				Description: cat.Description,
				Types:       typePrefsList,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"channels":   channels,
			"categories": groups,
		})
	}
}

// UpdateNotificationPreferences handles PUT /api/v1/notifications/preferences.
// Body: { "preferences": [{ "type": "proposal.new", "channel": "in_app", "enabled": true }, ...] }
func UpdateNotificationPreferences(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var body struct {
			Preferences []struct {
				Type    string `json:"type"`
				Channel string `json:"channel"`
				Enabled bool   `json:"enabled"`
			} `json:"preferences"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		for _, p := range body.Preferences {
			notifications.SetUserPreference(db, user.ID, notifications.NotificationType(p.Type), p.Channel, p.Enabled)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// GetPatchNotifConfig handles GET /api/v1/nodes/{slug}/notification-config.
func GetPatchNotifConfig(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var nodeID string
		if err := db.QueryRow(`SELECT id FROM nodes WHERE slug = ? AND removed_at IS NULL`, slug).Scan(&nodeID); err != nil {
			http.Error(w, `{"error":"patch not found"}`, http.StatusNotFound)
			return
		}

		config := notifications.GetPatchNotificationConfig(db, nodeID)

		type CategoryToggle struct {
			ID          string `json:"id"`
			Label       string `json:"label"`
			Description string `json:"description"`
			Enabled     bool   `json:"enabled"`
		}

		var result []CategoryToggle
		for _, cat := range notifications.AllCategories() {
			result = append(result, CategoryToggle{
				ID:          string(cat.ID),
				Label:       cat.Label,
				Description: cat.Description,
				Enabled:     config[cat.ID],
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"categories": result})
	}
}

// UpdatePatchNotifConfig handles PUT /api/v1/nodes/{slug}/notification-config.
// Body: { "categories": { "proposals": true, "events": false, ... } }
func UpdatePatchNotifConfig(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var nodeID string
		if err := db.QueryRow(`SELECT id FROM nodes WHERE slug = ? AND removed_at IS NULL`, slug).Scan(&nodeID); err != nil {
			http.Error(w, `{"error":"patch not found"}`, http.StatusNotFound)
			return
		}

		var body struct {
			Categories map[string]bool `json:"categories"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		for cat, enabled := range body.Categories {
			notifications.SetCategoryEnabled(db, nodeID, notifications.Category(cat), enabled)
		}

		auth.LogAuditEvent(db, middleware.UserFromContext(r.Context()).ID, "notification_config.updated", "node", nodeID, "", "")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
