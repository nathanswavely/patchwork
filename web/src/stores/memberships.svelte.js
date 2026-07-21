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
 */
export function getMembershipRoles() {
  const map = new Map();
  for (const m of memberships) {
    map.set(m.node_slug, m.role);
  }
  return map;
}
