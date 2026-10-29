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
    expect(map()).toContain('window.matchMedia(media.handheld)');
    expect(map()).toMatch(/isNarrow \? 'topleft' : 'bottomleft'/);
    // A viewport that crosses the breakpoint (a tablet rotating) moves it.
    expect(map()).toContain("narrow.addEventListener('change', onBreakpoint)");
    expect(map()).toContain("narrow.removeEventListener('change', onBreakpoint)");
  });

  it('has the bottom-left corner to itself on desktop', () => {
    // The chips moved to the canvas's top edge and the Quilt/Map switch to
    // its bottom-right, so nothing of ours shares the control's corner.
    expect(shell()).toMatch(/\.quilt-chips \{[^}]*top: 68px;/);
    expect(shell()).not.toMatch(/\.quilt-chips \{[^}]*bottom:/);
    expect(shell()).toMatch(/\.canvas-view \{[^}]*right: calc\(var\(--pw-cards-pane-w/);
    expect(shell()).not.toMatch(/\.canvas-view \{[^}]*left:/);
  });

  it('keeps the top corner under the glass bar for the mobile placement', () => {
    expect(map()).toMatch(/\.leaflet-top\)\s*\{\s*top:\s*64px/);
  });
});
