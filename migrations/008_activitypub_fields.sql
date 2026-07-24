-- Add ActivityPub-compatible fields to support future federation.
-- These columns are NULL until AP is activated in Phase 6.
-- SQLite doesn't support ALTER TABLE ADD COLUMN ... UNIQUE, so we add
-- the columns first and create unique indexes separately.

ALTER TABLE nodes ADD COLUMN ap_id TEXT;
ALTER TABLE nodes ADD COLUMN ap_type TEXT NOT NULL DEFAULT 'Organization';

ALTER TABLE events ADD COLUMN ap_id TEXT;
ALTER TABLE events ADD COLUMN ap_type TEXT NOT NULL DEFAULT 'Event';

ALTER TABLE users ADD COLUMN ap_id TEXT;
ALTER TABLE users ADD COLUMN ap_type TEXT NOT NULL DEFAULT 'Person';

CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_ap_id ON nodes(ap_id) WHERE ap_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_ap_id ON events(ap_id) WHERE ap_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_ap_id ON users(ap_id) WHERE ap_id IS NOT NULL;
