/**
 * Dates and times, named once.
 *
 * Formatting was re-derived on every page that needed it — six copies of
 * formatDate in three densities, five of formatTime in two unrelated
 * meanings. The duplication was harmless; the naming was not. A list row
 * and a detail headline genuinely want different densities, so the fix is
 * to name the densities rather than to collapse them into one function
 * with a flag.
 *
 * Everything here reads and writes the *viewer's* browser timezone: times
 * are stored as UTC and no instance timezone exists anywhere in the stack,
 * so a local evening event can show on the wrong day to a viewer whose
 * clock is set elsewhere. docs/adr/045 decides that an event's time belongs
 * to the place it happens rather than to its reader, and this module is the
 * seam that change lands on — nothing below has been given a zone yet.
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
  // A bare YYYY-MM-DD is a calendar date, not an instant: it carries no zone,
  // so there is nothing to convert it *from*. `new Date('2026-03-14')` parses
  // it as UTC midnight, and west of Greenwich that renders as the 13th — an
  // attestation recorded for the day of a meeting would name the day before
  // it. Read the parts directly and let the month name be the only thing the
  // locale decides.
  const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (dateOnly) {
    const [, y, m, d] = dateOnly;
    return new Date(Number(y), Number(m) - 1, Number(d)).toLocaleDateString('en-US', {
      month: 'short', day: 'numeric', year: 'numeric',
    });
  }
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

// ---------------------------------------------------------------------------
// Reading and writing a <input type="datetime-local">.
//
// A datetime-local value carries no zone, so both directions have to name the
// one they mean, and they have to name the same one. Prefilling by slicing the
// stored ISO string (`iso.slice(0, 16)`) read UTC digits into a control that
// means local, which moved an event by the editor's offset on every save.

const pad = (n) => String(n).padStart(2, '0');

/** UTC instant -> "YYYY-MM-DDTHH:MM" read in the browser's zone. */
export function toLocalInputValue(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

/**
 * "YYYY-MM-DDTHH:MM" in the browser's zone -> UTC instant.
 *
 * A date-time form with no offset is parsed as local by spec, which is the
 * inverse of the above; a date-only form would be parsed as UTC, so the
 * caller must pass a value that carries a time.
 */
export function fromLocalInputValue(value) {
  if (!value) return undefined;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}

// ---------------------------------------------------------------------------
// Day ranges for the events list filter.
//
// A calendar day is a span in some zone, and the API compares `starts_at` as
// text (`events.go`, `e.starts_at <= ?`). So a day filter has to be sent as
// the two instants that bound it: `2026-07-26` sorts *before* every timestamp
// on 2026-07-26, so sending the bare date as `to` dropped the day it named —
// and when `from` and `to` were the same day, the whole range.

const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());

const shiftDays = (base, days) => {
  const d = new Date(base);
  d.setDate(d.getDate() + days);
  return d;
};

// Weeks start on Monday, so a weekend is the tail of one week rather than a
// thing split across two. That matters more here than the US convention of
// starting on Sunday does: "this weekend" is a first-class idea on an events
// calendar, and a Sunday-start week puts Saturday and Sunday in different
// weeks — which is exactly how "this week" (which used to end on Saturday)
// and "next week" (which started on Monday) came to disagree, leaving every
// Sunday in a gap that neither preset could reach.
//
// getDay() counts from Sunday and returns 0 for it, which is what broke the
// day arithmetic at the week's edges. isoWeekday counts from Monday.
const isoWeekday = (d) => (d.getDay() + 6) % 7; // Mon 0 … Sun 6

/** The first instant of a local day. */
export function dayStart(d) {
  return startOfDay(d).toISOString();
}

/**
 * The last second of a local day, inclusive, because the API's `to` is `<=`.
 * Derived from the next day's midnight rather than by adding 24 hours, so a
 * day that is 23 or 25 hours long still ends where it should.
 *
 * Emitted without milliseconds on purpose. `starts_at` holds two precisions —
 * the event form writes `.000Z`, feed ingest writes none — and the comparison
 * is text, where 'Z' sorts after '.'. A `...59.999Z` bound would therefore
 * have excluded a zero-fraction event landing on the day's last second.
 * Dropping the fraction makes the bound the largest string that day can take.
 */
export function dayEnd(d) {
  const next = new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1);
  return new Date(next.getTime() - 1000).toISOString().replace(/\.\d{3}Z$/, 'Z');
}

// "YYYY-MM-DD" from an <input type="date"> to a local Date. Passing it to
// `new Date()` directly would parse it as UTC — a date-only form is the one
// case the spec reads that way.
function fromDateInput(s) {
  const [y, m, d] = String(s || '').split('-').map(Number);
  if (!y || !m || !d) return null;
  const parsed = new Date(y, m - 1, d);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

/**
 * Bounds for one of the events list's date presets. `now` is injectable so
 * the presets can be tested against a fixed day.
 */
export function eventDateRange(preset, opts = {}) {
  const { customFrom = '', customTo = '', now = new Date() } = opts;
  const today = startOfDay(now);

  switch (preset) {
    case 'today':
      return { from: dayStart(today), to: dayEnd(today) };
    case 'tomorrow': {
      const tom = shiftDays(today, 1);
      return { from: dayStart(tom), to: dayEnd(tom) };
    }
    case 'weekend': {
      // This week's weekend, never next week's. On Sunday that weekend has
      // already begun, so it starts today rather than yesterday — the list
      // looks forward, and the old arithmetic skipped a whole week here.
      const saturday = shiftDays(today, 5 - isoWeekday(today));
      const start = saturday < today ? today : saturday;
      return { from: dayStart(start), to: dayEnd(shiftDays(saturday, 1)) };
    }
    case 'week':
      // Today through Sunday. On Sunday that is today alone, which is honest:
      // Sunday is the last day of its week, not the first of the next.
      return { from: dayStart(today), to: dayEnd(shiftDays(today, 6 - isoWeekday(today))) };
    case 'nextweek': {
      // The Monday after this week's Sunday, so the two presets meet exactly.
      const monday = shiftDays(today, 7 - isoWeekday(today));
      return { from: dayStart(monday), to: dayEnd(shiftDays(monday, 6)) };
    }
    case 'month': {
      const end = new Date(today.getFullYear(), today.getMonth() + 1, 0);
      return { from: dayStart(today), to: dayEnd(end) };
    }
    case 'custom': {
      const f = fromDateInput(customFrom);
      const t = fromDateInput(customTo);
      return { from: dayStart(f || today), to: t ? dayEnd(t) : '' };
    }
    default: // 'any'
      return { from: dayStart(today), to: '' };
  }
}
