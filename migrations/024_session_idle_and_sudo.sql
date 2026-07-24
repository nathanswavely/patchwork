-- 024_session_idle_and_sudo.sql
-- Two-sided session expiry and step-up auth (docs/adr/017).
--
-- last_used_at makes idle timeout expressible: a session dies at whichever
-- comes first, its absolute expires_at or last_used_at + the configured idle
-- timeout. It is stamped during validation, throttled to at most once an hour
-- per session so a read-heavy page does not turn every request into a write
-- on a Pi's SD card.
--
-- sudo_until records the five-minute window opened by a fresh WebAuthn
-- assertion. Instance wipe, instance export, and promotion to instance admin
-- require it — holding a month-old cookie is no longer proof of presence for
-- the three irreversible actions. It lives on the session row so it cannot
-- outlive the session: logging out deletes the row and the window with it.

ALTER TABLE sessions ADD COLUMN last_used_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN sudo_until TEXT NOT NULL DEFAULT '';

-- Existing sessions have never been stamped. Seed from created_at so they
-- start their idle clock at creation rather than being treated as either
-- brand new or infinitely stale.
UPDATE sessions SET last_used_at = created_at WHERE last_used_at = '';
