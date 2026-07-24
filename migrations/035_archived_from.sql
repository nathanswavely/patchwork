-- 035_archived_from.sql
-- Restore for archived patches (docs/adr/034). Archiving overwrites
-- nodes.status, but instance admins archive unclaimed patches too, so a
-- blind restore-to-active would resurrect an ex-unclaimed patch as claimed
-- with zero admins — unreachable and unclaimable. Both archive paths now
-- record the prior status here; restore reads it once and clears it.
--
-- NULL means "not archived" or "archived before this column existed".
-- Legacy archived rows restore by inference instead: any active admin
-- membership means the patch was active, otherwise unclaimed — sound
-- because the last-admin guards keep active patches at one or more admins
-- and an archived patch's memberships are frozen.

ALTER TABLE nodes ADD COLUMN archived_from TEXT;
