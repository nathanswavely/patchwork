# ADR 078: The map says where; the card says who

Date: 2026-08-31. Status: **accepted**; implementation pending (the
clustering half is prototyped and measured, the rest is not built).
Decided while grilling the map's UX. Applies ADR 022's lens rule to the
one surface that never honoured it, and borrows the quilt's identity-versus-
status discipline (docs/adr/004, docs/adr/029) to a second surface.

## Context

The map has been the weakest discovery surface since it shipped. It draws
one teardrop per placed patch in the patch's identity color with a white
contrast dot at its centre, and a popup carrying a name, a description
snippet, and a link that navigates away. It answers "where are the patches"
— which the cards pane beside it also answers, with more detail and less
effort. Nothing on it says what a patch *is*, and nothing about it rewards
staying.

Three things found while grilling it are load-bearing for what follows.

**The pile is real and framing cannot fix it.** All 33 seeded patches sit
inside 1.2 km × 1.0 km. The closest pairs are 11 m, 20 m, 33 m and 34 m
apart — Binns Park and the Lancaster Arts District are eleven metres from
each other. At the zoom that frames them all, **15 of 33 markers occlude
another marker**, and separating the closest pair by one marker width would
need roughly z=19, a view containing three patches. There is no framing
that shows this community without a pile.

**The map does not narrow under the filter lens.** The cards apply tags and
query; the map applies query only — `getSelectedTags()` is never consulted.
ADR 022 is explicit that all four discovery surfaces "narrow live under the
same lenses". The map is the one that doesn't.

**Both features asked for already exist under other names.** The **motif**
is the quilt tile's corner mark for "what this patch is", resolving through
`appearance.icon` → first motif-bearing tag → the quilt mark. The **name
badge** is the quilt's zoom-aware label, with a tuned reveal engine behind
it. Neither needed inventing; one of them needed *not* to be copied.

## Decision

**1. A marker carries the motif and nothing else.** A quilt tile wears
three corner marks — motif, unclaimed, role. A 26px teardrop carries one
glyph, and the glossary already says which: the **Unclaimed mark**'s homes
are "a quilt tile… and a patch card", never the map, and the patch card
already carries the viewer's standing. So the marker says *what kind of
thing is here* and the card says *who runs it and what you are to it*. The
glyph takes its color from `textOnColor`, the same answer the quilt reached
for marks on arbitrary fabric.

**2. The marker keeps its identity color.** Google colors a POI by
category, so glyph and color reinforce each other; ours colors by identity,
so the color means nothing to a stranger. Kept anyway: it is the only
visual continuity between the quilt and the map — the same patch is the
same color on both — and the category is already carried by the glyph, so
the channel is not wasted, only doing a different job.

**3. Names are placed by separation, not by zoom.** On the quilt a tile has
area, and area is the gate: `LABEL_MIN_PX` refuses a badge to a tile too
small to be worth naming. A marker is a point. It has no area, and zooming
does not enlarge it — it moves markers apart. So the quilt's *primary* gate
has no analogue here and must not be copied; what ports is its second half,
the `LABEL_GAP` / `LABEL_KEEP_GAP` hysteresis that stops a name blinking off
and back on mid-pinch, which was priced over a 34-step sweep and is the
expensive part of that file.

The felt behaviour is unchanged — zoom in, more names — but the rule is
honest about the mechanism, and it handles the case zoom cannot: two
patches at one address never separate at any zoom, and a zoom threshold
would stack their names forever.

**4. Priority is quilt activity.** When two names collide, the winner is
the higher `patchActivity` — members + events + followers/3, the measure
the quilt sizes tiles with. Today the map draws in `ORDER BY n.name ASC`,
so with no ordering decision the alphabet would decide, which no viewer
could infer. ADR 074 already set the precedent that other surfaces read
quilt order rather than inventing their own. Stacking order follows the
same measure, so the marker you can click is the one whose name you can
read.

**5. Two tiers: some names always, the rest on demand.** Names that clear
the separation rule are drawn; markers that lose keep their motif and lose
their name; pointing at or tapping any marker reveals its name and raises
it above its neighbours. The always-on tier is what makes the map scannable
at a glance; the on-demand tier is the repair for what it had to drop.

**6. The map clusters, and a cluster is not a patch.** Markers within a
pixel radius of each other collapse into one disc carrying a count. Because
a cluster is not a patch it wears no identity color, no motif and no name —
it has as many of each as it has members — and it takes the neutral dark
disc the quilt already gives to status. Clicking one fits its members.
Clusters are never label candidates, which leaves the crowded zooms with
fewer names competing and every name attached to something clickable.

Clustering applies at every zoom, by pixel distance, with no zoom
threshold. `QuiltCanvas` paid for that lesson already: "Nothing about a
tile's size should be a cliff."

