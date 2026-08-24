# ADR 066: A seam belongs to the boundary, not to either tile

Date: 2026-08-24. Status: **accepted**. Leaves ADR 004's appearance model
and ADR 029's drafted blocks untouched — a patch still chooses a block, a
rotation and a bundle, and they are still stored the same way. This is
about how the quilt *draws* what a patch chose, and about who owns the
line between two tiles.

## Context

There was no seam. Every grey line on the quilt was the page background
showing through a hole, and the hole was cut twice — once by each
neighbour, independently.

Each tile clipped itself to one of five `RAW_EDGES` polygons chosen from
its own id, whose edges sat 0.2%–1.2% inside its own box. Tile A's right
edge and tile B's left edge came from different variants, so the gap
between them was the sum of two unrelated insets: anywhere from 0.4% to
2.4% of a tile, pinching and flaring along a single boundary. A seam is a
property of the boundary between two tiles. Storing it on the tile is
what made it impossible to close.

("Seam" was already spoken for: in the block drafter it is a line sewn
*within* one block, with a budget. CONTEXT.md now carries both — **tile
seam** for the line between two tiles, plain **seam** for the drafter's.
Same word at two scales, as in real quilting.)

It also lived in world geometry, and the canvas zooms `[0.3, 6]`. The
base unit is 30–80 world px, so the inset was 0.06–0.96 world px per
side: sub-pixel and invisible looking at the whole quilt, roughly eleven
screen pixels of grey slab reading one patch. The 1.5px thread stroke
scaled with it. The effect disappeared exactly where the texture was
wanted and dominated exactly where it wasn't.

Three more faults compounded it. Every tile stroked its whole perimeter,
so each interior boundary carried two overlapping strokes at slightly
different positions — double the paint, and a darkness that varied with
how far apart the two edges happened to land. The block was drawn in a
rigid square and then clipped, so wherever an edge bowed *outward* the
fabric had already run out: sampling just inside every tile boundary,
23% of it was uncovered at the shipped wobble, 38% at a stronger one.
And the depth pass was an HTML `<div>` per tile in a parallel layer,
transform-synced on every zoom tick, carrying inset box-shadows plus two
gradients whose **angles** came from the patch-id hash — neighbouring
tiles lit from opposing directions, which cancels the illusion of a
surface outright.

The performance work found something worth recording, because it
contradicts the obvious move. Merging every same-coloured piece in the
quilt into one path cut the node count from 18,895 to 226 and the
non-scaling strokes from 5,592 to 5. The frame time did not move:
**91.7ms before, 91.7ms after**. Node count was never the bottleneck.
Worse, at 6× zoom the merged version was *twice as slow*, because a path
is only skipped when its bounding box misses the viewport and a
quilt-spanning path never misses. The merge had deleted the culling that
had been quietly doing the work.

## Decision

**1. The wobble moves to a lattice both neighbours read.** Every grid
intersection gets a hashed drift; every one-unit segment gets a hashed
bow. A tile traces those shared points and curves, walking them backwards
on the far side — a reversed quadratic is the identical curve, so the
join is exact. The quilt is watertight (measured: zero background hits at
4× zoom, against a non-zero count before), and the variation survives. It
simply moves from the tile to the seam it shares, which is also more
truthful: fabric is pushed around by its neighbours.

**2. The seam becomes ink on top, not a hole underneath.** With the tiles
watertight, each unique boundary segment is stroked once — deduplicated,
so an interior seam is no longer drawn twice — with
`vector-effect: non-scaling-stroke`. The stitch reads the same at 0.3×
and at 6×.

The line between what scales and what doesn't is deliberate and is stated
in the constants: **seam ink is screen pixels, fabric depth is world
units.** A stitch is a mark on a map and should not fatten as you
approach; a seam's puff is a physical property of the cloth and should.

**3. The block stretches onto the lattice rather than being clipped to
it.** A tile is a `k×k` field of lattice cells, each bounded by the same
four shared curves the seams are drawn from; a point in block coordinates
lands in one cell and that cell's Coons patch carries it to world space,
interpolating all four boundary curves exactly. Cells share their curves,
so the map is continuous across the tile, and the tile's outer curves
*are* its seams — the fabric arrives at the seam with nothing clipped off
and nothing left short. Measured coverage goes from 23% missed to 0%.

This is what unlocks the amplitude. The wobble could not be turned up
before because the block stopped arriving, not because it looked wrong.

A `pull` term eases the warp back toward the rigid square as you move
inward, weighted 1 along the tile's own boundary so the seam stays exact
at any setting. It decides only how much a pinwheel's centre may drift.

**4. Depth and light are properties of the seam, not of the tile.** The
bevel, the batting puff and the raking light are strokes along the
boundary: one wide stroke centred on a seam darkens both sides at once,
which is what an inner lip on each of two neighbours added up to anyway,
and three stacked widths stand in for a soft falloff without a filter.
Doming is two strokes offset toward and away from a single lamp.

