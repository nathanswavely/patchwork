-- 053: aggregators and the crosswalk (docs/adr/056).
--
-- An aggregator is an instance-level feed that lists events it does not
-- own — a city tourism calendar, a chamber of commerce, an alt-weekly.
-- It owns nothing, has no tile, and creates no event until a crosswalk
-- entry addresses one.
--
-- A crosswalk entry is an event source whose feed is one name inside an
-- aggregator: event_sources gains aggregator_id + name_key, and every
-- rule ADR 031 already enforces (read-only imports, detach, skip lists,
-- silent first sync) applies unchanged. That reuse is what anchors
-- announcement on the entry rather than the aggregator — the entry's own
-- last_success_at makes its first routing pass silent back-fill.

CREATE TABLE aggregators (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'ics',
    url TEXT NOT NULL UNIQUE,
    added_by TEXT NOT NULL REFERENCES users(id),
    -- Paused aggregators never fetch. A seamrip import arrives paused
    -- (docs/adr/056): the crosswalk is community labour and travels, the
    -- standing to write onto patches is re-vouched by the new steward.
    paused INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    last_fetch_at TEXT,
    last_success_at TEXT,
    last_error TEXT,
    etag TEXT,
    last_modified TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- What the most recent successful fetch carried. Cached so N crosswalk
-- entries mean one fetch rather than N, and so unrouted names have
-- somewhere to live: a name is only mappable if an admin can see it.
-- Replaced wholesale each successful sync.
CREATE TABLE aggregator_listings (
    aggregator_id TEXT NOT NULL REFERENCES aggregators(id) ON DELETE CASCADE,
    uid TEXT NOT NULL,
    occurrence TEXT NOT NULL DEFAULT '',
    -- name_key is the normalized first field of the location
    -- (docs/adr/046): case-folded, punctuation and whitespace collapsed.
    -- display_name is what the feed actually said, shown to the admin
    -- who decides whether it means anything.
    name_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    latitude REAL,
    longitude REAL,
    starts_at TEXT NOT NULL,
    ends_at TEXT,
    -- The feed's own page for this listing. Deciding whether a name
    -- means an organization usually needs the listing as its publisher
    -- wrote it, and no summary substitutes for going and looking.
    url TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (aggregator_id, uid, occurrence)
);

CREATE INDEX idx_aggregator_listings_name ON aggregator_listings(aggregator_id, name_key);

-- A crosswalk entry: one name in one aggregator, addressed to one patch.
-- node_id is the patch; the row also carries the ADR 031 source state,
-- so routing is reconcile() with a different item source.
--
-- Its url is the aggregator's address with the name as a fragment —
-- "the Binns Park part of that feed", which is what fragments are for.
-- It has to be distinct per entry because event_sources carries a
-- table-level UNIQUE (node_id, url) from migration 033, and a patch
-- legitimately answers to several names on one aggregator. Rebuilding
-- that table to relax the constraint would mean dropping a parent of
-- events.source_id inside a migration transaction, where PRAGMA
-- foreign_keys is a no-op; a fragment costs nothing and lies about
-- nothing.
ALTER TABLE event_sources ADD COLUMN aggregator_id TEXT REFERENCES aggregators(id) ON DELETE CASCADE;
ALTER TABLE event_sources ADD COLUMN name_key TEXT;

-- A suggesting entry routes into the patch's review queue instead of
-- publishing (docs/adr/056). Set when the entry was made by the instance
-- admin on a claimed patch that accepts event suggestions: the patch said
-- "suggest to me", which ADR 031 distinguishes from accepting a feed that
-- produces events indefinitely. Its own admins mapping themselves is the
-- standing consent, and publishes directly.
--
-- Recorded rather than derived from who added it: roles change, and the
-- decision belongs to the moment the entry was made.
ALTER TABLE event_sources ADD COLUMN suggests INTEGER NOT NULL DEFAULT 0;

-- One patch per name per aggregator. A patch may hold several names —
-- Binns Park arrives four ways — but a name addresses one patch, or the
-- same event would land twice.
CREATE UNIQUE INDEX idx_event_sources_crosswalk
    ON event_sources(aggregator_id, name_key)
    WHERE aggregator_id IS NOT NULL;

-- A listing whose start instant collides with an event the patch already
-- has. Held rather than created, and rather than guessed at: the patch's
-- own event wins until one of its admins says otherwise (docs/adr/056).
CREATE TABLE aggregator_holds (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES event_sources(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    uid TEXT NOT NULL,
    occurrence TEXT NOT NULL DEFAULT '',
    -- The patch's own event this collided with, so the admin compares
    -- two things rather than judging one in the abstract.
    rival_event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    location TEXT NOT NULL DEFAULT '',
    starts_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (source_id, uid, occurrence)
);

CREATE INDEX idx_aggregator_holds_node ON aggregator_holds(node_id);

-- Names the instance admin has judged to mean no organization — "PA",
-- "Downtown", a room number. Deciding a name means nothing is the same
-- act of curation as deciding what it means, so it is recorded rather
-- than re-made every time the list is opened.
--
-- It hides the name from the instance admin's list only. A patch's own
-- admins still see every unmapped name their picker carries: whether a
-- patch answers to a name is that patch's judgement, and this table must
-- not become a way for instance authority to pre-empt it (docs/adr/056).
CREATE TABLE aggregator_ignored_names (
    aggregator_id TEXT NOT NULL REFERENCES aggregators(id) ON DELETE CASCADE,
    name_key TEXT NOT NULL,
    ignored_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (aggregator_id, name_key)
);
