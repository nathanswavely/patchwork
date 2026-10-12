# The design system is Liquid Textile, vendored

Date: 2026-10-11. Status: **accepted**, partly implemented (steps 1 and 2
of the migration plan). Applies docs/adr/077 to a stylesheet; keeps
docs/adr/038's spacing tokens and gives its reserved `[data-density]` a
home.

## Context

Patchwork carries its own design system: the tokens and global classes in
`app.css`, a button, chip, pill, banner, empty state, menu, tab row,
dialog and switch rewritten inside the components that use them, and
`lib/textile.css`, a hand-copied subset of Liquid Textile (the quilt's
canvas ground, the vellum name badges, the resin filter chips).

Liquid Textile was extracted from Patchwork to grow on its own, and it
has since been audited against Patchwork and brought up to what
Patchwork needs: the same tokens under `--lt-` names, the components
Patchwork hand-rolls, a responsive and density contract, framework-free
behaviours, and every color pair measured. The copy in `textile.css` had
already drifted from it. Two copies of a design system drift; one does
not.

## Decision

**Liquid Textile is Patchwork's design system.** Patchwork's tokens,
global classes and generic components come from it, and Patchwork's
Svelte components become thin wrappers over its classes and behaviours.
What stays in Patchwork is what only Patchwork has: the quilt, the map,
governance, the shells, and the vocabulary.

**It is vendored, not installed.** Liquid Textile is private, and a
dependency every fork inherits cannot require a key (docs/adr/077). The
files live in `web/src/vendor/liquid-textile/` with its LICENSE (MIT,
inside AGPL) and a README naming the version and commit, and
`scripts/sync-lt.sh <path-to-liquid-textile>` is the only way they
change: change Liquid Textile, re-run the script, review the diff. The
folder is never edited in place, which is how `textile.css` drifted.
When Liquid Textile is public, the folder becomes a package and the
imports in `web/src/main.js` are the lines that move.

**Patchwork's tokens read Liquid Textile's.** Every `--color-*`, the
`--pw-*` spacing and measures, `--radius`, the fonts and the `--btn-*`
sizes are aliases (`--color-text: var(--lt-color-text)`), so the files
that read them keep working and the values live in one place. Tokens
that describe Patchwork's own things stay defined in `app.css`: ink on
fabric, the quilt's name badges and hover dim, the canvas chrome, and
the tab bar's footprint.

**The light theme is warm white.** Liquid Textile's light theme moved
from cream (`#f4f0e8`) to a barely warm white (`#f6f5f2`); the cream read
as old rather than as cotton. Patchwork takes it, and its own light
tokens (the quilt's hover dim, the name badges) move with it. Every
documented contrast pair holds or improves; muted text on the page goes
from 5.1:1 to 5.5:1.

**Three values stay Patchwork's for now.** Where the two systems
differed without a recorded reason (the overlay hover, the shadow, the
dark scrim), Patchwork sets Liquid Textile's token to its own value, so
Liquid Textile's components agree with Patchwork's while the choice is
open.

**The rest of the migration follows Liquid Textile's plan**
(`docs/patchwork-migration.md` in that repository): the global class
rename, the leaf components, the responsive contract, the behaviours
behind one wrapper module (`web/src/lib/lt.js`), the theme store, and
deleting what is left. Each step ships alone and keeps the suites green.

## Consequences

- `lib/textile.css` is gone. Its classes and tokens are Liquid Textile's
  under the same names, so the e2e specs that assert `lt-fill-canvas` and
  `lt-texture-grain` hold.
- The resin filter chips take Liquid Textile's frosted finish: a tag's
  color tints the chip at 22% with body-colored text, where the old
  finish blended it in at 35%. Active chips are unchanged; they set their
  color inline.
- Tokens Patchwork read but never defined are now theme tokens:
  `--color-danger` (15 reads, most with a fixed fallback that ignored the
  dark theme) is `--color-error`; `--font-mono`, `--color-warning-bg`
  exist; `--border`, `--radius-sm`, `--radius-md`, `--font-size-sm`,
  `--space-lg` and the `*-subtle`/`*-alt`/`*-hover` one-offs read the
  token they meant. Two borders, an error color and one rounded corner
  appear where an undefined variable had silently dropped them.
- The shipped stylesheet grows by about 9 KB gzipped (52.5 to 61.8 KB)
  until the later steps delete Patchwork's copies of the same components.
- A Liquid Textile improvement reaches Patchwork by re-running one script.
- docs/adr/038 reserved `[data-density]` on `<html>`; Liquid Textile ships
  it (comfortable by default, `data-density="compact"` per page or box).
  Patchwork need not offer it to readers yet.
