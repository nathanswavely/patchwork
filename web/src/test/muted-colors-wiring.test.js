import { describe, it, expect, afterEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { paletteForPatch, identityColorForPatch, colorForTag, ghostPalette } from '../lib/quiltTheme.js';
import { getColorMode, setColorMode } from '../stores/colors.svelte.js';
import { CHROMA_CAP, SLOT_LIGHTNESS, hexToOklch } from '../lib/mutedColors.js';

// docs/adr/112. mutedColors.js proves the transform; this proves it is
// actually plugged in, and that the one place it must NOT be plugged in
// stays unplugged.

const ID = '0192f0000000dead';

afterEach(() => setColorMode('default'));

describe('the register reaches everything that stands for a patch', () => {
  it('starts in default on a browser that has never chosen', () => {
    expect(getColorMode()).toBe('default');
  });

  it('changes a tile, and changes it back', () => {
    const loud = paletteForPatch(ID, null);
    setColorMode('muted');
    const quiet = paletteForPatch(ID, null);
    expect(quiet.primary).not.toBe(loud.primary);
    expect(hexToOklch(quiet.primary).C).toBeLessThanOrEqual(CHROMA_CAP + 0.005);
    setColorMode('default');
    expect(paletteForPatch(ID, null).primary).toBe(loud.primary);
  });

  it('carries the tile and the identity color to the same answer', () => {
    // The quilt-to-map continuity ADR 078 kept identity color for. If these
    // two ever disagree, a patch is one color on the quilt and another on
    // its map marker and card.
    setColorMode('muted');
    expect(identityColorForPatch({ id: ID, appearance: null }))
      .toBe(paletteForPatch(ID, null).primary);
  });

  it('mutes tag chips, which are most of what sits under the quilt', () => {
    const loud = colorForTag('printmaking');
    setColorMode('muted');
    const quiet = colorForTag('printmaking');
    expect(quiet).not.toBe(loud);
    expect(hexToOklch(quiet).C).toBeLessThanOrEqual(CHROMA_CAP + 0.005);
  });

  it('mutes the filler tiles, so the quilt has no loud edge', () => {
    setColorMode('muted');
    const g = ghostPalette(3);
    for (const hex of [g.primary, g.secondary, g.bg]) {
      expect(hexToOklch(hex).C).toBeLessThanOrEqual(CHROMA_CAP + 0.005);
    }
  });

  it('leaves the empty-tag color alone', () => {
    setColorMode('muted');
    expect(colorForTag('')).toBe('#7a7870');
  });
});

describe('muted keeps a block the shape the patch drafted', () => {
  it('returns as many fabrics as were chosen, never more', () => {
    // Muted may take color away; it may not add structure. A one-fabric
    // bundle draws one flat fabric in both registers.
    const one = ['#DA0956'];
    setColorMode('default');
    expect(paletteForPatch(ID, { bundle: one }).slots).toHaveLength(1);
    setColorMode('muted');
    expect(paletteForPatch(ID, { bundle: one }).slots).toHaveLength(1);
  });

  it('keeps six drafted fabrics distinguishable', () => {
    const six = ['#DA0956', '#FCFD1B', '#0A0A0A', '#1493CC', '#88DE16', '#E3480B'];
    setColorMode('muted');
    const slots = paletteForPatch(ID, { bundle: six }).slots;
    expect(slots).toHaveLength(6);
    expect(new Set(slots).size).toBe(6);
  });

  it('draws the ground darker than the identity color in front of it', () => {
    setColorMode('muted');
    const p = paletteForPatch(ID, null);
    expect(hexToOklch(p.bg).L).toBeLessThan(hexToOklch(p.primary).L);
    expect(hexToOklch(p.bg).L).toBeCloseTo(SLOT_LIGHTNESS[2], 1);
  });
});

describe('the block drafter is exempt, and stays exempt', () => {
  it('gives raw callers what the patch chose, in either register', () => {
    const chosen = paletteForPatch(ID, null, { raw: true });
    setColorMode('muted');
    expect(paletteForPatch(ID, null, { raw: true })).toEqual(chosen);
  });

  const drafterFiles = [
    'src/pages/PatchSettingsAppearance.svelte',
    'src/pages/PatchForm.svelte',
  ];

  // A fabric picker that shows muted fabric lies to the admin using it: they
  // pick Hi-Vis, are shown something else, and save it. This is the kind of
  // exemption a later contributor "fixes" for consistency, so it is pinned.
  for (const f of drafterFiles) {
    it(`${f} seeds its picker raw`, () => {
      const src = readFileSync(resolve(__dirname, '..', f.replace('src/', '')), 'utf8');
      expect(src).toMatch(/paletteForPatch\([^)]*\{ raw: true \}\)/);
    });
  }
});

describe('the hover dim is a dim, on a dwell', () => {
  const src = readFileSync(resolve(__dirname, '../components/QuiltCanvas.svelte'), 'utf8');

  it('scrims the other tiles rather than darkening the hovered one', () => {
    // The inversion of what shipped before. If this comes back, hovering
    // marks the tile you are pointing at as the odd one out.
    expect(src).not.toMatch(/select\('\.overlay'\)\.attr\('fill', 'var\(--color-overlay-hover\)'\)/);
    expect(src).toMatch(/selectAll\('\.overlay'\)\.attr\('fill', 'var\(--color-quilt-dim\)'\)/);
  });

  it('moves fill on existing overlays rather than recolouring fabric', () => {
    // A recolour invalidates every paint batch (docs/adr/066), on every
    // pointer move. The overlay rects already exist on every tile.
    expect(src).toMatch(/function paintDim/);
    expect(src).not.toMatch(/paintFabric\([^)]*\)\s*;?\s*\/\/ hover/);
  });

  it('engages on a dwell and releases on a delay', () => {
    expect(src).toMatch(/DIM_DWELL_MS\s*=\s*\d+/);
    expect(src).toMatch(/DIM_RELEASE_MS\s*=\s*\d+/);
    expect(src).toMatch(/setTimeout\(clearDim, DIM_RELEASE_MS\)/);
  });

  it('moves the lit hole instead of restarting when crossing tiles', () => {
    // Without this a pan across a dense quilt strobes: dim, clear, dim,
    // clear, once per tile boundary.
    expect(src).toMatch(/if \(dimLitPatchId !== null\)/);
  });

  it('is offered to the name badge too, which is part of its patch', () => {
    expect(src).toMatch(/engageDim\(tileData\.id\)/);
  });
});

describe('a register change rebuilds the quilt', () => {
  const src = readFileSync(resolve(__dirname, '../components/QuiltCanvas.svelte'), 'utf8');

  it('watches the store and rebuilds, because paint is batched on the way in', () => {
    expect(src).toMatch(/import \{ getColorMode \}/);
    expect(src).toMatch(/prevColorMode/);
    expect(src).toMatch(/layoutBuilt = false;\s*\n\s*tileMap = new Map\(\);\s*\n\s*buildLayout\(\);/);
  });
});
