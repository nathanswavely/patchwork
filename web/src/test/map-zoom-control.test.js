import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// On desktop the nav rail floats over the map's top-left corner, open or
// collapsed, and Leaflet's default zoom control sat straight under it:
// elementFromPoint on the +/- buttons returned the rail, so no click ever
// reached them. The control lives bottom-left on desktop — the one corner
// nothing else of ours claims — and keeps the top-left on mobile, where the
// rail is the bottom bar and the filter FAB stacks above it. The frontend
// suite reads source text, so this pins the placement, not the pixels;
// the pixels were measured in the browser when it moved.
describe('map zoom control placement', () => {
  const map = () => source('components/MapView.svelte');
  const shell = () => source('components/SocialShell.svelte');

  it('takes the control away from the default corner and places it by breakpoint', () => {
    expect(map()).toMatch(/zoomControl:\s*false/);
    expect(map()).toContain("window.matchMedia('(max-width: 768px)')");
    expect(map()).toMatch(/isNarrow \? 'topleft' : 'bottomleft'/);
    // A viewport that crosses the breakpoint (a tablet rotating) moves it.
    expect(map()).toContain("narrow.addEventListener('change', onBreakpoint)");
    expect(map()).toContain("narrow.removeEventListener('change', onBreakpoint)");
  });

  it('stays left of the filter chips with the rail collapsed', () => {
    // The control's right edge: its left margin plus Leaflet's 30px button
    // column with our 1px border each side.
    const margin = map().match(/\.leaflet-bottom \.leaflet-control-zoom\)\s*\{[^}]*margin-left:\s*(\d+)px/);
    expect(margin, 'zoom control left margin').not.toBeNull();
    const rightEdge = Number(margin[1]) + 32;
    const chips = shell().match(/\.quilt-chips\.rail-collapsed\s*\{[^}]*left:\s*(\d+)px/);
    expect(chips, 'collapsed chips offset').not.toBeNull();
    expect(Number(chips[1])).toBeGreaterThan(rightEdge);
  });

  it('keeps the top corner under the glass bar for the mobile placement', () => {
    expect(map()).toMatch(/\.leaflet-top\)\s*\{\s*top:\s*64px/);
  });
});
