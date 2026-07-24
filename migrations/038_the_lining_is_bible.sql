-- Migration 037: governance_docs.kind — the lining becomes machine-identifiable.
--
-- The lining was only ever identified by its title constant, and titles are
-- editable; one rename and every lining rule goes blind. The kind column is
-- the durable identity (docs/adr/037). Extensible by design; only 'lining'
-- carries behavior today.
--
-- Backfill matches both title constants the lining has shipped under:
-- "Community Standards" (post-ADR-011) and "Community Lining" (pre-ADR-011
-- rows and older seeds).
--
-- The lining is always public (docs/adr/037) — the one document ADR 036's
-- members-only default can never touch.

ALTER TABLE governance_docs ADD COLUMN kind TEXT NOT NULL DEFAULT 'charter'
    CHECK (kind IN ('charter', 'lining'));

UPDATE governance_docs SET kind = 'lining'
    WHERE title IN ('Community Standards', 'Community Lining');

UPDATE governance_docs SET visibility = 'public' WHERE kind = 'lining';

-- Personal discovery filter (docs/adr/037): hide amended-lining patches from
-- this user's quilt, search, map, and public feeds. The instance-wide policy
-- twin lives in instance_settings ('hide_amended_linings'); strictest wins.
ALTER TABLE users ADD COLUMN hide_amended_linings INTEGER NOT NULL DEFAULT 0;
