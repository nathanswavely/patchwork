# ADR 094: A tap hands back the patch, not a card about it

Date: 2026-09-08. Status: **accepted**; **built 2026-09-08 – 09-09**, all
nine decisions. Three of them were corrected by building — see "What
building corrected" at the foot. Supersedes ADR 078's
decision 7 in its object and keeps it in its rule — a preview still costs a
gesture the device can spare; what the gesture hands back changes. Applies
ADR 022's lens rule and ADR 074's "the discovery surfaces read the quilt"
to a surface that did not exist when either was written. Decided while
grilling the docked card against the profile it was standing in for.

## Context

ADR 078 built the **docked card** so that tapping a tile on a phone would
stop spending the reader's zoom and pan to answer "what is that one?". It
worked, and using it exposed what it is: a second rendering of a patch,
sitting where the patch's own face should be. Its corrections tell the
story — "View patch" had to be *invented* and labelled, because nothing
told a reader that another tap opens the patch. That label is the seam
showing. A sheet that has to name the way to the real thing is standing in
for the real thing.

Three facts made the alternative cheap enough to take seriously.

**The profile already has the seam.** `PatchProfile.svelte` is a header
(cover, name, counts, state notices) followed by a relationship row and
then one glimpse per room. Head, then glimpses. That is exactly the split
a sheet at two heights wants, and nobody designed it for this.

**The quilt already carries the head.** `TreeNode` sends `description`,
`appearance`, the three counts, `is_unclaimed`, `amended_lining` and
`moved_to`; ADR 088's memberships store already refuses to report a
pending request as a role. So a head can render on the first frame from
data the surface has in hand, with no request at all.

**The desktop half was never built.** Clicking a card navigates away from
the canvas — the same context loss on a 27-inch screen that ADR 078 fixed
on a phone, and the cards pane already floats over the right 45% of the
canvas with `insetRight` telling the canvas to keep clear of it. There is
a slot the width of a profile sitting there, and a click that leaves it.

## Decision

**1. A discovery surface hands back the patch's own profile, docked.** Not
a card about the patch, not a summary of the profile — the same rendering
the page shows. A patch has one face and it cannot drift, which is the
whole reason to spend the effort. The **docked card retires**; the patch
card keeps its cards-pane home, which on a phone is the list view filling
the screen. What retires with it is the action row: there is no second tap
to teach when the sheet *is* the patch, and the standing the row spelled
out is the relationship row's job, where CONTEXT.md already assigns it.

**2. One address, two containers, and the room picks.** A docked profile
takes `/patches/:slug` — the same address the page has. Where a discovery
surface is on screen to keep, the profile docks over it; where there is
none, it is the page. That is a fact about the **room**, not about how the
reader arrived, which matters because ADR 078 forbade the other kind
("never which surface the person came from") and this is the same file. A
link opened cold has no surface to sit over, so it renders the page; a tap
on a tile has one, so it docks. Opening is a navigation. The *height* is
not: there is no address for a half-open sheet.

**3. The canvas is never unmounted under it.** The router matches one route
and `App.svelte` mounts one page, so `/patches/:slug` today unmounts
`SocialHome` — the Leaflet instance, the treemap, the zoom and pan, the
fetched set. Keeping the profile's address therefore needs an **overlay
route**: `/patches/:slug` renders over the surface beneath it rather than
instead of it. Dismissing returns the reader to the view they tapped from,
at the zoom and pan they left it, because nothing was torn down. This is
the one piece of new machinery in the change, and it is load-bearing: an
overlay that rebuilt the canvas on dismissal would be a page navigation
wearing an animation.

**4. Two heights on a phone; none in the pane's slot.** On a phone the
sheet rests at the profile's **head** and pulls up to **full screen** —
and the pull is what fetches the glimpses, so a tap costs one request
rather than five. (This decision first said a tap costs *nothing*, on the
strength of decision 8's seeded head. Building it found three facts a quilt
row cannot carry — see the consequences — so the head paints from the seed
on the first frame and one request completes it.) There
is no third height and no strip of canvas kept at full screen: a reader
who wants the profile with nothing behind it wants the page, which exists
at the same address. In the pane's slot there are no heights at all. That
room already shows a surface and a profile at once, which is the reason
that form is a **panel beside the surface** rather than a sheet over it;
a drag there would buy symmetry by spending the thing the desktop form is
for.

