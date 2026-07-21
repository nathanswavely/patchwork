// My Quilt regions (docs/adr/024): remote follows render grouped by
// source quilt, never intermingled with home patches. Each group is its
// own quiltLayout pack; regions sit side by side framed by sashing.
import { quiltLayout } from './quiltLayout.js';

/** Fetch a remote quilt's public tree cross-origin. Returns
 * {tree, affinity} or throws (unreachable / CORS-declined). */
export async function fetchRemoteTree(origin) {
  const res = await fetch(`${origin}/api/v1/nodes/tree`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const resp = await res.json();
  if (resp.tree) return { tree: resp.tree, affinity: resp.affinity || [] };
  return { tree: resp, affinity: [] };
}

/** Build the per-source-quilt groups for My Quilt. `follows` are the
 * person's remote_follows rows grouped by quilt_url; each group's
 * children come from the quilt's live public tree filtered to followed
 * slugs, falling back to stored snapshots when the quilt is unreachable
 * (snapshot tiles, marked — docs/adr/024). Returns group descriptors;
 * onLiveNode fires per matched live node for snapshot refreshing. */
export async function buildRemoteGroups(follows, { fetchInfo, colorFor, onLiveNode } = {}) {
  const byQuilt = new Map();
  for (const f of follows) {
    if (!byQuilt.has(f.quilt_url)) byQuilt.set(f.quilt_url, []);
    byQuilt.get(f.quilt_url).push(f);
  }

  const groups = await Promise.all(
    [...byQuilt.entries()].map(async ([origin, quiltFollows]) => {
      const info = fetchInfo ? await fetchInfo(origin) : null;
      const name = info?.name || origin.replace(/^https?:\/\//, '');
      const color = colorFor ? colorFor(origin, info) : '#888';

      let children = null;
      let reachable = false;
      try {
        const { tree } = await fetchRemoteTree(origin);
        const bySlug = new Map((tree.children || []).map((n) => [n.slug, n]));
        reachable = true;
        children = quiltFollows.map((f) => {
          const live = bySlug.get(f.node_slug);
          if (live) {
            if (onLiveNode) onLiveNode(f, live);
            return { ...live, _source: origin, _follow: f };
          }
          // Reachable quilt, but the patch is gone from public view —
          // deleted and gone-private look identical from outside, so
          // render the snapshot, marked; never auto-unfollow.
          return snapshotNode(f, origin, { missing: true });
        });
      } catch {
        children = quiltFollows.map((f) => snapshotNode(f, origin, { missing: false }));
      }

      return { key: origin, name, color, reachable, children };
    })
  );

  return groups.filter((g) => g.children.length > 0);
}

function snapshotNode(follow, origin, { missing }) {
  const snap = follow.snapshot || {};
  return {
    id: follow.node_ap_id,
    slug: follow.node_slug,
    name: follow.node_name || follow.node_slug,
    description: snap.description || '',
    tags: snap.tags || [],
    icon: snap.icon || '',
    appearance: snap.appearance || null,
    member_count: snap.member_count || 0,
    event_count: snap.event_count || 0,
    is_unclaimed: !!snap.is_unclaimed,
    _source: origin,
    _follow: follow,
    _snapshot: true,
    _missing: missing,
  };
}

/** Sashing geometry: gap between regions, in grid columns. */
const REGION_GAP_COLS = 2;

/** Run quiltLayout per group and compose the packs side by side into one
 * layout-shaped object ({tiles, minCol, minRow, maxCol, maxRow}) plus
 * per-group grid bounds for sashing. Groups are vertically centered
 * against the tallest region. */
export function composeGroupLayouts(groups, affinityByGroup = new Map(), fixedSizes = null) {
  const packed = groups.map((g) => {
    const layout = quiltLayout(g.children, affinityByGroup.get(g.key) || [], fixedSizes || undefined);
    return { group: g, layout, cols: layout.maxCol - layout.minCol, rows: layout.maxRow - layout.minRow };
  });

  const maxRows = Math.max(...packed.map((p) => p.rows), 1);
  const tiles = [];
  const groupBounds = [];
  let colOffset = 0;

  for (const p of packed) {
    const rowShift = Math.floor((maxRows - p.rows) / 2) - p.layout.minRow;
    const colShift = colOffset - p.layout.minCol;
    for (const t of p.layout.tiles) {
      tiles.push({
        ...t,
        // Filler ids repeat per pack — namespace them by group so the
        // canvas tileMap stays collision-free.
        id: t.isFiller ? `${p.group.key}:${t.id}` : t.id,
        gridPos: { col: t.gridPos.col + colShift, row: t.gridPos.row + rowShift },
        _groupKey: p.group.key,
      });
    }
    groupBounds.push({
      key: p.group.key,
      name: p.group.name,
      color: p.group.color,
      reachable: p.group.reachable,
      home: !!p.group.home,
      minCol: colOffset,
      maxCol: colOffset + p.cols,
      minRow: Math.floor((maxRows - p.rows) / 2),
      maxRow: Math.floor((maxRows - p.rows) / 2) + p.rows,
    });
    colOffset += p.cols + REGION_GAP_COLS;
  }

  return {
    tiles,
    minCol: 0,
    minRow: 0,
    maxCol: Math.max(colOffset - REGION_GAP_COLS, 1),
    maxRow: maxRows,
    groups: groupBounds,
  };
}
