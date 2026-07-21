-- Phase: Data model simplification
-- 1. Flatten node hierarchy (no more stitches/containers)
-- 2. Add follower role, remove moderator role
-- 3. Remove edges table entirely (connections inferred from shared members)

-- 1. Flatten node hierarchy
UPDATE nodes SET parent_id = NULL, node_type = 'leaf';

-- 2. Recreate memberships with updated role constraint
--    SQLite can't ALTER CHECK constraints, so recreate the table.
--    moderator → admin (role consolidation)
CREATE TABLE memberships_new (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin', 'follower')),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'left')),
  joined_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE(user_id, node_id)
);

INSERT INTO memberships_new (id, user_id, node_id, role, status, joined_at)
  SELECT id, user_id, node_id,
    CASE WHEN role = 'moderator' THEN 'admin' ELSE role END,
    status, joined_at
  FROM memberships;

DROP TABLE memberships;
ALTER TABLE memberships_new RENAME TO memberships;

CREATE INDEX idx_memberships_user_id ON memberships(user_id);
CREATE INDEX idx_memberships_node_id ON memberships(node_id);
CREATE INDEX idx_memberships_connections ON memberships(node_id, user_id, status, role);

-- 3. Drop edges table
DROP TABLE IF EXISTS edges;
