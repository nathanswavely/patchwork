/**
 * User memberships store.
 * Loads once on auth, provides lookup of user's relationship to patches.
 */
import { api } from '../lib/api.js';

let memberships = $state([]);
let loaded = $state(false);

/**
 * Patches the person has been invited into and has not answered
 * (docs/adr/098). Held apart from `memberships` on purpose: every consumer
 * of getMemberships() — the zero-memberships onboarding redirect in
 * App.svelte, the event form's host picker, the unlock panel — counts what
 * it holds as "my patches", and an invitation is not one. The server keeps
 * them apart the same way (me/nodes never serves an invited row; they are
 * users/me/invitations), so a caller here cannot inherit the mistake.
 */
let invitations = $state([]);

/**
 * Load the current user's memberships. Call after auth check.
 */
export async function loadMemberships() {
  const [mine, invited] = await Promise.all([
    api('me/nodes').catch(() => null),
    api('users/me/invitations').catch(() => null),
  ]);
  memberships = mine?.items || mine || [];
  // Filtered by status even though the endpoint serves nothing else, so a
  // stub that answers every path with the same rows cannot turn a
  // membership into an invitation.
  invitations = (invited?.items || []).filter((i) => i.status === 'invited');
  loaded = true;
}

/**
 * Clear memberships (on logout).
 */
export function clearMemberships() {
  memberships = [];
  invitations = [];
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

/**
 * Get a Set of slugs where the user holds an unanswered invitation
 * (docs/adr/098). Read by name, like a pending request: the relationship
 * row offers Accept and Decline here instead of Follow, and nothing else
 * consults it, because being asked in is not being in.
 */
export function getInvitedMembershipSlugs() {
  const set = new Set();
  for (const i of invitations) set.add(i.node_slug);
  return set;
}

/**
 * The invitations themselves — slug, patch name, when — for a surface that
 * lists them rather than checks one.
 */
export function getInvitations() {
  return invitations;
}
