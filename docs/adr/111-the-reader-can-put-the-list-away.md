# ADR 111: The reader can put the list away

Date: 2026-09-16. Status: accepted. Decided while grilling a request for a
button to resize or hide the patch cards panel, and revised after building
the resizing half and using it. Amends ADR 074 (the control rule, and the
lens's availability gate) and ADR 094 (the docked profile's slot can now be
zero-width).

## Context

The cards pane is 45% of a desktop window and has been since it was drawn.
Nobody chose 45% for the reader; it was chosen for the layout, and ADR 074
and ADR 094 both then leaned on it — the first to make the list and the
canvas one instrument, the second to find "a slot the width of a profile
sitting there".

The quilt is the signature visual, and a reader who wants to look at it is
looking at 55% of one. There has never been a way to ask for the rest.

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

**The pane is shown or it is hidden, and the reader says which.** Five parts.

**1. One bit, and no widths between.** Shown is exactly what shipped before
this control existed: 45%, two columns, the same cards. Hidden is zero, and
the canvas gets the whole window. Nothing about the default moves, and a
reader who never presses the button sees no change at all.

This is the revision. The first build of this ADR had three stops named in
cards — two columns, one column, hidden — with one column at `22.5% + 10px`,
arithmetically the width at which a single card is as wide as each of the two
were. It worked, it was tested, and using it settled the question against
itself: **a narrower pane is a worse list and a barely better quilt.** One
column shows half as many patches in the same scroll, and the quilt gains a
strip it cannot do much with. The want the control answers is "get out of the
way", and that want has no middle. A stop nobody would choose is a press
everybody pays on the way past.

**2. The control has one home, and it is not the header.** It parks against
the pane's left edge, on the canvas chrome layer the view switcher and the
chips already share, reading the same published width the pane does — so it
travels with the pane and stays in the same place relative to the thing it
moves, whether that thing is there or not.

ADR 074 decision 3 says a control belongs to the surface it changes, and that
is why Quilt/Map left this very header for the canvas. This one changes
*both* panes, so by that rule it is neither pane's — it arranges the room,
and the shell owns the room.

The first build put it in the pane's header while there was a header, and on
the canvas once there was not. That is two places to learn for one control,
and the reason it could not simply stay in the header is the reason it should
never have been there: **a control that can delete its own container cannot
live inside it.** The header was also the wrong shape for it — at the
one-column stop the header's own contents wrapped, and the width control was
the thing pushed off the end, unreachable at a stop it existed to reach.

