-- Migration 063: the noticeboard (docs/adr/081).
--
-- A patch's members-only room for notices. Read by the patch's active admins
-- and members and by nobody else — the check is in the handler, never a
-- hidden tab (docs/adr/050 is about tabs that hide public reads; this is
-- the second thing in a workspace that is genuinely withheld, after
-- members-only charters).
--
-- A notice is born quiet: `members_told` records that its author checked
-- "Tell members", the one way a notice reaches the bell. `replies_open` is
-- the author's (or an admin's) per-notice switch, flippable at any time;
-- switching it off keeps the replies already made and removes the box.
-- Replies are flat — no parent, no reactions — so a removed reply orphans
-- nothing and there is no tombstone state to carry (decision 2).
--
-- Two patch settings: who may put up a notice, and whether new notices
-- take replies by default. Both travel with the patch in a seamrip.

CREATE TABLE notices (
  id           TEXT PRIMARY KEY,
  node_id      TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  author_id    TEXT NOT NULL REFERENCES users(id),
  title        TEXT NOT NULL,
  body         TEXT NOT NULL DEFAULT '',
  image_url    TEXT NOT NULL DEFAULT '',
  image_alt    TEXT NOT NULL DEFAULT '',
  replies_open INTEGER NOT NULL DEFAULT 1,
  members_told INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_notices_node ON notices(node_id, id);

CREATE TABLE notice_replies (
  id         TEXT PRIMARY KEY,
  notice_id  TEXT NOT NULL REFERENCES notices(id) ON DELETE CASCADE,
  author_id  TEXT NOT NULL REFERENCES users(id),
  body       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_notice_replies_notice ON notice_replies(notice_id, id);

-- Who may put up a notice: the patch's admins, or its members too.
ALTER TABLE nodes ADD COLUMN notice_posting TEXT NOT NULL DEFAULT 'members'
  CHECK (notice_posting IN ('admins', 'members'));
-- Whether a new notice takes replies unless its author says otherwise.
ALTER TABLE nodes ADD COLUMN notice_replies_default INTEGER NOT NULL DEFAULT 1;

-- A report about a notice or a reply goes to the patch's admins, not the
-- instance's, who cannot read the room (decision 2, tool 3). `node_id` is
-- what routes it: set for entity_type notice/reply, NULL for the report
-- kinds the instance panel handles.
ALTER TABLE content_reports ADD COLUMN node_id TEXT REFERENCES nodes(id) ON DELETE CASCADE;
CREATE INDEX idx_content_reports_node ON content_reports(node_id, status);
