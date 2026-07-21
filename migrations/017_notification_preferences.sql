-- User per-type, per-channel notification preferences.
-- One row per (user, type, channel). Missing rows = use defaults from Go code.
-- channel is 'in_app', 'email', or any future channel name.
CREATE TABLE IF NOT EXISTS notification_preferences (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT 'in_app',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(user_id, notification_type, channel)
);
CREATE INDEX IF NOT EXISTS idx_notification_prefs_user ON notification_preferences(user_id);

-- Patch-level category toggles. Admins control which notification categories
-- are active for their patch. Missing rows = all categories enabled.
CREATE TABLE IF NOT EXISTS patch_notification_config (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(node_id, category)
);
CREATE INDEX IF NOT EXISTS idx_patch_notif_config_node ON patch_notification_config(node_id);

-- Reminder dedup. Prevents double-sending deadline/event reminders.
-- entity_type is 'proposal' or 'event', reminder_type is 'deadline' or 'reminder'.
CREATE TABLE IF NOT EXISTS notification_reminders_sent (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    reminder_type TEXT NOT NULL,
    sent_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(entity_type, entity_id, reminder_type)
);
