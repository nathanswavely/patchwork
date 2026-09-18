import { api } from '../lib/api.js';
import { loadMemberships, clearMemberships } from './memberships.svelte.js';

let user = $state(null);
// Whether the initial checkAuth() round-trip has resolved (success or
// failure). Routes that behave differently for logged-in vs logged-out
// visitors (e.g. /login) should wait on this before deciding what to
// render, so a session cookie that hasn't been validated yet doesn't
// cause a flash of the wrong content.
let authChecked = $state(false);

/**
 * Get the current user (reactive).
 */
export function getUser() {
  return user;
}

/**
 * Whether the user is logged in.
 */
export function isLoggedIn() {
  return user !== null;
}

/**
 * Whether the initial auth check has completed.
 */
export function isAuthChecked() {
  return authChecked;
}

/**
 * Whether the user is an admin.
 */
export function isAdmin() {
  return user?.role === 'admin';
}

/**
 * Whether the user holds the trusted-contributor grant (docs/adr/026):
 * an instance-level grant that reaches unclaimed patches only. Not a rung
 * between member and admin — never use it to widen anything on a claimed
 * patch.
 */
export function isTrustedContributor() {
  return user?.trusted_contributor === true;
}

/**
 * The per-patch scope of the same grant (docs/adr/2026-09-18-trust-has-a-
 * scope-and-a-suggestion-carries-its-calendar): the unclaimed patches this
 * person may speak for without holding the quilt-wide flag, as `auth/me`
 * lists them — `{ id, slug, name }`, already filtered to patches still
 * unclaimed. Memberships cannot carry this (an unclaimed patch admits
 * nobody), so it is the one place a client can enumerate that reach. Like
 * the flag, it widens nothing on a claimed patch; the server has already
 * dropped those, and a caller still checks status before acting.
 */
export function getTrustedPatches() {
  return user?.trusted_patches || [];
}

/**
 * Called after auth succeeds. Fetches user profile.
 */
export async function login() {
  try {
    user = await api('auth/me');
    loadMemberships();
  } catch {
    user = null;
  } finally {
    authChecked = true;
  }
}

/**
 * Log out the current user.
 */
export async function logout() {
  try {
    await api('auth/logout', { method: 'POST' });
  } catch {
    // Ignore errors
  }
  user = null;
  clearMemberships();
}

/**
 * Check auth on app init. Sets user if session is valid.
 */
export async function checkAuth() {
  try {
    user = await api('auth/me');
    loadMemberships();
  } catch {
    user = null;
  } finally {
    authChecked = true;
  }
}
