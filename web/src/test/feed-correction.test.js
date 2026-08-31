import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { reinterpretUTCAsLocal, formatEventTime } from '../lib/datetime.js';

// The defect this corrects, taken from Tellus360's own markup: the page
// says "21+ | 7pm" and the schema.org block says "2026-08-28T15:00:00-04:00"
// — 19:00Z. A faithful rendering of the wrong instant, which is why every
// parser believed it.
describe('correcting a publisher that stamps local time as UTC', () => {
  const NY = 'America/New_York';

  it('turns the published instant back into the clock the venue meant', () => {
    expect(reinterpretUTCAsLocal('2026-08-28T19:00:00Z', NY))
      .toBe('2026-08-28T23:00:00.000Z');
    expect(formatEventTime('2026-08-28T23:00:00.000Z', NY)).toMatch(/^7:00 PM/);
  });

  it('reads the same for a feed that printed its offset', () => {
    // 15:00-04:00 and 19:00Z are the same instant; the correction cannot
    // depend on which spelling arrived.
    expect(reinterpretUTCAsLocal('2026-08-28T15:00:00-04:00', NY))
      .toBe(reinterpretUTCAsLocal('2026-08-28T19:00:00Z', NY));
  });

  // The reason this is a switch and not a number of hours.
  it('moves by four hours in summer and five in winter', () => {
    const summer = reinterpretUTCAsLocal('2026-08-28T19:00:00Z', NY);
    const winter = reinterpretUTCAsLocal('2026-01-15T19:00:00Z', NY);
    expect(new Date(summer) - new Date('2026-08-28T19:00:00Z')).toBe(4 * 3600 * 1000);
    expect(new Date(winter) - new Date('2026-01-15T19:00:00Z')).toBe(5 * 3600 * 1000);
    // And 7pm both times, which is the property a fixed offset can't hold.
    expect(formatEventTime(summer, NY)).toMatch(/^7:00 PM/);
    expect(formatEventTime(winter, NY)).toMatch(/^7:00 PM/);
  });

  it('leaves values it cannot read, and does nothing without a zone', () => {
    expect(reinterpretUTCAsLocal('', NY)).toBe('');
    expect(reinterpretUTCAsLocal('not-a-time', NY)).toBe('not-a-time');
    expect(reinterpretUTCAsLocal('2026-08-28T19:00:00Z', '')).toBe('2026-08-28T19:00:00Z');
  });

  it('agrees with the Go side, which is the one that actually rewrites', () => {
    // internal/eventsource/stamped_utc_test.go pins these same two cases.
    // If the two ever disagree the preview lies about what a sync will do.
    expect(reinterpretUTCAsLocal('2026-08-28T19:00:00Z', NY))
      .toBe(new Date('2026-08-28T23:00:00Z').toISOString());
    expect(reinterpretUTCAsLocal('2026-01-15T19:00:00Z', NY))
      .toBe(new Date('2026-01-16T00:00:00Z').toISOString());
  });
});

// No render library here, so the wiring is asserted against source.
describe('the source settings page offers the correction', () => {
  const src = readFileSync(
    resolve(process.cwd(), 'src/pages/PatchSettingsSources.svelte'),
    'utf8',
  );

  it('binds the checkbox to the stored flag, not to local state', () => {
    expect(src).toContain('checked={source.local_time_stamped_utc}');
    expect(src).toContain('setStampedUTC(source.id, e.target.checked)');
  });

  it('saves through PATCH on the source', () => {
    expect(src).toContain("method: 'PATCH'");
    expect(src).toContain('local_time_stamped_utc: value');
  });

  it('previews against a real event before anything is saved', () => {
    expect(src).toContain('previewFor(source)');
    // Both halves rendered in the patch's zone — "3:00 PM to 7:00 PM" is
    // what an admin can check against the venue's page; instants are not.
    expect(src).toContain('formatEventTime(source.sample_starts_at, source.timezone)');
  });

  it('does not offer a preview for a source already corrected', () => {
    expect(src).toContain('if (source.local_time_stamped_utc) return null;');
  });
});
