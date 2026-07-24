// Unread notification count, shared between the global bar's bell and the
// mobile bottom-shelf item. The bell owns the polling (it mounts on every
// screen via GlobalBar); consumers just read.

let unread = $state(0);

export function getUnread() {
  return unread;
}

export function setUnread(n) {
  unread = n;
}
