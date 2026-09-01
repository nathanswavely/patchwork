import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { RASTER_FALLBACK_CLASS } from '../lib/basemap.js';

// The raster fallback and the surface it lands on each apply a CSS filter,
// and CSS filters on nested elements compose: the layer's runs, then the
// pane's runs over the result. MapView's dark tint is written for tiles that
// arrive dark (the vector styles); the fallback's dark recipe *makes* dark by
// inverting light OSM tiles. Inverting and then darkening produced a
// near-black rectangle with markers floating on it — a map that reads as
// broken. The two must never both apply.
describe('the raster basemap fallback is filtered exactly once', () => {
  const basemap = readFileSync(resolve(__dirname, '../lib/basemap.js'), 'utf8');
  const mapView = readFileSync(
    resolve(__dirname, '../components/MapView.svelte'),
    'utf8',
  );

  it('marks its container when the fallback engages', () => {
    expect(basemap).toMatch(
      /map\.getContainer\(\)\?\.classList\.add\(RASTER_FALLBACK_CLASS\)/,
    );
  });

  // The class is a constant on one side and a literal in a stylesheet on the
  // other — nothing but this test makes them agree, and a silent rename would
  // restore the black map.
  it('names the same class the stylesheet opts out on', () => {
    expect(RASTER_FALLBACK_CLASS).toBe('basemap-raster');
    expect(mapView).toContain(`.leaflet-container.${RASTER_FALLBACK_CLASS} .leaflet-tile-pane`);
  });

  it('stands the pane tint down for that container', () => {
    const rule = mapView.match(
      /\.leaflet-container\.basemap-raster \.leaflet-tile-pane\)\s*\{([^}]*)\}/,
    );
    expect(rule).not.toBeNull();
    expect(rule[1]).toMatch(/filter:\s*none/);
  });

  // Guards the composition hazard itself rather than the fix: if the pane
  // ever stops tinting, or the fallback stops inverting, the opt-out above is
  // no longer load-bearing and this test should be revisited deliberately.
  it('still has the two filters that must not meet', () => {
    expect(basemap).toMatch(/dark:\s*'invert\(1\)/);
    expect(mapView).toMatch(
      /\[data-theme="dark"\][\s\S]{0,160}?filter:\s*brightness\(0\.82\)/,
    );
  });
});
