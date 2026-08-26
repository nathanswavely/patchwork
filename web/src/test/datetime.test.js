import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  toLocalInputValue,
  fromLocalInputValue,
  eventDateRange,
} from '../lib/datetime.js';

// The event edit form prefills a datetime-local input from a stored UTC
// instant and converts it back on save. Those two steps have to name the same
// zone or every open-and-save moves the event, so the round trip is the
// property worth pinning rather than either half's output.
describe('datetime-local round trip', () => {
  const instants = [
    '2026-07-23T00:00:00Z', // Lancaster 8pm EDT — the motivating case
    '2026-07-22T23:00:00.000Z', // millisecond precision, as the form emits
    '2026-01-15T18:30:00Z', // standard time, the other side of a DST boundary
    '2026-11-01T05:30:00Z', // during the US fall-back transition
  ];

  for (const iso of instants) {
    it(`preserves ${iso} across prefill and save`, () => {
      const once = fromLocalInputValue(toLocalInputValue(iso));
      expect(new Date(once).getTime()).toBe(new Date(iso).getTime());
    });

    it(`does not drift when ${iso} is edited repeatedly`, () => {
      let value = iso;
      for (let i = 0; i < 5; i++) {
        value = fromLocalInputValue(toLocalInputValue(value));
      }
      expect(new Date(value).getTime()).toBe(new Date(iso).getTime());
    });
  }

  it('reads the instant in the local zone, not off the UTC digits', () => {
    const iso = '2026-07-23T00:00:00Z';
    const d = new Date(iso);
    const expected =
      `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}` +
      `-${String(d.getDate()).padStart(2, '0')}` +
      `T${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    expect(toLocalInputValue(iso)).toBe(expected);
    // The old `iso.slice(0, 16)` agreed only for a viewer already on UTC.
    if (d.getTimezoneOffset() !== 0) {
      expect(toLocalInputValue(iso)).not.toBe(iso.slice(0, 16));
    }
  });
});

describe('empty and invalid values', () => {
  it('renders a missing end time as an empty input', () => {
    expect(toLocalInputValue(null)).toBe('');
    expect(toLocalInputValue(undefined)).toBe('');
    expect(toLocalInputValue('')).toBe('');
    expect(toLocalInputValue('not a date')).toBe('');
  });

  it('omits an empty end time from the request body', () => {
    expect(fromLocalInputValue('')).toBeUndefined();
    expect(fromLocalInputValue(null)).toBeUndefined();
  });

  it('does not throw on an unparseable input value', () => {
    expect(fromLocalInputValue('not a date')).toBeUndefined();
  });
});

describe('eventDateRange bounds a day with instants', () => {
  // A Thursday, mid-afternoon local.
  const now = new Date(2026, 6, 23, 15, 30);
  const range = (preset, opts) => eventDateRange(preset, { now, ...opts });

  // The bug: both bounds were bare dates, and `to` is compared with `<=`
  // against a full timestamp, so the named day sorted out of its own range.
  // With from == to that emptied the whole filter.
  it('returns a non-empty span for a single day', () => {
    for (const preset of ['today', 'tomorrow']) {
      const { from, to } = range(preset);
      expect(new Date(to).getTime()).toBeGreaterThan(new Date(from).getTime());
    }
  });

  it('admits an evening event on the last day of the range', () => {
    // 8pm local on the filtered day — the case that used to sort out.
    const evening = new Date(2026, 6, 23, 20, 0).toISOString();
    const { from, to } = range('today');
    expect(from <= evening && evening <= to).toBe(true);
  });

  it('admits an event at the very last instant of the day', () => {
    const lastMoment = new Date(2026, 6, 23, 23, 59, 59, 999).toISOString();
    const { to } = range('today');
    expect(lastMoment <= to).toBe(true);
  });

  // starts_at holds two precisions: the event form writes `.000Z`, feed
  // ingest writes none. Comparison is text and 'Z' sorts after '.', so a
  // `...59.999Z` bound excluded exactly the zero-fraction case — an imported
  // event landing on the day's last second.
  it('admits a zero-fraction timestamp on the last second', () => {
    const lastSecond = new Date(2026, 6, 23, 23, 59, 59, 0)
      .toISOString()
      .replace(/\.\d{3}Z$/, 'Z');
    const { to } = range('today');
    expect(lastSecond <= to).toBe(true);
    // The bound that used to be emitted would have sorted it out.
    const fractional = new Date(2026, 6, 24, 0, 0, 0, 0 - 1).toISOString();
    expect(lastSecond <= fractional).toBe(false);
  });

  it('excludes the next day', () => {
    const nextMidnight = new Date(2026, 6, 24, 0, 0, 0, 0).toISOString();
    const { to } = range('today');
    expect(nextMidnight <= to).toBe(false);
  });

  it('starts today at local midnight, not at a UTC date boundary', () => {
    const { from } = range('any');
    expect(from).toBe(new Date(2026, 6, 23, 0, 0, 0, 0).toISOString());
    expect(range('any').to).toBe('');
  });

  it('sends instants, never bare dates', () => {
    for (const preset of ['today', 'tomorrow', 'weekend', 'week', 'nextweek', 'month']) {
      const { from, to } = range(preset);
      expect(from).toMatch(/T.*Z$/);
      expect(to).toMatch(/T.*Z$/);
    }
  });

  it('reads a custom range in the local zone', () => {
    const { from, to } = range('custom', {
      customFrom: '2026-07-23',
      customTo: '2026-07-25',
    });
    // `new Date('2026-07-23')` would have parsed as UTC and shifted the day.
    expect(from).toBe(new Date(2026, 6, 23, 0, 0, 0, 0).toISOString());
    const lastMoment = new Date(2026, 6, 25, 23, 30).toISOString();
    expect(lastMoment <= to).toBe(true);
  });

  it('leaves the end open when a custom range has no end', () => {
    expect(range('custom', { customFrom: '2026-07-23' }).to).toBe('');
  });

  it('spans the whole month, last day included', () => {
    const { to } = range('month');
    const julyLast = new Date(2026, 6, 31, 22, 0).toISOString();
    expect(julyLast <= to).toBe(true);
    expect(new Date(2026, 7, 1, 0, 0).toISOString() <= to).toBe(false);
  });

  it('covers both weekend days', () => {
    const { from, to } = range('weekend');
    const sat = new Date(2026, 6, 25, 21, 0).toISOString();
    const sun = new Date(2026, 6, 26, 21, 0).toISOString();
    expect(from <= sat && sat <= to).toBe(true);
    expect(from <= sun && sun <= to).toBe(true);
  });
});

// The week presets ran on two different week-start conventions: "this week"
// ended on Saturday, "next week" began on Monday. Every Sunday therefore fell
// between them and was unreachable from either preset — on every day of the
// week, not just on Sundays. Weeks now start on Monday throughout, which also
// keeps a weekend inside one week instead of straddling two.
describe('week presets start weeks on Monday', () => {
  // Mon 2026-07-20 through Sun 2026-07-26; next week is Mon 27 – Sun Aug 2.
  const WEEK = [20, 21, 22, 23, 24, 25, 26];
  const DAY_NAME = {
    20: 'Monday', 21: 'Tuesday', 22: 'Wednesday', 23: 'Thursday',
    24: 'Friday', 25: 'Saturday', 26: 'Sunday',
  };

  // Noon on a given local day, as the instant an event that day would carry.
  const noon = (month, day) => new Date(2026, month, day, 12).toISOString();
  const covers = (r, iso) => r.from <= iso && (!r.to || iso <= r.to);

  for (const d of WEEK) {
    const now = new Date(2026, 6, d, 15, 0);
    const on = `on ${DAY_NAME[d]}`;

    it(`runs "this week" from today to Sunday ${on}`, () => {
      const r = eventDateRange('week', { now });
      expect(covers(r, noon(6, d))).toBe(true);
      expect(covers(r, noon(6, 26))).toBe(true); // Sunday, the week's last day
      expect(covers(r, noon(6, 27))).toBe(false); // next Monday belongs to next week
    });

    it(`runs "next week" Mon 27 to Sun Aug 2 ${on}`, () => {
      const r = eventDateRange('nextweek', { now });
      expect(covers(r, noon(6, 27))).toBe(true);
      expect(covers(r, noon(7, 2))).toBe(true);
      expect(covers(r, noon(6, 26))).toBe(false);
      expect(covers(r, noon(7, 3))).toBe(false);
    });

    // The regression that motivated the pass.
    it(`leaves no day unreachable between the two ${on}`, () => {
      const wk = eventDateRange('week', { now });
      const nx = eventDateRange('nextweek', { now });
      for (let day = d; day <= 26; day++) {
        expect(covers(wk, noon(6, day)) || covers(nx, noon(6, day))).toBe(true);
      }
      for (let day = 27; day <= 31; day++) {
        expect(covers(wk, noon(6, day)) || covers(nx, noon(6, day))).toBe(true);
      }
    });

    it(`points "this weekend" at Jul 25–26, never next weekend, ${on}`, () => {
      const r = eventDateRange('weekend', { now });
      expect(covers(r, noon(6, 26))).toBe(true); // Sunday is in it from any day
      expect(covers(r, noon(6, 25))).toBe(d <= 25); // Saturday, unless it has passed
      expect(covers(r, noon(7, 1))).toBe(false); // never a week ahead
    });

    it(`never starts a range before today ${on}`, () => {
      const midnight = new Date(2026, 6, d).toISOString();
      for (const preset of ['weekend', 'week', 'nextweek', 'month', 'any']) {
        expect(eventDateRange(preset, { now }).from >= midnight).toBe(true);
      }
    });
  }

  // A week whose Sunday is 23 or 25 hours long. Day arithmetic that adds
  // 24-hour multiples drifts across these; calendar arithmetic does not.
  for (const [label, month, sunday] of [
    ['spring forward', 2, 8], // Sun 2026-03-08 in the US
    ['fall back', 10, 1], // Sun 2026-11-01 in the US
  ]) {
    it(`covers a week containing a ${label} transition`, () => {
      const monday = new Date(2026, month, sunday - 6, 15, 0);
      const r = eventDateRange('week', { now: monday });
      for (let day = sunday - 6; day <= sunday; day++) {
        expect(covers(r, new Date(2026, month, day, 12).toISOString())).toBe(true);
      }
      // The transition day's evening is still inside the week.
      expect(covers(r, new Date(2026, month, sunday, 23, 0).toISOString())).toBe(true);
      // The Monday after is not.
      expect(covers(r, new Date(2026, month, sunday + 1, 0, 30).toISOString())).toBe(false);
    });
  }

  it('treats Sunday as the end of its week, not the start of the next', () => {
    const sunday = new Date(2026, 6, 26, 15, 0);
    // "This week" is Sunday alone, and next week still begins the next day.
    expect(covers(eventDateRange('week', { now: sunday }), noon(6, 26))).toBe(true);
    expect(covers(eventDateRange('nextweek', { now: sunday }), noon(6, 27))).toBe(true);
    // "This weekend" is today — the old arithmetic jumped a full week here.
    const weekend = eventDateRange('weekend', { now: sunday });
    expect(covers(weekend, noon(6, 26))).toBe(true);
    expect(covers(weekend, noon(7, 1))).toBe(false);
    expect(covers(weekend, noon(7, 2))).toBe(false);
  });
});

// There is no Svelte render library in this project (see
// patch-profile-window.test.js), so the wiring is asserted against source.
describe('EventForm uses the pairing on both sides', () => {
  const src = readFileSync(
    resolve(process.cwd(), 'src/pages/EventForm.svelte'),
    'utf8',
  );

  it('prefills and submits through the helpers, naming the same zone', () => {
    // The pairing is the property: both directions have to name one zone
    // or every open-and-save moves the event. Since docs/adr/045 that zone
    // is the event's rather than the editor's, so the halves are the
    // zoned helpers and both are passed `timezone`.
    expect(src).toContain("from '../lib/datetime.js'");
    expect(src).toContain('startsAt = toZonedInputValue(event.starts_at, timezone)');
    expect(src).toContain('endsAt = toZonedInputValue(event.ends_at, timezone)');
    expect(src).toContain('starts_at: fromZonedInputValue(startsAt, timezone)');
    expect(src).toContain('ends_at: fromZonedInputValue(endsAt, timezone)');
  });

  it('never reads or writes a form time in the browser’s zone', () => {
    // toLocalInputValue/fromLocalInputValue still exist and are still the
    // right answer for a form with no event behind it. On this form they
    // would silently reintroduce the editor's clock.
    expect(src).not.toMatch(/\btoLocalInputValue\(/);
    expect(src).not.toMatch(/\bfromLocalInputValue\(/);
  });

  it('never reads a datetime-local value off the raw ISO digits', () => {
    expect(src).not.toMatch(/starts_at\s*\.\s*slice/);
    expect(src).not.toMatch(/ends_at\s*\.\s*slice/);
  });

  it('binds both inputs to the state the helpers write', () => {
    expect(src).toContain('type="datetime-local" bind:value={startsAt}');
    expect(src).toContain('type="datetime-local" bind:value={endsAt}');
  });
});

describe('EventsPage filters through eventDateRange', () => {
  const src = readFileSync(
    resolve(process.cwd(), 'src/pages/EventsPage.svelte'),
    'utf8',
  );

  it('derives its range from the shared helper', () => {
    expect(src).toContain("from '../lib/datetime.js'");
    expect(src).toContain('eventDateRange(datePreset, { customFrom, customTo })');
  });

  it('keeps no local date arithmetic of its own', () => {
    expect(src).not.toContain('function getDateRange');
    expect(src).not.toMatch(/toISOString\(\)\.slice\(0,\s*10\)/);
  });
});
