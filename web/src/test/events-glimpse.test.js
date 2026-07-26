/**
 * The events glimpse on the patch profile, and the date/time vocabulary
 * underneath it (docs/adr/046, CONTEXT.md "Upcoming events").
 *
 * Formatters are pure and tested as such. Markup and CSS are asserted
 * against source text — there is no Svelte render library in this project
 * (see patch-profile-window.test.js).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  formatEventDate,
  formatEventDateStamped,
  formatEventDateLong,
  formatEventTime,
  formatRelative,
  upcomingFrom,
} from '../lib/datetime.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const AUG_2026 = '2026-08-12T19:30:00Z';

describe('the date densities are named, not re-derived', () => {
  it('gives three distinct densities', () => {
    const compact = formatEventDate(AUG_2026);
    const stamped = formatEventDateStamped(AUG_2026);
    const long = formatEventDateLong(AUG_2026);

    expect(compact).not.toContain('2026');
    expect(stamped).toContain('2026');
    expect(long).toContain('2026');
    // The long form spells its words out; the stamped one abbreviates.
    expect(long.length).toBeGreaterThan(stamped.length);
  });

  it('returns empty for a missing timestamp rather than "Invalid Date"', () => {
    for (const fn of [formatEventDate, formatEventDateStamped, formatEventDateLong, formatEventTime, formatRelative]) {
      expect(fn('')).toBe('');
      expect(fn(null)).toBe('');
      expect(fn(undefined)).toBe('');
    }
  });
});

describe('clock time and elapsed time are different functions', () => {
  // They were both called formatTime in different files. That is the bug
  // this naming fixes — deduplication was never the point.
  it('formats a clock time', () => {
    expect(formatEventTime(AUG_2026)).toMatch(/^\d{1,2}:\d{2}\s?(AM|PM)$/i);
  });

  it('formats elapsed time, counting from now', () => {
    expect(formatRelative(new Date().toISOString())).toBe('just now');
    expect(formatRelative(new Date(Date.now() - 5 * 60_000).toISOString())).toBe('5m ago');
    expect(formatRelative(new Date(Date.now() - 3 * 3_600_000).toISOString())).toBe('3h ago');
    expect(formatRelative(new Date(Date.now() - 4 * 86_400_000).toISOString())).toBe('4d ago');
  });

  it('falls back to a compact date once "ago" stops being useful', () => {
    const longAgo = new Date(Date.now() - 200 * 86_400_000).toISOString();
    expect(formatRelative(longAgo)).toBe(formatEventDate(longAgo));
  });
});

describe('a list that says upcoming asks for upcoming', () => {
  // GET /api/v1/events has no default lower bound and sorts ascending, so
  // omitting `from` returns a patch's *oldest* events under a heading that
  // promises the opposite.
  it('upcomingFrom is an instant, not a date', () => {
    expect(upcomingFrom()).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/);
  });

  it.each([
    ['pages/PatchProfile.svelte'],
    ['pages/Dashboard.svelte'],
    ['pages/RemotePatch.svelte'],
  ])('%s bounds its events fetch', (file) => {
    const src = source(file);
    expect(src).toContain('upcomingFrom');
    expect(src).toMatch(/events\?[^`'"]*from=/);
  });
});

describe('the event row stacks instead of clipping', () => {
  const src = source('pages/PatchProfile.svelte');

  // The old row was date | title | location on one line, where location
  // was flex-shrink: 0 and the title carried the ellipsis — so a postal
  // address squeezed the title to a single letter.
  it('puts date and time in their own column, title and location in another', () => {
    expect(src).toMatch(/<span class="event-when">[\s\S]*event-date[\s\S]*event-time[\s\S]*<\/span>/);
    expect(src).toMatch(/<span class="event-info">[\s\S]*event-name[\s\S]*event-location[\s\S]*<\/span>/);
  });

  it('lets the info block shrink, which is what makes the ellipsis work', () => {
    expect(src).toMatch(/\.event-info\s*\{[^}]*min-width:\s*0/);
  });

  it('clamps the location to one line rather than letting it push the title', () => {
    expect(src).toMatch(/\.event-location\s*\{[^}]*text-overflow:\s*ellipsis/);
    expect(src).toMatch(/\.event-location\s*\{[^}]*white-space:\s*nowrap/);
    expect(src).not.toMatch(/\.event-location\s*\{[^}]*flex-shrink:\s*0/);
  });

  it('leaves the governance rows on one line', () => {
    expect(src).toMatch(/\.row-item\s*\{[^}]*align-items:\s*center/);
    // .event-name used to share the ellipsis rule with .row-title; the two
    // now sit in different layouts and must not be re-merged.
    expect(src).not.toMatch(/\.event-name,\s*\n?\s*\.row-title/);
  });

  it('shows three events, not the five it used to fetch', () => {
    expect(src).toMatch(/GLIMPSE_EVENTS\s*=\s*3/);
    expect(src).toMatch(/limit=\$\{GLIMPSE_EVENTS\}/);
  });
});

describe('a count is a count, never the length of a capped page', () => {
  it('the profile reads the server total', () => {
    const src = source('pages/PatchProfile.svelte');
    expect(src).toMatch(/node\.upcoming_event_count[^}]*\}\s*Upcoming Events/);
    expect(src).not.toMatch(/recentEvents\.length\}?\s*Upcoming Events/);
  });

  it('SocialHome does not call an all-time count "upcoming"', () => {
    const src = source('pages/SocialHome.svelte');
    expect(src).not.toMatch(/event_count[^}]*\}\s*-?\s*Upcoming/i);
  });
});

describe('no page re-derives a formatter', () => {
  it('leaves date and time formatting to lib/datetime.js', () => {
    const dir = resolve(process.cwd(), 'src', 'pages');
    const offenders = readdirSync(dir)
      .filter((f) => f.endsWith('.svelte'))
      .filter((f) => /function\s+format(Date|Time|EventDate|EventTime)\s*\(/.test(
        readFileSync(resolve(dir, f), 'utf8'),
      ));
    expect(offenders).toEqual([]);
  });
});
