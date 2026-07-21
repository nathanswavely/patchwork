/**
 * Simple client-side router using History API.
 * Supports path parameters like :slug and :id.
 */

let currentPath = $state(window.location.pathname);
let currentParams = $state({});
let currentQuery = $state(new URLSearchParams(window.location.search));

const routes = [];

export function getPath() {
  return currentPath;
}

export function getParams() {
  return currentParams;
}

export function getQuery() {
  return currentQuery;
}

/**
 * Whether a redirect target (typically from a `?redirect=` query param) is
 * safe to hand to navigate()/replaceRoute(). Only same-origin relative
 * paths are allowed — an absolute URL (`https://evil.example`) or a
 * protocol-relative one (`//evil.example`, or the backslash variant
 * browsers normalize the same way) would send the user off-site, which is
 * exactly what an open-redirect check exists to prevent.
 */
export function isSafeRedirectPath(path) {
  return (
    typeof path === 'string'
    && path.startsWith('/')
    && !path.startsWith('//')
    && !path.startsWith('/\\')
  );
}

/**
 * Register a route pattern. Call once at startup.
 *
 * Params are recomputed after every registration: this module initializes
 * before any routes exist, so without this, a direct page load on a deep
 * link (e.g. /patches/x/manage/governance/123) would leave getParams()
 * empty until the first in-SPA navigation.
 */
export function addRoute(pattern, name) {
  const paramNames = [];
  // A trailing '/*' matches one or more remaining segments into `rest`
  // (used for redirect aliases of retired URL schemes — see ADR 003).
  let regexStr = pattern;
  let wildcard = false;
  if (regexStr.endsWith('/*')) {
    wildcard = true;
    regexStr = regexStr.slice(0, -2);
  }
  regexStr = regexStr.replace(/:([^/]+)/g, (_, paramName) => {
    paramNames.push(paramName);
    return '([^/]+)';
  });
  if (wildcard) {
    paramNames.push('rest');
    regexStr += '/(.+)';
  }
  routes.push({
    pattern,
    name,
    regex: new RegExp(`^${regexStr}$`),
    paramNames,
    // 1 for a static segment, 0 for a :param, -1 for the wildcard — used
    // to prefer /events/new over /events/:id regardless of registration
    // order, and any explicit route over a wildcard alias.
    specificity: pattern
      .split('/')
      .map((seg) => (seg === '*' ? -1 : seg.startsWith(':') ? 0 : 1)),
  });
  updateParams();
}

/**
 * Match the current path against registered routes.
 * When several patterns match, the one with static segments in the
 * earliest positions wins (so /events/new beats /events/:id).
 * Returns { name, params } or null.
 */
export function matchRoute() {
  let best = null;
  let bestMatch = null;
  for (const route of routes) {
    const match = currentPath.match(route.regex);
    if (!match) continue;
    if (best && !moreSpecific(route.specificity, best.specificity)) continue;
    best = route;
    bestMatch = match;
  }
  if (!best) return null;
  const params = {};
  best.paramNames.forEach((name, i) => {
    params[name] = decodeURIComponent(bestMatch[i + 1]);
  });
  return { name: best.name, params };
}

// True when a beats b: first position where they differ, static (1) wins.
// Equal specificity keeps the earlier registration (returns false).
function moreSpecific(a, b) {
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const av = a[i] ?? -1;
    const bv = b[i] ?? -1;
    if (av !== bv) return av > bv;
  }
  return false;
}

/**
 * Navigate to a new path.
 */
export function navigate(path, options = {}) {
  const [pathname, search] = path.split('?');
  if (pathname === currentPath && !options.force) return;
  window.history.pushState({}, '', path);
  currentPath = pathname;
  currentQuery = new URLSearchParams(search || '');
  updateParams();
}

/**
 * Replace the current history entry.
 */
export function replaceRoute(path) {
  const [pathname, search] = path.split('?');
  window.history.replaceState({}, '', path);
  currentPath = pathname;
  currentQuery = new URLSearchParams(search || '');
  updateParams();
}

function updateParams() {
  const matched = matchRoute();
  currentParams = matched?.params || {};
}

/**
 * Handle browser back/forward.
 */
function handlePopState() {
  currentPath = window.location.pathname;
  currentQuery = new URLSearchParams(window.location.search);
  updateParams();
}

// Initialize
window.addEventListener('popstate', handlePopState);
updateParams();
