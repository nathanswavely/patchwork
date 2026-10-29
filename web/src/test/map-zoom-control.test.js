import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// One pair of zoom buttons for both canvases, beside the Quilt/Map switch at
// the canvas's bottom-right. The map used to wear Leaflet's own control in a
// corner of its own (moved once already, from under the nav rail, where no
// click reached it), and the quilt had none. The frontend suite reads source
// text, so this pins the wiring, not the pixels; the pixels were measured in
// the browser.
describe('canvas zoom controls', () => {
  const map = () => source('components/MapView.svelte');
  const quilt = () => source('components/QuiltCanvas.svelte');
  const shell = () => source('components/SocialShell.svelte');
  const store = () => source('stores/canvasZoom.svelte.js');

  it('stand beside the Quilt/Map switch, with or without a map', () => {
    expect(shell()).toMatch(/\.canvas-controls \{[^}]*right: calc\(var\(--pw-cards-pane-w/);
    expect(shell()).toMatch(/\.canvas-controls \{[^}]*bottom: 36px/);
    // The switch only exists where the map does; the zoom is always there.
    expect(shell()).toMatch(/<div class="canvas-controls">\s*\{#if mapEnabled\}\s*<div class="canvas-view"/);
    expect(shell()).toMatch(/\{#if hasCanvasZoom\(\)\}\s*<div class="canvas-zoom"/);
    expect(shell()).toContain('aria-label="Zoom in"');
    expect(shell()).toContain('aria-label="Zoom out"');
  });

  it('stop at the canvas’s limits', () => {
    expect(shell()).toContain('disabled={!canZoomIn()}');
    expect(shell()).toContain('disabled={!canZoomOut()}');
    expect(quilt()).toMatch(/reportCanvasZoom\(event\.transform\.k < ZOOM_MAX/);
    expect(map()).toMatch(/instance\.getZoom\(\) < instance\.getMaxZoom\(\)/);
  });

  it('drive whichever canvas is showing', () => {
    expect(quilt()).toMatch(/if \(!interactive\) return;\s*return attachCanvasZoom\(stepZoom\);/);
    expect(map()).toContain('attachCanvasZoom(');
    // Switching surfaces mounts the next canvas before the last one's
    // cleanup runs, so a detach must not clear its successor.
    expect(store()).toMatch(/if \(driver === d\)/);
  });

  it('zoom about the middle the reader can see, not behind the cards pane', () => {
    expect(quilt()).toMatch(/function stepZoom[\s\S]{0,400}vw \* insetRight/);
    expect(map()).toMatch(/setZoomAround\(middle/);
  });

  it('replace Leaflet’s own control, on every width', () => {
    expect(map()).toMatch(/zoomControl:\s*false/);
    expect(map()).not.toContain('L.control.zoom');
    expect(map()).not.toContain('.leaflet-control-zoom');
    // Phones pinch: the buttons are desktop chrome.
    expect(shell()).toMatch(/\.canvas-controls \{\s*display: none;/);
  });

  it('keeps the top corner under the glass bar', () => {
    expect(map()).toMatch(/\.leaflet-top\)\s*\{\s*top:\s*64px/);
  });
});
