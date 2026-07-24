-- 036_start_on_my_quilt.sql
-- Scope lives in the URL (docs/adr/035). The whole quilt is the default
-- landing at "/", identical for everyone; a person who mostly lives in
-- their own quilt can opt to start on My Quilt instead. Per-person and
-- account-backed so the choice travels across devices.
--
-- 0 = start on the whole quilt (default). 1 = start on My Quilt.

ALTER TABLE users ADD COLUMN start_on_my_quilt INTEGER NOT NULL DEFAULT 0;
