import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { luminance, contrastRatio, textOnColor } from '../lib/quiltTheme.js';

// How the quilt says what a patch is (CONTEXT.md "Corner mark", "Name
// badge"). The motif used to be a disc in the patch's identity color, with a
// seam ring and a shadow, in the corner of every tile, and the status marks
// wore dark discs beside it: up to three discs per tile, plus the name badge.
// Now a mark is a bare glyph on the fabric, and a tile that wins a name badge
// carries its motif inside the badge instead of in its corner, so it wears
// one mark rather than two.
//
// These read source text, as every frontend test here does; what the quilt
// looks like was checked in the running app.
const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');
const quilt = read('components/QuiltCanvas.svelte');

function fn(name) {
  const start = quilt.indexOf(`function ${name}(`);
  expect(start).toBeGreaterThan(-1);
  const next = quilt.indexOf('\n  function ', start + 1);
  return quilt.slice(start, next === -1 ? undefined : next);
}

describe('a corner mark', () => {
  it('is a glyph on a soft shadow, with no disc and no outline', () => {
    const draw = fn('drawCornerMark');
    // One circle, and it is the shadow: filled with the fading gradient,
    // never a solid fill, a ring, or an offset copy.
    expect(draw.match(/append\('circle'\)/g)).toHaveLength(1);
    expect(draw).toMatch(/\.attr\('fill', `url\(#\$\{MARK_SHADOW_ID\}\)`\)/);
    expect(draw).not.toMatch(/'stroke'/);
    // A dark edge traced around each glyph was tried and read as busy.
    expect(draw).not.toMatch(/paint-order/);
  });

  it('wears the shadow only where the fabric under it needs one', () => {
    const draw = fn('drawCornerMark');
    expect(draw).toMatch(/if \(markNeedsShadow\(glyph\.getAttribute\('fill'\), fabricsUnderCorner\(entry, corner\)\)\) \{\s*g\.append\('circle'\)/);
    const decide = fn('markNeedsShadow');
    // Below the floor against the worst fabric, and only if a dark wash
    // helps: it never does for a glyph darker than the fabric.
    expect(decide).toMatch(/return bare < MARK_MIN_CONTRAST && shaded > bare;/);
    expect(quilt).toMatch(/const MARK_MIN_CONTRAST = 3;/);
  });

  it('samples the block under the corner, not the screen', () => {
    const sample = fn('fabricsUnderCorner');
    expect(sample).toMatch(/blockPieces\(data\.id, paletteForPatch\(data\.id, data\.appearance\), data\.appearance\)/);
    // The block is rotated into the tile, so the rotation is undone first.
    expect(sample).toMatch(/-getRotation\(data\.id, data\.appearance\)/);
    // Topmost piece wins: later pieces paint over earlier ones.
    expect(sample).toMatch(/for \(let p = entry\.pieces\.length - 1; p >= 0; p--\)/);
    // Cached per tile, so the role mark redrawn on a follow costs nothing.
    expect(sample).toMatch(/if \(entry\.fabrics\[corner\]\) return entry\.fabrics\[corner\];/);
  });

  it('fades the shadow to nothing at its edge', () => {
    expect(quilt).toMatch(/const MARK_SHADOW_STOPS = \[\[0, [\d.]+\], \[[\d.]+, [\d.]+\], \[1, 0\]\]/);
    expect(quilt).toMatch(/append\('radialGradient'\)\.attr\('id', MARK_SHADOW_ID\)/);
  });

  it('draws the motif in one color, not the patch identity color', () => {
    expect(quilt).not.toMatch(/identityColorForPatch/);
    expect(fn('syncMotifMark')).toMatch(/createMotifGroup\(entry\.tile\.data, MARK_GLYPH_PX, MARK_MOTIF_FILL\)/);
    expect(quilt).toMatch(/const MARK_MOTIF_FILL = '#fff'/);
  });

  it('keeps the status colors that tell star, heart and motif apart', () => {
    const role = fn('syncRoleMark');
    expect(role).toMatch(/createMyPatchStarGroup\(MARK_GLYPH_PX\)/);
    expect(role).toMatch(/createFollowedHeartGroup\(MARK_GLYPH_PX\)/);
  });
});

describe('a name badge', () => {
  it('leads with the patch motif', () => {
    const create = fn('createLabelElement');
    expect(create).toMatch(/createMotifElement\(tile\.data, 12, 'currentColor'\)/);
    expect(create).toMatch(/classList\.add\('label-motif'\)/);
    expect(quilt).toMatch(/:global\(\.patch-label \.label-motif\) \{/);
  });

  it('counts the motif in the footprint collision is tested against', () => {
    // A pill measured without its glyph is narrower than the one drawn, and
    // two badges that "clear" each other would overlap on screen.
    expect(fn('badgeType')).toMatch(/motifX:/);
    expect(fn('updateLabels')).toMatch(/const chromeX = pillX \+ motifX/);
  });

  it('takes the motif off its tile while it is on screen', () => {
    expect(fn('placeCornerMarks')).toMatch(/badged && slot === 'motif'/);
    expect(fn('updateLabels')).toMatch(/syncMotifMarks\(heldBadge\)/);
  });

  it('gives it back when badges are switched off', () => {
    const update = fn('updateLabels');
    expect(update).toMatch(/if \(!showLabels\) \{[\s\S]{0,200}labeledPatchIds = new Set\(\);\s*syncMotifMarks\(before\)/);
  });
});

describe('a motif that resolves after the quilt is built', () => {
  it('is redrawn when the tag vocabulary arrives', () => {
    // On a cold load the quilt usually builds before GET /tags answers, and
    // every tile wore the fallback quilt mark until a relayout.
    expect(quilt).toMatch(/getTagVocabulary\(\);\s*untrack\(\(\) => \{[\s\S]{0,200}syncMotifMark\(entry\)/);
    expect(fn('syncMotifMark')).toMatch(/if \(entry\.motif === key\) return;/);
  });

  it('rebuilds a badge whose motif changed under the same data', () => {
    expect(quilt).toMatch(/label\.__motif !== motif/);
  });
});

describe('the contrast arithmetic', () => {
  it('reads WCAG luminance from every hex form a bundle allows', () => {
    expect(luminance('#ffffff')).toBeCloseTo(1, 5);
    expect(luminance('#fff')).toBeCloseTo(1, 5);
    expect(luminance('#000000')).toBe(0);
    expect(luminance('#ffffff80')).toBeCloseTo(1, 5);
    expect(luminance('var(--x)')).toBeNull();
    expect(luminance('rgb(0,0,0)')).toBeNull();
  });

  it('darkens under a black wash the way a browser composites one', () => {
    // 50% black over white is #808080 in sRGB, luminance ~0.216.
    expect(luminance('#ffffff', 0.5)).toBeCloseTo(luminance('#808080'), 2);
  });

  it('gives white on black the full 21:1', () => {
    expect(contrastRatio(1, 0)).toBeCloseTo(21, 5);
    expect(contrastRatio(0, 1)).toBeCloseTo(21, 5);
  });

  it('leaves textOnColor answering as it did', () => {
    expect(textOnColor('#ffffff')).toBe('#151820');
    expect(textOnColor('#0a0a0a')).toBe('#ffffff');
    expect(textOnColor('#fff')).toBe('#ffffff');
  });
});