> **Amended 2026-09-20.** The list is now drawn as one full-height glass
> card (header and cards on a single surface, hung from the same 56px line
> as the rail's card), and the control ends that card's own header row,
> on the edge it puts the card away toward. Two of the three objections
> above no longer hold: the header is a row of a card that does not wrap
> the control off its end, and the container the control deletes is the
> card it sits in, as the rail's toggle sits in the bar it collapses. The
> third stands and is paid knowingly: put away, there is no card, so a
> canvas copy stands in at the window's edge. Same glyph, same top line,
> one press either way. While a profile is docked in the list's slot there
> is no control at all: the pane is open for the profile rather than by the
> reader's bit, and the profile's dismiss is what ends that. This retires
> the press-while-docked rule below (under "A docked profile opens the
> pane for as long as it is docked"), which dismissed the
> profile and put the pane away in one act; there is no longer a press to
> give that meaning to.

**3. It is the rail's toggle, mirrored.** Same `SidebarSimple` glyph, same
two weights (`duotone` put away, `fill` in place), flipped on X, sitting at
`top: 68px` on the right exactly as the canvas view switcher sits on the
left. These are the two edges of one room, and a reader who has learned the
left one has learned this without being taught twice. A chevron was tried
first and says only "something moves that way".

**4. The bit is an arrangement, so it persists.** The codebase already draws
this line and it holds: the collapsed rail, the collapsed chips and the theme
go to `localStorage`; the filter, the order and the in-view lens are
session-ephemeral module state on purpose (ADR 022, ADR 074). Deciding how
the room is divided is the first kind. It lives in `quilt.svelte.js` beside
`chipsCollapsed`, because the shell's chips need it as much as the surface
does — which is the same reason 45% must stop being a literal in three files
and become one published custom property.

Desktop only. Below 768px the panes already toggle full-screen and the mobile
pill is the control that answers this question; a pane hidden on a wide
screen must not follow a reader to their phone and leave them with no list.

**5. Hiding suspends the in-view lens; it does not clear it.** ADR 074 gated
the lens on `winW > 768` to close "the resize case, where a lens set at
desktop width would otherwise keep narrowing a list whose control had just
disappeared". Hidden is that case exactly, so the gate grows a second term
rather than gaining a second behaviour: the lens needs two panes on screen,
and hidden leaves one. Suspended and not cleared, matching what the mobile
gate already does — the setting is the reader's and not the app's to discard.

**A docked profile opens the pane for as long as it is docked.** ADR 094 puts
the desktop docked profile *in the cards pane's slot*; a zero-width slot is
no slot. The alternatives were to dock a phone-style sheet (but ADR 094
decision 2 says the form follows the room, and a wide screen with a hidden
list is not a phone room), or to reopen the pane and leave it open (which
silently discards a setting). Instead the stored bit is untouched and
overridden while a profile is docked; dismissing returns the pane to hidden.
This is ADR 094's own shape — the surface underneath "stays alive but idle" —
applied to the arrangement instead of the canvas.

The override forces the button's meaning to be stated. **It means one thing
in every state: the canvas alone, or something beside it** — so pressing it
while a profile is docked dismisses the profile as well as putting the pane
away. A plain toggle was wrong here and wrong invisibly: with a profile
docked over a pane the reader had hidden, the stored bit says hidden while
the pane is visibly open, so a press labelled "Hide the patch list" would set
the bit to *shown*, destroy the setting, and change nothing on screen.

**The quilt re-centres and never re-zooms. The map does neither.** Hiding the
pane must reveal quilt, not background, so the canvas shifts by half the
width delta — which is precisely what `resizeViewport()` already does for
window resizes, and which needed a trigger rather than an implementation. It
is not a zoom-fit: putting the list away is not a request to be taken
somewhere, and ADR 094 spends real effort on returning a reader to the zoom
and pan they left.

The map keeps its own stated rule that "narrowing never moves the viewport by
itself" and only re-places its labels, which its existing effect already does
on an inset change. The asymmetry is the point: quilt space is re-sewn as
membership changes and holds no coordinate a reader remembers, so sliding it
costs nothing; map space is real geography, and moving it under somebody is
disorienting. Markers behind the pane simply become visible, and the
off-screen notice — which already positions itself from `insetRight` —
corrects itself.

## Considered options

- **Intermediate widths. Built, used, and removed** — see decision 1. The
  arithmetic was sound and the implementation was clean; the idea was wrong,
  and only using it showed that. Worth recording because it will be proposed
  again: the pitch is "let the reader choose", and the answer is that the
  choice on offer was between a good list and a bad one.
- **A drag handle on the pane's edge.** The universal divider idiom, and a
  reader on an ultrawide picks their own number. Rejected on the same ground
  the stops were, plus its own: it needs a refit policy for a continuous
  stream of widths (per frame is churn, on release is a lurch), arrow-key
  support to be reachable, and a second affordance anyway for the hidden
  state, where there is no edge to grab.
- **A fixed natural card width.** 45% is a percentage, so the card it holds
  runs from 266px on a 1280 laptop to 752px on a 3440 monitor against a cover
  fixed at 100px tall — a 7.5:1 letterbox strip nobody designed. A fixed card
  would fix that outright. Rejected as a different change wearing this one's
  clothes: it moves the default layout on every screen above 1440, for every
  reader, including the ones who never press the button. **The letterbox card
  is a real defect and stays one**, to be decided on its own evidence.
- **The control in the list's header.** Where the request pointed, and where
  the first build put it. Rejected above.
- **Zoom-fit the quilt when the pane moves.** Consistent with ADR 074's "the
  canvas zoom-fits at rest". Rejected above.
- **The docked profile as a phone-style sheet when the pane is hidden.**
  Rejected above.
- **Clearing the in-view lens on hide.** Reopening would then always give the
  whole list back, with no surprise narrowing on return. Rejected: the mobile
  gate deliberately chose suspension, and one surface should not answer this
  question twice.

## Consequences

- `quiltInset`, `.cards-pane { width }` and `.quilt-chips { right }` stop
  being three literals and become one published fact, which the control's own
  offset now reads too. The chips' offset is the one most likely to be
  forgotten: it lives in `SocialShell.svelte`, which has no other reason to
  know the pane exists.
- `QuiltCanvas` gains its first reason to act on an `insetRight` change. Its
  ResizeObserver cannot supply one — `.quilt-pane` is `inset: 0` and never
  changes size — so the trigger is explicit, and the 150ms width transition,
  the control's own `right` transition and the re-centre all share a duration
  or the three arrive separately.
- `lensAvailable` gains a term, and every empty state that reads
  `inViewActive` follows it for free.
- The hidden pane is clipped rather than unmounted, so the transition has
  something to carry out — which left forty controls in the tab order until
  it was marked `inert`. Anything added to the pane inherits that.
- There is no Svelte render library in this project, so none of this is
  test-coverable beyond source assertions: the toggle, the travelling
  control, the dock override and the re-centre all have to be checked in a
  browser at more than one window width. The two defects the first build
  shipped past a green suite — the wrapping header and the tab order — were
  both found that way.
- ADR 074's "a control belongs to the surface it changes" now has its first
  exception, and it is a clean one rather than a crack: a control that
  changes both surfaces belongs to neither. The rule is amended to name the
  shell as the owner in that case, which is where the Quilt/Map switch would
  have gone had it changed both.
