/**
 * The admin panel's shape (docs/adr/2026-09-17-an-admin-tab-answers-one-question.md): five tabs, two of which carry a
 * sidebar. Kept as pure data and functions so the tab row, the sidebars,
 * the URL scheme and the redirects from the retired flat scheme can be
 * asserted without rendering — AdminShell and App.svelte only map what is
 * returned here onto icons, routes and page components.
 *
 * Every tab answers one question an instance admin arrives with:
 *
 *   Overview   what is waiting on me, and what is unattended
 *   Review     the decision queues themselves
 *   Users      who is on this quilt
 *   Settings   how this quilt is configured
 *   Audit log  what happened
 *
 * A section's id is its last URL segment. A tab with sections has no page
 * of its own: its bare URL lands on the first section.
 */

export const ADMIN_TABS = [
  { id: 'overview', label: 'Overview', href: '/admin' },
  {
    id: 'review',
    label: 'Review',
    href: '/admin/review',
    sections: [
      // `queue` names the inbox line on GET /api/v1/admin/overview that
      // counts this section's pending items, so the badge and the page
      // count the same thing.
      { id: 'reports', label: 'Reports', queue: 'reports' },
      { id: 'submissions', label: 'Patch submissions', queue: 'submissions' },
      { id: 'event-submissions', label: 'Event submissions', queue: 'event_submissions' },
      { id: 'claims', label: 'Claims', queue: 'claims' },
      { id: 'tags', label: 'Suggested tags', queue: 'tag_suggestions' },
    ],
  },
  { id: 'users', label: 'Users', href: '/admin/users' },
  {
    id: 'settings',
    label: 'Settings',
    href: '/admin/settings',
    sections: [
      { id: 'quilt', label: 'Quilt' },
      { id: 'label', label: 'Label' },
      { id: 'legal', label: 'Legal' },
      { id: 'tags', label: 'Tags' },
      { id: 'neighbors', label: 'Neighbors' },
      { id: 'aggregators', label: 'Aggregators' },
      // Visitor counting: the switch that turns it on lives on this page,
      // which is what makes it a setting rather than a report.
      { id: 'usage', label: 'Usage' },
      { id: 'archived', label: 'Archived patches' },
      { id: 'attestation', label: 'Prove admin' },
      // The native apps this quilt vouches for
      // (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md). Last,
      // beside Prove admin, for the same two reasons: it is rare, and the
      // act on it is step-up gated.
      { id: 'apps', label: 'Apps' },
    ],
  },
  { id: 'audit', label: 'Audit log', href: '/admin/audit' },
];

/**
 * The tabs with every section's href filled in.
 * @returns {Array<{id: string, label: string, href: string, sections?: Array<{id: string, label: string, href: string, queue?: string}>}>}
 */
export function adminTabs() {
  return ADMIN_TABS.map((t) => ({
    ...t,
    sections: t.sections?.map((s) => ({ ...s, href: `${t.href}/${s.id}` })),
  }));
}

/**
 * Which tab a path belongs to, or null for a path the panel does not own.
 * /admin is Overview and nothing else; every other tab owns its prefix.
 */
export function adminTabForPath(path) {
  if (path === '/admin' || path === '/admin/') return 'overview';
  for (const t of ADMIN_TABS) {
    if (t.id === 'overview') continue;
    if (path === t.href || path.startsWith(t.href + '/')) return t.id;
  }
  return null;
}

/**
 * Where a tab's bare URL lands: its first section, or itself when it has
 * none.
 */
export function adminTabLanding(tabId) {
  const t = adminTabs().find((x) => x.id === tabId);
  if (!t) return null;
  return t.sections ? t.sections[0].href : t.href;
}

// The flat scheme the panel had before docs/adr/2026-09-17-an-admin-tab-answers-one-question.md, when every page was
// a top-level tab. Each retired path maps onto the section that page
// became; links in old notifications and bookmarks land where they meant
// to. Tag suggestions were a section of the Tags page then, so the old
// Tags path goes to the vocabulary — the page it named — and the queue
// link in a new notification goes to the queue.
const LEGACY = {
  reports: '/admin/review/reports',
  submissions: '/admin/review/submissions',
  'event-submissions': '/admin/review/event-submissions',
  claims: '/admin/review/claims',
  quilt: '/admin/settings/quilt',
  label: '/admin/settings/label',
  legal: '/admin/settings/legal',
  tags: '/admin/settings/tags',
  neighbors: '/admin/settings/neighbors',
  aggregators: '/admin/settings/aggregators',
  usage: '/admin/settings/usage',
  archived: '/admin/settings/archived',
  attestation: '/admin/settings/attestation',
};

/**
 * The canonical path for a retired flat admin path's remainder (the part
 * after /admin/), or null when there never was such a page — a null is a
 * genuine not-found, not a redirect.
 */
export function legacyAdminPath(rest) {
  return LEGACY[rest] || null;
}
