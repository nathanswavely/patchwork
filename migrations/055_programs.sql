-- 054: programs and their offers (docs/adr/063).
--
-- A crosswalk entry answers "whose calendar is this?" from the feed's
-- LOCATION. Nothing in the feed answers "who runs it?" — Lancaster's
-- city calendar carries no ORGANIZER on any event, and the organization
-- behind its heritage walking tour is not named in any title,
-- description, or CATEGORIES. So a person recognizes it, and a program
-- records what they recognized.
--
-- A program never routes. It decides nothing about who owns an event,
-- only who else belongs on one, and what it produces is an offer that a
-- person turns into an ordinary event link (docs/adr/032) which the
-- owning patch confirms. That is why title matching is safe here and
-- would not be safe in the crosswalk: a wrong program is declined, a
-- wrong route lands silently.

-- The normalized SUMMARY, alongside the normalized LOCATION that already
-- lives here. Grouping by title collapses this feed's 186 listings to
-- 111; grouping by Tockify's own series id manages only 154, because it
-- gave one seven-date tour seven different series ids.
ALTER TABLE aggregator_listings ADD COLUMN title_key TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_aggregator_listings_title
    ON aggregator_listings(aggregator_id, name_key, title_key);

-- One recurring title, under one name, credited to one patch.
--
-- Scoped to a name rather than to the whole aggregator: in this feed
-- exactly one title of 110 appears under two locations, so the cost is a
-- second program, and the benefit is that crediting says something about
-- a place you looked at rather than about every stage in the county.
--
-- node_id is the credited patch, never the owner. The owning patch is
-- untouched by a program: its calendar changes only if someone proposes
-- a link and its own admins confirm.
--
-- The unique index deliberately includes node_id, so two patches may be
-- credited the same program — a co-production is two links on one event,
-- which is what event_links already models. What it forbids is crediting
-- the same patch twice for the same title.
CREATE TABLE aggregator_programs (
    id TEXT PRIMARY KEY,
    aggregator_id TEXT NOT NULL REFERENCES aggregators(id) ON DELETE CASCADE,
    name_key TEXT NOT NULL,
    title_key TEXT NOT NULL,
    -- What the feed actually said, for the row the admin reads.
    display_title TEXT NOT NULL,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    -- Whoever spoke for the credited patch at the time (docs/adr/063).
    -- Recorded rather than derived: roles change, and the decision
    -- belongs to the moment it was made.
    credited_by TEXT NOT NULL REFERENCES users(id),
    -- Set once the first back-fill has run. Crediting is silent — nobody
    -- wants six notifications for a decision they just made — and every
    -- offer after that announces. Anchored on the program for the same
    -- reason docs/adr/056 anchored announcement on the crosswalk entry.
    backfilled_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX idx_aggregator_programs_unique
    ON aggregator_programs(aggregator_id, name_key, title_key, node_id);

CREATE INDEX idx_aggregator_programs_node ON aggregator_programs(node_id);

-- An offer the credited patch declined. The only stored part of an offer:
-- offers themselves are a query result, so there is nothing for the
-- reconciler to write and a program cannot quietly become a route
-- (docs/adr/063). Stored for the reason docs/adr/056 skip-lists a
-- rejected suggestion — otherwise the next sync re-offers it and the same
-- refusal is owed every hour.
CREATE TABLE aggregator_offer_dismissals (
    program_id TEXT NOT NULL REFERENCES aggregator_programs(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    dismissed_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (program_id, event_id)
);
