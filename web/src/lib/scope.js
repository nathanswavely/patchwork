/**
 * Scope lives in the URL (docs/adr/035).
 *
 * Scope — My Quilt vs the whole quilt — is a path suffix, not in-memory
 * state. The whole quilt is the *unmarked* default and keeps the URLs it
 * always had (`/`, `/map`, `/events`); My Quilt is the marked variant, the
 * `my` suffix. The shape is `/[surface]/[scope]`, both parts optional:
 *
 *     quilt  → /              /my
 *     map    → /map           /map/my
 *     events → /events        /events/my
 *
 * These helpers are the single source of truth for that mapping, so no
 * component has to hand-build a scoped path or hand-parse one.
 */

// route name → the discovery surface it renders and the scope it carries.
// Routes not in this table are not scope-able (patch profiles, settings…).
const ROUTE_SCOPE = {
  home: { surface: 'quilt', scope: 'local' },
  homeMy: { surface: 'quilt', scope: 'my' },
  // /patches is a legacy alias for the quilt surface; local only (a
  // `/patches/my` would collide with the /patches/:slug patch route).
  patchList: { surface: 'quilt', scope: 'local' },
  map: { surface: 'map', scope: 'local' },
  mapMy: { surface: 'map', scope: 'my' },
  eventList: { surface: 'events', scope: 'local' },
  eventListMy: { surface: 'events', scope: 'my' },
};

// surface → { scope → path }. The whole quilt is the bare surface.
const SURFACE_PATH = {
  quilt: { local: '/', my: '/my' },
  map: { local: '/map', my: '/map/my' },
  events: { local: '/events', my: '/events/my' },
};

/** The scope a route carries: 'my' or 'local' (default 'local'). */
export function scopeForRoute(routeName) {
  return ROUTE_SCOPE[routeName]?.scope || 'local';
}

/** The discovery surface a route renders, or null if it isn't scope-able. */
export function surfaceForRoute(routeName) {
  return ROUTE_SCOPE[routeName]?.surface || null;
}

/** Whether a route participates in scope at all. */
export function isScopedRoute(routeName) {
  return routeName in ROUTE_SCOPE;
}

/**
 * The URL for a surface + scope. Falls back to the quilt surface and, for
 * an unknown scope, to the whole quilt — a path is always returned.
 */
export function scopedPath(surface, scope) {
  const bySurface = SURFACE_PATH[surface] || SURFACE_PATH.quilt;
  return bySurface[scope] || bySurface.local;
}
