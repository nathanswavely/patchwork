import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// docs/adr/112. Hovering a card in the list beside the quilt dims every tile
// but that patch's, using the same mechanism a tile's own pointerenter
// engages. Two triggers, one behaviour — written twice they would drift into
// two different ideas of what "pointing at this one" looks like.

const canvas = readFileSync(
  resolve(__dirname, '../components/QuiltCanvas.svelte'),
  'utf8',
);
const home = readFileSync(resolve(__dirname, '../pages/SocialHome.svelte'), 'utf8');

describe('the card list can point at a tile', () => {
  it('takes the patch as a prop rather than reaching into the canvas', () => {
    expect(canvas).toMatch(/focusPatchId = null,/);
    expect(home).toMatch(/focusPatchId=\{hoveredCardId\}/);
  });

  it('reuses the hover dim instead of a second effect of its own', () => {
    // If this ever grows its own paint path, the card and the tile will
    // disagree about what focus looks like, and one of them will drift.
    const effect = canvas.match(/const id = focusPatchId;[\s\S]*?\n  \}\);/);
    expect(effect, 'focusPatchId effect not found').toBeTruthy();
    expect(effect[0]).toMatch(/engageDim\(id\)/);
    expect(effect[0]).toMatch(/releaseDim\(\)/);
    expect(effect[0]).not.toMatch(/paintDim\(/);
  });

  it('dims nothing when the patch is scrolled out of view', () => {
    // Dimming every visible tile for one the reader cannot see leaves the
    // whole quilt washed with nothing lit, which reads as a bug.
    const effect = canvas.match(/const id = focusPatchId;[\s\S]*?\n  \}\);/)[0];
    expect(effect).toMatch(/computeInView\(\)/);
    expect(effect).toMatch(/!inView\.includes\(id\)/);
  });

  it('releases when the pointer leaves the card', () => {
    const effect = canvas.match(/const id = focusPatchId;[\s\S]*?\n  \}\);/)[0];
    expect(effect).toMatch(/if \(!id\) \{\s*\n\s*releaseDim\(\);/);
  });

  it('does nothing on a static hero, which has no pointer to answer', () => {
    const effect = canvas.match(/const id = focusPatchId;[\s\S]*?\n  \}\);/)[0];
    expect(effect).toMatch(/if \(!interactive\) return;/);
  });
});

describe('the card sets it, and only the card', () => {
  it('sets and clears the id on the card, behind the pointer check', () => {
    expect(home).toMatch(/onmouseenter=\{\(\) => \{ if \(hasPointer\) \{ preview\(patch\); hoveredCardId = patch\.id; \} \}\}/);
    expect(home).toMatch(/onmouseleave=\{\(\) => \{ if \(hasPointer\) \{ preview\(null\); hoveredCardId = null; \} \}\}/);
  });

  it('does not feed the shared preview id back into the canvas', () => {
    // `previewing` is set by BOTH surfaces (docs/adr/078), so passing it as
    // focusPatchId would have a hovered *tile* ask the canvas to focus the
    // tile already under the pointer — a loop, and a different meaning.
    expect(home).not.toMatch(/focusPatchId=\{previewing\}/);
    expect(home).toMatch(/let hoveredCardId = \$state\(null\);/);
  });
});
