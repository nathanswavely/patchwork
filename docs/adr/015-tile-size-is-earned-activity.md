# ADR 015: Tile size is earned activity, not rank — absolute floors with scaling caps

Date: 2026-07-19. Status: accepted, implemented in `web/src/lib/quiltLayout.js`.

## Context

A patch's tile on the quilt is 1x1, 2x2, 3x3, or 4x4 grid cells. Size was
assigned by **rank alone**: sort patches by `member_count + event_count + 1`,
then hand the top-ranked patch a 4x4 (whenever the quilt had ≥12 patches), the
next ~12% a 3x3, the next 40% a 2x2. Nothing about the assignment consulted how
active a patch actually was — only where it landed in a sorted list.

Two failures follow directly, and both were live on lancasterpatchwork.org.

**Rank invents a winner from a field of zeros.** Production is 27 patches, 26 of
them unclaimed directory entries; 26 have zero activity and the maximum anywhere
on the instance is 1. Rank-only tiering still rendered
`{1x1: 16, 2x2: 7, 3x3: 3, 4x4: 1}` — eleven inflated tiles. American Music
Theatre, Decades, and Demuth Museum of Art on King each drew a 3x3 while having
no members and no events. The visual claim "these are the big ones" was
produced entirely by sort order.

**Rank splits ties arbitrarily.** Because the tier came from array index, two
patches with identical activity could render at different sizes. On seed data,
Flicker & Still and Floorwork Dance Collective both had activity 11; one got a
4x4 and the other a 3x3. Commits & Coffee and SoWe Community Garden both had 9;
one got 3x3, the other 2x2. There is no honest basis for picking which of two
equally active patches looks bigger.

A third problem sits underneath both. Membership is impossible on an unclaimed
patch, and an unclaimed patch has no events, so its `member_count +
event_count` is pinned at 0 for as long as it stays unclaimed. Followers are the
only signal it can accrue, and followers were excluded from sizing entirely.
On a directory-seeded instance — Patchwork's normal starting state, and
Lancaster's today at 26 of 27 patches — that means almost nothing on the quilt
could ever grow.

## Decision

**Size is the smaller of what activity earns and what quilt position allows.**

```
idealSize = min(rankCap(rank, n), earnedSize(activity))
```

**Activity is measured in member-equivalents**, with followers discounted:

```
activity = member_count + event_count + floor(follower_count / 3)
```

Followers are observers, not participants (CLAUDE.md's contributor ladder), so
they must not size a patch the way membership does — but excluding them
outright is what pins unclaimed patches at minimum size forever. A 3:1 discount
lets real interest register without letting it masquerade as participation.
Integer division keeps activity a whole number so ties compare exactly.

**Absolute floors decide what a size is worth.** 2x2 at activity 3, 3x3 at 10,
4x4 at 24. A large tile is earned against a fixed bar, not against whoever else
happens to be on the quilt, so a quiet instance stays uniformly small instead of
crowning someone.

**Rank caps scale with the quilt and only ever demote.** 4x4 is available to the
top 4% of patches, 3x3 to the top 12%, 2x2 to the top 40%. These are ceilings,
not entitlements: clearing a cap grants nothing without also clearing the floor.
Caps exist so a large, genuinely busy quilt keeps visual hierarchy rather than
becoming a wall of maximum tiles — and because they scale with `n`, a big quilt
can show several 4x4s where the old rule allowed exactly one.

**Ties are resolved by competition ranking**: every patch tied on activity
shares the best rank in its group, so equally active patches always receive the
same cap and therefore the same size. Without this the cap reintroduces the
arbitrariness the floors exist to remove — it just moves it to whichever tier
boundary a tie happens to straddle.

Sizing stays frontend-owned, like appearance (ADR 004). The tree endpoint serves
counts; `quiltLayout.js` decides what they mean. Note that `idealSize` is the
tier this ADR governs — the placement pass may still flex a tile ±1 to keep
affinity clustering intact, so `currentSize` is what actually renders.

## Considered options

- **Keep pure rank** (status quo): rejected — it is the defect. Rank alone
  cannot distinguish "leads the quilt" from "leads a quilt where nothing has
  happened," which on a new instance is every quilt.
- **Absolute floors with no rank cap**: rejected, though close. Nothing bounds
  the grid when many patches clear the top floor. Kept as a cap rather than
  dropped because the cap costs nothing when floors bind, which is the common
  case.
- **Keep the hard "exactly one 4x4" cap**: rejected — arbitrary at both ends. It
  forces a 4x4 in a quilt with no large patches and forbids a second one in a
  quilt with ten.
- **Rank without competition ranking** (index-based, tie-broken by id):
  rejected — deterministic but still arbitrary. Stable arbitrariness is still
  arbitrary; two equal patches would just render differently the same way every
  time.
- **Exclude followers from sizing**: rejected — leaves every unclaimed patch
  permanently 1x1 (see Context). This was the pre-existing behavior and is only
  invisible today because Lancaster's follower counts are still 1.
- **Count followers at parity with members**: rejected — collapses the
  distinction the contributor ladder is built on, and makes a patch with 200
  fans outrank a working collective. Note this ADR changes only *sizing*;
  inferred threads still ignore followers, and public counts still report
  members and followers separately.

## Consequences

- A brand-new or directory-seeded instance renders a uniform quilt. Lancaster
  today is `{1x1: 27}`. This is intended — flat until something real happens —
  but it does mean the signature visual carries no hierarchy at launch, and
  texture arrives only as patches are claimed or followed.
- An unclaimed patch can grow on followers alone: 9 followers for 2x2, 30 for
  3x3, 72 for 4x4. The follower term is inert on production today (max 1
  follower, and `floor(1/3) = 0`) and is forward-looking.
- **The upper floors are uncalibrated.** Production's maximum activity is 1 and
  seed data's is 12; nothing anywhere has approached 24. The 3x3 and 4x4 floors
  are reasoned defaults that no real data has yet tested. Revisit once an
  instance has genuine participation — this is the number most likely to be
  wrong.
- The 3:1 follower discount is likewise a judgment call, not a measurement.
- Sizes shift when a patch is claimed, gains a member, runs an event, or crosses
  a follower multiple of 3. Tiles are deliberately frozen at their current sizes
  across filter changes (`fixedSizes`) so filtering does not reshuffle the quilt.
