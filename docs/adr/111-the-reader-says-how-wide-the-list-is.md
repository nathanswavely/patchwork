# ADR 111: The reader says how wide the list is

Date: 2026-09-16. Status: accepted. Decided while grilling a request for a
button to resize or hide the patch cards panel. Amends ADR 074 (the control
rule, and the lens's availability gate) and ADR 094 (the docked profile's
slot can now be zero-width).

## Context

The cards pane is 45% of a desktop window and has been since it was drawn.
Nobody chose 45% for the reader; it was chosen for the layout, and ADR 074
and ADR 094 both then leaned on it — the first to make the list and the
canvas one instrument, the second to find "a slot the width of a profile
sitting there".

Two things have been true the whole time and neither had a way out. The
quilt is the signature visual, and a reader who wants to look at it is
looking at 55% of one. And 45% is a percentage, so the card it holds ranges
from 266px on a 1280 laptop to 752px on a 3440 monitor against a cover fixed
at 100px tall — a 7.5:1 letterbox strip nobody designed.

The number is also written three times in two components: `.cards-pane`'s
width, `quiltInset` (fed to both canvases, where it drives zoom-fit padding,
the in-view edge test and every empty state's padding), and
`.quilt-chips { right: calc(45% + 16px) }` over in the shell, whose comment
says "clear the cards pane". Three files agree by luck.

And the canvases do not react to it. `QuiltCanvas.handleResize` is driven by
`containerEl.clientWidth`, but `.quilt-pane` is `inset: 0` — full-bleed
*behind* the pane — so its width never changes when the pane's does. Only
`computeInView` tracks `insetRight`. Widening the pane today would leave the
quilt centred for the old width.

## Decision

**The pane's width is one fact the reader sets, and it is the shell's.**
Six parts.

**1. Three stops, named in cards.** Two columns, one column, hidden. The
stop names how many cards sit side by side, not a percentage, because the
pane exists to hold cards and that is the only unit a reader can see.
Hidden is the width's zero, not a second concept: one fact, one control,
one stored value.

One column is `22.5% + 10px` — arithmetically the width at which a single
card is exactly as wide as each of the two were. So changing stops gives
back half the pane and leaves the card's size alone. **What the reader gains
is quilt, not a bigger card**, and the patch card keeps CONTEXT.md's promise
that it is "deliberately the same one wherever it appears".

Measured in a browser it is half a scrollbar out — 295px at two columns
against 287px at one, on Windows, on a 1440 window. The scrollbar is inside
the pane and is subtracted once either way, so the exact term is
`+ 10px + scrollbar/2`, and the scrollbar is a platform fact: 15–17px on
Windows, zero under macOS overlay scrollbars, and present only while the
list overflows. Measuring it at runtime would make the pane's width depend
on how many patches this quilt happens to have, which is worse than 8px.
The constant stays platform-independent and the claim is "the card does not
resize", not "to the pixel".

Two columns stays 45%. Nothing about the default changes for a reader who
never touches the button.

**2. The grid is `repeat(N, minmax(0, 1fr))`, N from the stop.** The
project's `repeat(auto-fill, minmax(...))` idiom — eleven other grids use it
— is the wrong tool here, and was tried first. `auto-fill` derives columns
*from* width; the stop derives width *from* columns. At the one-column stop
the two disagree above roughly 1600px: a 1920 window would show two columns
and a 2560 window three, while the button said one. An explicit N cannot
disagree with a width computed from N.

The "fall back to one column when two will not fit" rule this replaced turns
out to have no cases. Two columns at 45% only pushes the card under 170px
below an 853px window, and between 768 and 853 it lands at 160–170px — the
width the phone already ships. Mobile keeps its own `repeat(2, 1fr)`
untouched; the stops are desktop-only.

**3. Width is an arrangement, so it persists.** The codebase already draws
this line and it holds: the collapsed rail, the collapsed chips and the
theme go to `localStorage`; the filter, the order and the in-view lens are
session-ephemeral module state on purpose (ADR 022, ADR 074). Deciding how
the room is divided is the first kind. It lives in `quilt.svelte.js` beside
`chipsCollapsed`, because the shell's chips need it as much as the surface
does — which is the same reason it must stop being a literal in three files
and become one published fact.

**4. The control is the shell's, and it migrates.** ADR 074 decision 3 says
a control belongs to the surface it changes, and that is why Quilt/Map left
this very header for the canvas. Width changes *both* panes, so by that rule
it is neither pane's — it arranges the room. It still sits in the pane's
header while there is a header to sit in, because that is where a reader
looks for it; at the hidden stop it is canvas chrome, on the floating layer
the view switcher and the chips already share. One button either way, its
chevron pointing the way the pane's edge will travel, cycling two columns →
one column → hidden → two columns. **A control that can delete its own
container was never the container's**, and the hidden stop is what proves
it.

**5. Hiding suspends the in-view lens; it does not clear it.** ADR 074 gated
the lens on `winW > 768` to close "the resize case, where a lens set at
desktop width would otherwise keep narrowing a list whose control had just
disappeared". The hidden stop is that case exactly, so the gate grows a
second term rather than gaining a second behaviour: the lens needs two panes
on screen, and hidden leaves one. Suspended and not cleared, matching what
the mobile gate already does — the setting is the reader's and not the app's
to discard.

**6. A docked profile opens the pane to its last stop while it is docked.**
ADR 094 puts the desktop docked profile *in the cards pane's slot*; a
zero-width slot is no slot. The alternatives were to dock a phone-style
sheet (but ADR 094 decision 2 says the form follows the room, and a wide
screen with a hidden list is not a phone room), or to reopen the pane and
leave it open (which silently discards a setting). Instead the stored stop
is untouched and overridden for as long as a profile is docked; dismissing
returns the pane to hidden. This is ADR 094's own shape — the surface
underneath "stays alive but idle" — applied to the arrangement instead of
the canvas.

**The quilt re-centres and never re-zooms. The map does neither.** Hiding
the pane must reveal quilt, not background, so the canvas shifts by half the
width delta — which is precisely what `resizeViewport()` already does for
window resizes, and which needs a trigger rather than an implementation. It
is not a zoom-fit: moving a divider is not a request to be taken somewhere,
and ADR 094 spends real effort on returning a reader to the zoom and pan
they left.

The map keeps its own stated rule that "narrowing never moves the viewport
by itself" and only re-places its labels, which its existing effect already
does on an inset change. The asymmetry is the point: quilt space is re-sewn
as membership changes and holds no coordinate a reader remembers, so sliding
it costs nothing; map space is real geography, and moving it under somebody
is disorienting. Markers behind the pane simply become visible, and the
off-screen notice — which already positions itself from `insetRight` —
corrects itself.

## Considered options

- **A drag handle on the pane's edge.** The universal divider idiom, and a
  reader on an ultrawide picks their own number. Rejected: it needs a refit
  policy for a continuous stream of widths (per frame is churn, on release
  is a lurch), arrow-key support to be reachable, and a second affordance
  anyway for the hidden stop, where there is no edge to grab.
- **Fixed pixel stops, or a fixed natural card width.** Would have fixed the
  752px letterbox card outright, and was recommended. Rejected as a bigger
  change than the request: it moves the default layout on every screen above
  1440 for every reader, including the ones who never press the button. The
  letterbox card is a real defect and stays one — it is now a separate
  decision that can be taken on its own evidence.
- **`auto-fill`, with the stops renamed wide/narrow/hidden.** Uses the
  existing idiom honestly, and three columns on a 2560 monitor becomes a
  feature rather than a lie. Rejected: "wide" and "narrow" name nothing a
  reader can check, and counting cards is the one thing they can.
- **Zoom-fit the quilt on every width change.** Consistent with ADR 074's
  "the canvas zoom-fits at rest". Rejected above.
- **The docked profile as a phone-style sheet when the pane is hidden.**
  Rejected above.
- **Clearing the in-view lens on hide.** Reopening would then always give
  the whole list back, with no surprise narrowing on return. Rejected: the
  mobile gate deliberately chose suspension, and one surface should not
  answer this question twice.

## Consequences

- `quiltInset`, `.cards-pane { width }` and `.quilt-chips { right }` stop
  being three literals and become one published fact. The chips' offset is
  the one most likely to be forgotten: it lives in `SocialShell.svelte`,
  which has no other reason to know the pane exists.
- `QuiltCanvas` gains its first reason to act on an `insetRight` change. Its
  ResizeObserver cannot supply one — `.quilt-pane` is `inset: 0` and never
  changes size — so the trigger is explicit, and the 150ms width transition
  and the re-centre must share a duration or the quilt lurches after the
  edge has stopped.
- `lensAvailable` gains a term, and every empty state that reads
  `inViewActive` follows it for free.
- The pane's header and the floating chrome are now two homes for one
  button, and only one of them exists at a time. A change to either has to
  remember the other.
- There is no Svelte render library in this project, so none of this is
  test-coverable beyond source assertions: the stops, the migration, the
  dock override and the re-centre all have to be checked in a browser at
  more than one window width.
- ADR 074's "a control belongs to the surface it changes" now has its first
  exception, and it is a clean one rather than a crack: a control that
  changes both surfaces belongs to neither. The rule is amended to name the
  shell as the owner in that case, which is where the Quilt/Map switch would
  have gone had it changed both.