**7. A preview costs a gesture the device can spare.** Where there is a
pointer, pointing at a patch previews it — its card highlights in the cards
pane — and clicking opens the patch. Where there is no pointer there is one
gesture, so the first tap previews into the **docked card**, and the card
itself is how the patch is opened. The quilt and the map behave the same as
each other on the same hardware; the platforms differ because the input
does, not because the surfaces do.

This is what makes the docked card worth building, and it is not a map
feature: the quilt has the same hole. Tapping a tile navigates away, which
on a phone spends the person's zoom and pan to answer "what is that one?",
and the tiles small enough to provoke the question are exactly the ones
whose badges `LABEL_MIN_PX` has already suppressed.

**8. The map narrows under every lens, and fits once.** The filter lens is
wired, completing ADR 022. The viewport is fitted on the first paint that
has something to fit, and never again: narrowing a set is not a request to
be moved somewhere else. A filter whose matches all fall outside the view
gets an affordance naming them, not a jump — the same discipline as ADR
022's rule that narrowing to nothing must explain itself.

## Considered and rejected

**Events on the map.** The map shows patches only. Events would mean a
second kind of marker, a time lens the map does not have, and a ruling on
whether an event with a `location` string but no coordinates is invisible
or inherits its patch's point. Deferred deliberately, not forgotten —
`events.latitude` and `events.longitude` exist and this is the obvious
place they would land.

**Category colors, Google's model.** Rejected on three counts: it would
sever the quilt↔map color continuity, it would need a curated `tags.color`
vocabulary that does not exist (`tags.motif` does), and it would override
an appearance choice ADR 004 and 029 place with a patch's own admins.

**Desaturating unclaimed markers.** Tempting — a stranger tapping a venue
pin has no idea nobody runs it. Rejected because it makes color mean two
things at once, and the quilt refuses that exact move: a tile's own color
never means "unclaimed".

**Copying the quilt's badge engine wholesale.** Its size gate and its
`LABEL_ROOMY_PX` ramp are properties of tiles, which have area. Ported to
points they would be superstition.

**`leaflet.markercluster`.** Prototyped against the seeded data alongside
the hand-rolled version. Both produced *identical* groupings — 13 lone
markers and 6 clusters of [7, 4, 3, 2, 2, 2] — so the choice was never
about output. Rejected on integration: the label engine needs the visible
set and its priority order, and the hand-rolled pass returns exactly that,
already sorted, while the plugin keeps the visible set inside itself and
would leave two sources of truth to agree on every zoom. We would also
disable most of what we imported (spiderfy, coverage-on-hover) and restyle
its icon to get the neutral disc. Its 8.8 kB gz against 66 lines was the
smaller argument. What it would have bought — chunked loading, animation
polish, keyboard traversal — is scale we do not have; the hand-rolled pass
is O(n²) per zoom, which is 1,089 operations at 33 patches and wants a grid
index somewhere in the low thousands.

**Re-fitting the viewport as the set narrows.** This is what fixing the
filter lens would have done for free, and it would have been a worse map
than the broken one: tap "music" and the view leaves downtown for whatever
matched furthest away.

**Geocoding the address.** Closed already: an Address is "never parsed and
never geocoded", and a map location "is a separate act".

## Consequences

- `CONTEXT.md` gains **Patch card**, a term it used three times and defined
  nowhere while defining "remote patch card" in full. The entry carries the
  two homes and the preview rule from decision 7.
- The **Event** entry's justification for retiring "pin" claimed that
  "events literally appear as map pins on the Leaflet map." They never
  have. Corrected: the map's markers are literal teardrop pins and every
  one of them is a patch — which is the half of the collision that was
  always true. **ADR 027 carries the same false sentence in its first
  bullet** and is left as written, being a record; its decision is
  unaffected, since the collision it names survives the correction.
- Tapping a quilt tile stops opening the patch on touch devices, on a live
  instance, where it has meant "go there" since launch. It is one extra tap
  for someone who knew where they were going, and it is the reversible
  direction: selection → navigation is a line, and re-teaching a gesture
  twice is not.
- **What the cards pane lists in map mode is deliberately unresolved.** The
  map shows only placed patches while the header counts every patch that
  passes the lenses, so the count can promise more than the map paints. The
  pane's composition belongs to a concurrent piece of work; until it lands
  the discrepancy stands, and this ADR does not rule on it.
- Fixing the fit exposed a defect worth recording: the first `updateMarkers`
  pass runs before the fetch lands, and an empty pass consuming the
  one-time fit leaves every marker piled at the default zoom. The latch
  only closes on a pass with something to fit.
- The measurements here were read from the DOM, not from screenshots. The
  preview pane served stale composites throughout — a single "33" disc on
  screen while the DOM held 6 clusters and 13 markers.
