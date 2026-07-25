// Unread notification count, shared between the global bar's bell and the
// mobile bottom-shelf item. The bell owns the polling (it mounts on every
// screen via GlobalBar); consumers read it, and every surface that marks
// something read tells this module so the badge drops immediately.
//
// The badge used to move only on the poll, so reading a notification left it
// sitting there for up to a minute — long enough to read as broken (issue
// #55). Marking read is a local fact; the poll is just reconciliation.

import { api } from '../lib/api.js';

let unread = $state(0);

export function getUnread() {
  return unread;
}

export function setUnread(n) {
  unread = Math.max(0, n || 0);
}

// Pull the authoritative count. Silent on failure — a bell without a badge
// beats an error for something nobody asked for.
export async function refreshUnread() {
  try {
    const data = await api('notifications/count');
    setUnread(data.unread || 0);
  } catch {
    // Leave the last known count in place.
  }
}

// One notification just went from unread to read.
export function decrementUnread(n = 1) {
  unread = Math.max(0, unread - n);
}

// Everything is read — "mark all read" clears the whole table server-side,
// not just the page in view, so the count goes to zero rather than down by
// the number of rows currently rendered.
export function clearUnread() {
  unread = 0;
}