Depth is constant per seam rather than proportional to tile size. The old
`s * 0.03` made a 4×4 look puffier than a 1×1, which is backwards — the
seam allowance is the same width whatever it joins.

The parallel HTML shadow layer is deleted. Intensity may vary per tile;
**direction never may**.

**5. A fold is geometry first and shading second.** One height field over
the quilt is used twice: the lattice moves along its negative gradient —
cloth drawn toward each fold, so seams curve and blocks stretch with them
and nothing tears, because it is still the same shared lattice — and the
shading is baked from the gradient of that same field. They agree because
they are one field. Geometry without shading reads as distortion;
shading without geometry reads as a smudge on the glass.

The shading is one canvas-baked image masked to the quilt's silhouette,
composited with ordinary alpha rather than a blend mode, which would
force the quilt into an isolated compositing buffer. Its resolution is a
fixed pixel budget, not the quilt's size: folds are low frequency and the
image stretches, and sized to the quilt a 779-patch board spent 341ms
baking.

**6. Weave belongs to the fabric, not to the fill.** One swatch is baked
once and three patterns point at it with different scales and angles, so
one texture is uploaded however many fabrics a quilt has. Pieces sharing
a colour share one overlay. Per-*fill* texture was considered and
rejected: it multiplies element count for a difference nobody can resolve
at quilt zoom, and a bundle slot is already the right grain of "a
fabric".

**7. Batch within a chunk, because that is what measured faster.**
Geometry is grouped by paint *and* by a small block of grid cells — few
enough paths to be cheap, small enough bounding boxes to be culled. At
three cells per chunk: 9 → 30 fps at 779 patches, 22 → 79 at 269,
80 → 233 at 71. Neither extreme wins; the whole-quilt merge and the
per-tile split are both worse than the middle.

**8. Zoom is a transform, and only a crossed detail tier is a rebuild.**
Scaling costs a browser no more than translating. What costs is
rebuilding the scene on every tick. Topstitch and weave have zoom tiers;
crossing one is a different scene, and everything else is the same scene
seen from closer up.

Pattern fills are the one paint that must be re-tiled when the scale
changes — free to pan across, and the dominant cost of every zoom (12ms
of a 41ms frame, where the bevel, the subdivision and the doming each
cost under a millisecond). The weave therefore stands down for the
duration of a zoom gesture and returns when the view settles.

**9. A repack re-cuts the fabric.** Filtering repacks the quilt, and a
tile landing in a different grid cell was cut to fit the seams of the
cell it left. It is re-warped where it lands rather than slid across;
carrying the old cut would open exactly the gaps this closes. The cloth
layers stand down while the layout is in motion and are rebuilt against
where the tiles actually landed.

**10. Block geometry has one source of truth.** The warp needs polygons,
not appended elements, so `blockPieces()` captures what the existing
twelve renderers already emit through a recording stand-in for the d3
selection. A second copy of all twelve emitting point lists would be a
second place for Pinwheel to drift, and the drift would show as a tile
that looks different on the quilt than in the block drafter.

**11. The tuned constants are a design decision and are pinned.** They
live in one exported `CLOTH` block with each value's unit stated, because
a value in *tiles* survives any base unit and a value in *screen pixels*
deliberately ignores zoom, and confusing the two is easy. A test asserts
the headline values so that changing how every quilt looks has to be
deliberate.

## Consequences

Building a layout costs about 1.65× what it did — roughly 8ms at 71
patches, 24ms at 269, 71ms at 779, against 5 / 17 / 50 before. That is
build cost, paid on layout and filter changes, not on pan or zoom. The
warp and the piecing hairlines are most of it, because they multiply the
path geometry; the folds and the weave add almost nothing.

There is a one-time texture bake at first paint. It is cached and
independent of quilt size, but it is on the critical path and could be
moved off it if a cold load ever feels slow.

Corner marks live inside the tile groups so they animate with them, which
puts them *under* the cloth layers. At the shipped bevel the widest
stroke is narrower than the mark inset, so this should not be visible; if
it ever is, the fix costs an extra layer and a re-anchoring of the marks
into world coordinates.

Chunking splits the stroke layers, so a seam junction on a chunk boundary
is composited more than once. At these alphas it is expected to be
invisible. If it shows as a faint grid, group opacity fixes it exactly,
at the cost of an offscreen buffer per layer.

The fold field is defined in grid units, so it is a property of the
layout rather than of the quilt. A new patch joining subtly re-folds the
whole cloth. Anchoring it to something stable — the instance id — would
make the folds a fixed property of *this* quilt instead, and is a small
change if that is wanted; it is left as it is because no instance has
enough churn yet for anyone to notice.

The Go icon renderer (`internal/handler/icon_blocks.go`, ADR 043) draws a
single block with no neighbours and therefore no lattice, and stays
unwarped. That is correct rather than merely tolerated: an instance icon
is a mark, not a piece of cloth, and it has no seam to reach.

Nothing here touches the database, the API, or the seamrip boundary. A
quilt drawn by an older build and one drawn by this one hold the same
data; only the pixels differ.
