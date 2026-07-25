-- Migration 043: heal proposals whose `state` never followed their `status`.
--
-- Proposals carry two lifecycle columns: `status` (the original, four values
-- enforced by a CHECK) and `state` (migration 016's richer state machine).
-- The proposal page renders from `state`. Three write paths — withdraw,
-- vote resolution, and amendment auto-apply — moved only `status`, so a
-- withdrawn or decided proposal kept showing an open vote. The code fix
-- lands with this migration; every row already written is still stale.
--
-- Migration 016 backfilled `state` once, at its own run time. Anything that
-- reached a terminal status *after* that and *before* this is what we repair.
--
-- Scoped to rows whose `state` is not already terminal, so a row that took
-- the correct path is never rewritten. That guard also covers the pre-terminal
-- states ('draft', 'discussion') a proposal could be withdrawn from, not just
-- 'voting'. Idempotent: a second run matches nothing.

-- Withdrawn and rejected map straight across.
UPDATE proposals
SET state = status
WHERE status IN ('withdrawn', 'rejected')
  AND state NOT IN ('withdrawn', 'rejected', 'approved', 'in_effect');

-- 'approved' is a resting state, not the end: the community decided, and an
-- admin still makes it official (approved → in_effect). A row already
-- carrying applied_at has had that second step happen — auto-apply merged
-- the branch — so it belongs in_effect rather than back at approved.
UPDATE proposals
SET state = CASE WHEN applied_at IS NOT NULL AND applied_at != '' THEN 'in_effect' ELSE 'approved' END
WHERE status = 'approved'
  AND state NOT IN ('withdrawn', 'rejected', 'approved', 'in_effect');
