-- An event says who it is for: public / followers / members.
-- docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md
--
-- events.visibility has held ('public','private','unlisted') since
-- migration 001 — two tiers in practice and the wrong two words, because
-- the role an event is for is the thing a patch actually wants to name.
-- The new set names roles: 'followers' reaches the patch's followers,
-- members and admins; 'members' stops at members and admins.
--
-- 'private' and 'unlisted' both become 'members' — the non-exposing
-- direction, rather than promoting rows somebody deliberately hid. The
-- CSV upload (internal/handler/event_upload.go) is the one door that has
-- ever written either value, so there is live data to map.
--
-- This touches events.visibility only. nodes.visibility is a different
-- column with the same three old words and its own meaning (a private
-- patch is unlisted, not locked) and keeps them; CONTEXT.md now says the
-- two vocabularies have parted.
--
-- SQLite cannot alter a CHECK in place, so the table is rebuilt the way
-- 065 and 071 rebuilt memberships. Carried across: all 25 columns, and
-- all five indexes (001's node_id and starts_at, 008's partial unique
-- ap_id, 030's status, 033's partial unique source identity).
--
-- One thing those two precedents did not have to deal with. memberships
-- is nobody's parent; `events` is, four times over — event_links.event_id
-- and .absorb_event_id, event_mentions.event_id, aggregator_holds
-- .rival_event_id, aggregator_offer_dismissals.event_id. The migration
-- runner wraps each file in a transaction (internal/database/database.go),
-- PRAGMA foreign_keys cannot be changed inside one, and the pool opens
-- with it ON — so `DROP TABLE events` performs its implicit DELETE FROM
-- with cascades live and takes every one of those child rows with it.
-- Measured, not assumed: a plain rebuild of this table empties
-- event_links. PRAGMA defer_foreign_keys does not help (it defers the
-- constraint check, not the ON DELETE action) and PRAGMA
-- legacy_alter_table is not honoured inside the transaction either.
--
-- So the children are set aside and put back. Enumerating them by hand
-- is right here and cannot rot: a migration runs once, against the schema
-- as it stands today, and a table added tomorrow is not this file's
-- problem. The rebuild's own test carries a row in each of the four.

CREATE TEMP TABLE _mig_event_links AS SELECT * FROM event_links;
CREATE TEMP TABLE _mig_event_mentions AS SELECT * FROM event_mentions;
CREATE TEMP TABLE _mig_aggregator_holds AS SELECT * FROM aggregator_holds;
CREATE TEMP TABLE _mig_offer_dismissals AS SELECT * FROM aggregator_offer_dismissals;

CREATE TABLE events_new (
    id                TEXT PRIMARY KEY,
    node_id           TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_by        TEXT NOT NULL REFERENCES users(id),
    title             TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    latitude          REAL,
    longitude         REAL,
    starts_at         TEXT NOT NULL,
    ends_at           TEXT,
    recurrence        TEXT NOT NULL DEFAULT '',
    visibility        TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'followers', 'members')),
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    removed_at        TEXT,
    ap_id             TEXT,
    ap_type           TEXT NOT NULL DEFAULT 'Event',
    status            TEXT NOT NULL DEFAULT 'active',
    source_id         TEXT REFERENCES event_sources(id),
    source_uid        TEXT,
    source_occurrence TEXT NOT NULL DEFAULT '',
    image_url         TEXT NOT NULL DEFAULT '',
    image_alt         TEXT NOT NULL DEFAULT '',
    timezone          TEXT,
    event_url         TEXT NOT NULL DEFAULT ''
);

INSERT INTO events_new
    (id, node_id, created_by, title, description, location, latitude, longitude,
     starts_at, ends_at, recurrence, visibility, created_at, updated_at, removed_at,
     ap_id, ap_type, status, source_id, source_uid, source_occurrence,
     image_url, image_alt, timezone, event_url)
  SELECT id, node_id, created_by, title, description, location, latitude, longitude,
     starts_at, ends_at, recurrence,
     CASE WHEN visibility IN ('private', 'unlisted') THEN 'members' ELSE visibility END,
     created_at, updated_at, removed_at,
     ap_id, ap_type, status, source_id, source_uid, source_occurrence,
     image_url, image_alt, timezone, event_url
  FROM events;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

CREATE INDEX idx_events_node_id ON events(node_id);
CREATE INDEX idx_events_starts_at ON events(starts_at);
CREATE INDEX idx_events_status ON events(status);
CREATE UNIQUE INDEX idx_events_ap_id ON events(ap_id) WHERE ap_id IS NOT NULL;
CREATE UNIQUE INDEX idx_events_source_identity
    ON events(source_id, source_uid, source_occurrence)
    WHERE source_id IS NOT NULL;

INSERT INTO event_links SELECT * FROM _mig_event_links;
INSERT INTO event_mentions SELECT * FROM _mig_event_mentions;
INSERT INTO aggregator_holds SELECT * FROM _mig_aggregator_holds;
INSERT INTO aggregator_offer_dismissals SELECT * FROM _mig_offer_dismissals;

DROP TABLE _mig_event_links;
DROP TABLE _mig_event_mentions;
DROP TABLE _mig_aggregator_holds;
DROP TABLE _mig_offer_dismissals;

-- The per-source default tier, applied when the sync inserts a row and
-- never when it updates one (internal/eventsource/sync.go): visibility is
-- local policy, not a fact the feed is authoritative about. Existing feeds
-- keep publishing publicly, which is what they have always done.
ALTER TABLE event_sources ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public'
    CHECK (visibility IN ('public', 'followers', 'members'));
