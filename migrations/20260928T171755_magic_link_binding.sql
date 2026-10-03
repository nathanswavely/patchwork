-- A sign-in link knows which browser asked for it
-- (docs/adr/2026-09-28-a-link-knows-where-it-was-asked-for.md).
--
-- Asking for a link sets a short-lived cookie holding a random nonce in the
-- browser that asked, and the row records the nonce's sha256 hex here.
-- Opening the link signs in only a browser whose cookie matches; anywhere
-- else the page shows the code instead. NULL for a row asked for by a client
-- that sends no cookie back (a native app, a CLI), which is never bound and
-- always shows the code.
ALTER TABLE magic_links ADD COLUMN requester_hash TEXT;

-- The six-digit code itself, so the link's page can show it again on a later
-- open. `code_hash` (migration 20260920T212534) stays the column
-- verification reads. The hash never protected the code at rest: a
-- six-digit code's sha256 is recovered by trying all million of them. What
-- bounds a code is its fifteen-minute life, single use, and the five-guess
-- cap on the row. NULL on rows minted before this migration, whose page then
-- offers only "Sign in on this device".
ALTER TABLE magic_links ADD COLUMN code TEXT;
