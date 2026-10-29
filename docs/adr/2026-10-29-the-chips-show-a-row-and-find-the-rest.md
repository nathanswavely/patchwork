# The filter chips show one row and find the rest

**Status:** accepted, 2026-10-29. Amends docs/adr/033.

## Context

docs/adr/033 put the tag vocabulary on every discovery surface as
toggleable filter chips, and said the chips render the whole vocabulary.
It leaned on docs/adr/021's curated restraint: a vocabulary that outgrew a
couple of wrapped lines was an admin problem before it was a UI one.

Two things changed. docs/adr/114 lets a patch suggest a word, so the
vocabulary now grows from below as well as from the admin panel. And the
desktop canvas chips had drifted from the top edge 033 gave them to the
bottom-left corner, where they grew upward over the quilt and shared the
corner with the Quilt/Map switch and the map's zoom control. With 21 tags
the row already wrapped to three lines on a common laptop width, and every
line was quilt nobody could see.

## Decision

**The chips show one row of the most-used tags, and every tag is found by
typing.**

- **One row, in usage order.** Overlay (quilt, map) and flow (events)
  plan a single row from `getRankedTags`, the per-tag patch count already
  used for ordering, measured against the row's real width rather than a
  fixed number, then a "N more" button. The order never changes to fit:
  the row stops early instead.
- **An active filter is never behind the count.** A selected tag is always
  in the row, wherever it ranks, in rank order. This is 033's "announced
  where it bites" rule, and the reason the row is planned rather than cut.
- **"N more" grows the row into a card.** The card hangs from the line the
  rail and the patch list hang from, spans the row's width, and wears the
  same frosted recipe as the patch list (`.pw-canvas-card`). A find field
  takes the row's place and the row slides down whole, with the remaining
  tags after it, so no chip moves relative to another on open. The field
  keeps the width a tag needs; the line's far end holds a tag count, Clear
  and Done. Toggling applies at once. The card is not modal, because the
  canvas narrowing beside it is how a person sees what a tag does; a click
  on the canvas closes it and does nothing else. On the events page the
  card is solid, since text behind glass reads as smudge.
- **When every tag fits,** a search button stands where "N more" would, so
  finding by typing is always one click away.
- **Show and hide are one button that never moves.** Hidden, the chips are
  a funnel button at the row's start; shown, the same spot holds a hide
  button (an eye shut, not an X, which read as clearing the filter).
- **Phones keep the filter sheet**, which shows every tag under the same
  find field. Desktop and phone share one find rule (`lib/tagFind.js`).
- **The canvas chips return to the top edge**, as 033 first placed them,
  and the Quilt/Map switch moves to the canvas's bottom-right. The quilt's
  fit leaves room for the row.
- **One zoom for both canvases stands beside the switch.** The map wore
  Leaflet's control in the bottom-left and the quilt had none; now a pair of
  buttons drives whichever canvas is showing, about the middle the reader
  can see. On a phone the switch rides the floating pill, so the zoom
  stands alone on its side above the filter button, and the filter sheet
  opens over it.

## Consequences

- 033's consequence that "the chips render the full vocabulary" no longer
  holds on desktop. Its placement rules, the shared collapse preference and
  the sheet's exemption all stand.
- The chips still carry no counts. The ranking uses whole-quilt patch
  counts, which on My Quilt and the events page describe a corpus other
  than the one being narrowed; the order is a reasonable default there, a
  number on a chip would be a false statement.
- The row is planned from measured widths, so its contents depend on the
  pane width and the rail state. Frontend tests read source text and cannot
  see that; the browser is where it is checked.
