# ADR 021: Tags are the only classification — placement affinity gains a tag term that is not a thread

Date: 2026-07-20. Status: accepted. Decided while grilling issue #64.

## Context

Issue #64 asked for three things: a list of **categories** for patches, a
"sticky" term in the placement algorithm so same-category patches land near
each other, and category icons "replacing the motif idea." The stated purpose:
give organization to brand-new quilts and thin patches that don't yet have the
member overlap the layout runs on — a cold-start fix, deliberately a last
resort behind real people-signal.

The repo already contained a category-shaped system nobody could maintain:
a `tags` table curated by instance admins (`POST/DELETE /api/v1/admin/tags`),
attached to patches many-to-many via `node_tags`, read-exposed everywhere
(quilt tag filter, onboarding interests, tag-derived motifs via a hardcoded
`TAG_MOTIFS` map) — but with **no write path at all**: no tag picker on any
patch surface, no admin page calling the CRUD endpoints. Adding a parallel
"category" concept on top of that would have meant two classification systems
on the same entity, a permanent boundary to police in CONTEXT.md, and a
platform-fixed category list that violates config-over-code (a disc golf
quilt needs different categories than an arts quilt).

The in-the-wild pattern is clean: platforms run tags-only when tags power
search/discovery/similarity (GitHub topics, Stack Overflow, hashtags), and
add a separate category concept only when something structural demands
**exactly-one** semantics — navigation taxonomies, permissions, genre rails
(Discourse, Eventbrite, itch.io). Patchwork has exactly one exactly-one
consumer: the motif (a patch can't wear three icons), and that was already
solved by first-tag derivation with explicit choice as override.

## Decision

**One concept. Tags are the classification system; "category" is an _Avoid_
word (CONTEXT.md).** The vocabulary stays instance-curated; patch admins
choose from it — many per patch, validated at the API boundary (unknown names
rejected, never auto-created). Ownership is **authority, not exclusivity**:
the instance admin may set tags on any patch — required for unclaimed patches,
which have no admins, and for the one-time production backfill — and a patch
admin's later edit simply wins as the latest write. The `tags` array order on
create/update is the stored order; it is the patch admin's priority order.

**Placement affinity gains a shared-tag term, computed server-side into the
same links array** (`internal/handler/tree.go`), alongside shared
admins/members ×3, shared event participation ×2, shared followers ×1:

```
tagStrength = min(sharedTagCount, 2) × massFactor
```

where `massFactor` scales with the **larger** patch's member count,
normalized so the strongest possible tag link stays **below 3** — no amount
of declared similarity ever outweighs a single real shared human. The mass
scaling does the issue's "gravitation": a thin new `music` patch is pulled
harder toward the 40-member venue than toward another two-member band.

"Last resort" is enforced by **relative weakness, not a conditional**: a
patch with real people-overlap is dominated by those terms; a patch with
nothing follows its tag links entirely. There is no zero-overlap gate.

**A tag link is not a thread.** Thread — the user-facing concept — remains
inferred from shared admin/member overlap only. The tag term is an internal
placement detail: unbranded (no textile coinage), never rendered (affinity
links feed the layout engine and are not drawn), and named in code by plain
technical terms. Shared followers likewise count ×1 in placement affinity
while never creating threads — this was already the code's behavior; the
"followers don't count" line in CLAUDE.md described threads and was corrected
to say so.

**The tag→motif mapping becomes vocabulary data, not frontend code.** Each
tag carries an optional motif (nullable column on `tags`), set by the
instance admin when curating the vocabulary, picked from the frontend's
existing curated motif registry (same slugs; unknown slugs fall through per
ADR 004, never error). The hardcoded `TAG_MOTIFS` map in
`web/src/lib/patchIcons.js` survives only as the migration's backfill values
for the existing vocabulary, then dies. Motif itself stays **chooseable**
(ADR 004 stands): derivation order is explicit choice → first motif-bearing
tag → quilt mark.

## Considered options

- **A distinct category concept** (exactly-one, platform-fixed list):
  rejected. Every in-the-wild precedent for the split is driven by a
  structural exactly-one need Patchwork doesn't have; the one consumer
  (motif) was already solved. Two taxonomies on one entity is a permanent
  explanation burden, and a fixed list breaks white-label.
- **Strict fallback** (emit tag links only for pairs with zero
  people-affinity): rejected. Purer "last resort," but the day one person
  joins both patches the tag pull vanishes and the tile can leap — a
  discontinuity for no gain, and it does nothing for gravitation.
- **Tag clustering as a separate layout force** (people-links and tag-links
  as distinct inputs the frontend balances): rejected mechanically, kept
  linguistically. The layout engine doesn't care why two patches attract;
  one sum means no layout rework. The vocabulary distinction (thread ≠ tag
  attraction) is preserved in docs instead.
- **Strict tag-derived icons, motif choice removed** (the issue's literal
  ask): rejected. It would silently replace motifs already chosen by live
  Lancaster patch admins, delete the expressive marks (skull, lightning,
  butterfly — identity, not classification), and reopen exactly-one via a
  primary-tag field. With full vocabulary motif coverage and deterministic
  ordering, every patch that never touched the picker wears its tag's icon
  anyway — which is what the issue needs, if not what it says.
- **Keep `TAG_MOTIFS` hardcoded**: rejected. It bakes Lancaster's vocabulary
  into the SPA; any other instance's tags would silently fall through to the
  generic quilt mark, and the icons-represent-tags feature wouldn't exist
  off the reference instance.

## Consequences

- The tag weights (cap 2, sub-3 normalization, mass scaling) are reasoned
  defaults no real data has tested — same caveat as ADR 015's size floors.
  Revisit once an instance has both real membership overlap and real tag
  coverage.
- Tag links are O(k²) per tag across patches sharing it. Fine at Lancaster
  scale (dozens of patches); worth a look before an instance has hundreds of
  patches sharing popular tags.
- Production needs a backfill pass: the instance admin tags existing patches
  (including all unclaimed ones) via the new write path. Until then the term
  is inert.
- Node create/update grow a `tags` field with vocabulary validation; the
  admin panel grows the vocabulary page (list/create/delete, motif per tag)
  that the orphaned endpoints have been waiting for.
- Deleting a tag from the vocabulary silently strips it from every patch
  wearing it (existing cascade behavior, now consequential).
- The first-tag-wins derivation is finally deterministic (stored array
  order); the previous `ORDER BY`-less tag query could flip a patch's
  derived motif between page loads.
