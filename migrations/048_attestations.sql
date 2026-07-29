-- Decisions can happen elsewhere, and be recorded here (docs/adr/052).
--
-- A community that arrives with governance already in place — a board elected
-- last March at its annual meeting, terms running to next March — had two
-- options and both were lies: adopt `elected` and Patchwork opens an election
-- nobody needs, or don't, and describe a structure the patch doesn't have.
--
-- An attestation is the record of a decision the community made somewhere
-- Patchwork was not. Its claim is "the membership decided this, elsewhere",
-- which is a much larger claim than a direct change's "an admin decided this",
-- so it is its own table rather than another kind of proposal: a proposal
-- carries a vote Patchwork conducted, and the whole point here is that it
-- didn't.

CREATE TABLE attestations (
    id         TEXT PRIMARY KEY,
    node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    -- 'leadership' is the only kind today. docs/adr/052 decided the venue
    -- question is asked twice — once of proposals, once of leadership — and
    -- only the leadership half ships with this; a 'proposal' kind lands when
    -- proposal attestation does, rather than sitting here unread.
    kind       TEXT NOT NULL CHECK (kind IN ('leadership')),
    -- When the community actually decided, which is not when it was typed in.
    -- The gap between the two is ordinary and worth showing.
    decided_at TEXT NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    -- Who asserted it. Patchwork cannot check an attestation and does not
    -- pretend to; it records who said what, and when, in public.
    recorded_by TEXT NOT NULL REFERENCES users(id),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    -- Corrections supersede, never edit (docs/adr/052). A record that can be
    -- rewritten unseen is worth nothing, and an immutable one gets worked
    -- around with a contradictory second record claiming no relation to the
    -- first. Both stay; the later governs.
    supersedes_id TEXT REFERENCES attestations(id) ON DELETE SET NULL
);
CREATE INDEX idx_attestations_node ON attestations(node_id);
CREATE INDEX idx_attestations_supersedes ON attestations(supersedes_id);

-- The people an attestation names.
--
-- The record may name anyone; the effect lands only on members
-- (docs/adr/052). A record of what a meeting decided is the community's own
-- statement about itself — the same standing as an About page listing staff —
-- so `display_name` is always present and is the community's own words. Its
-- `user_id` is what turns a name into a relationship inside the platform, and
-- that needs the person to have arrived and consented.
--
-- NULL user_id is an *unrealized name*: it holds no role, counts toward
-- nothing, touches no affinity, and receives nothing. It is never quietly
-- upgraded — an admin links it once the person is a member, and that linking
-- is what applies the effect.
CREATE TABLE attestation_names (
    id             TEXT PRIMARY KEY,
    attestation_id TEXT NOT NULL REFERENCES attestations(id) ON DELETE CASCADE,
    user_id        TEXT REFERENCES users(id) ON DELETE SET NULL,
    display_name   TEXT NOT NULL,
    position       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_attestation_names_attestation ON attestation_names(attestation_id);
