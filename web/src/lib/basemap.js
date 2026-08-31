// The basemap under every Patchwork map (docs/adr/077).
//
// OpenFreeMap: no API key, no rate limit, and self-hostable by anyone who
// outgrows the public server — which is what a platform meant to be
// seamripped needs. CARTO's Positron and Dark Matter were keyless too,
// until CARTO began stamping "API KEY REQUIRED" across anonymous tiles;
// these styles are the same cartography without the key.
//
// The tiles are vector, so the layer is a MapLibre GL canvas sitting in
// Leaflet's tile pane. Everything above it — markers, popups, fitBounds —
// stays ordinary Leaflet.
import L from 'leaflet';

const STYLE_URLS = {
  light: 'https://tiles.openfreemap.org/styles/positron',
  dark: 'https://tiles.openfreemap.org/styles/dark',
};

const ATTRIBUTION =
  '&copy; <a href="https://openfreemap.org/">OpenFreeMap</a> ' +
  '&copy; <a href="https://www.openmaptiles.org/">OpenMapTiles</a> ' +
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';

// Vector tiles need WebGL, and a browser only grants so many contexts. Where
// one can't be had — old hardware, a driver blocklist, a page that has spent
// its budget — a blank map is the worst answer, so fall back to plain OSM
// raster and approximate the two styles with a filter.
const RASTER_URL = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
const RASTER_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
const RASTER_FILTERS = {
  light: 'grayscale(0.4) brightness(1.04)',
  dark: 'invert(1) hue-rotate(180deg) brightness(0.95) contrast(0.9)',
};

// How long the GL map gets to draw its first frame before we give up on it.
const GL_LOAD_TIMEOUT_MS = 12000;

// The zoom ceiling the map is given; the vector styles overzoom past their
// native level, so this is a UI choice rather than a data limit.
export const BASEMAP_MAX_ZOOM = 20;

// MapLibre is a quarter of the whole bundle and only the map surfaces need
// it, so it arrives in its own chunk the first time a map is drawn — never
// on the quilt, a patch page, or a feed.
let glModule;
function loadGL() {
  glModule ||= Promise.all([
    import('maplibre-gl/dist/maplibre-gl.css'),
    import('@maplibre/maplibre-gl-leaflet'),
  ]);
  return glModule;
}

// Answered once and remembered. A probe context is a real WebGL context and
// counts against the browser's limit; probing on every mount starved the
// live maps of contexts and left them blank, so the probe hands its context
// straight back and is never repeated.
let webGLSupported;
function hasWebGL() {
  if (webGLSupported !== undefined) return webGLSupported;
  try {
    const canvas = document.createElement('canvas');
    const gl = canvas.getContext('webgl2') || canvas.getContext('webgl');
    gl?.getExtension('WEBGL_lose_context')?.loseContext();
    webGLSupported = !!gl;
  } catch {
    webGLSupported = false;
  }
  return webGLSupported;
}

// Leaflet deletes _containerId in map.remove(), and has no public way to
// ask. A map can be torn down while the GL chunk is still in flight.
function isRemoved(map) {
  return !map || map._containerId === undefined;
}

function addRasterLayer(map, theme) {
  const layer = L.tileLayer(RASTER_URL, {
    attribution: RASTER_ATTRIBUTION,
    maxZoom: BASEMAP_MAX_ZOOM,
  }).addTo(map);
  const paint = (t) => {
    const el = layer.getContainer();
    if (el) el.style.filter = RASTER_FILTERS[t] || RASTER_FILTERS.light;
  };
  paint(theme);
  return { layer, paint };
}

// Adds the basemap to `map` and resolves to a handle whose setTheme()
// restyles it in place — swapping the style is far cheaper than tearing
// down a GL context on every light/dark toggle. Resolves to null if the
// map was removed before the layer could be added.
export async function addBasemap(map, theme = 'light') {
  if (isRemoved(map)) return null;

  let current = theme;

  if (!hasWebGL()) {
    const raster = addRasterLayer(map, current);
    return {
      setTheme(t) {
        current = t;
        raster.paint(t);
      },
    };
  }

  await loadGL();
  if (isRemoved(map)) return null;

  let glLayer = L.maplibreGL({
    style: STYLE_URLS[current] || STYLE_URLS.light,
    attribution: ATTRIBUTION,
  }).addTo(map);
  let raster = null;

  // A GL map that never paints leaves a blank rectangle, which reads as a
  // broken page rather than a missing basemap. Give it a fair window, and
  // if it hasn't loaded — or the context is lost out from under it — fall
  // back to raster tiles.
  const gl = glLayer.getMaplibreMap?.();
  const fallBack = () => {
    if (raster || isRemoved(map)) return;
    clearTimeout(timer);
    glLayer.remove();
    glLayer = null;
    raster = addRasterLayer(map, current);
  };
  // `load` — style parsed and the first frame drawn — is the signal that the
  // map is alive. Not `loaded()`, which also waits on every tile in view and
  // stays false for a long time on a slow link.
  let painted = false;
  // A hidden tab throttles the animation frames MapLibre draws in, so a map
  // in the background hasn't failed — it just hasn't been asked to paint.
  // Give it another window once someone is looking.
  const check = () => {
    if (painted) return;
    if (document.hidden) {
      timer = setTimeout(check, GL_LOAD_TIMEOUT_MS);
      return;
    }
    fallBack();
  };
  let timer = setTimeout(check, GL_LOAD_TIMEOUT_MS);
  if (gl) {
    gl.once('load', () => {
      painted = true;
      clearTimeout(timer);
    });
    gl.getCanvas()?.addEventListener('webglcontextlost', fallBack);
  }

  return {
    setTheme(t) {
      current = t;
      if (raster) {
        raster.paint(t);
        return;
      }
      gl?.setStyle(STYLE_URLS[t] || STYLE_URLS.light);
    },
  };
}
