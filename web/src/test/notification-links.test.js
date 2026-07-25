/**
 * Notification, email, and feed links are built in Go (internal/weblink) and
 * resolved here. Nothing in the build checks the two agree, and they didn't:
 * `/patches/{slug}/events/{id}` matched no route, so every event notification
 * and reminder email landed on the home quilt, and a charter's link resolved
 * to the proposal route and rendered "not found" (issue #56).
 *
 * So this test closes the loop from the frontend side. It reads App.svelte's
 * real registrations, feeds them to the real router, and asserts each link
 * shape lands where it's meant to. The Go mirror is
 * internal/weblink/weblink_test.go — the two lists must stay in step.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Every addRoute('<pattern>', '<name>') in App.svelte, in source order —
// order matters to the router's specificity tie-breaks.
function appRoutes() {
  const src = readFileSync(resolve(process.cwd(), 'src/App.svelte'), 'utf8');
  return [...src.matchAll(/addRoute\('([^']+)',\s*'([^']+)'\)/g)].map((m) => ({
    pattern: m[1],
    name: m[2],
  }));
}

describe('notification and feed link targets', () => {
  let router;

  beforeEach(async () => {
    vi.resetModules();
    delete window.location;
    window.location = new URL('http://localhost/');
    window.location.pathname = '/';
    window.location.search = '';
    window.history.pushState = vi.fn();
    window.history.replaceState = vi.fn();

    router = await import('../stores/router.svelte.js');
    const routes = appRoutes();
    expect(routes.length).toBeGreaterThan(50); // the regex still finds them
    for (const { pattern, name } of routes) router.addRoute(pattern, name);
  });

  function match(path) {
    router.navigate(path, { force: true });
    return router.matchRoute();
  }

  // Mirrors internal/weblink/weblink.go, one case per exported helper.
  const cases = [
    { built: 'Patch', path: '/patches/gallery-row', route: 'patchProfile' },
    { built: 'PatchEvents', path: '/patches/gallery-row/events', route: 'patchEvents' },
    { built: 'PatchMembers', path: '/patches/gallery-row/members', route: 'patchMembers' },
    { built: 'PatchSetup', path: '/patches/gallery-row/setup', route: 'patchSetup' },
    { built: 'Event', path: '/events/019f-abc', route: 'eventDetail' },
    { built: 'Proposal', path: '/patches/gallery-row/governance/019f-pr', route: 'governanceProposal' },
    { built: 'GovernanceDoc', path: '/patches/gallery-row/governance/docs/019f-doc', route: 'governanceDocDetail' },
    { built: 'RemotePatch', path: '/quilts/other.example/patches/their-patch', route: 'remotePatch' },
  ];

  for (const { built, path, route } of cases) {
    it(`weblink.${built} resolves to ${route}`, () => {
      const m = match(path);
      expect(m, `${path} matched no route`).not.toBeNull();
      expect(m.name).toBe(route);
    });
  }

  it('a query string on a link does not stop the path from matching', () => {
    // weblink.PatchMembersPending — the filter rides along in the query.
    const m = match('/patches/gallery-row/members?status=pending');
    expect(m.name).toBe('patchMembers');
    expect(router.getQuery().get('status')).toBe('pending');
  });

  it('the charter and proposal shapes resolve to different pages', () => {
    // The bug: both take a bare UUID, so a charter link missing its `docs/`
    // segment rendered as a proposal that does not exist.
    expect(match('/patches/gallery-row/governance/x').name).toBe('governanceProposal');
    expect(match('/patches/gallery-row/governance/docs/x').name).toBe('governanceDocDetail');
  });

  it('the legacy patch-scoped event link still reaches the event', () => {
    // Links in already-sent emails and already-subscribed calendars.
    const m = match('/patches/gallery-row/events/019f-abc');
    expect(m, 'patch-scoped event links fall through to the home quilt').not.toBeNull();
    expect(m.name).toBe('redirectPatchScopedEvent');
    expect(m.params.id).toBe('019f-abc');
  });

  it('the events list route is not shadowed by the legacy alias', () => {
    expect(match('/patches/gallery-row/events').name).toBe('patchEvents');
  });
});

describe('the legacy event alias redirects to the canonical URL', () => {
  const app = readFileSync(resolve(process.cwd(), 'src/App.svelte'), 'utf8');

  it('maps redirectPatchScopedEvent to /events/:id', () => {
    expect(app).toContain('redirectPatchScopedEvent: (p) => `/events/${p.id}`');
  });
});
