-- 007_moderation_and_notifications.sql
-- Add suspension support, content removal support, and notifications table.

-- Add suspension support
ALTER TABLE users ADD COLUMN suspended_at TEXT;

-- Add soft-delete support for content moderation
ALTER TABLE nodes ADD COLUMN removed_at TEXT;
ALTER TABLE events ADD COLUMN removed_at TEXT;

-- Notifications table
CREATE TABLE notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    link TEXT NOT NULL DEFAULT '',
    read_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_notifications_user_id ON notifications(user_id);
CREATE INDEX idx_notifications_read ON notifications(user_id, read_at);
