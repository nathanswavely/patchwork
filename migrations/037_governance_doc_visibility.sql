-- Migration 037: per-document visibility for governance docs (charters).
--
-- Charters were unconditionally world-readable: the list and detail endpoints
-- took no session at all. A patch's rules are often drafted before they are
-- meant to be read, so new docs now default to members-only and a patch admin
-- publishes each one deliberately (docs/adr/036).
--
-- Rows that already exist were public under the old rule, and a community may
-- already be pointing people at them. Retracting them silently would be its own
-- surprise, so they are pinned to 'public' here; the default only governs docs
-- created from this migration forward.

ALTER TABLE governance_docs ADD COLUMN visibility TEXT NOT NULL DEFAULT 'members'
    CHECK (visibility IN ('public', 'members'));

UPDATE governance_docs SET visibility = 'public';
