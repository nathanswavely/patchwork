# The quilt icon is drafted, not uploaded

The quilt icon was the one place in Patchwork where identity meant
finding a graphic designer. ADR 014 gave admins a file picker (PNG or
JPEG, square, ≤512KB) and six hardcoded SVG blocks as fallbacks, on the
reasoning that a single bounded image is not a media pipeline. That held
until the block drafter shipped: since ADR 029 any patch admin can piece
a block on a grid with seams and fabrics and get a tile nobody else has.
The quilt itself — the thing every patch on it belongs to — was still
asking for a file upload.

We decided **the quilt icon is a drafted block, made in the same drafter
patches use**, and **the upload path is removed**.

- **Stored as a design, not an image.** `instance_settings.icon_design`
  holds `{block, bundle}`: a drafted block (grid, seams, piece colors, in
  quarter-cell units) plus 1–6 fabrics off the wall. The block goes
  through the same structural validator a patch's drafted appearance goes
  through (ADR 029) — one drafter, one set of rules. No rotation: a
  drafted block is already drawn facing the way its author wanted, unlike
  a hash-assigned tile.
- **Rendered server-side to SVG.** `GET /api/v1/instance/icon` composes
  the SVG from validated numbers and hex colors — nothing an admin typed
  reaches the document. So ADR 014's real constraint survives intact: the
  instance origin still never serves user-supplied SVG. The endpoint
  keeps its ETag and short max-age, and every consumer (switcher,
  Connected Quilts, favicon, other quilts' `<img>`) still gets a plain
  image URL.
- **The geometry lives in both languages, deliberately.**
  `internal/handler/draft_geometry.go` mirrors
  `web/src/lib/draftGeometry.js`: same chord-splitting, same face
  ordering by centroid. The drafter needs the pieces in the browser to
  let you click one; the endpoint needs them in Go to serve an image to
  clients that never run our JS. A test asserts the pieces of every
  starter tile their cell exactly, and the two renderers were checked
  piece-for-piece against each other.
- **The old defaults became starter blocks.** Pinwheel, Ohio Star, Nine
  Patch, Flying Geese, Log Cabin, and Churn Dash are no longer fixed
  templates you pick between — they are drafts you open and take apart.
  Their piece colors are materialized through the geometry code at
  startup rather than written out by hand, so a starter is honest draft
  data the drafter can reopen. An instance that has drafted nothing still
  wears one, hash-assigned from the quilt's name: stable but not chosen,
  the rule tiles already follow (ADR 004).

**This narrows ADR 014's exception rather than widening it.** That ADR
argued a single bounded image was affordable on a Pi. True, but it was
never free: a stored blob, format validation, dimension checks, an
upload endpoint, a delete endpoint, and a second way for a quilt to
have a picture. All of that is gone. Patch media still waits for ADR
007's provider, and now nothing at all in the product accepts an image
upload.

**What an existing instance sees.** No design migrates: an instance with
an uploaded icon wears an assigned starter until an admin drafts one.
Migration 044 drops the `instance_icon` table and the stale
`icon_default` key — the bytes go with it, one way. That is deliberate
rather than tidy-later: the endpoint stops serving the upload the moment
the new binary boots, so a table nobody reads is only a 512KB blob
waiting to confuse the next reader. An operator who wants the old image
keeps a backup from before the upgrade; it never traveled in a seamrip
either way.

## Considered options

- **Keep upload alongside drafting**: rejected — two ways to have an
  icon means every surface asks "which one wins," and the upload path
  keeps its whole cost for the minority case. A quilt that wants a
  photographic logo already has `branding.logo_url`.
- **Curated blocks server-side, drafting client-side**: rejected —
  porting twelve named block renderers to Go would put thirteen drawing
  routines in two languages instead of one geometry engine, and every
  new block would need writing twice.
- **Store the SVG the browser rendered**: rejected — that is a
  user-supplied SVG on the instance origin wearing a costume, exactly
  what ADR 014 refused.
- **Let the icon reuse `nodes.appearance` wholesale** (palette, motif,
  rotation): rejected — a quilt is not a patch, and motif/palette are
  tile concepts. The icon takes only what a block needs.

## Consequences

- Drafting the icon is an admin's own work, so the drafter's seam budget
  (24) and grid range (1×1–10×10) now bound what a quilt's identity can
  be. That is the same expressive ceiling every patch lives with.
- The SVG is vector, so it scales to whatever size a consumer asks for;
  the old 64–1024 px advice is gone with the uploader.
- A quilt's icon is portable as data — it survives an export as a short
  JSON value, though instance identity still does not travel in a
  seamrip (ADR 002, ADR 014).
