# ADR 112: The quilt opens loud, and muted only ever takes away

Date: 2026-09-16. Status: **accepted**, built 2026-09-16. The gate below ran and failed, so the hash-palette widening it names landed first, as its own change. Builds on
ADR 004 (a patch chooses its appearance), ADR 029 (the fabric wall and the
block drafter), ADR 066 (how a tile is drawn), ADR 074 (a control belongs
to the surface it changes — and the limit of that rule), ADR 078 (why a
patch has an identity color at all), ADR 040 (the sidebar's anonymous
entries).

## Context

A reader told us the quilt looks cluttered and busy. The complaint is real
and the answer is not to fix it: the loudness is the position. A platform
built for grassroots organizing, against the register of software designed
by committee for the masses, is allowed to look handmade and a little bit
much. What the complaint earns is an *alternative*, not a correction.

**What is actually loud is narrower than it looks.** The curated block
renderers in `web/src/lib/quiltBlocks.js` only ever reach for
`p.primary`, `p.secondary` and `p.bg` — three fabrics. Only a drafted
block (ADR 029) uses all six bundle slots. So the noise is not hue
*count*. It is that all three sit at maximum chroma with the widest
possible value spread — Stage Black beside Hi-Vis beside Magenta — and
then the neighbouring tile does the same thing in different hues. Two
channels are shouting: chroma, and value contrast both within a tile and
across the quilt.

**The colors run through one chokepoint.** Everything that stands for a
patch resolves in `web/src/lib/quiltTheme.js` — `paletteForPatch` for
tile fabrics, `identityColorForPatch` for card covers, map markers and
profile banners, `ghostPalette` for filler tiles, `colorForTag` for chips
and (via `colorForQuilt`) neighbour-quilt sashing. A viewer-side
transform has exactly one place to live, and every renderer inherits it
without being touched.

## Decision

**1. One transform, at that chokepoint. Hue is kept, lightness is
normalized, chroma is only ever capped.** Every fabric in a tile becomes
that patch's identity color at a fixed step on a lightness ramp shared by
every tile in the quilt; chroma is reduced where it exceeds a ceiling and
never raised. So a muted quilt varies by hue alone — and the contract
states itself in one line a patch admin can be told:

> Muted never adds a color a patch didn't choose. It takes away what it
> has to and keeps the rest.

Capping rather than setting is what makes that true, and it dissolves the
achromatic case instead of answering it: a patch whose identity color is
Stage Black draws a grey ramp because it genuinely chose to be
achromatic. No fallback chain, no special case. A patch that chose
neutrals stays neutral; in a field of muted color it is the most
distinctive tile on the quilt, not the least.

**2. The transform stays injective on slots.** Six fabrics in, six
distinguishable fabrics out. `renderDraftBlock` colors pieces by
`slots[slot] ?? slots[slot % slots.length]`, so any transform that
*shrinks* the bundle wraps the indices and lands two adjacent pieces on
one color — the seam between them stops existing and a drafted Ohio Star
flattens into a rectangle. The block is the half of a patch's identity
that isn't color. A mode that exists to preserve identity may not delete
the drafter's work to do it. Fixed ramp steps also *guarantee* value
separation between pieces, which building the ramp around the chosen
color could not promise.

**3. Default on every instance, and no instance setting moves it.**
Loud is the creator's intent; muted is an accommodation. This is the
reason the label reads **Default** rather than **Full**: a configurable
instance default would make the word a lie on the first fork that
flipped it. A fork may change its name, its lining and its rules — the
quilt opens loud. One less admin setting, and a stronger statement of the
project's position than a knob would be.

**4. It is a standing preference in the Display menu, held per browser.**
Two rows: **Theme** (light · dark · system) and **Colors** (default ·
muted). Signed in it lives in the account menu in the global bar; signed
out it is that same slot, rendered as a Display button, so **the control
does not move when somebody joins**. One component in both homes
(`DisplayMenu.svelte`) — written twice, the two would drift.

The Theme row *replaces* the single Light/Dark toggle that was there, which
is a small widening rather than a rewrite: the store has always held
`system` as its default, and no UI ever offered it back. A reader who had
never touched the toggle was on `system` and could only leave it.

ADR 074 says a control belongs to the surface it changes, which argued for
putting this on the canvas. That was the wrong reading: 074's controls
(in-view, quilt order, Quilt/Map) are *lenses on the current view*, while
this is a standing statement about how Patchwork looks to you — the same
kind of thing as light/dark, which `GlobalBar.svelte` already puts in the
account menu rather than on any canvas. The rule stands; this is not one
of its cases.

