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
      // Searching a patch should reach what already happened — you look up
      // last month's show by name as readily as next month's.
      api(`events?node_slug=${encodeURIComponent(slug)}&include_past=true`).catch(() => null),
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

/**
 * The patch picker's corpus (CONTEXT.md "Patch picker"): every public
 * patch on the quilt, active and unclaimed alike.
 *
 * Paginated, deliberately. The listing caps `limit` at 100 server-side
 * (parsePaginationParams), so a single large-limit request is a silent
 * truncation — and a picker that truncates doesn't degrade, it lies: the
 * missing patch reads as "not on this quilt", which is exactly the
 * reading that sends someone to the suggest row to create a duplicate of
 * a patch that already exists.
 *
 * `decorate(node)` supplies what the surface's question needs — the
 * group heading, the sublabel, whether the row is refused — because the
 * corpus is shared but the question isn't: routing an aggregator name
 * cares about `accept_event_suggestions` (docs/adr/056), proposing an
 * event link cares about who is already linked (docs/adr/032). Returning
 * null from decorate drops the row.
 */
async function fetchNodePages(extra = '') {
  const nodes = [];
  let after = '';
  try {
    // Bounded against a cursor that never advances: 20 pages is 2000
    // patches, well past community scale and short of a hung tab.
    for (let page = 0; page < 20; page++) {
      const data = await api(`nodes?limit=100${extra}${after ? `&after=${encodeURIComponent(after)}` : ''}`);
      const items = data.items || [];
      nodes.push(...items);
      if (!data.next_cursor || items.length === 0) break;
      after = data.next_cursor;
    }
  } catch {
    // Partial beats empty: a picker over the pages that did arrive still
    // finds most patches, where an empty corpus finds none.
  }
  return nodes;
}

function decorateNodes(nodes, decorate) {
  const items = [];
  for (const n of nodes) {
    const row = decorate(n);
    if (row) items.push({ label: n.name, href: `/patches/${n.slug}`, slug: n.slug, ...row });
  }
  return items;
}

export async function patchPickerProvider(decorate) {
  return decorateNodes(await fetchNodePages(), decorate);
}

/**
 * The same corpus, widened by the caller's own patches.
 *
 * The unscoped listing is the public set — ListNodes pins `visibility =
 * 'public'` unconditionally — so a private patch the caller belongs to and
 * could post to directly is missing from it. That absence reads as "not on
 * this quilt", which is the one thing a picker must never say about a patch
 * the person is standing in. `scope=my` returns exactly the patches they
 * hold an active relationship on, private ones included, so the union is the
 * honest answer to "where could this go". Same row shape either way, so
 * `decorate` cannot tell which source a node arrived from — and shouldn't.
 */
export async function reachablePatchPickerProvider(decorate) {
  const [listed, mine] = await Promise.all([fetchNodePages(), fetchNodePages('&scope=my')]);
  const byId = new Map();
  for (const n of [...listed, ...mine]) byId.set(n.id, n);
  return decorateNodes([...byId.values()], decorate);
}
