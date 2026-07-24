-- 032_session_user_agent.sql
-- The session manager (issue #3, follow-up to docs/adr/017) lets a person see
-- and revoke their own active sessions. A list of bare timestamps is useless
-- for deciding which session to cut, so we capture the raw User-Agent at
-- creation and render it into a coarse label ("Chrome on Windows") — never
-- shown raw, never parsed forensically.
--
-- Stored on every login path (magic link, invite, passkey, recovery). Existing
-- sessions predate capture and carry the empty default, rendering as an
-- "Unknown device" rather than being dropped.

ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
