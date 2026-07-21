import { api } from '../lib/api.js';
import { loadMemberships, clearMemberships } from './memberships.svelte.js';

let user = $state(null);

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
  }
}
