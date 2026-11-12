import { formatDay } from './datetime.js';

/**
 * The line under an outstanding invitation on the admins' lists (F-044,
 * F-048): who asked, and when. Shared by the Members tab and Roles and
 * requests so the two say the same thing about the same row.
 *
 * `invited_by` is blank for an invitation made before the inviter was
 * recorded; that row says only when.
 *
 * @param {{ invited_at?: string, invited_by?: { display_name?: string, username?: string } }} person
 * @returns {string}
 */
export function invitationNote(person) {
  const by = person?.invited_by?.display_name || person?.invited_by?.username || '';
  const when = formatDay(person?.invited_at || '');
  if (by && when) return `Invited by ${by}, ${when}`;
  if (by) return `Invited by ${by}`;
  return when ? `Invited ${when}` : 'Invited';
}
