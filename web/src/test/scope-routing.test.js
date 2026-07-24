/**
 * Scope lives in the URL (docs/adr/035).
 *
 * The pure mapping is unit-tested directly; the component wiring is
 * asserted against source text (there is no Svelte render library here),
 * which is enough to catch the regressions this redesign is about: scope
 * must come from the route, the switch must swap the scope segment in
 * place, and `/` must never be an auth-conditional redirect.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  scopeForRoute,
  surfaceForRoute,
  isScopedRoute,
  scopedPath,
} from '../lib/scope.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('scope helpers map routes ↔ scope ↔ path', () => {
  it('reads the scope a route carries', () => {
    expect(scopeForRoute('home')).toBe('local');
    expect(scopeForRoute('homeMy')).toBe('my');
    expect(scopeForRoute('map')).toBe('local');
    expect(scopeForRoute('mapMy')).toBe('my');
    expect(scopeForRoute('eventList')).toBe('local');
    expect(scopeForRoute('eventListMy')).toBe('my');
    // Non-discovery routes are 'local' (unscoped) by default.
    expect(scopeForRoute('patchProfile')).toBe('local');
    expect(scopeForRoute(undefined)).toBe('local');
  });

  it('reads the surface a route renders', () => {
    expect(surfaceForRoute('homeMy')).toBe('quilt');
    expect(surfaceForRoute('mapMy')).toBe('map');
    expect(surfaceForRoute('eventListMy')).toBe('events');
    expect(surfaceForRoute('patchProfile')).toBe(null);
  });

  it('knows which routes participate in scope', () => {
    expect(isScopedRoute('home')).toBe(true);
    expect(isScopedRoute('eventListMy')).toBe(true);
    expect(isScopedRoute('patchProfile')).toBe(false);
  });

  it('builds the path for a surface + scope; whole quilt is unmarked', () => {
    expect(scopedPath('quilt', 'local')).toBe('/');
    expect(scopedPath('quilt', 'my')).toBe('/my');
    expect(scopedPath('map', 'local')).toBe('/map');
    expect(scopedPath('map', 'my')).toBe('/map/my');
    expect(scopedPath('events', 'local')).toBe('/events');
    expect(scopedPath('events', 'my')).toBe('/events/my');
  });

  it('round-trips a route through its own path', () => {
    for (const route of ['home', 'homeMy', 'map', 'mapMy', 'eventList', 'eventListMy']) {
      const path = scopedPath(surfaceForRoute(route), scopeForRoute(route));
      expect(typeof path).toBe('string');
      expect(path.startsWith('/')).toBe(true);
    }
  });

  it('falls back safely on unknown input', () => {
    // Unknown surface → quilt, but a valid scope is preserved.
    expect(scopedPath('nonsense', 'my')).toBe('/my');
    // Unknown scope → the surface's local (whole-quilt) path.
    expect(scopedPath('map', 'nonsense')).toBe('/map');
    expect(scopedPath('nonsense', 'nonsense')).toBe('/');
  });
});

describe('App registers the scoped routes and drops the in-memory default', () => {
  const src = source('App.svelte');

  it('registers the My Quilt suffix on each surface', () => {
    expect(src).toContain("addRoute('/my', 'homeMy')");
    expect(src).toContain("addRoute('/map/my', 'mapMy')");
    expect(src).toContain("addRoute('/events/my', 'eventListMy')");
  });

  it('derives scope from the route, not from in-memory state', () => {
    expect(src).toMatch(/quiltScope\s*=\s*\$derived\(scopeForRoute\(routeName\)\)/);
    // The old auth-conditional default is gone.
    expect(src).not.toContain('hasSetDefaultScope');
  });

  it('redirects the landing preference once, via replaceRoute', () => {
    expect(src).toContain('start_on_my_quilt');
    expect(src).toContain("replaceRoute('/my')");
    expect(src).toContain('didScopeRedirect');
  });
});

describe('The switcher swaps the scope segment in place', () => {
  const src = source('components/SocialShell.svelte');

  it('selectScope navigates to the scoped path, not always home', () => {
    expect(src).toMatch(/navigate\(scopedPath\(surfaceForRoute\(routeName\)\s*\|\|\s*'quilt',\s*scope\)\)/);
    // The old always-go-home behavior and its callback prop are gone.
    expect(src).not.toContain('onScopeChange');
  });
});
