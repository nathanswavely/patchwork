package notifications

import (
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Preference represents a user's choice for a specific type+channel.
type Preference struct {
	Type    NotificationType `json:"type"`
	Channel string           `json:"channel"`
	Enabled bool             `json:"enabled"`
}

// IsChannelEnabled checks whether a user has a specific channel enabled for a notification type.
// Falls back to TypeRegistry defaults when no preference row exists.
func IsChannelEnabled(db *database.DB, userID string, t NotificationType, channel string) bool {
	var enabled int
	err := db.QueryRow(
		`SELECT enabled FROM notification_preferences WHERE user_id = ? AND notification_type = ? AND channel = ?`,
		userID, string(t), channel,
	).Scan(&enabled)
	if err != nil {
		// No preference row — use default.
		return DefaultEnabled(t, channel)
	}
	return enabled == 1
}

// GetUserPreferences returns all preferences for a user, merged with defaults.
// Returns a map keyed by "type:channel".
func GetUserPreferences(db *database.DB, userID string, channels []string) []Preference {
	// Start with defaults for all types × channels.
	prefs := make(map[string]Preference)
	for t := range TypeRegistry {
		for _, ch := range channels {
			key := string(t) + ":" + ch
			prefs[key] = Preference{
				Type:    t,
				Channel: ch,
				Enabled: DefaultEnabled(t, ch),
			}
		}
	}

	// Override with user's saved preferences.
	rows, err := db.Query(
		`SELECT notification_type, channel, enabled FROM notification_preferences WHERE user_id = ?`,
		userID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t, ch string
			var enabled int
			if err := rows.Scan(&t, &ch, &enabled); err == nil {
				key := t + ":" + ch
				if p, ok := prefs[key]; ok {
					p.Enabled = enabled == 1
					prefs[key] = p
				}
			}
		}
	}

	// Flatten to slice.
	result := make([]Preference, 0, len(prefs))
	for _, p := range prefs {
		result = append(result, p)
	}
	return result
}

// SetUserPreference upserts a single preference.
func SetUserPreference(db *database.DB, userID string, t NotificationType, channel string, enabled bool) {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	id := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO notification_preferences (id, user_id, notification_type, channel, enabled)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, notification_type, channel) DO UPDATE SET enabled = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		id, userID, string(t), channel, enabledInt, enabledInt,
	)
}

// IsCategoryEnabled checks whether a notification category is enabled for a patch.
// Missing rows default to enabled.
func IsCategoryEnabled(db *database.DB, nodeID string, cat Category) bool {
	var enabled int
	err := db.QueryRow(
		`SELECT enabled FROM patch_notification_config WHERE node_id = ? AND category = ?`,
		nodeID, string(cat),
	).Scan(&enabled)
	if err != nil {
		return true // Default: all categories enabled.
	}
	return enabled == 1
}

// SetCategoryEnabled upserts a patch notification category toggle.
func SetCategoryEnabled(db *database.DB, nodeID string, cat Category, enabled bool) {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	id := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO patch_notification_config (id, node_id, category, enabled)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(node_id, category) DO UPDATE SET enabled = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		id, nodeID, string(cat), enabledInt, enabledInt,
	)
}

// GetPatchNotificationConfig returns all category toggles for a patch, merged with defaults.
func GetPatchNotificationConfig(db *database.DB, nodeID string) map[Category]bool {
	config := make(map[Category]bool)
	for _, ci := range AllCategories() {
		config[ci.ID] = true // Default: all enabled.
	}

	rows, err := db.Query(
		`SELECT category, enabled FROM patch_notification_config WHERE node_id = ?`,
		nodeID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat string
			var enabled int
			if err := rows.Scan(&cat, &enabled); err == nil {
				config[Category(cat)] = enabled == 1
			}
		}
	}

	return config
}
