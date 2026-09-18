/**
 * The admin panel's shape (docs/adr/118): five tabs, two of which carry a
 * sidebar, in place of the fifteen-tab row that grew one tab per feature.
 *
 * The registry in lib/adminPanel.js is pure, so the tab row, the URL
 * scheme and the redirects from the flat scheme are asserted directly.
 * The wiring (routes, shells, inbound links) is asserted against source
 * text, since nothing here renders.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { adminTabs, adminTabForPath, adminTabLanding, legacyAdminPath, ADMIN_TABS } from '../lib/adminPanel.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('docs/adr/118: the tab row', () => {
  it('is five tabs, each answering one question', () => {
    expect(adminTabs().map((t) => t.id)).toEqual(['overview', 'review', 'users', 'settings', 'audit']);
  });

  it('carries a sidebar under Review and Settings and nowhere else', () => {
    const withSections = adminTabs().filter((t) => t.sections).map((t) => t.id);
    expect(withSections).toEqual(['review', 'settings']);
  });

  it('puts every decision queue under Review, each tied to an inbox line', () => {
    const review = adminTabs().find((t) => t.id === 'review');
    expect(review.sections.map((s) => s.id)).toEqual(['reports', 'submissions', 'event-submissions', 'claims', 'tags']);
    expect(review.sections.map((s) => s.queue)).toEqual(['reports', 'submissions', 'event_submissions', 'claims', 'tag_suggestions']);
  });

  it('puts every configuration page under Settings, with the rare step-up tool last', () => {
    const settings = adminTabs().find((t) => t.id === 'settings');
    expect(settings.sections.map((s) => s.id)).toEqual(['quilt', 'label', 'legal', 'tags', 'neighbors', 'aggregators', 'archived', 'attestation']);
    expect(settings.sections.at(-1).label).toBe('Prove admin');
  });

  it('gives a section the URL of its tab plus its id', () => {
    const review = adminTabs().find((t) => t.id === 'review');
    expect(review.sections.find((s) => s.id === 'claims').href).toBe('/admin/review/claims');
    const settings = adminTabs().find((t) => t.id === 'settings');
    expect(settings.sections.find((s) => s.id === 'tags').href).toBe('/admin/settings/tags');
  });

  it('does not mutate the registry when filling hrefs in', () => {
    adminTabs();
    expect(ADMIN_TABS[1].sections[0].href).toBeUndefined();
  });
});

describe('docs/adr/118: which tab a path belongs to', () => {
  it('is Overview for /admin alone', () => {
    expect(adminTabForPath('/admin')).toBe('overview');
    expect(adminTabForPath('/admin/')).toBe('overview');
  });

  it('is the tab whose prefix the path carries', () => {
    expect(adminTabForPath('/admin/review/reports')).toBe('review');
    expect(adminTabForPath('/admin/settings/legal')).toBe('settings');
    expect(adminTabForPath('/admin/users')).toBe('users');
    expect(adminTabForPath('/admin/audit')).toBe('audit');
  });

  it('is nothing for a path the panel does not own', () => {
    expect(adminTabForPath('/admin/reports')).toBeNull();
    expect(adminTabForPath('/patches/x/settings')).toBeNull();
  });

  it('lands a tab with sections on its first section', () => {
    expect(adminTabLanding('review')).toBe('/admin/review/reports');
    expect(adminTabLanding('settings')).toBe('/admin/settings/quilt');
    expect(adminTabLanding('users')).toBe('/admin/users');
    expect(adminTabLanding('nope')).toBeNull();
  });
});

describe('docs/adr/118: the flat scheme redirects', () => {
  it('maps every retired page onto the section it became', () => {
    expect(legacyAdminPath('reports')).toBe('/admin/review/reports');
    expect(legacyAdminPath('submissions')).toBe('/admin/review/submissions');
    expect(legacyAdminPath('event-submissions')).toBe('/admin/review/event-submissions');
    expect(legacyAdminPath('claims')).toBe('/admin/review/claims');
    expect(legacyAdminPath('quilt')).toBe('/admin/settings/quilt');
    expect(legacyAdminPath('label')).toBe('/admin/settings/label');
    expect(legacyAdminPath('legal')).toBe('/admin/settings/legal');
    expect(legacyAdminPath('neighbors')).toBe('/admin/settings/neighbors');
    expect(legacyAdminPath('aggregators')).toBe('/admin/settings/aggregators');
    expect(legacyAdminPath('archived')).toBe('/admin/settings/archived');
    expect(legacyAdminPath('attestation')).toBe('/admin/settings/attestation');
  });

  it('sends the old Tags page to the vocabulary, which is the page it named', () => {
    expect(legacyAdminPath('tags')).toBe('/admin/settings/tags');
  });

  it('answers null for a path that never existed, so it stays a not-found', () => {
    expect(legacyAdminPath('nope')).toBeNull();
    expect(legacyAdminPath('review/reports')).toBeNull();
  });

  it('is wired as a wildcard alias behind every explicit admin route', () => {
    const app = source('App.svelte');
    expect(app).toContain("addRoute('/admin/*', 'redirectAdminLegacy')");
    expect(app).toContain('redirectAdminLegacy: (p) => legacyAdminPath(p.rest)');
    expect(app).toContain("adminReviewIndex: () => adminTabLanding('review')");
    expect(app).toContain("adminSettingsIndex: () => adminTabLanding('settings')");
  });
});

describe('docs/adr/118: the shell', () => {
  const shell = source('components/AdminShell.svelte');

  it('draws the tabs from the registry rather than its own list', () => {
    expect(shell).toContain("import { adminTabs, adminTabForPath } from '../lib/adminPanel.js'");
    expect(shell).not.toContain("label: 'Neighbors'");
  });

  it('wraps a tab with sections in the same SettingsShell a patch workspace uses', () => {
    expect(shell).toContain("import SettingsShell from './SettingsShell.svelte'");
    expect(shell).toContain('{#if activeTab?.sections}');
    expect(shell).toContain('<SettingsShell title={activeTab.label} sections={sidebarSections}>');
  });

  it('counts the Review badge and the sidebar from the Overview endpoint, so they never disagree', () => {
    expect(shell).toContain("api('admin/overview')");
    expect(shell).toContain("{#if tab.id === 'review' && reviewTotal > 0}");
    expect(shell).toContain('count: s.queue ? inboxCounts[s.queue] || 0 : undefined');
    const settingsShell = source('components/SettingsShell.svelte');
    expect(settingsShell).toContain('{#if section.count > 0}');
  });
});

describe('docs/adr/118: every route is registered and every page is in every list', () => {
  const app = source('App.svelte');
  const routes = [
    ['/admin', 'adminDashboard'],
    ['/admin/review', 'adminReviewIndex'],
    ['/admin/review/reports', 'adminReports'],
    ['/admin/review/submissions', 'adminSubmissions'],
    ['/admin/review/event-submissions', 'adminEventSubmissions'],
    ['/admin/review/claims', 'adminClaims'],
    ['/admin/review/tags', 'adminTagSuggestions'],
    ['/admin/users', 'adminUsers'],
    ['/admin/settings', 'adminSettingsIndex'],
    ['/admin/settings/quilt', 'adminQuilt'],
    ['/admin/settings/label', 'adminLabel'],
    ['/admin/settings/legal', 'adminLegal'],
    ['/admin/settings/tags', 'adminTags'],
    ['/admin/settings/neighbors', 'adminNeighbors'],
    ['/admin/settings/aggregators', 'adminAggregators'],
    ['/admin/settings/archived', 'adminArchived'],
    ['/admin/settings/attestation', 'adminAttestation'],
    ['/admin/audit', 'adminAudit'],
  ];

  for (const [path, name] of routes) {
    it(`${path} → ${name}, in the admin shell and behind the session gate`, () => {
      expect(app).toContain(`addRoute('${path}', '${name}')`);
      const adminSet = app.match(/const adminRoutes = new Set\(\[[^\]]*\]\)/s)?.[0] || '';
      expect(adminSet).toContain(`'${name}'`);
      const authGuard = app.match(/let authRequired = \$derived\(\s*\[[^\]]*\]/s)?.[0] || '';
      expect(authGuard).toContain(`'${name}'`);
    });
  }

  it('no flat admin route survives as a registered page', () => {
    for (const rest of ['reports', 'tags', 'claims', 'quilt', 'legal', 'attestation']) {
      expect(app).not.toContain(`addRoute('/admin/${rest}',`);
    }
  });
});

describe('docs/adr/118: the suggested-tag queue is a Review section, not a corner of Tags', () => {
  it('has its own page that decides and links back to the vocabulary', () => {
    const page = source('pages/AdminTagSuggestions.svelte');
    expect(page).toContain("api('admin/tag-suggestions')");
    expect(page).toContain('admin/tag-suggestions/${suggestion.id}');
    expect(page).toContain('No suggested tags waiting.');
    expect(page).toContain("handleNav(e, '/admin/settings/tags')");
  });

  it('leaves the Tags page to the vocabulary, pointing at the queue', () => {
    const tags = source('pages/AdminTags.svelte');
    expect(tags).not.toContain('admin/tag-suggestions');
    expect(tags).toContain("navigate('/admin/review/tags')");
  });
});

describe('docs/adr/118: inbound links land on the nested paths', () => {
  it('from the Overview', () => {
    const src = source('pages/AdminDashboard.svelte');
    expect(src).not.toMatch(/'\/admin\/(reports|submissions|event-submissions|claims|tags|aggregators)'/);
  });

  it('from the patch profile and verification settings', () => {
    expect(source('components/PatchProfileHead.svelte')).toContain("'/admin/review/claims'");
    expect(source('pages/PatchSettingsVerification.svelte')).toContain("'/admin/review/claims'");
  });

  it('from the admin finder, which lists every section', () => {
    const providers = source('lib/finderProviders.js');
    for (const t of adminTabs()) {
      for (const s of t.sections || []) expect(providers).toContain(`href: '${s.href}'`);
    }
  });
});
