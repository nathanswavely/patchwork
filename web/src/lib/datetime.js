/**
 * Date and time formatting, named once.
 *
 * These were re-derived on every page that needed them — six copies of
 * formatDate in three densities, five of formatTime in two unrelated
 * meanings. The duplication was harmless; the naming was not. A list row
 * and a detail headline genuinely want different densities, so the fix is
 * to name the densities rather than to collapse them into one function
 * with a flag.
 *
 * Everything renders in the *viewer's* browser timezone: times are stored
 * as UTC and no instance timezone exists anywhere in the stack, so a local
 * evening event can show on the wrong day to a viewer whose clock is set
 * elsewhere. That is a known gap, not a decision made here.
 */

/**
 * The `from` bound for a list that calls itself upcoming.
 *
 * GET /api/v1/events has no default lower bound and orders by starts_at
 * ascending, so a caller that omits `from` gets a patch's *oldest* events.
 * Three surfaces headed "upcoming events" did exactly that. It reads
 * correctly on a young instance, where every event is still ahead, and
 * silently inverts as a calendar ages.
 *
 * "Now", not the start of today, so this matches the server's
 * upcoming_event_count exactly — a list and a count that disagree is the
 * thing being fixed.
 */
export function upcomingFrom() {
  return new Date().toISOString();
}

/** "Sun, Jul 26" — list rows, cards, anywhere the year is implied. */
export function formatEventDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', {
    weekday: 'short', month: 'short', day: 'numeric',
  });
}

/** "Sun, Jul 26, 2026" — queues and admin tables, where rows span years. */
export function formatEventDateStamped(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', {
    weekday: 'short', month: 'short', day: 'numeric', year: 'numeric',
  });
}

/** "Sunday, July 26, 2026" — a detail page's headline, read once. */
export function formatEventDateLong(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', {
    weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
  });
}

/**
 * "Jul 26, 2026" — a calendar date with no weekday: records, audit rows,
 * "claimed on", "joined on". The weekday earns its place on something you
 * might attend and is noise on something that merely happened.
 */
export function formatDay(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
  });
}

/** "July 2026" — coarse enough that the day would overstate the precision. */
export function formatMonth(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'long', year: 'numeric',
  });
}

/** "8:00 PM". */
export function formatEventTime(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-US', {
    hour: 'numeric', minute: '2-digit',
  });
}

/**
 * "just now" / "5m ago" / "3d ago" — elapsed time, not clock time. Named
 * apart from formatEventTime on purpose: the two were both called
 * `formatTime` in different files and mean entirely different things.
 */
export function formatRelative(iso) {
  if (!iso) return '';
  const mins = Math.floor((Date.now() - new Date(iso)) / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return formatEventDate(iso);
}
