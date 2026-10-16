-- A page made for someone tells them so
-- (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md).
--
-- The address an intro goes to. Entered by a person, never discovered
-- (decision 3). Workspace-only: it never appears in a public read, on the
-- actor, or in the seamrip export, because it was given to this steward
-- for one correspondence. Empty means nobody has entered one.
ALTER TABLE nodes ADD COLUMN contact_email TEXT NOT NULL DEFAULT '';

-- One row per intro sent (decision 5). `situation` is which calendar
-- paragraph went out: own, aggregator, squarespace, wordpress, other,
-- none. `delivery` says whether the relay sent it or the admin copied the
-- text out and sent it elsewhere (decision 8).
--
-- The claim shortcut (decision 6): when the address is at the patch's
-- verification domain, the message carries `claim_token`, a single-use
-- token that stands in for the email leg of a docs/adr/030 claim. It
-- signs nobody in. NULL when the address was at some other domain, or
-- when the admin cut the claim link out of the message before sending.
CREATE TABLE intro_sends (
  id                     TEXT PRIMARY KEY,
  node_id                TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  email                  TEXT NOT NULL,
  sent_by                TEXT NOT NULL REFERENCES users(id),
  situation              TEXT NOT NULL,
  delivery               TEXT NOT NULL CHECK (delivery IN ('smtp', 'copied')),
  subject                TEXT NOT NULL,
  body                   TEXT NOT NULL,
  claim_token            TEXT,
  claim_token_expires_at TEXT,
  claim_token_used_at    TEXT,
  created_at             TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_intro_sends_node ON intro_sends(node_id, created_at);
CREATE UNIQUE INDEX idx_intro_sends_claim_token ON intro_sends(claim_token) WHERE claim_token IS NOT NULL;
