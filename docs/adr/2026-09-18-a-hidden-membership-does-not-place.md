# A hidden membership does not place

Date: 2026-09-18. Status: accepted. Amends the affinity description in
docs/adr/021; relates to docs/adr/006 (the membership-visibility switch)
and docs/adr/095 (member_count stays public).

## Context

Placement affinity, the internal weight table `internal/handler/tree.go`
computes for the quilt layout, counts shared admins/members (weight 3),
shared followers (weight 1) and shared event participation (weight 2)
between every pair of public patches. Until now those three queries
counted every active membership row, including one a member had hidden
with the per-membership `visible` switch (`memberships.visible`,
migration 019, docs/adr/006).

The scores are public, returned in full on `GET /api/v1/nodes/tree`, and
unnormalized: a shared member is worth exactly 3, a shared follower
exactly 1, a shared event-creator exactly 2. In a small patch that makes
the score legible in a way a percentage or a normalized weight would not.
If two patches share four members and one of them hides their membership
at one of the two patches, the public score between those patches drops
from 12 to 9, a change of exactly 3, the moment they flip the switch. A
patch admin watching the quilt, or anyone polling the tree endpoint, can
read that as "someone just hid themselves," and in a patch small enough
that the admin roster is known, work out who.

That is the opposite of what the switch is for. A member who hides their
membership at a patch is asking that patch's fact of their presence not
be inferable from the outside. The score doing exactly that was left
unresolved in PR #315, which added the query's visibility-based filtering
for private, removed and suspended patches but left the memberships
themselves unfiltered, with a comment that the memberships.visible
question was a separate call nobody had made.

## Decision

**A membership either side of a pairwise affinity score has hidden does
not count.** The shared-admin/member query, the shared-follower query and
the shared-event-participation query in `internal/handler/tree.go` each
gate on `visible = 1` for every membership row that feeds the score. A
hidden membership contributes nothing to affinity between the two patches
it would otherwise have connected, on either side of the pair. This
applies to both the default tree response and the `scope=my` one, since
both share one affinity map built after the node list is fetched.

Two things do not change. The shared confirmed event-links query and the
shared-tag term do not read `memberships` at all, so there is nothing to
gate there. And `member_count`/`follower_count`, the numbers shown on
every patch's tile, keep counting every active membership regardless of
`visible` (docs/adr/095): a count says how big a patch is, and nobody's
hidden membership is named by a total. A pairwise affinity score is a
different kind of fact: it asserts a specific overlap exists between two
named patches, and with a hidden row inside it, a specific person's
membership becomes inferable from a public number. Affinity treats a
hidden membership as absent for that reason; the count does not, because
absence would misstate the patch's size for everyone else.

## Consequences

A patch loses a small, occasional slice of its placement pull whenever a
member hides their membership there: the tile can drift slightly further
from a patch it used to be pulled toward. That is the intended effect and
the same trade docs/adr/095 already made for the roster: the worst this
costs is a less precise layout, never an exposed person.

The tag term (docs/adr/021) still applies with the same weight and cap
whether or not the people it would otherwise connect are hidden, so a
patch that loses its people-derived pull to a hidden membership can still
land near its kind through declared tags, which was already the "cold
start" behaviour ADR 021 built for thin patches with no visible overlap
at all.

No migration is needed: the filter is computed at read time from the
existing `memberships.visible` column, so it applies retroactively to
every membership already hidden.
