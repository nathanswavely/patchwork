import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Pointing at a patch previews it (docs/adr/078, decision 7). The rule is
// about input, not about surfaces: where there is a pointer, pointing
// previews and clicking opens; where there is none there is a single
// gesture, so the first tap previews into the docked card. The quilt and
// the map therefore behave the same as each other on the same hardware,
// which is the property these guard.
const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');
const home = read('pages/SocialHome.svelte');
const map = read('components/MapView.svelte');
const quilt = read('components/QuiltCanvas.svelte');

describe('previewing a patch', () => {
  it('is one fact about the page, not one per surface', () => {
    // A previewed id per surface would let the map and the quilt disagree
    // about what is being pointed at, and the card would have to pick.
    expect(home).toMatch(/let previewing = \$state\(null\)/);
    expect(home).toMatch(/function preview\(patch, fromMap = false\)/);
  });

  it('is reported by both surfaces through the same door', () => {
    expect(map).toMatch(/onPatchHover/);
    expect(quilt).toMatch(/onPatchHover/);
    // Both funnel into the one preview(), rather than each keeping its own.
    expect(home.match(/onPatchHover=\{\(\w+\) => hasPointer && preview\(\w+, true\)\}/g) || [])
      .toHaveLength(2);
  });

  it('never fires where there is no pointer', () => {
    // On a touch screen the single gesture belongs to the docked card. A
    // synthesised mouseover on tap must not preview as well, or a tap would
    // do two things at once.
    expect(home).toMatch(/hasPointer && preview\(/);
    expect(home).toMatch(/hasPointer = \$state\(window\.matchMedia/);
  });

  it('brings a card to the reader only when the pointer is elsewhere', () => {
    // Scrolling the list under a pointer that is *on* the list would move
    // the card out from under it — the hover would chase itself.
    expect(home).toMatch(/if \(!fromMap \|\| !patch\) return;/);
    expect(home).toMatch(/scrollIntoView/);
  });

  it('emphasises the marker without rebuilding the layer', () => {
    // The pointer crosses cards far faster than a marker layer can be torn
    // down and stitched again.
    expect(map).toMatch(/markerById/);
    expect(map).toMatch(/classList\.add\('is-previewing'\)/);
    expect(map).not.toMatch(/hoveredId[\s\S]{0,200}updateMarkers\(\)/);
  });

  it('emphasises with transforms, so nothing reflows', () => {
    // A card that changes size on hover shifts the list; a pin that changes
    // its footprint moves off its own coordinate.
    expect(home).toMatch(/\.patch-card\.previewing \{[^}]*border-color/);
    expect(home).not.toMatch(/\.patch-card\.previewing \{[^}]*(width|height|padding|margin):/);
    // And the scale goes on the child, never the marker: Leaflet positions
    // the marker with a transform, and CSS composes `scale` *with* it rather
    // than beside it — scaling the marker multiplies its translation and
    // throws the pin across the map.
    expect(map).toMatch(/\.patch-marker\.is-previewing svg\) \{[^}]*scale:/);
    expect(map).not.toMatch(/\.patch-marker\.is-previewing\) \{[^}]*scale:/);
  });
});
