# ADR 074: The list reads the quilt

Date: 2026-08-31. Status: accepted. Decided while grilling discoverability
on the home quilt.

## Context

`GET /api/v1/nodes/tree` orders `n.name ASC` (`internal/handler/tree.go`),
and `SocialHome.svelte` renders that order straight into the cards pane.
Nobody decided this; it is the default nobody revisited. On the reference
instance it means a stranger's first impression of Lancaster is Binns Park,
because B — and that one of the largest venues in the city sits under T.

An alphabetical list serves *lookup*: "I know the name, help me find it."
ADR 033 gave that job to the search dropdown — type, see results, pick one,
land there. Search took lookup and the list never noticed. It is now the
discovery surface, still ordered as a directory.

Beside it sits a canvas with real pan and zoom that the list ignores
completely. Two panes side by side with no relationship between them:
scrolling the list and reading the quilt are unrelated activities. Zillow
and Google Maps — the closest UX analogues — make the sidebar the
viewport's contents, so that moving the map *is* the primary narrowing act.

The pane headers had drifted too. `cards-header` holds `<h2>Patches</h2>`,
the result count, and the Quilt/Map toggle: two controls about the list and
one about the other pane, which navigates a route (ADR 035) and changes
nothing the list shows.

Volume is coming. The tree endpoint is unpaginated by necessity — the
treemap needs every patch to lay out — so a thriving quilt is one large
fetch and several hundred alphabetical cards.

## Decision

**The list and the canvas are one instrument.** Three parts.

**1. Quilt order is the default order.** The cards list renders in the
order the layout engine placed the tiles: the largest tile at the origin,
then repeatedly the tile with the highest total affinity to what is already
placed, set on the frontier cell nearest its affinity target
(`web/src/lib/quiltLayout.js`). That is centre-out by construction, and it
is a **return value, not a computation** — the layout already produces this
order and currently discards it.

A→Z remains as the one alternative, for looking a name up rather than
finding something. "Recently added" joins them once the tree carries an
arrival timestamp (ADR 076).

**Amended 2026-09-04, on first contact with the reference instance's real
data.** "Recently added" orders by arrival *where there is one*, and falls
back to when the listing appeared where there is not. Ordering by arrival
alone was correct about arrivals and useless as an order: 47 of Lancaster's
52 patches are unclaimed listings, which have no arrival by definition, so
they all landed in an undated tail — throwing away the fact that 24 were
seeded on launch day and 22 more were added across the month after it.

The conflation was treating one timestamp as the answer to two questions.
"When did a community arrive" is the bulletin's question and must stay
strict, or an admin importing a directory announces arrivals that never
happened (ADR 076). "How new is this to the quilt" is an ordering's
question, and every patch can answer it. They coincide for a patch someone
created, which is why the difference went unnoticed. The column keeps the
strict meaning; only the sort takes the looser one.

Activity rank was the obvious alternative, and ADR 015 already refuted it
for tile *size*: "rank invents a winner from a field of zeros." On today's
Lancaster a `member_count + event_count` sort is twenty-six patches tied at
zero in arbitrary order — the same lie in a different medium.

The deeper reason is that quilt order **makes no new editorial claim**. It
surfaces the ranking that has been drawn on screen the whole time. Nobody
has to trust it; they can look at it. That is a property no feed can have,
and it is why this is an ordering a quilt is allowed to ship.

**2. The in-view lens.** A toggle narrows the list to the patches currently
inside the canvas's viewport. It is a lens in ADR 022's sense — standing,
announced, session-ephemeral, independent of the others — and the **first
surface-local one**: it cannot narrow `/events`, because a viewport has no
meaning there. ADR 022's "applied in place, on every discovery surface" is
amended to *every surface the lens can bite*.

Default off. The canvas zoom-fits at rest and after every relayout
(`QuiltCanvas.svelte`), so the lens composes with the filter for free: a
filter change re-sews the quilt, refits the viewport, and puts the whole
filtered result in view. The lens never silently eats results a filter has
just produced.

