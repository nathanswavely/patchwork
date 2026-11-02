/**
 * Providers for the global bar's scoped finder (WorkspaceSearch).
 *
 * Two halves, because a context holds two kinds of thing. A provider returns
 * what is small and whole (a patch's charters, the admin panel's sections,
 * the quilt's tree): fetched once on first focus, cached for the visit,
 * filtered in the browser (docs/adr/005). A search function asks the server
 * for matches in what is paged (events, proposals, people, queues), once per
 * query, debounced by the component.
 *
 * The paged lists used to be fetched once and filtered here too. A paged
 * list fetched once is its first twenty rows, so a search inside a patch
 * with 264 events never found the twenty-first. Each entry:
 * { type, label, sublabel, href }.
 */
import { api } from './api.js';
import { formatDay, upcomingFrom } from './datetime.js';

// Matches asked for per kind of thing. The finder shows twelve rows, so
// four of each leaves room for the in-memory kinds beside them.
const PER_TYPE = 4;

export const WORKSPACE_TYPE_ORDER = ['People', 'Proposals', 'Documents', 'Events'];

/**
 * What is whole inside a patch workspace: its charters, one short list.
 */
export function workspaceFinderProvider(slug) {
  return async () => {
    const base = `/patches/${slug}`;
    const docs = await api(`nodes/${slug}/governance`).catch(() => null);

    const items = [];
    for (const d of docs?.items || []) {
      items.push({
        type: 'Documents',
        label: d.title,
        sublabel: '',
        href: `${base}/governance/docs/${d.id}`,
      });
    }
    return items;
  };
}

/**
 * The paged half of a patch workspace, asked per query. People land on their
 * profile pages (/users/:username). The members endpoint is viewer-aware and
 * searches inside its own roster gate, so the finder never reveals more than
 * the member list itself would (docs/adr/006).
 */
export function workspaceFinderSearch(slug) {
  return async (query) => {
    const base = `/patches/${slug}`;
    const q = encodeURIComponent(query);
    const node = encodeURIComponent(slug);
    const now = encodeURIComponent(upcomingFrom());
    const [proposals, upcoming, past, members] = await Promise.all([
      api(`nodes/${slug}/proposals?search=${q}&limit=${PER_TYPE}`).catch(() => null),
      // Searching a patch reaches what already happened too: you look up
      // last month's show by name as readily as next month's. Upcoming
      // matches lead, because the list sorts oldest first and a weekly night
      // would otherwise answer with its first four from years ago.
      api(`events?node_slug=${node}&from=${now}&search=${q}&limit=${PER_TYPE}`).catch(() => null),
      api(`events?node_slug=${node}&include_past=true&to=${now}&search=${q}&limit=${PER_TYPE}`).catch(() => null),
      api(`nodes/${slug}/members?search=${q}&limit=${PER_TYPE}`).catch(() => null),
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
    const events = [...(upcoming?.items || []), ...(past?.items || [])].slice(0, PER_TYPE);
    for (const e of events) {
      items.push({
        type: 'Events',
        label: e.title,
        sublabel: e.starts_at ? formatDay(e.starts_at) : '',
        href: `/events/${e.id}`,
      });
    }
    return items;
  };
}

/**
 * The paged half of the admin panel, asked per query: users, reports,
 * patch submissions. Users land on their profile pages; suspend/role actions
 * stay in the Users tab.
 */
export function adminFinderSearch() {
  return async (query) => {
    const q = encodeURIComponent(query);
    const [reports, submissions, users] = await Promise.all([
      api(`admin/reports?search=${q}&limit=${PER_TYPE}`).catch(() => null),
      api(`admin/submissions?search=${q}&limit=${PER_TYPE}`).catch(() => null),
      api(`admin/users?search=${q}&limit=${PER_TYPE}`).catch(() => null),
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
    return items;
  };
}

/**
 * What is whole in the admin panel: its own sections.
 */
export function adminFinderProvider() {
  return async () => {
    const items = [];
    // The panel's sections (docs/adr/2026-09-17-an-admin-tab-answers-one-question.md), a level down from the tab row,
    // so a typed name still lands on the page.
    const review = [
      { label: 'Reports', sublabel: 'the report queue', href: '/admin/review/reports' },
      { label: 'Patch submissions', sublabel: 'names proposed for the quilt', href: '/admin/review/submissions' },
      { label: 'Event submissions', sublabel: 'events proposed on unclaimed patches', href: '/admin/review/event-submissions' },
      { label: 'Claims', sublabel: 'people claiming an unclaimed patch', href: '/admin/review/claims' },
      { label: 'Suggested tags', sublabel: 'words patch admins asked for', href: '/admin/review/tags' },
    ];
    for (const s of review) items.push({ type: 'Review', ...s });
    const settings = [
      { label: 'Quilt Settings', sublabel: 'rename, icon, export, danger zone', href: '/admin/settings/quilt' },
      { label: 'Label', sublabel: 'who runs this quilt and what it costs', href: '/admin/settings/label' },
      { label: 'Legal', sublabel: 'privacy policy and terms', href: '/admin/settings/legal' },
      { label: 'Tags', sublabel: 'the tag vocabulary and per-tag motifs', href: '/admin/settings/tags' },
      { label: 'Neighbors', sublabel: 'quilts listed beside this one', href: '/admin/settings/neighbors' },
      { label: 'Aggregators', sublabel: 'calendar feeds routed onto patches', href: '/admin/settings/aggregators' },
      { label: 'Outreach', sublabel: 'intro emails to unclaimed patches’ owners', href: '/admin/settings/outreach' },
      { label: 'Usage', sublabel: 'daily page views and visitors, counted on the server', href: '/admin/settings/usage' },
      { label: 'Archived patches', sublabel: 'restore a patch that was archived', href: '/admin/settings/archived' },
      { label: 'Prove admin', sublabel: 'sign an outside party’s nonce as this quilt', href: '/admin/settings/attestation' },
      { label: 'Apps', sublabel: 'native apps this quilt vouches for', href: '/admin/settings/apps' },
    ];
    for (const s of settings) items.push({ type: 'Settings', ...s });
    return items;
  };
}

export const DISCOVERY_TYPE_ORDER = ['Patches', 'Events'];

/**
 * The discovery corpus (docs/adr/033): every public patch, from the tree,
 * which is whole. Deliberately no people (no instance-wide people search;
 * people are discovered through patches).
 */
export function discoveryFinderProvider() {
  return async () => {
    const treeResp = await api('nodes/tree').catch(() => null);

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
    return items;
  };
}

/**
 * Upcoming events matching a query, quilt-wide. Deliberately not the deep
 * past: the events page's date controls are the tool for archaeology. This
 * used to read the first hundred upcoming events once and filter them, which
 * on a busy quilt is a few weeks of calendar.
 */
export function discoveryFinderSearch() {
  return async (query) => {
    const today = new Date().toISOString().slice(0, 10);
    const events = await api(`events?from=${today}&search=${encodeURIComponent(query)}&limit=${PER_TYPE}`).catch(() => null);

    const items = [];
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
