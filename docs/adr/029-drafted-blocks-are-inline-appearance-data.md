# Drafted blocks are inline appearance data, not entities

Patch admins can draft their own block in the block drafter (docs/adr/004
covers curated appearance; CONTEXT.md "Tile appearance" carries the
drafting vocabulary). A drafted block is stored inline in the existing
`nodes.appearance` JSON column: `block` is either a curated slug (as
before) or an embedded draft object — grid size, seam list, piece color
map, bundle. There is no blocks table, no ownership, no reference to
follow.

The draft grammar is the actual grammar of pieced quilting, and the
constraints are structural, not rule-list: a square grid (1×1–10×10) is
the skeleton; seams connect wall anchors only (corners + fixed subdivision
points; anchor density drops to midpoints-only above 5×5, since finer
grids get expressiveness from density instead); a seam may span cells but
acts locally — every piece is a region of exactly one cell, so edits never
re-split geometry elsewhere and rendering needs no global arrangement
engine; seams are budgeted (24 — every curated block needs at most 8);
pieces are colored by bundle slot (up to 6 fabrics chosen from the one
curated fabric wall), never by raw color value, so designs stay
recolorable, slot one remains the identity color, and the quilt keeps
reading as one quilt. There is no image upload, no text, and no free color
picker — a brand that wants its logo drafts it out of seams like everyone
else.

Inline storage keeps every existing boundary intact for free: drafts
travel in a seamrip with their patch (no portability-boundary change),
render on foreign quilts via the appearance already carried in
remote-follow snapshots and merged views (no federation work), and degrade
on old clients exactly per docs/adr/004 (an unrecognized block value falls
back to hash assignment). The cost is honest backend validation: the
"shape only, opaque slugs" contract of docs/adr/004 is amended — a draft
object is validated structurally (grid bounds, seam budget, anchors legal
for the grid, slot indices in range), though never aesthetically.

Abuse is a people problem, not a tool problem: drafts go live on save
with no pre-moderation, the existing report path covers appearance (it is
part of the patch), and the moderation response is one instance-admin
action — reset the patch's appearance to hash-assigned ("the quilt
decides now"). A geometric hate-symbol blocklist was rejected: it is an
arms race, it false-positives on innocent traditional blocks (whirling-log
patterns predate the swastika and were destroyed by it), and it
substitutes automation for stewardship.

## Considered options

- A `blocks` table with drafts as first-class shareable entities: rejected
  for now — it drags in ownership, lifecycle, dangling references, seamrip
  and federation changes, all to serve a pattern-library feature nobody
  has asked for. Inline storage doesn't foreclose it: a future pattern
  book is a named copy of an inline draft, and "use this pattern" copies
  it into your appearance — like copying a pattern from a quilting book,
  no live reference.
- Extending the opaque-slug contract with server-registered custom slugs:
  rejected — a draft referenced by slug still has to live somewhere, and
  that somewhere is the blocks table above.
