-- Federation: ActivityPub support

-- Keypairs for HTTP Signatures (RSA, PEM-encoded)
ALTER TABLE users ADD COLUMN public_key TEXT;
ALTER TABLE users ADD COLUMN private_key TEXT;

-- Same for nodes (patches are also AP actors)
ALTER TABLE nodes ADD COLUMN public_key TEXT;
ALTER TABLE nodes ADD COLUMN private_key TEXT;

-- Remote followers (actors from other instances following local actors)
CREATE TABLE ap_followers (
  id TEXT PRIMARY KEY,
  local_actor_type TEXT NOT NULL CHECK (local_actor_type IN ('user', 'node')),
  local_actor_id TEXT NOT NULL,
  remote_actor_id TEXT NOT NULL,
  remote_inbox TEXT,
  accepted BOOLEAN NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE(local_actor_id, remote_actor_id)
);
CREATE INDEX idx_ap_followers_local ON ap_followers(local_actor_type, local_actor_id);

-- Activity delivery queue (outbound)
CREATE TABLE ap_outbox_queue (
  id TEXT PRIMARY KEY,
  activity_json TEXT NOT NULL,
  target_inbox TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  next_retry_at TEXT
);
CREATE INDEX idx_ap_outbox_status ON ap_outbox_queue(status, next_retry_at);

-- AP IDs on proposals and votes (nodes/users/events already have ap_id from migration 008)
ALTER TABLE proposals ADD COLUMN ap_id TEXT;
ALTER TABLE proposals ADD COLUMN ap_type TEXT DEFAULT 'gv:Proposal';
ALTER TABLE votes ADD COLUMN ap_id TEXT;

-- Governance config on nodes
ALTER TABLE nodes ADD COLUMN governance_config TEXT DEFAULT '{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0}';

-- Amendment fields on proposals
ALTER TABLE proposals ADD COLUMN target_doc TEXT;
ALTER TABLE proposals ADD COLUMN proposed_branch TEXT;
ALTER TABLE proposals ADD COLUMN proposed_body TEXT;
ALTER TABLE proposals ADD COLUMN proposed_title TEXT;
ALTER TABLE proposals ADD COLUMN git_sha TEXT;

-- Populate AP IDs for existing entities that don't have them
-- (The domain will be set at runtime via config, so we use a placeholder
--  that gets updated on first startup. See handler/ap.go PopulateAPIds)
