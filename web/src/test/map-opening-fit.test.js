import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import L from 'leaflet';
import { fitPadding, hasRoomToFit, FIT_PADDING_PX } from '../lib/mapFit.js';
import { BASEMAP_MAX_ZOOM } from '../lib/basemap.js';

// A map that mounts before its container has a width (a hidden pane, a
// collapsed layout, a webview not yet on screen) used to spend its opening
// fit there. Leaflet does not refuse a fit with no room: the size left after
// padding goes negative, the zoom it solves for is NaN, getScaleZoom turns
// that into Infinity, and the clamp lands it on the ceiling. Showing the map
// later kept that view, so it opened at street level over empty ground with
// every marker off screen, and the GL basemap under it gave up and fell back
// to raster tiles. OSM serves nothing at the ceiling, so the fallback's own
// tiles came back 400 as well.
describe('the opening fit waits for a map with room to frame', () => {
  function mapOfSize(width, height) {
    const el = document.createElement('div');
    Object.defineProperty(el, 'clientWidth', { value: width });
    Object.defineProperty(el, 'clientHeight', { value: height });
    document.body.appendChild(el);
    return L.map(el, { maxZoom: BASEMAP_MAX_ZOOM }).setView([40.0379, -76.3055], 12);
  }
  const lancaster = L.latLngBounds([
    [40.02, -76.33],
    [40.06, -76.28],
  ]);

  // The hazard itself, run through real Leaflet so a version that starts
  // refusing degenerate fits shows up here rather than staying guarded
  // against forever.
  it('a fit with no width pins the zoom at the ceiling', () => {
    const map = mapOfSize(0, 0);
    map.fitBounds(lancaster, { ...fitPadding(0), animate: false });
    expect(map.getZoom()).toBe(BASEMAP_MAX_ZOOM);
    map.remove();
  });

  it('a fit with room frames the bounds', () => {
    const map = mapOfSize(1400, 900);
    map.fitBounds(lancaster, { ...fitPadding(1400), animate: false });
    expect(map.getZoom()).toBeLessThan(16);
    expect(map.getZoom()).toBeGreaterThan(10);
    map.remove();
  });

  it('reports no room until the padding leaves some', () => {
    const edge = FIT_PADDING_PX * 2;
    expect(hasRoomToFit({ x: 0, y: 0 }, fitPadding(0))).toBe(false);
    expect(hasRoomToFit({ x: 0, y: 400 }, fitPadding(0))).toBe(false);
    expect(hasRoomToFit({ x: edge, y: 900 }, fitPadding(edge))).toBe(false);
    expect(hasRoomToFit({ x: edge + 1, y: 900 }, fitPadding(edge + 1))).toBe(true);
    expect(hasRoomToFit({ x: 1400, y: 900 }, fitPadding(1400))).toBe(true);
  });

  // The floating cards pane takes its share of the width before the
  // markers do, so a desktop map narrower than that share has no room.
  it('counts the width a floating pane covers', () => {
    expect(fitPadding(1000, 0.4).paddingBottomRight).toEqual([FIT_PADDING_PX + 400, FIT_PADDING_PX]);
    expect(hasRoomToFit({ x: 80, y: 900 }, fitPadding(80, 0.4))).toBe(false);
  });

  describe('MapView', () => {
    const view = readFileSync(resolve(__dirname, '../components/MapView.svelte'), 'utf8');

    it('gates the opening fit on room to frame', () => {
      expect(view).toMatch(
        /if \(!hasFit && placed\.length > 0 && hasRoomToFit\(map\.getSize\(\), openingPadding\(\)\)\)/,
      );
    });

    // The fit has to come back when the room arrives, or a map that mounted
    // hidden never frames its markers at all.
    it('retries the fit when the container is resized', () => {
      const ro = view.match(/new ResizeObserver\(\(\) => \{([\s\S]*?)\}\);/);
      expect(ro).not.toBeNull();
      expect(ro[1]).toContain('map.invalidateSize()');
      expect(ro[1]).toMatch(/if \(!hasFit\) updateMarkers\(\)/);
    });

    it('fits with the same padding it measured room against', () => {
      const showAll = view.match(/function showAll\([\s\S]*?\n {2}\}/);
      expect(showAll).not.toBeNull();
      expect(showAll[0]).toContain('...openingPadding()');
    });
  });
});
