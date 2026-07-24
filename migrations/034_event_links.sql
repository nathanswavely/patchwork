-- 034: event links (docs/adr/032).
-- An explicit, mutual association between an event and a patch beyond its
-- owner: one side's admins propose, the other side's confirm. Pending
-- links are invisible everywhere. The event stays the owner's to edit —
-- a link grants presence, not control. The sync never writes here.

CREATE TABLE event_links (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'confirmed')),
    -- Which side proposed: 'owner' (the event's patch tagged node_id, so
    -- node_id's admins confirm) or 'linked' (node_id's admins requested
    -- onto the event, so the owner's admins confirm).
    initiated_by TEXT NOT NULL CHECK (initiated_by IN ('owner', 'linked')),
    requested_by TEXT NOT NULL REFERENCES users(id),
    -- The requester's duplicate of the same real-world event, chosen by a
    -- human in the request flow (docs/adr/032): deleted (and skip-listed
    -- if imported) when the link is confirmed. Never matched by machine.
    absorb_event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    confirmed_at TEXT,
    UNIQUE (event_id, node_id)
);
CREATE INDEX idx_event_links_node ON event_links(node_id, status);

-- Cross-quilt mentions (docs/adr/032): a display-only doorway on the
-- event page to a patch on another quilt. No handshake, no surfaces —
-- the standing of naming the band in the description. Owner-editable.
CREATE TABLE event_mentions (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    host TEXT NOT NULL,
    slug TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (event_id, host, slug)
);
