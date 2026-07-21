-- 025_recovery_codes.sql
-- Single-use recovery codes (docs/adr/020). Generated in batches of ten from
-- the security settings page; each signs the holder in once, so a lost
-- passkey is not a lockout even on an instance with no SMTP. Stored hashed
-- like every other auth token. Regenerating a batch replaces the old one.
-- Ephemeral auth state: excluded from seamrip like sessions and credentials.

CREATE TABLE recovery_codes (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code        TEXT NOT NULL UNIQUE,       -- SHA-256 hex of the normalized code
    used        INTEGER NOT NULL DEFAULT 0,
    used_at     TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_recovery_codes_user_id ON recovery_codes(user_id);