`localStorage`, like `theme.svelte.js`, and never a `users` column —
because the menu must reach a reader with no account. That choice also
means the preference never becomes user data: no endpoint, no migration,
no personal-export field, and nothing for
`TestEveryTableHasABoundaryDecision` to ask about.

A consequence worth naming: an anonymous visitor currently has **no theme
control at all** — `GlobalBar.svelte` renders only Log In and Sign Up when
signed out, so a reader on a dark-OS machine gets a dark Patchwork and
cannot change it. The Display menu closes that gap on the way past.

Under 640px the signed-out bar already drops Log In and keeps Sign Up
(`GlobalBar.svelte:206`, the unified-auth-page pattern — sign-in is one tap
in from /login). **Display survives that cut and Log In stays dropped**: a
phone is the device most likely to be on a dark OS and least able to afford
a loud quilt, the button is an icon rather than a word, and /login is still
one tap away. So the narrow bar is Display, then Sign Up.

**5. It reaches every color that stands for something — with one
exception, and one impossibility.** Tiles, card covers, map markers,
profile banners, filler tiles, the static hero, tag chips, neighbour-quilt
sashing and source chips. Muting the quilt and leaving forty full-chroma
tag pills directly beneath it would answer the complaint on a third of the
pixels.

*The exception:* **the block drafter always draws Default**
(`PatchSettingsAppearance.svelte`, `PatchForm.svelte`). A tool for
choosing fabric must show the fabric that was chosen; an admin who picks
Hi-Vis and is shown a muted swatch has been lied to by the picker. This is
recorded because it looks like an inconsistency and will be "fixed" by
somebody who does not know why it is there.

*The impossibility:* the instance icon is rendered server-side to SVG from
`icon_design` (ADR 043), so a client-side preference cannot reach it. In
Muted, the quilt's own icon in the global bar stays full color. Accepted:
it is one small mark, it is the instance's identity rather than a patch's,
and the alternative is a server-side render variant for a per-browser
choice.

**6. Hover dims the rest; it does not mute them.** Pointing at a tile
scrims every *other* tile, replacing the previous behavior of darkening
the hovered one (`QuiltCanvas.svelte`, the `.overlay` rect and its
`pointerenter`). Dim and mute must stay
on separate channels: if hover muted, then for a reader already in Muted
mode hovering would do nothing at all — the two features would cancel
exactly where they are most wanted.

Implemented by inverting the `.overlay` rect every tile already carries:
fifty-odd single-attribute writes, no geometry rebuilt. Recoloring on
hover is not available — ADR 066 builds geometry once and batches it by
paint within spatial chunks, and a recolor invalidates every one of those
batches, on every pointer move.

It engages after a short dwell and eases, and holds the scrim while the
pointer crosses between tiles rather than releasing and re-applying. The
badge engine already learned this lesson for a single pill — `LABEL_KEEP`,
asymmetric appear/stay thresholds, a ramp instead of a cutoff, and the
note that *nothing about a tile's size should be a cliff*. A whole-canvas
dim flipping at every tile boundary is that same failure across fifty
times the area. Desktop only; `isHoverPointer` gates it, and touch has its
own answer in ADR 094.

**6b. The card list is a second trigger for that same dim** (added
2026-09-16). Hovering a patch's card in the list beside the quilt dims every
other tile, exactly as hovering the tile does. The canvas takes a
`focusPatchId` prop and answers it by calling the same `engageDim`, so the
two surfaces cannot drift into two different ideas of what "this one" looks
like, and the card inherits the dwell for free.

It dims rather than mutes for the reason above, and the reason is sharper
here: the card list is where a reader who finds the quilt hard to read
spends their time, so a focus gesture that did nothing in Muted would be
missing for exactly the person it is for.

**It does nothing when that patch's tile is scrolled out of view.** Dimming
every visible tile for one the reader cannot see leaves the whole quilt
washed with nothing lit, which reads as the quilt breaking rather than as an
answer. The canvas already computes the in-view set for ADR 074's lens, so
it checks its own answer before engaging.

The flow is one-directional on purpose. The list points at the quilt; the
quilt answers its own pointer itself. Feeding ADR 078's shared `previewing`
id back in would have a hovered *tile* ask the canvas to focus the tile
already under the pointer.

## Considered options

