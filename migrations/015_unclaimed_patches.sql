-- Sentinel system user to own unclaimed patches (owner_id is NOT NULL FK).
INSERT OR IGNORE INTO users (id, username, display_name, role, bio, avatar_url, created_at, updated_at)
VALUES (
  '00000000-0000-0000-0000-000000000000',
  '_system',
  'Community',
  'member',
  '',
  '',
  strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
  strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
);

-- Track who submitted an unclaimed patch and how it got there.
ALTER TABLE nodes ADD COLUMN submitted_by TEXT REFERENCES users(id);
ALTER TABLE nodes ADD COLUMN submission_source TEXT NOT NULL DEFAULT 'owner';
-- submission_source values: 'owner' (existing claimed patches), 'community', 'admin', 'agent'

-- Claim requests: users request ownership of unclaimed patches.
CREATE TABLE claim_requests (
  id                 TEXT PRIMARY KEY,
  node_id            TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  user_id            TEXT NOT NULL REFERENCES users(id),
  method             TEXT NOT NULL, -- 'dns', 'meta_tag', 'email', 'admin'
  evidence           TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
  reviewed_by        TEXT REFERENCES users(id),
  review_note        TEXT NOT NULL DEFAULT '',
  verification_token TEXT,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_claim_requests_node   ON claim_requests(node_id);
CREATE INDEX idx_claim_requests_status ON claim_requests(status);