**Not in the URL.** Scope is addressable (ADR 035); the filter deliberately
is not (ADR 022) — lenses differ, and this one follows the filter. Google
Maps and Zillow put viewports in URLs because latitude and longitude are
eternal. Quilt space is not: the layout re-runs as membership changes, so a
bookmarked `x,y,k` would silently come to mean different patches. A link
that rots without saying so is worse than no link.

**"In view", never "visible".** `visibleIds` and `item.visible` in
`QuiltCanvas.svelte` already mean *passes the filter*. The viewport concept
takes its own word rather than overloading that one.

**3. A control belongs to the surface it changes.** The in-view toggle and
the order control live in the list's header, because they change the list.
The Quilt/Map switch moves onto the canvas, because it changes the canvas.
The count states the lens: "12 of 49 in view".

## Considered options

- **Keep alphabetical as the default.** Rejected: search owns lookup, and
  A→Z survives as the explicit alternative for those who want it.
- **Sort by activity.** Rejected on ADR 015's reasoning, which was written
  about this exact data on this exact instance.
- **The in-view lens as a chip in the filter row.** Attractive — ADR 033
  already rides a non-tag chip there, which would have bought announcement,
  clearing, and the mobile sheet for free. Rejected on the control rule: the
  chips live on canvases *and* on `/events`, and this lens changes the list
  and cannot exist on `/events`.
- **Bind the viewport with no toggle.** Rejected: `SocialHome`'s panes
  *toggle* on mobile rather than sitting side by side, so the lens would be
  invisible while the list is being read — precisely the silent-lens failure
  ADR 022 was written against and ADR 033 was written to fix.
- **Viewport binding on the map only, the quilt getting order alone.**
  Coherent and cheaper. Rejected because the explicit toggle removes the
  mobile objection that motivated it, and because making the canvas do
  something is the point.
- **Viewport in the URL.** Rejected above. Worth revisiting for the map,
  where the coordinates are real and eternal.

## Consequences

- `quiltLayout()` must return its placement order and `SocialHome` must
  sort by it; today that order is computed and thrown away.
- A shipped control moves on a live instance: the Quilt/Map toggle leaves
  `cards-header` for the canvas. **Only the desktop form moves.** This ADR
  first said both did, on the assumption that the mobile pill fusing
  Quilt/Map/List had the same defect; building it showed otherwise. The pill
  is not the list's header — it floats in shell chrome above *both* panes, at
  the same z-index layer the stylesheet already calls "the canvas chrome
  layer", and on mobile the panes toggle, so all three of its buttons answer
  one question the shell owns: which pane am I looking at. Splitting it would
  have cost a tap to satisfy a rule it never broke.
- **The in-view lens is desktop-only, and that is the rule holding rather than
  bending.** `.cards-header` is `display: none` below 768px, so there is
  nowhere on mobile for the lens's control to live — and the panes toggle, so
  there would be no visible canvas to see it working against either. The lens
  is therefore gated on the same breakpoint (`lensAvailable`). This is an
  absence, not the two-behaviours conditional the Considered options rejected:
  the lens needs two panes on screen at once, and mobile has one. It also
  closes the resize case, where a lens set at desktop width would otherwise
  keep narrowing a list whose control had just disappeared.
- Empty states gain the lens: "No patches in view — N elsewhere on the
  quilt," with the toggle as the one-step way back.
- `relayoutGrouped()` — My Quilt with remote regions — deliberately does not
  repack or refit, so tiles pop in and out at fixed positions. The in-view
  lens therefore behaves differently on `/my` than on `/`; that difference
  protects the sashing (ADR 024) and is not a bug to fix.
- ADR 022's rejection of per-tag counts on chips ("the tags endpoint doesn't
  serve them") is now factually stale: `node_count` ships on
  `GET /api/v1/tags` and is already parsed into the quilt store on every
  page load. That rejection must be re-decided on its merits, not inherited.
