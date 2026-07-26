import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { toLocalInputValue, fromLocalInputValue } from '../lib/datetime.js';

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

// There is no Svelte render library in this project (see
// patch-profile-window.test.js), so the wiring is asserted against source.
describe('EventForm uses the pairing on both sides', () => {
  const src = readFileSync(
    resolve(process.cwd(), 'src/pages/EventForm.svelte'),
    'utf8',
  );

  it('prefills and submits through the helpers', () => {
    expect(src).toContain("from '../lib/datetime.js'");
    expect(src).toContain('startsAt = toLocalInputValue(event.starts_at)');
    expect(src).toContain('endsAt = toLocalInputValue(event.ends_at)');
    expect(src).toContain('starts_at: fromLocalInputValue(startsAt)');
    expect(src).toContain('ends_at: fromLocalInputValue(endsAt)');
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
