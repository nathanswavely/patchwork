-- 060: when a patch joined the quilt.
--
-- A patch joins when a community arrives — someone creates it, or a claim
-- completes through patch setup — never when a directory row is inserted
-- (docs/adr/076). `created_at` cannot stand in for that and neither can
-- `updated_at`: completing a claim writes `updated_at` and then every later
-- edit moves it, and the reference instance is 26 unclaimed listings of 27,
-- so a "23 patches joined" announcement was one backfill away.
--
-- NULL means "has not joined": unclaimed listings and pending submissions
-- carry no date, and get one only if a claim completes.
ALTER TABLE nodes ADD COLUMN activated_at TEXT;

-- Backfill is an approximation, and knowingly so. For a patch someone
-- created, `created_at` is exactly right. For a patch that was claimed it is
-- the day the *listing* was made rather than the day its community arrived —
-- earlier than the truth, because nothing recorded the truth. Nothing better
-- exists to read: an approved claim is only the right to enter setup
-- (CONTEXT.md), so even `claims.updated_at` names a different moment.
--
-- Erring early is the safe direction for both consumers. A patch that looks
-- older sorts later in "Recently added" (docs/adr/074) and falls before any
-- bulletin's window rather than after it (docs/adr/076) — under-surfaced,
-- never falsely announced as new.
--
-- Archived patches that were active keep a date too, so restoring one
-- (docs/adr/034) returns it with its history rather than as a new arrival.
UPDATE nodes
   SET activated_at = created_at
 WHERE status = 'active' OR archived_from = 'active';

CREATE INDEX idx_nodes_activated_at ON nodes(activated_at);
