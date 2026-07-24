// Cross-quilt state (docs/adr/024). Connected quilts and remote follows
// are account-backed rows on the person's home instance; the registry
// param is a session-only overlay (a discovery flyer, never persisted).
import { api } from '../lib/api.js';
import { colorForTag } from '../lib/quiltTheme.js';

let connectedQuilts = $state([]); // personal, from /users/me/quilts
let registryQuilts = $state([]); // session-only overlay from ?registry=
let remoteFollows = $state([]); // from /users/me/remote-follows
let loaded = $state(false);

// Per-origin instance document cache: url -> {ok, info} promise results.
const infoCache = new Map();

export function getConnectedQuilts() {
  return connectedQuilts;
}

export function getRegistryQuilts() {
  return registryQuilts;
}

export function getRemoteFollows() {
  return remoteFollows;
}

export function isMultiQuiltLoaded() {
  return loaded;
}

export function setRegistryQuilts(entries) {
  registryQuilts = (entries || []).map((e) => ({
    url: normalizeOrigin(e.url),
    name: e.name || '',
    tags: e.tags || [],
  }));
}

export function normalizeOrigin(url) {
  try {
    const u = new URL(url.trim());
    return `${u.protocol}//${u.host}`;
  } catch {
    return url.trim().replace(/\/+$/, '');
  }
}

/** Load account-backed cross-quilt state. No-op for anonymous visitors. */
export async function loadMultiQuilt() {
  try {
    const [quiltsResp, followsResp] = await Promise.all([
      api('users/me/quilts'),
      api('users/me/remote-follows'),
    ]);
    connectedQuilts = quiltsResp.quilts || [];
    remoteFollows = followsResp.remote_follows || [];
  } catch {
    connectedQuilts = [];
    remoteFollows = [];
  } finally {
    loaded = true;
  }
}

export function clearMultiQuilt() {
  connectedQuilts = [];
  remoteFollows = [];
  loaded = false;
}

export async function connectQuilt(url, name = '') {
  const quilt = await api('users/me/quilts', { method: 'POST', body: { url, name } });
  if (!connectedQuilts.some((q) => q.id === quilt.id)) {
    connectedQuilts = [...connectedQuilts, quilt];
  } else {
    connectedQuilts = connectedQuilts.map((q) => (q.id === quilt.id ? quilt : q));
  }
  return quilt;
}

export async function disconnectQuilt(id) {
  await api(`users/me/quilts/${id}`, { method: 'DELETE' });
  connectedQuilts = connectedQuilts.filter((q) => q.id !== id);
}

/** Follow a patch on another quilt. The row lives at home (docs/adr/024). */
export async function followRemotePatch({ quiltUrl, node }) {
  const follow = await api('users/me/remote-follows', {
    method: 'POST',
    body: {
      quilt_url: quiltUrl,
      node_ap_id: node.ap_id || `${normalizeOrigin(quiltUrl)}/ap/nodes/${node.id}`,
      node_slug: node.slug,
      node_name: node.name || '',
      snapshot: snapshotFromNode(node),
    },
  });
  remoteFollows = [...remoteFollows.filter((f) => f.id !== follow.id), follow];
  return follow;
}

export async function unfollowRemotePatch(id) {
  await api(`users/me/remote-follows/${id}`, { method: 'DELETE' });
  remoteFollows = remoteFollows.filter((f) => f.id !== id);
}

export function findRemoteFollow(quiltUrl, slug) {
  const origin = normalizeOrigin(quiltUrl);
  return remoteFollows.find((f) => f.quilt_url === origin && f.node_slug === slug) || null;
}

/** The display snapshot stored with a follow — enough to draw the tile
 * while the remote quilt is unreachable. */
export function snapshotFromNode(node) {
  return {
    appearance: node.appearance || null,
    tags: node.tags || [],
    icon: node.icon || '',
    description: (node.description || '').slice(0, 200),
    member_count: node.member_count || 0,
    event_count: node.event_count || 0,
    is_unclaimed: !!node.is_unclaimed,
  };
}

/** Opportunistic snapshot refresh after a successful remote fetch. */
export function refreshFollowSnapshot(follow, node) {
  const snapshot = snapshotFromNode(node);
  const changed =
    JSON.stringify(snapshot) !== JSON.stringify(follow.snapshot || {}) ||
    (node.name && node.name !== follow.node_name);
  if (!changed) return;
  follow.snapshot = snapshot;
  if (node.name) follow.node_name = node.name;
  api(`users/me/remote-follows/${follow.id}`, {
    method: 'PATCH',
    body: { snapshot, node_name: node.name || follow.node_name },
  }).catch(() => {});
}

/** Fetch (and cache) another quilt's public instance document. */
export async function fetchQuiltInfo(url) {
  const origin = normalizeOrigin(url);
  if (infoCache.has(origin)) return infoCache.get(origin);
  const promise = fetch(`${origin}/api/v1/instance`)
    .then((res) => (res.ok ? res.json() : null))
    .catch(() => null);
  infoCache.set(origin, promise);
  return promise;
}

export function clearQuiltInfoCache() {
  infoCache.clear();
}

/** A quilt's identity color for sashing and source chips: its own
 * branding color when it declares one, else a stable hash color. */
export function colorForQuilt(url, info = null) {
  if (info?.branding?.color) return info.branding.color;
  return colorForTag(normalizeOrigin(url));
}

/** The switcher's full list: admin-curated neighbors first, then
 * personal connected quilts, then session registry entries — deduped by
 * origin, home excluded. */
export function switcherQuilts(neighborQuilts, homeOrigin) {
  const seen = new Set([normalizeOrigin(homeOrigin || window.location.origin)]);
  const out = [];
  for (const [list, kind] of [
    [neighborQuilts || [], 'neighbor'],
    [connectedQuilts, 'connected'],
    [registryQuilts, 'registry'],
  ]) {
    for (const q of list) {
      const origin = normalizeOrigin(q.url);
      if (seen.has(origin)) continue;
      seen.add(origin);
      out.push({ ...q, url: origin, kind });
    }
  }
  return out;
}
