/**
 * User memberships store.
 * Loads once on auth, provides lookup of user's relationship to patches.
 */
import { api } from '../lib/api.js';

let memberships = $state([]);
let loaded = $state(false);

/**
 * Load the current user's memberships. Call after auth check.
 */
export async function loadMemberships() {
  try {
    const data = await api('me/nodes');
    memberships = data.items || data || [];
  } catch {
    memberships = [];
  }
  loaded = true;
}

/**
 * Clear memberships (on logout).
 */
export function clearMemberships() {
  memberships = [];
  loaded = false;
}

/**
 * Get all memberships.
 */
export function getMemberships() {
  return memberships;
}

/**
 * Whether memberships have been loaded.
 */
export function isMembershipsLoaded() {
  return loaded;
}

/**
 * Get a Map of slug → role for the user's memberships.
 *
 * Active rows only. `me/nodes` also serves pending join requests, and a
 * requester is outside the ladder: they hold an open request, not a
 * relationship, so they have no role (docs/adr/088).
 *
 * The server no longer sends one either — `role` is omitted for a row
 * that is not active, matching what `GET /nodes/{slug}` always did. This
 * filter is the second line rather than the only one: without it an
 * absent role would land in the map as undefined, and a store that states
 * standing should decide what it holds rather than inherit it.
 *
 * A pending request is not standing, so it is not a role — ask for it by
 * name with getPendingMembershipSlugs().
 */
export function getMembershipRoles() {
  const map = new Map();
  for (const m of memberships) {
    if (m.status !== 'active') continue;
    map.set(m.node_slug, m.role);
  }
  return map;
}

/**
 * Get a Set of slugs where the user has an unanswered join request.
 *
 * The information getMembershipRoles() deliberately drops, kept where a
 * caller can ask for it: a surface offering "Follow" or "Become a member"
 * to someone already waiting on an answer is offering a control the server
 * refuses with a 409 (docs/adr/042 — an absent door beats a 403).
 */
export function getPendingMembershipSlugs() {
  const set = new Set();
  for (const m of memberships) {
    if (m.status === 'pending') set.add(m.node_slug);
  }
  return set;
}
