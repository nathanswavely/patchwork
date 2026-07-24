# Tile appearance: one JSON column of opaque keys, frontend-owned palette registry

Status: amended by docs/adr/029 — `appearance.block` may also be an
embedded drafted-block object (validated structurally, not as a slug),
and `appearance.bundle` carries chosen fabrics. The one-JSON-column,
frontend-owned-registry, and graceful-degradation decisions stand.

Patch admins can choose their tile's palette, block, and rotation. These are
stored as a single `nodes.appearance` JSON column (`{"palette": "anthem",
"block": "pinwheel", "rotation": 90}`, NULL = unset → deterministic hash
assignment from the patch ID, the pre-feature behavior). This replaces the
`theme` column (its value migrates to `appearance.palette`) because palette +
block + rotation are one concept, and the table already uses JSON TEXT for
structured-but-never-queried data (`links`, `follower_permissions`,
`governance_config`).

The appearance object also carries an optional `icon` key — the patch's
**motif**, the small mark drawn beside its name on quilt label badges and
patch cards. The motif rides in the same column even though it renders on
the label rather than the tile: palette, block, rotation, and motif are one
"how this patch presents on the quilt" concept, chosen on one settings
screen, saved and cleared together. Unset (or unknown — e.g. a foreign
instance's custom motif in a merged multi-quilt view) falls back to the
motif's pre-feature derivation: first matching tag → the quilt mark. The
motif registry is frontend-owned like palettes and blocks, and curated —
patch-level appearance never includes uploaded images.

The backend treats palette/block/icon values as opaque slugs and validates
shape only (slug charset/length, rotation ∈ {0, 90, 180, 270}) — never
membership.
Palette and block definitions live in frontend code; a future
`patchwork.yaml` extension can add instance palettes served via
`/api/v1/instance` (already fetched per quilt in merged multi-quilt views).
Unknown keys fall back to hash assignment at render time, so foreign
instances' custom palettes and retired palettes degrade gracefully instead
of erroring.

## Considered options

- Backend-authoritative registry (Go serves all palette definitions):
  rejected — puts color data in a binary that otherwise never touches
  rendering, and bloats every tree payload for the common built-in case.
- Separate `block`/`rotation` columns beside the existing `theme` column:
  rejected — smears one concept across three columns and leaves a column
  named `theme` holding a palette, contradicting the project glossary.
