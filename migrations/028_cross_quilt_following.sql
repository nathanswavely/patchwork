-- 027_cross_quilt_following.sql
-- Cross-quilt following (docs/adr/024). A person's follows of patches on
-- other quilts live on their home instance and upgrade to a federated
-- Follow relayed by the instance service actor — never by the person's
-- own actor (followers collections are publicly enumerable; docs/adr/024).

-- A follow of a patch on another quilt. Keyed by the remote patch's AP id;
-- snapshot holds enough public display data (name, appearance, counts) to
-- draw the tile while the remote quilt is unreachable. Never auto-deleted:
-- deletion and going-private are indistinguishable from outside.
CREATE TABLE remote_follows (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    quilt_url   TEXT NOT NULL,              -- normalized origin, e.g. https://arts.lancaster.example
    node_ap_id  TEXT NOT NULL,              -- remote patch actor AP id
    node_slug   TEXT NOT NULL,
    node_name   TEXT NOT NULL DEFAULT '',
    snapshot    TEXT NOT NULL DEFAULT '{}', -- JSON display snapshot, refreshed opportunistically
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(user_id, node_ap_id)
);
CREATE INDEX idx_remote_follows_user ON remote_follows(user_id);
CREATE INDEX idx_remote_follows_node ON remote_follows(node_ap_id);

-- Personal connected quilts: quilts a signed-in person browses via the
-- switcher, on top of the instance's neighbor quilts. Account-backed so
-- the switcher survives a fresh device (docs/adr/024).
CREATE TABLE user_quilts (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,              -- normalized origin
    name        TEXT NOT NULL DEFAULT '',   -- display name captured at add time
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(user_id, url)
);
CREATE INDEX idx_user_quilts_user ON user_quilts(user_id);

-- Neighbor quilts: the instance's own public statement of adjacency,
-- curated by the instance admin, visible to every visitor (docs/adr/024).
CREATE TABLE neighbor_quilts (
    id          TEXT PRIMARY KEY,
    url         TEXT NOT NULL UNIQUE,       -- normalized origin
    name        TEXT NOT NULL DEFAULT '',
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- The instance service actor: one Application actor per quilt that relays
-- Follows for all its people, so no person is ever enumerable in a remote
-- followers collection (docs/adr/024).
CREATE TABLE instance_actor (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    ap_id       TEXT NOT NULL,
    public_key  TEXT NOT NULL,
    private_key TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Outbound follows sent by the instance actor. One row per remote patch,
-- regardless of how many local people follow it; accepted flips when the
-- remote quilt's Accept arrives.
CREATE TABLE ap_following (
    id              TEXT PRIMARY KEY,
    remote_actor_id TEXT NOT NULL UNIQUE,   -- remote patch actor AP id
    remote_inbox    TEXT NOT NULL DEFAULT '',
    accepted        INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
