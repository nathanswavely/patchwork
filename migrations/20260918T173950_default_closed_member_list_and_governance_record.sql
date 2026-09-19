-- The default should match the assumption
-- (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
--
-- Patch admins on the reference instance assumed their member list was not
-- public. It was. Every control involved worked as documented — migration
-- 069 defaulted public_member_list to 'everyone' because that is what every
-- existing patch already did, and docs/adr/036 left charters members-only —
-- but a patch was still born enumerable, and its deliberation was born
-- world-readable with no control over it at all.
--
-- Two changes here, and they pull in the same direction.
--
-- 1. A new column for the patch's deliberation: proposals and their bodies,
--    the discussion under them, and the attestations that record decisions
--    taken elsewhere. Two values rather than three, unlike its sibling: a
--    roster is a list of names with a natural subset, so 'admins' is a
--    coherent middle rung there. A deliberation record is prose that names
--    people inside itself — a nomination carries target_user_id and usually
--    names its subject in its own title — so there is no state between open
--    and closed that says what it means.
--
-- 2. Both columns are retracted on every existing patch, not grandfathered.
--    This departs from docs/adr/036 ("the default governs what comes next,
--    not what shipped") and from 069 on purpose. Grandfathering would help
--    nobody currently exposed, and the patches that already exist are the
--    whole problem. The deciding argument is the asymmetry docs/adr/095 §6
--    already accepted for this control: a wrong retraction costs an admin
--    one click, a wrong exposure cannot be recalled.
--
--    Some patches did deliberately choose 'everyone' and are being
--    overridden. They cannot be identified: node.update is audited with an
--    empty '{}' detail, so nothing records which field an edit touched.
--    Accepted, and the operator is notifying the affected admins directly.
--
-- NOT NULL with a CHECK like every other enumerated column on this table, so
-- a bad value is caught at the write rather than becoming a 500.

ALTER TABLE nodes ADD COLUMN public_governance_record TEXT NOT NULL DEFAULT 'nobody'
    CHECK (public_governance_record IN ('everyone', 'nobody'));

-- The retraction. The column default above governs patches created from here
-- on; this governs the ones that already exist. Both are needed: a DEFAULT
-- never touches an existing row.
--
-- public_governance_record is already 'nobody' on every existing row by the
-- DEFAULT above, so only the roster needs restating.
UPDATE nodes SET public_member_list = 'nobody' WHERE public_member_list != 'nobody';

-- migrations/069 left public_member_list DEFAULT 'everyone', and it stays
-- that way: SQLite cannot ALTER a column's default, and rebuilding the nodes
-- table to change one would be a far larger risk than the stale default is.
--
-- So the DEFAULT no longer describes the product. CreateNode writes both
-- columns explicitly, which is the same arrangement follower_permissions has
-- lived under since migration 012 (its DEFAULT still says charters:true while
-- docs/adr/116 requires false). The rule that comes with it is the one that
-- file's test helper already learned the hard way: anything inserting a node
-- outside CreateNode — fixtures included — has to write these columns, or it
-- is testing a patch the product cannot create.
