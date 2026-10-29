import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

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
