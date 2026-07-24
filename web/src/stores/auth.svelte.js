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