- **Mute what the admin chose — clamp chroma, keep every slot.** Every
  choice survives, quieter. Rejected: it reads as exactly what it is,
  somebody dragging a saturation slider left, which is the register this
  project exists in opposition to. It also loses distinctions silently —
  two high-chroma hues at the same lightness converge into the same mud.
- **Build the ramp around the identity color where it actually sits.**
  Maximum fidelity to the admin's pick. Rejected: it quiets each tile and
  leaves the quilt just as loud from across the room, which is the
  distance the complaint was made from. It also cannot guarantee three
  separable steps for a color near either end of the lightness range.
- **Set chroma to a fixed value rather than capping it.** Perfectly even.
  Rejected: it invents a hue for Stage Black out of rounding noise and
  makes Flax — a deliberately chalky yellow — as loud as Hi-Vis. The mode
  would be adding color nobody chose, which is the one thing it cannot do
  and still be called identity-preserving.
- **A control on the canvas** (ADR 074's rule). Rejected — see decision 4.
- **A `users` column, like `hide_amended_linings`.** Follows a member
  across devices. Rejected: it cannot reach an anonymous reader, and it
  drags a migration, an endpoint and three boundary lists behind it for a
  preference light/dark already proves does not need them.
- **An instance default.** Rejected — see decision 3.
- **Naming the two registers after quilting traditions — "Scrap" and
  "Solid".** Both are genuine craft vocabulary and accurately describe
  what is drawn. Rejected as too cute for a control: a reader should not
  have to learn a word to turn their colors down.
- **Naming the muted register after the Amish quilt**, which is the design
  lineage — solid colors, no prints, bold geometry, a shared dark ground.
  Rejected outright. This instance serves Lancaster County, where the
  Amish are neighbours rather than an aesthetic; taking a living religious
  community's tradition as the name of a display toggle is not ours to do.
  The influence is kept and the label is not.
- **Giving hash-assigned patches a hue-spread color in Muted only.**
  Tempting, since nothing was chosen for them, so no admin intent is
  violated. Rejected: it would make Default and Muted disagree about a
  patch's *hue*, and hue is the one channel this design promises to carry
  through — the tile, the card and the map marker would stop agreeing
  about what color a patch is.

## Consequences

**Patches with adjacent hues become hard to tell apart.** Scarlet and
Tomato normalize to nearly the same tile. Accepted: the block, the name
badge and the motif all carry identity in parallel, and ADR 078 already
concluded that a patch's identity color "means nothing to a stranger" and
kept it only for quilt↔map continuity — which normalized hue preserves
exactly.

**The eight hash-assigned palettes are clustered in hue, and Muted exposes
it.** Their primaries sit at 1°, 5°, 13°, 17°, 323°, 338°, 86° and 200° —
**six of eight inside a 55° arc through red**. Default hides this, because
the variety a reader sees comes from the secondaries and grounds; Muted
strips exactly those and leaves hue as the only variable.

**Measured on the live instance, 2026-09-16** — 58 patches off
`/api/v1/nodes/tree`, each resolved through the real
`identityColorForPatch` and muted:

| | |
|---|---|
| Patches that chose their own colors | **4 of 58** |
| Hash-assigned | **54 of 58** |
| Distinct muted tile colors | **9 of 58** |
| Inside the 320°–40° red arc | **39 = 67%** |
| Largest single 30° bin | **17 = 29%** |

**The gate fails, so the hash-assignment widening is a prerequisite.**
Muted would draw a 58-patch quilt in nine colors, two thirds of them red.
The blocks still differ, so tiles are not identical — but color would stop
being an identifying channel almost entirely, which is the one thing this
design promised it would preserve.

Widening hash assignment to draw from the 57-swatch fabric wall lands as
**its own change, first**, because it alters what every unclaimed patch
looks like in Default too, and a viewer-side mode must not smuggle that
in. It improves Default on its own merits: the clustering is there today,
doing quieter damage.

Note what the measurement also says about the shape of the problem — only
4 of 58 patches have chosen anything. This thins out as patches are
claimed and pick off the wall, the same way the unclaimed-mark density
does. But "it gets better later" is not a reason to ship the accommodation
in the state where it helps least, on the instance whose reader asked for
it.

**Hover-dim replaces shipped behavior**, rather than adding to it. The
hovered tile cannot both be the one full-color tile and carry a darkening
scrim.

**Nothing crosses the server boundary.** No migration, no endpoint, no
seamrip decision, no export field. A patch's stored `appearance` is never
written by any of this — what changes is how a reader is shown it.
