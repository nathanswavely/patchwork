-- 033: event sources and calendar feeds (docs/adr/031).
-- A standing feed a patch pulls events from. Attaching is vouching for
-- the feed once (owner-only: patch admins on their own patch, instance
-- admin on unclaimed), so imported events publish without per-event
-- review. The source stays authoritative: imported events are read-only
-- until detached.

CREATE TABLE event_sources (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    type TEXT NOT NULL DEFAULT 'ics',
    url TEXT NOT NULL,
    added_by TEXT NOT NULL REFERENCES users(id),
    -- pending: attached, first sync not yet completed.
    -- ok / error: outcome of the most recent fetch. A failed fetch only
    -- records itself here; it never touches events (docs/adr/031).
    status TEXT NOT NULL DEFAULT 'pending',
    last_fetch_at TEXT,
    last_success_at TEXT,
    last_error TEXT,
    -- Conditional-GET state from the most recent successful fetch.
    etag TEXT,
    last_modified TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (node_id, url)
);

-- Feed items an admin has cut loose (detach) or deleted: the sync
-- reconciler never touches these keys again. occurrence is '' for
-- non-recurring items, the occurrence date for expanded RRULE rows.
CREATE TABLE event_source_skips (
    source_id TEXT NOT NULL REFERENCES event_sources(id) ON DELETE CASCADE,
    uid TEXT NOT NULL,
    occurrence TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (source_id, uid, occurrence)
);

-- Provenance on imported events. NULL source_id means an ordinary local
-- event (including detached ones — detach nulls these columns).
ALTER TABLE events ADD COLUMN source_id TEXT REFERENCES event_sources(id);
ALTER TABLE events ADD COLUMN source_uid TEXT;
ALTER TABLE events ADD COLUMN source_occurrence TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_events_source_identity
    ON events(source_id, source_uid, source_occurrence)
    WHERE source_id IS NOT NULL;

-- Personal feed secret (docs/adr/031): hashed like session tokens, so
-- the feed URL is shown once at generation. NULL means no personal feed.
ALTER TABLE users ADD COLUMN feed_secret_hash TEXT;

CREATE INDEX idx_users_feed_secret ON users(feed_secret_hash)
    WHERE feed_secret_hash IS NOT NULL;
