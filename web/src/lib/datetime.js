// Conversions between the UTC instants the API stores in `starts_at` and the
// wall-clock strings an <input type="datetime-local"> reads and writes.
//
// A datetime-local value carries no zone, so both directions have to name the
// one they mean. Today that is the browser's zone, which is what every
// rendering surface in the app already assumes. docs/adr/045 replaces that
// assumption with the event's own zone; these two functions are the seam
// where that change lands, which is why the pairing lives here rather than
// inline in the form.

const pad = (n) => String(n).padStart(2, '0');

// UTC instant -> "YYYY-MM-DDTHH:MM" read in the browser's zone.
//
// Slicing the ISO string instead (`iso.slice(0, 16)`) reads UTC digits into a
// control that means local, which moved the event by the editor's offset on
// every save.
export function toLocalInputValue(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

// "YYYY-MM-DDTHH:MM" in the browser's zone -> UTC instant.
//
// A date-time form with no offset is parsed as local by spec, which is the
// inverse of the above; a date-only form would be parsed as UTC, so the
// caller must pass a value that carries a time.
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
//
// The zone is the browser's, as everywhere else for now; docs/adr/045 moves
// it to the event's own, which is why this sits beside the conversions above.

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

// The first instant of a local day.
export function dayStart(d) {
  return startOfDay(d).toISOString();
}

// The last second of a local day, inclusive, because the API's `to` is `<=`.
// Derived from the next day's midnight rather than by adding 24 hours, so a
// day that is 23 or 25 hours long still ends where it should.
//
// Emitted without milliseconds on purpose. `starts_at` holds two precisions —
// the event form writes `.000Z`, feed ingest writes none — and the comparison
// is text, where 'Z' sorts after '.'. A `...59.999Z` bound would therefore
// have excluded a zero-fraction event landing on the day's last second.
// Dropping the fraction makes the bound the largest string that day can take.
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

// Bounds for one of the events list's date presets. `now` is injectable so
// the presets can be tested against a fixed day.
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
