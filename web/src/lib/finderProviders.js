/**
 * Providers for the global bar's scoped finder (WorkspaceSearch).
 *
 * A provider returns the full entity list for its context — fetched once on
 * first focus, cached by the component for the visit, filtered client-side.
 * Community-scale data (a patch has dozens of members, a handful of
 * proposals), so no server index and no per-keystroke requests; see
 * docs/adr/005. Each entry: { type, label, sublabel, href }.
 */
import { api } from './api.js';

/**
 * Everything searchable inside a patch workspace.
 * People land on their profile pages (/users/:username). The members
 * endpoint is viewer-aware: outsiders only get visible member/admin rows,
 * so the finder never reveals more than the member list itself would
 * (docs/adr/006).
 */
export function workspaceFinderProvider(slug) {
  return async () => {
    const base = `/patches/${slug}`;
    const [proposals, docs, events, members] = await Promise.all([
      api(`nodes/${slug}/proposals`).catch(() => null),
      api(`nodes/${slug}/governance`).catch(() => null),
      api(`events?node_slug=${encodeURIComponent(slug)}`).catch(() => null),
      api(`nodes/${slug}/members`).catch(() => null),
    ]);

    const items = [];
    for (const m of members?.items || []) {
      items.push({
        type: 'People',
        label: m.display_name || m.username,
        sublabel: m.role,
        href: `/users/${m.username}`,
      });
    }
    for (const p of proposals?.items || []) {
      items.push({
        type: 'Proposals',
        label: p.title,
        sublabel: p.status,
        href: `${base}/governance/${p.id}`,
      });
    }
    for (const d of docs?.items || []) {
      items.push({
        type: 'Documents',
        label: d.title,
        sublabel: '',
        href: `${base}/governance/docs/${d.id}`,
      });
    }
    for (const e of events?.items || []) {
      items.push({
        type: 'Events',
        label: e.title,
        sublabel: e.starts_at ? new Date(e.starts_at).toLocaleDateString() : '',
        href: `/events/${e.id}`,
      });
    }
    return items;
  };
}

/**
 * Everything searchable inside the admin panel.
 * Users land on their profile pages; suspend/role actions stay in the
 * Users tab.
 */
export function adminFinderProvider() {
  return async () => {
    const [reports, submissions, users] = await Promise.all([
      api('admin/reports').catch(() => null),
      api('admin/submissions').catch(() => null),
      api('admin/users').catch(() => null),
    ]);

    const items = [];
    for (const u of users?.items || []) {
      items.push({
        type: 'Users',
        label: u.display_name || u.username,
        sublabel: u.suspended_at ? 'suspended' : u.role,
        href: `/users/${u.username}`,
      });
    }
    for (const r of reports?.items || []) {
      items.push({
        type: 'Reports',
        label: r.reason || `Report ${r.id?.slice(0, 8)}`,
        sublabel: r.status,
        href: '/admin/reports',
      });
    }
    for (const s of submissions?.items || []) {
      items.push({
        type: 'Submissions',
        label: s.name,
        sublabel: s.status,
        href: '/admin/submissions',
      });
    }
    items.push({
      type: 'Settings',
      label: 'Quilt Settings',
      sublabel: 'rename, icon, export, danger zone',
      href: '/admin/quilt',
    });
    items.push({
      type: 'Settings',
      label: 'Tags',
      sublabel: 'the tag vocabulary and per-tag motifs',
      href: '/admin/tags',
    });
    return items;
  };
}

/**
 * The discovery corpus (docs/adr/033): every public patch plus upcoming
 * events. Deliberately not the deep past — the events page's date controls
 * are the tool for archaeology — and deliberately no people (no
 * instance-wide people search; people are discovered through patches).
 */
export function discoveryFinderProvider() {
  return async () => {
    const today = new Date().toISOString().slice(0, 10);
    const [treeResp, events] = await Promise.all([
      api('nodes/tree').catch(() => null),
      api(`events?from=${today}&limit=100`).catch(() => null),
    ]);

    const items = [];
    const tree = treeResp?.tree || treeResp;
    for (const p of tree?.children || []) {
      items.push({
        type: 'Patches',
        label: p.name,
        sublabel: (p.tags || []).slice(0, 2).join(', '),
        href: `/patches/${p.slug}`,
      });
    }
    for (const e of events?.items || []) {
      items.push({
        type: 'Events',
        label: e.title,
        sublabel: e.starts_at
          ? new Date(e.starts_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
          : '',
        href: `/events/${e.id}`,
      });
    }
    return items;
  };
}
