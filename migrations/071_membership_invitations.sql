-- Migration 071: a membership can be 'invited' (docs/adr/098).
--
-- An invite-only patch has had no door. JoinNode refuses on invite_only,
-- instance invite links (docs/adr/001) create accounts rather than admit
-- anyone to a patch, and there has been no add-member route — so a person
-- told "you're in" could only Follow. The fix is an invitation with the
-- consent left where it belongs: an admin invites by username, the person
-- accepts or declines, and until they answer the row is status 'invited'.
--
-- 'invited' is not membership anywhere. It is outside the ladder the way
-- 'pending' is (docs/adr/088): no role, no thread, absent from every count,
-- every listing, every audience and the electorate. Every membership query
-- already pins status = 'active', so this migration widens the CHECK and
-- nothing else.
--
-- SQLite cannot alter a CHECK in place, so the table is rebuilt the way 065
-- rebuilt it. Carried across: every column (visible from 019, join_message
-- from 040, share_contact from 062), the UNIQUE(user_id, node_id) pair, and
-- all three indexes. Nothing is rewritten in the copy — no existing row can
-- hold 'invited', since until now nothing could write it.

CREATE TABLE memberships_new (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin', 'follower')),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'left', 'banned', 'invited')),
  joined_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  visible INTEGER NOT NULL DEFAULT 1,
  join_message TEXT,
  share_contact INTEGER NOT NULL DEFAULT 0,
  UNIQUE(user_id, node_id)
);

INSERT INTO memberships_new
    (id, user_id, node_id, role, status, joined_at, visible, join_message, share_contact)
  SELECT id, user_id, node_id, role, status, joined_at, visible, join_message, share_contact
  FROM memberships;

DROP TABLE memberships;
ALTER TABLE memberships_new RENAME TO memberships;

CREATE INDEX idx_memberships_user_id ON memberships(user_id);
CREATE INDEX idx_memberships_node_id ON memberships(node_id);
CREATE INDEX idx_memberships_connections ON memberships(node_id, user_id, status, role);
