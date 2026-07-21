-- Claims are assertions, not reservations (docs/adr/030).

-- Vetted trust anchor for self-service claim verification. Set only via
-- admin/trusted paths; the cosmetic website field carries no trust.
-- NULL = not yet processed by the startup backfill; '' = processed, none set.
ALTER TABLE nodes ADD COLUMN verification_domain TEXT;

-- Rebuild claim_requests: 'withdrawn' joins the status CHECK, and the email
-- method grows real columns (address, token expiry, send-rate tracking).
CREATE TABLE claim_requests_new (
  id                     TEXT PRIMARY KEY,
  node_id                TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  user_id                TEXT NOT NULL REFERENCES users(id),
  method                 TEXT NOT NULL, -- 'dns', 'meta_tag', 'email', 'admin'
  evidence               TEXT NOT NULL DEFAULT '',
  status                 TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'withdrawn')),
  reviewed_by            TEXT REFERENCES users(id),
  review_note            TEXT NOT NULL DEFAULT '',
  verification_token     TEXT,
  email                  TEXT NOT NULL DEFAULT '',
  email_token_expires_at TEXT,
  email_send_count       INTEGER NOT NULL DEFAULT 0,
  email_window_start     TEXT,
  created_at             TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at             TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO claim_requests_new
  (id, node_id, user_id, method, evidence, status, reviewed_by, review_note, verification_token, created_at, updated_at)
SELECT
  id, node_id, user_id, method, evidence, status, reviewed_by, review_note, verification_token, created_at, updated_at
FROM claim_requests;

DROP TABLE claim_requests;
ALTER TABLE claim_requests_new RENAME TO claim_requests;

CREATE INDEX idx_claim_requests_node   ON claim_requests(node_id);
CREATE INDEX idx_claim_requests_status ON claim_requests(status);

-- One open claim per user per patch — enforced at the database, not just
-- the handler. (The old global lock guaranteed at most one pending row per
-- node, so no existing data can violate this.)
CREATE UNIQUE INDEX idx_claim_requests_one_open
  ON claim_requests(node_id, user_id) WHERE status = 'pending';

-- Legacy email-method claims could never complete: verification was a stub
-- and no address was ever recorded. Close them; concurrency now lets the
-- claimant simply open a fresh claim.
UPDATE claim_requests
SET status = 'rejected',
    review_note = 'Closed by migration: the old email verification could not complete. Please open a new claim.',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE method = 'email' AND status = 'pending';
