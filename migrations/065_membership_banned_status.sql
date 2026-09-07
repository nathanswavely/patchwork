-- Migration 065: let a membership actually reach status 'banned'.
--
-- Removing a member from a patch has never worked. The handler writes
-- `UPDATE memberships SET status = 'banned'` (internal/handler/memberships.go),
-- but the column's CHECK constraint has read `status IN ('active','pending',
-- 'left')` since migration 004 and unchanged through 009's rebuild. The write
-- fails the constraint, the admin gets a 500, and the row stays active — the
-- person is not removed.
--
-- Everything downstream of that status is dead code in consequence: JoinNode's
-- 403 for a removed person, the reinstate branch and its notification, the
-- is_banned flag on GET /nodes/{slug}, the "Removed from this community" notice
-- in the SPA, and the admin-only ?status=banned member filter. None of them
-- were wrong; none of them could ever be true.
--
-- SQLite cannot alter a CHECK in place, so the table is rebuilt the way 009
-- rebuilt it. Carried across: every column added since (visible from 019,
-- join_message from 040, share_contact from 062), the UNIQUE(user_id, node_id)
-- pair, and all three indexes. Nothing is rewritten in the copy — no existing
-- row can hold 'banned', since until now nothing could write it.

CREATE TABLE memberships_new (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin', 'follower')),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'left', 'banned')),
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
