// Map-location helpers (issue #4).
//
// A patch's map location is a numeric latitude/longitude pair driving its
// Leaflet marker. It is deliberately independent of the address field: an
// address is prose a person reads, a map location is a placed marker, and
// naming one never implies the other (CONTEXT.md → "Map location").
//
// Placement is map-drag only — no geocoder — so these helpers only ever
// format and range-check numbers the map already produced.

export const LAT_MIN = -90;
export const LAT_MAX = 90;
export const LNG_MIN = -180;
export const LNG_MAX = 180;

/**
 * True when both coordinates are present and finite. A patch with a map
 * location renders a marker; without one it is simply off the map (there is
 * no separate on/off flag — unset position = not shown).
 * @param {number|null|undefined} lat
 * @param {number|null|undefined} lng
 */
export function hasMapLocation(lat, lng) {
  return isFinite(lat) && isFinite(lng) && lat != null && lng != null;
}

/**
 * Range-check a coordinate pair. Mirrors the backend validation
 * (validateCoordinate in internal/handler/nodes.go).
 * @param {number} lat
 * @param {number} lng
 */
export function isValidCoord(lat, lng) {
  if (!isFinite(lat) || !isFinite(lng)) return false;
  return lat >= LAT_MIN && lat <= LAT_MAX && lng >= LNG_MIN && lng <= LNG_MAX;
}

/**
 * Format a coordinate pair for display, e.g. "40.03790, -76.30550".
 * Fixed to 5 decimals (~1.1 m) — enough precision to read back a placed
 * marker without implying survey accuracy. Returns '' when unset.
 * @param {number|null|undefined} lat
 * @param {number|null|undefined} lng
 */
export function formatCoord(lat, lng) {
  if (!hasMapLocation(lat, lng)) return '';
  return `${Number(lat).toFixed(5)}, ${Number(lng).toFixed(5)}`;
}

/**
 * Round a coordinate to storage precision (5 decimals). Keeps stored values
 * tidy and stable regardless of the exact pixel a drag or click landed on.
 * @param {number} n
 */
export function roundCoord(n) {
  return Math.round(n * 1e5) / 1e5;
}
