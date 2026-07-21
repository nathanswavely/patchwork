import { describe, it, expect, beforeEach, vi } from 'vitest';

// We need to mock window.location and history before importing the router
let routerModule;

describe('router', () => {
  beforeEach(async () => {
    // Reset modules so router re-initializes with clean state
    vi.resetModules();

    // Mock window.location
    delete window.location;
    window.location = new URL('http://localhost/');
    window.location.pathname = '/';
    window.location.search = '';

    // Mock history
    window.history.pushState = vi.fn();
    window.history.replaceState = vi.fn();

    routerModule = await import('../stores/router.svelte.js');
  });

  it('addRoute registers patterns and matchRoute finds them', () => {
    routerModule.addRoute('/patches/:slug', 'patchOverview');
    routerModule.addRoute('/events/:id', 'eventDetail');

    // Router reads currentPath from window.location.pathname which is '/'
    // We need to navigate first
    routerModule.navigate('/patches/gallery-row');

    const match = routerModule.matchRoute();
    expect(match).not.toBeNull();
    expect(match.name).toBe('patchOverview');
    expect(match.params.slug).toBe('gallery-row');
  });

  it('matches exact routes', () => {
    routerModule.addRoute('/', 'home');
    routerModule.addRoute('/dashboard', 'dashboard');

    routerModule.navigate('/dashboard');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('dashboard');
  });

  it('returns null for unmatched routes', () => {
    routerModule.addRoute('/', 'home');

    routerModule.navigate('/nonexistent');
    const match = routerModule.matchRoute();
    expect(match).toBeNull();
  });

  it('navigate calls history.pushState', () => {
    routerModule.navigate('/patches/new');
    expect(window.history.pushState).toHaveBeenCalledWith({}, '', '/patches/new');
  });

  it('navigate is idempotent for same path', () => {
    routerModule.navigate('/test');
    routerModule.navigate('/test');
    // Should only push once (second call is a no-op)
    expect(window.history.pushState).toHaveBeenCalledTimes(1);
  });

  it('handles multi-segment params', () => {
    routerModule.addRoute('/patches/:slug/proposals/:id', 'proposalDetail');

    routerModule.navigate('/patches/gallery-row/proposals/abc-123');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('proposalDetail');
    expect(match.params.slug).toBe('gallery-row');
    expect(match.params.id).toBe('abc-123');
  });

  it('decodes URI params', () => {
    routerModule.addRoute('/patches/:slug', 'patchOverview');

    routerModule.navigate('/patches/caf%C3%A9-collective');
    const match = routerModule.matchRoute();
    expect(match.params.slug).toBe('café-collective');
  });

  it('more specific routes match before general ones', () => {
    routerModule.addRoute('/patches/new', 'patchNew');
    routerModule.addRoute('/patches/:slug', 'patchOverview');

    routerModule.navigate('/patches/new');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('patchNew');
  });

  it('replaceRoute uses replaceState', () => {
    routerModule.replaceRoute('/settings');
    expect(window.history.replaceState).toHaveBeenCalledWith({}, '', '/settings');
  });

  it('matches patch members route', () => {
    routerModule.addRoute('/patches/:slug/members', 'patchMembers');

    routerModule.navigate('/patches/gallery-row/members');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('patchMembers');
    expect(match.params.slug).toBe('gallery-row');
  });

  it('matches patch events route', () => {
    routerModule.addRoute('/patches/:slug/events', 'patchEvents');

    routerModule.navigate('/patches/gallery-row/events');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('patchEvents');
    expect(match.params.slug).toBe('gallery-row');
  });

  it('matches patch settings route', () => {
    routerModule.addRoute('/patches/:slug/settings', 'patchSettings');

    routerModule.navigate('/patches/gallery-row/settings');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('patchSettings');
    expect(match.params.slug).toBe('gallery-row');
  });

  it('matches patch settings sub-pages', () => {
    routerModule.addRoute('/patches/:slug/settings/info', 'patchSettingsInfo');
    routerModule.addRoute('/patches/:slug/settings/members', 'patchSettingsMembers');
    routerModule.addRoute('/patches/:slug/settings/danger', 'patchSettingsDanger');

    routerModule.navigate('/patches/gallery-row/settings/info');
    let match = routerModule.matchRoute();
    expect(match.name).toBe('patchSettingsInfo');
    expect(match.params.slug).toBe('gallery-row');

    routerModule.navigate('/patches/gallery-row/settings/members', { force: true });
    match = routerModule.matchRoute();
    expect(match.name).toBe('patchSettingsMembers');

    routerModule.navigate('/patches/gallery-row/settings/danger', { force: true });
    match = routerModule.matchRoute();
    expect(match.name).toBe('patchSettingsDanger');
  });

  it('matches scoped charter routes', () => {
    routerModule.addRoute('/patches/:slug/charters/:id', 'charterDetail');
    routerModule.addRoute('/patches/:slug/charters/:id/edit', 'charterEdit');

    routerModule.navigate('/patches/gallery-row/charters/charter-456');
    let match = routerModule.matchRoute();
    expect(match.name).toBe('charterDetail');
    expect(match.params.slug).toBe('gallery-row');
    expect(match.params.id).toBe('charter-456');

    routerModule.navigate('/patches/gallery-row/charters/charter-456/edit');
    match = routerModule.matchRoute();
    expect(match.name).toBe('charterEdit');
    expect(match.params.slug).toBe('gallery-row');
    expect(match.params.id).toBe('charter-456');
  });

  it('matches user passkeys settings route', () => {
    routerModule.addRoute('/settings/passkeys', 'settingsPasskeys');

    routerModule.navigate('/settings/passkeys');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('settingsPasskeys');
  });

  it('matches user patches settings route', () => {
    routerModule.addRoute('/settings/patches', 'settingsPatches');

    routerModule.navigate('/settings/patches');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('settingsPatches');
  });

  it('prefers static segments over params regardless of registration order', () => {
    // Mirrors App.svelte: the param route is registered FIRST.
    routerModule.addRoute('/events/:id', 'eventDetail');
    routerModule.addRoute('/events/new', 'eventNew');

    routerModule.navigate('/events/new');
    expect(routerModule.matchRoute().name).toBe('eventNew');

    routerModule.navigate('/events/abc-123');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('eventDetail');
    expect(match.params.id).toBe('abc-123');
  });

  it('static-over-param applies at deeper positions too', () => {
    // Mirrors the governance hub registrations where :id precedes the
    // static 'proposals' list route.
    routerModule.addRoute('/patches/:slug/governance/:id', 'governanceProposal');
    routerModule.addRoute('/patches/:slug/governance/proposals', 'governanceProposals');

    routerModule.navigate('/patches/gallery-row/governance/proposals');
    expect(routerModule.matchRoute().name).toBe('governanceProposals');

    routerModule.navigate('/patches/gallery-row/governance/prop-9');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('governanceProposal');
    expect(match.params.id).toBe('prop-9');
  });

  it('equal specificity keeps first registration', () => {
    routerModule.addRoute('/things/:a', 'first');
    routerModule.addRoute('/things/:b', 'second');

    routerModule.navigate('/things/x');
    expect(routerModule.matchRoute().name).toBe('first');
  });

  it('trailing /* wildcard captures remaining segments as rest', () => {
    routerModule.addRoute('/patches/:slug/manage/*', 'redirectManageDeep');

    routerModule.navigate('/patches/gallery-row/manage/governance/docs/abc');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('redirectManageDeep');
    expect(match.params.slug).toBe('gallery-row');
    expect(match.params.rest).toBe('governance/docs/abc');
  });

  it('explicit routes beat wildcard aliases regardless of order', () => {
    routerModule.addRoute('/patches/:slug/manage/*', 'redirectManageDeep');
    routerModule.addRoute('/patches/:slug/manage/special', 'explicit');

    routerModule.navigate('/patches/gallery-row/manage/special');
    expect(routerModule.matchRoute().name).toBe('explicit');
  });
});
