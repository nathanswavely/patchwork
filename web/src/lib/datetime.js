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
 * An event's time belongs to the place it happens, not to its reader
 * (docs/adr/045). Every formatter that renders an *event* therefore takes
 * the event's zone as a second argument and goes through Intl with a
 * `timeZone`, rather than letting `toLocaleString` reach for whatever the
 * browser is set to. The API resolves that zone before it sends it — event,
 * else patch, else instance — so `tz` is never empty here and this module
 * never reimplements the fallback.
 *
 * Passing no zone still works and still means the viewer's, which is what
 * the record formatters below want: "joined on", "claimed on", an audit
 * row. Those are facts about the reader's own account and history, not
 * about a gathering somewhere.
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

/**
 * Options for an event formatter, with the zone applied only when one was
 * given. Passing `timeZone: undefined` is the same as omitting it, but
 * passing a bad name throws a RangeError that would take the page with it —
 * so an unresolvable zone falls back to the viewer's rather than blanking
 * a list. The server validates on the way in; this guards the case where a
 * stored zone outlives a tzdata rename.
 */
function withZone(opts, tz) {
  if (!tz) return opts;
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz });
  } catch {
    return opts;
  }
  return { ...opts, timeZone: tz };
}

/** "Sun, Jul 26" — list rows, cards, anywhere the year is implied. */
export function formatEventDate(iso, tz) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', withZone({
    weekday: 'short', month: 'short', day: 'numeric',
  }, tz));
}

/** "Sun, Jul 26, 2026" — queues and admin tables, where rows span years. */
export function formatEventDateStamped(iso, tz) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', withZone({
    weekday: 'short', month: 'short', day: 'numeric', year: 'numeric',
  }, tz));
}

/** "Sunday, July 26, 2026" — a detail page's headline, read once. */
export function formatEventDateLong(iso, tz) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-US', withZone({
    weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
  }, tz));
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

/**
 * "8:00 PM" at home, "8:00 PM EDT" when the event's zone is not the
 * reader's.
 *
 * Annotating every time teaches the reader to ignore the annotation;
 * annotating only the surprising ones is what makes a merged cross-quilt
 * feed legible (docs/adr/024). A Lancaster reader browsing Lancaster sees
 * no zone anywhere, which is correct — they are not doing a conversion.
 */
export function formatEventTime(iso, tz) {
  if (!iso) return '';
  const base = { hour: 'numeric', minute: '2-digit' };
  if (!tz || sameZoneAsViewer(tz)) {
    return new Date(iso).toLocaleTimeString('en-US', withZone(base, tz));
  }
  return new Date(iso).toLocaleTimeString('en-US',
    withZone({ ...base, timeZoneName: 'short' }, tz));
}

/**
 * Whether an event's zone is the one the reader is already in.
 *
 * Compared by name first, because that is the common case and is exact.
 * Two different names can still be the same clock — America/New_York and
 * America/Detroit never disagree — so a name mismatch falls through to
 * comparing what the two zones actually render for this moment. Otherwise
 * a reader in Detroit would see "EDT" stamped on every Lancaster event, so
 * the annotation would be noise exactly where it claims to be signal.
 */
export function sameZoneAsViewer(tz, at = Date.now()) {
  if (!tz) return true;
  let viewer;
  try {
    viewer = Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    return true;
  }
  if (!viewer || viewer === tz) return true;
  try {
    const opts = { dateStyle: 'short', timeStyle: 'long' };
    return new Intl.DateTimeFormat('en-US', { ...opts, timeZone: tz }).format(at)
      === new Intl.DateTimeFormat('en-US', { ...opts, timeZone: viewer }).format(at);
  } catch {
    return true;
  }
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

/**
 * The offset, in milliseconds, that `tz` was at the instant `t`.
 *
 * Derived by asking Intl what wall clock `tz` shows for `t` and treating
 * that reading as if it were UTC: the difference between the two is the
 * offset. There is no API that just says this, and the arithmetic has to
 * come from the same table that renders the event, or the form and the
 * page would disagree twice a year.
 */
function zoneOffsetMs(tz, t) {
  let parts;
  try {
    parts = new Intl.DateTimeFormat('en-US', {
      timeZone: tz, hour12: false,
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    }).formatToParts(new Date(t));
  } catch {
    return 0;
  }
  const g = {};
  for (const { type, value } of parts) g[type] = value;
  // Intl renders midnight as hour 24 under hour12:false in some engines.
  const hour = g.hour === '24' ? '00' : g.hour;
  const asIfUTC = Date.UTC(
    Number(g.year), Number(g.month) - 1, Number(g.day),
    Number(hour), Number(g.minute), Number(g.second),
  );
  return asIfUTC - t;
}

/**
 * UTC instant -> "YYYY-MM-DDTHH:MM" as the clock reads in `tz`.
 *
 * An organizer editing a Lancaster show should see 8:00 PM in the box
 * whether they are in Lancaster or on tour, because 8pm is the fact they
 * are editing. Without a zone this falls back to the browser's, which is
 * what a form with no event context still wants.
 */
export function toZonedInputValue(iso, tz) {
  if (!iso) return '';
  if (!tz) return toLocalInputValue(iso);
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '';
  const shifted = new Date(t + zoneOffsetMs(tz, t));
  return (
    `${shifted.getUTCFullYear()}-${pad(shifted.getUTCMonth() + 1)}-${pad(shifted.getUTCDate())}` +
    `T${pad(shifted.getUTCHours())}:${pad(shifted.getUTCMinutes())}`
  );
}

/**
 * "YYYY-MM-DDTHH:MM" as the clock reads in `tz` -> UTC instant.
 *
 * Resolved twice on purpose. The first pass guesses the offset using the
 * wall clock read as if it were UTC, which is wrong by at most an hour;
 * the second pass re-reads the offset at that corrected instant, which
 * settles it. One pass alone picks the wrong side of a DST boundary for
 * evening events on the two nights a year the clocks move — exactly the
 * shows most likely to be affected.
 */
export function fromZonedInputValue(value, tz) {
  if (!value) return undefined;
  if (!tz) return fromLocalInputValue(value);
  const asIfUTC = Date.parse(`${value}:00Z`);
  if (Number.isNaN(asIfUTC)) return fromLocalInputValue(value);
  let t = asIfUTC - zoneOffsetMs(tz, asIfUTC);
  t = asIfUTC - zoneOffsetMs(tz, t);
  return new Date(t).toISOString();
}

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

// ---------------------------------------------------------------------------
// Previewing a feed correction (docs/adr/073).

/**
 * What an instant becomes when a publisher's local-time-stamped-as-UTC
 * defect is corrected: take the UTC wall clock and read those same digits
 * in `tz`.
 *
 * The Go side (`eventsource.ReinterpretUTCAsLocal`) is what actually
 * rewrites the times on sync; this exists only so the settings page can
 * show an admin what the switch would do to a real event before they save
 * it. Kept deliberately small and equivalent — if the two ever disagree,
 * the server is right.
 */
export function reinterpretUTCAsLocal(iso, tz) {
  if (!iso || !tz) return iso;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const wall =
    `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}` +
    `T${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
  return fromZonedInputValue(wall, tz) || iso;
}
