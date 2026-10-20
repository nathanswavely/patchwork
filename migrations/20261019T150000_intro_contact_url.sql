-- A contact is a place to reach someone, not only an address
-- (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md, amended
-- 2026-10-19). Many of the organizations a young quilt lists have no public
-- email and live on Instagram; the intro reaches them as a message the
-- admin copies out and sends by hand.
--
-- The link to message them on, entered by a person (a profile URL, never
-- a guessed handle). Workspace-only and stays behind at the seamrip
-- boundary, like contact_email beside it. Empty means none.
ALTER TABLE nodes ADD COLUMN contact_url TEXT NOT NULL DEFAULT '';

-- Which way a sent intro went: 'email' through the relay or copied into
-- mail, 'dm' copied into a message on the link. The destination it went
-- to is in `email` either way; the column keeps its name rather than
-- rebuilding the table. A dm never carries a claim token: a social
-- account proves nothing about the verification domain (docs/adr/030).
ALTER TABLE intro_sends ADD COLUMN channel TEXT NOT NULL DEFAULT 'email' CHECK (channel IN ('email', 'dm'));
