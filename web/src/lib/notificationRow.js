// How a notification row reads: who sent it, what it says, and when.
//
// Not every notification comes from a patch, so the line above the title
// names whoever did send it, and the four senders are told apart by type:
//
// - a patch: its name, from the row's node_name (migration
//   notification_patch). Proposals, charters, members, notices, events.
// - another quilt: a followed patch there said something (docs/adr/024). The
//   row stores the remote patch's name as its body and the host in its link.
// - an instance queue the reader runs: "Admin".
// - this quilt, about the reader: moderation, account changes, answers to a
//   trust request or a suggestion, the bulletin. No line at all, because a
//   row with no sender reads as addressed to you, which is what these are.
//
// The type used to choose an icon instead (NotifIcon). The title already
// says the type, so the icon repeated it; the sender is what differs.

const ADMIN_TYPES = ['admin.', 'governance.succession_needed'];
const PERSONAL_TYPES = ['account.', 'report.', 'trust_request.', 'submission.', 'quilt.'];

function startsWithAny(type, prefixes) {
  return prefixes.some((p) => type.startsWith(p));
}

/**
 * The parts of a notification row as they are shown.
 * @returns {{ context: string, title: string, body: string }}
 */
export function describeNotification(n) {
  const type = n?.type || '';
  const title = n?.title || '';
  const body = n?.body || '';

  if (type.startsWith('remote.')) {
    const host = /^\/quilts\/([^/?]+)/.exec(n?.link || '')?.[1] || '';
    if (body && host) return { context: `${body} on ${host}`, title, body: '' };
    return { context: body || host, title, body: '' };
  }
  if (startsWithAny(type, ADMIN_TYPES)) return { context: 'Admin', title, body };
  if (startsWithAny(type, PERSONAL_TYPES)) return { context: '', title, body };
  return { context: n?.node_name || '', title, body };
}

/**
 * When a notification arrived, in words: it sits on a line of its own under
 * the row, so it has room to say "3 hours ago" rather than "3h". After a week
 * a date is more use than a count of days.
 */
export function notificationTime(iso, now = Date.now()) {
  if (!iso) return '';
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return '';
  const mins = Math.floor((now - then.getTime()) / 60000);
  if (mins < 1) return 'Just now';
  if (mins < 60) return mins === 1 ? '1 minute ago' : `${mins} minutes ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return hrs === 1 ? '1 hour ago' : `${hrs} hours ago`;
  const days = Math.floor(hrs / 24);
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days} days ago`;
  return then.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    ...(then.getFullYear() === new Date(now).getFullYear() ? {} : { year: 'numeric' }),
  });
}
