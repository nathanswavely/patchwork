import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// MapLibre parses vector tiles in a worker, and left alone it finds that
// worker by assembling a URL at runtime from `import.meta.url`. A bundler
// can only emit a file it can see, and that expression is computed, so Vite
// emitted none: in production the URL resolved against the app's own chunk,
// hit the SPA's index.html fallback, and came back as text/html. The worker
// never started, no tile was ever parsed, and the basemap was blank behind
// the markers on every map Patchwork drew.
//
// It failed silently in both directions, which is why it survived. Nothing
// in the build warns — the app still compiles and the chunk still loads —
// and the raster fallback that exists for a dead GL map did not fire,
// because the style had parsed and a frame had painted.
describe('the basemap hands MapLibre its tile worker', () => {
  const basemap = readFileSync(resolve(__dirname, '../lib/basemap.js'), 'utf8');
  const viteConfig = readFileSync(
    resolve(__dirname, '../../vite.config.js'),
    'utf8',
  );

  // `?worker&url` is what makes the worker a build input rather than a
  // guess: Vite bundles it, hashes it like every other asset, and hands
  // back the address it will actually be served at.
  it('imports the worker as a build input', () => {
    expect(basemap).toMatch(
      /import\(\s*'maplibre-gl\/dist\/maplibre-gl-worker\.mjs\?worker&url'\s*\)/,
    );
  });

  it('sets that address before any map is drawn', () => {
    expect(basemap).toMatch(/setWorkerUrl\(/);
    // Inside loadGL, which every GL map awaits — not somewhere a map could
    // race past.
    const loadGL = basemap.match(/function loadGL\(\)\s*\{[\s\S]*?\n\}/);
    expect(loadGL).not.toBeNull();
    expect(loadGL[0]).toContain('setWorkerUrl');
  });

  // MapLibre asks for a module worker first. Vite's default worker format is
  // an IIFE, which that constructor would load under module semantics.
  it('emits the worker as an ES module', () => {
    expect(viteConfig).toMatch(/worker:\s*\{[^}]*format:\s*'es'/);
  });

  // The timer that falls back to raster tiles is the safety net this bug
  // walked straight through, because `load` means "the style parsed and a
  // frame was drawn" and an empty frame counts. A parsed tile is the only
  // signal that cannot be true unless the worker is alive.
  it('proves the map alive with a parsed tile, not a painted frame', () => {
    expect(basemap).toMatch(/gl\.on\('sourcedata'/);
    expect(basemap).not.toMatch(/gl\.once\('load'/);
  });
});