**5. Non-modal on a desktop, and no lens closes what is open.** The panel
takes the pane's 45% slot — the canvas's `insetRight` is unchanged, so
nothing reflows and the marker under the cursor stays there — with **no
scrim**: the canvas stays live, hover still previews, panning still pans.
Which makes the filter chips reachable beside an open profile, so the rule
has to be stated: narrowing never closes a docked profile, and the patch
it names need not be in the narrowed set. ADR 078 already argued this for
the adjacent case ("narrowing a set is not a request to be moved somewhere
else"), and the search chip narrows per keystroke, so the alternative
closes a profile mid-sentence. On a phone the canvas chrome steps aside
while one is docked — one temporary overlay at a time — so the lenses
simply wait. Changing **scope** is not a lens but another address, and it
closes what is docked.

**6. A preview never replaces what is open.** Where there is a pointer,
sweeping the canvas previews as it always did; the docked profile holds the
patch the reader chose until they choose another. A panel that followed the
pointer would fire a profile's worth of requests per marker crossed, and
would confuse a glance with a decision.

**7. The container is chosen by the room, its occupant by the address.** So
a **remote patch** — the one patch that can have no local profile — docks
its **remote patch card** (ADR 024). One gesture on My Quilt, where local
and remote patches sit side by side, instead of a tap that means two things
depending on a source chip in a tile's corner. Its head draws from the
follow's display snapshot, which exists so a remote tile renders when the
other quilt is unreachable, and pays for the docked head for free.

**8. `membership_policy` joins the tree, because a head must offer the
right rung.** `PatchRelationship` gates the next rung on
`node?.membership_policy !== 'invite_only'` and defaults to `'open'` when
it is absent, so a head seeded from a tree row would offer "Become a
member" on an invite-only patch while the page correctly offers nothing —
the sheet at rest and the sheet pulled up disagreeing about the same patch,
which is precisely what decision 1 promises cannot happen. The field is
already public: an anonymous reader learns it from the profile. ADR 090 put
`moved_to` on the tree for the identical reason, and its comment states the
rule this follows: "the discovery surfaces read the quilt, so a card can
only wear the pointer if the tree carries it."

**9. The profile splits into a head and its glimpses.** Two components, one
for each side of the seam decision 1 found, composed by three containers:
the page renders both, the panel renders both, the phone sheet renders the
head and mounts the glimpses on the pull. The head takes an optional
**seed** — the tree row where there is one, nothing on a cold load, where
it fetches. The seed contract is the load-bearing half: **a seed may only
fill in what the surface already knows, never something the head would
otherwise fetch**, or the two heights state different things and decision 1
is a slogan. Decision 8 is what makes the contract keepable.

## Considered and rejected

**The card grows a third size.** Keep the docked card and let it expand to
show more card. Rejected because "View patch" and the pull would then both
promise to open the patch and do different things — the row going to the
page, the pull raising a taller card — and a card whose stated affordance
is a lie is worse than no card. It also keeps two faces for one patch,
which is the defect, not the mechanism.

**An intermediate surface — a "peek" that is neither card nor profile,**
with its own curated content. CONTEXT.md rejects the word and the concept
in the patch card's own avoid-list ("peek sheet, sheet, preview"), and a
third rendering doubles the drift it was meant to avoid.

**`?patch=slug` on the canvas's address.** Free back/forward and no router
work, since query params are already outside `matchRoute`. Rejected: it
mints a second address for a profile that has one, and it would be the only
canvas state that survives a copy-paste — share it and your reader gets the
sheet open over an *unfiltered* quilt, having lost the narrowing that
produced the tap.

**No address at all.** Expanding is not navigation, the URL stays put.
Cheapest by far, and it makes the thing a reader is reading unlinkable
while `Patch profile`'s own definition names the address it lives at.

**A third detent, Maps' peek/half/full.** Maps needs the third because at
full screen its map is gone; ours is the page. Two heights and a page beat
three heights.

**Expanding the desktop panel over the canvas.** Symmetric with the phone,
and it spends the non-modal, keep-hovering posture that is the only reason
the desktop form is worth building, to reach a state the page already is.

**Reusing `SidePanel.svelte`.** It is the obvious component and the wrong
posture: a 360px panel behind `.sidepanel-backdrop` with `--color-scrim`,
which says *the thing behind is paused*. Keeping context says the
opposite. Left to notifications rather than grown a scrimless variant here,
until a second caller wants one.

**A profile store, cached per slug.** Tidier-sounding and unpaid-for: the
head needs no request, so flicking around the quilt tapping tiles is
already free, and a cache would only save a repeated pull-up of the *same*
patch while costing invalidation on every join, leave, follow and claim
(`reloadStanding` reloads the node and the memberships store together).

**A new noun for the container** — "docked patch", "the dock", or a textile
coinage. The shell vocabulary is deliberately plain (global bar, context
crumb, cards pane, docked), so a textile word here would be the first
exception and every reader would trip on it. `Docked profile` reuses the
word ADR 078 got right and widens it from one edge to two: the word
survives, its object changes.

**Docking anywhere a patch is named** — the events list, search results, a
notification, inside the workspace. Rejected as scope: none of those is a
set being worked through, so there is no place to lose, and the blast
radius is better named than discovered.

## Consequences

- `CONTEXT.md` gains **Docked profile**, and two entries change: **Patch
  card** loses its docked home, its action row and "View patch", keeping
  the cards pane on every device; **Patch profile** gains two homes on the
  room-not-arrival rule. **Remote patch card** gains the same two homes.
  "Docked card" joins the patch card's avoid-list as retired.
- ADR 078 is left as written, being a record. Its decision 7 survives in
  full as a rule about gestures; only its object is superseded here. The
  same courtesy it paid ADR 027's false sentence.
- **`loadActivity()` stops being called from inside `loadNode()`.** The
  glimpse-visibility deriveds (`showEvents`, `showMembers`,
  `showGovernance`) read `isAdmin` and `followerPermissions`, which only
  `loadNode` sets, so the three data tiers — the tree row, `nodes/:slug`,
  and the four glimpse calls — are currently welded into one. Unwelding
  them is the real work behind "fetches on the pull".
- **A phone tap costs one request and a pull costs four more.** Three facts
  in the head cannot come from a quilt row, and were found by building it:
  `is_banned` reaches a client only through `nodes/:slug`, because
  `me/nodes` serves active and pending rows and never banned ones
  (docs/adr/088); an open claim is `claims/mine`; and the upcoming-event
  count is not on the tree, where substituting the all-time `event_count`
  beside it would break the rule that the two are never labelled with each
  other's word. So the seed paints the first frame and the payload
  completes it — still one against the docked card's zero, and the trade
  the two heights buy.
- **Choosing another patch while one is docked replaces its address; only
  opening from the surface pushes.** So a dismissal is one `history.back()`
  to the surface, rather than a walk back through every patch a reader
  glanced at, and the sheet resets to its head when the patch under it
  changes.
- **`display: none` is a legitimate idle, but only after the first build.**
  `handleResize` returns early on a 0×0 container ("Still collapsed — wait
  for a real size"), and List view already hides the canvas this way. But
  `buildLayout`'s own comment says a first build against 0×0 "produces a
  0x0 svg that nothing recovers from", retried up to 60 times on animation
  frames. So a canvas that has never built must not be hidden by the sheet
  going full screen.
- The mobile chrome rule extends one `SocialHome` already keeps for one of
  three controls (`class:hidden={!!docked}` on `.mobile-header`). The info
  and filter FABs (z-index 20) only avoid the collision ADR 078's
  corrections describe — "the filter button sat on the card's description"
  — by being painted over by the sheet at z-index 65, which is luck of
  stacking rather than a rule.
- The overlay route is new in `App.svelte` and the router: something must
  say "this route renders over that one", and something must decide whether
  a surface is behind the current address. That is the piece most likely to
  be got wrong twice.
- **Gesture mechanics are deliberately unresolved here** — pulling down
  from a scrolled full screen, and whether the head is a drag target or
  only the handle. They are where sheet implementations go wrong and they
  want a device in a hand, not a decision record. The dismissal set is
  fixed: handle, dismiss, back, and at rest a tap behind.
- **The rest height is measured from the sheet's top edge to the foot of
  the head, not from the head's own height.** Built the second way first,
  and the 26px handle above the head went unaccounted for: the relationship
  row sat 25 pixels below the fold, visible from the top and impossible to
  press. Same distinction IntroCard already records about publishing what
  it occupies rather than what it is.
- **Whether the retirement ships with the replacement, or the docked card
  stays until the sheet works, is not decided here.** Both are defensible;
  the sequencing belongs to whoever builds it.

## What building corrected, 2026-09-09

Three of the nine, none of which changes a decision:

- **A seeded head is not a free head.** Decision 8 put `membership_policy`
  on the tree so the relationship row would be right on the first frame,
  and decision 4 concluded from that a tap costs nothing. Two more head
  facts turned out to be unreachable from a quilt row, and one that must
  not be faked: `is_banned` reaches a client only through `nodes/:slug`
  (`me/nodes` serves active and pending rows, never banned ones —
  docs/adr/088), an open claim is `claims/mine`, and the upcoming-event
  count is not on the tree, where the all-time `event_count` beside it
  cannot stand in for it. So the seed paints the first frame and one
  request completes it. Adding three more fields to the busiest endpoint to
  save one request was the alternative, and the request is cheaper.
- **The rest height is measured to the head's foot, not from its height.**
  Built the other way first, and the 26px handle above the head went
  unaccounted for: the relationship row sat 25 pixels below the fold —
  visible from the top, impossible to press, and invisible in a screenshot.
  Found by comparing the row's rect against the viewport rather than by
  looking. IntroCard records the same distinction about publishing what it
  occupies rather than what it is.
- **Choosing another patch while one is docked replaces its address.** The
  decision said opening is a navigation and left it there. Pushing every
  glance would make one dismissal walk back through each patch a reader
  looked at, so only opening from the surface pushes; a swap replaces, and
  a dismissal is one `history.back()` to the surface.

And one collision worth recording, because it is the third time this
geometry has bitten: the dock's dismiss button overlapped the cover's own
overflow by six pixels — enough to eat the press, invisible to the eye. It
sits at the top *left* in both forms now. docs/adr/078 recorded the filter
button landing on the card's description; the corner a profile's cover
keeps for Settings and the overflow is not free.
