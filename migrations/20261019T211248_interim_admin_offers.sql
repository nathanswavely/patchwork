-- An interim admin is asked first
-- (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md).
--
-- One row per offer the inactivity sweep makes when a longest-tenure patch's
-- council empties. Nobody becomes an admin until the person answers; the row
-- records the question and, once given, the answer.
--
-- Answered rows are cleared when the sweep next finds the council filled, so a
-- later emptying may ask the same people again. The audit log keeps every
-- answer.
CREATE TABLE interim_admin_offers (
    id          TEXT PRIMARY KEY,
    node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    offered_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    answered_at TEXT,
    answer      TEXT CHECK (answer IN ('accepted', 'declined'))
);

-- One open offer per person per patch.
CREATE UNIQUE INDEX idx_interim_admin_offers_open
    ON interim_admin_offers(node_id, user_id) WHERE answered_at IS NULL;

CREATE INDEX idx_interim_admin_offers_user ON interim_admin_offers(user_id);
