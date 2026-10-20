import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import L from 'leaflet';

// The GL basemap gets a window to deliver its first tile before Patchwork
// gives up on it and draws raster tiles instead. That window used to be a
// setTimeout with a document.hidden escape, which counted time the map was
// never given a chance to use: MapLibre asks for tiles from inside animation
// frames, and a webview that is off screen while reporting itself visible
// draws none. The clock ran out with nothing drawn and a working map fell
// back. The window is now counted in frames.

const glMap = {
  handlers: {},
  on(type, fn) {
    (this.handlers[type] ||= []).push(fn);
  },
  off(type, fn) {
    this.handlers[type] = (this.handlers[type] || []).filter((h) => h !== fn);
  },
  fire(type, e) {
    for (const h of this.handlers[type] || []) h(e);
  },
  getCanvas: () => document.createElement('canvas'),
  setStyle() {},
};

vi.mock('maplibre-gl', () => ({ setWorkerUrl() {} }));
vi.mock('maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url', () => ({ default: 'worker.js' }));
vi.mock('maplibre-gl/dist/maplibre-gl.css', () => ({}));
vi.mock('@maplibre/maplibre-gl-leaflet', async () => {
  const { default: Leaflet } = await import('leaflet');
  Leaflet.maplibreGL = () => {
    const layer = new Leaflet.Layer();
    layer.onAdd = () => {};
    layer.onRemove = () => {};
    layer.getMaplibreMap = () => glMap;
    return layer;
  };
  return {};
});

describe('the GL basemap load window counts frames, not the clock', () => {
  let frames;
  let now;
  let map;

  // Hands out animation frames only when the test says so.
  function drawFrames(count, gapMs) {
    for (let i = 0; i < count; i++) {
      now += gapMs;
      const due = frames;
      frames = [];
      for (const cb of due) cb(now);
    }
  }

  beforeEach(async () => {
    frames = [];
    now = 0;
    glMap.handlers = {};
    // The clock is faked from the start, so a window measured by the clock
    // would run out under advanceTimersByTime below.
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "Date"] });
    vi.stubGlobal('requestAnimationFrame', (cb) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {
      frames = [];
    });
    // A canvas that grants a context, so the GL path is taken.
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ getExtension: () => null });
    const el = document.createElement('div');
    document.body.appendChild(el);
    map = L.map(el).setView([40.0379, -76.3055], 12);
    const { addBasemap } = await import('../lib/basemap.js');
    await addBasemap(map, 'light');
  });

  afterEach(() => {
    map.remove();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  const fellBack = () => map.getContainer().classList.contains('basemap-raster');

  it('waits as long as no frame is drawn', () => {
    vi.advanceTimersByTime(120000);
    expect(fellBack()).toBe(false);
  });

  it('falls back after twelve seconds of drawing with no tile', () => {
    drawFrames(60 * 11, 1000 / 60);
    expect(fellBack()).toBe(false);
    drawFrames(60 * 2, 1000 / 60);
    expect(fellBack()).toBe(true);
  });

  // A throttled page still draws, slowly. Each frame is credited a bounded
  // amount, so a frame a second is not twelve frames' worth of chances.
  it('credits a slow frame only a bounded amount', () => {
    drawFrames(30, 1000);
    expect(fellBack()).toBe(false);
  });

  it('keeps the GL map once a tile arrives', () => {
    drawFrames(10, 1000 / 60);
    glMap.fire('sourcedata', { sourceId: 'openmaptiles' }); // metadata only
    drawFrames(10, 1000 / 60);
    glMap.fire('sourcedata', { sourceId: 'openmaptiles', tile: {} });
    drawFrames(60 * 20, 1000 / 60);
    expect(fellBack()).toBe(false);
  });
});

describe('the raster fallback stays inside what OSM serves', () => {
  it('stretches its deepest tiles rather than asking past them', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    vi.resetModules();
    const { addBasemap, BASEMAP_MAX_ZOOM } = await import('../lib/basemap.js');
    const el = document.createElement('div');
    document.body.appendChild(el);
    const map = L.map(el).setView([40.0379, -76.3055], 12);
    await addBasemap(map, 'light');
    const tiles = [];
    map.eachLayer((l) => l instanceof L.TileLayer && tiles.push(l));
    expect(tiles).toHaveLength(1);
    expect(tiles[0].options.maxZoom).toBe(BASEMAP_MAX_ZOOM);
    expect(tiles[0].options.maxNativeZoom).toBe(19);
    map.remove();
    vi.restoreAllMocks();
  });
});
