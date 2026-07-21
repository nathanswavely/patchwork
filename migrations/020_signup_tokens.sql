-- 020_signup_tokens.sql
-- Signup tokens for the two-phase magic-link flow (docs/adr/013).
-- Clicking a magic link for an unknown email proves control of the address
-- but creates no user; it issues one of these instead. The account is
-- created only when the person chooses their permanent username.
-- Ephemeral auth state: excluded from seamrip like magic/invite links.

CREATE TABLE signup_tokens (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL,
    token       TEXT NOT NULL UNIQUE,       -- SHA-256 hex of the raw token
    expires_at  TEXT NOT NULL,
    used        INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_signup_tokens_token ON signup_tokens(token);
