/**
 * Three things an events page said that were not true.
 *
 * F-074 — setting a patch's timezone quietly changed what every event
 * read as. The instants never moved; the readings did (docs/adr/067), and
 * nothing said a word. The page now asks, once, with the count in it.
 * F-075 — "Repeats weekly" was a word the product could not keep, so the
 * control is gone and a stored word renders with its caveat.
 * F-076 — "4 Upcoming Events" over a list of three.
 *
 * There is no Svelte render library in this project, so component wiring
 * is asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { isPlaceZone } from '../lib/datetime.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('isPlaceZone — a zone is a place, not an offset', () => {
  it('takes IANA area/location names, and UTC', () => {
    for (const tz of ['America/New_York', 'Europe/Berlin', 'Pacific/Auckland', 'UTC', 'Etc/UTC']) {
      expect(isPlaceZone(tz), tz).toBe(true);
    }
  });

  it('refuses EST and the rest of the fixed-offset family', () => {
    // EST is the one the audit named: a fixed −05:00 that never observes
    // daylight saving, so a patch wearing it is an hour early from the
    // second Sunday of March 2027 with nothing anywhere complaining.
    for (const tz of ['EST', 'MST', 'HST', 'EST5EDT', 'CET', 'GMT', 'Local', 'Etc/GMT+5', '']) {
      expect(isPlaceZone(tz), tz).toBe(false);
    }
  });

  it('refuses a name no browser can resolve', () => {
    expect(isPlaceZone('Lancaster/PA')).toBe(false);
  });
});

describe('PatchSettingsInfo — changing the zone asks before it changes readings', () => {
  const src = source('pages/PatchSettingsInfo.svelte');

  it('checks the typed zone with the shared place rule, not with Intl alone', () => {
    expect(src).toMatch(/import \{ isPlaceZone \} from '\.\.\/lib\/datetime\.js'/);
    expect(src).toMatch(/if \(tz && !isPlaceZone\(tz\)\)/);
    // The old check was a bare Intl.DateTimeFormat try/catch, which
    // resolves EST happily.
    expect(src).not.toMatch(/timeZone: tz \}\);\s*\}\s*catch/);
  });

  it('catches the server 409 and holds the choice rather than toasting an error', () => {
    expect(src).toMatch(/e\?\.status === 409 && e\?\.data\?\.code === 'timezone_events_undecided'/);
    expect(src).toMatch(/zoneChoice = \{ \.\.\.e\.data, timezone: tz \}/);
  });

  it('offers exactly the two honest answers, and neither is preselected', () => {
    expect(src).toMatch(/resolveZoneChange\('keep_clock'\)/);
    expect(src).toMatch(/resolveZoneChange\('keep_instant'\)/);
    expect(src).toMatch(/Keep the listed time'/);
    expect(src).toMatch(/Leave the event where it is'/);
    expect(src).toMatch(/Leave the events where they are'/);
    expect(src).toMatch(/>Cancel</);
  });

  it('states the count and both zones in the question', () => {
    expect(src).toMatch(/zoneChoice\.events_affected/);
    expect(src).toMatch(/zoneChoice\.from\.replace/);
    expect(src).toMatch(/zoneChoice\.to\.replace/);
  });

  it('re-sends the choice as timezone_events and reports what it did', () => {
    expect(src).toMatch(/body: \{ timezone: zoneChoice\.timezone, timezone_events: mode \}/);
    expect(src).toMatch(/result\?\.timezone_change\?\.events_moved/);
    expect(src).toMatch(/result\?\.timezone_change\?\.events_affected/);
  });
});

describe('EventForm — no control offers a word the product cannot keep', () => {
  const src = source('pages/EventForm.svelte');

  it('has no recurrence control, state, or submitted field', () => {
    expect(src).not.toMatch(/recurrence/);
    expect(src).not.toMatch(/Every Two Weeks/);
  });

  it('validates a typed event zone with the shared place rule', () => {
    expect(src).toMatch(/import \{[^}]*isPlaceZone[^}]*\} from '\.\.\/lib\/datetime\.js'/);
    expect(src).toMatch(/const isValidZone = isPlaceZone/);
    expect(src).toMatch(/Timezone must name a place/);
  });
});

describe('EventDetail — a stored recurrence tells the truth about itself', () => {
  const src = source('pages/EventDetail.svelte');

  it('attributes the claim to the organizer rather than to the calendar', () => {
    expect(src).toMatch(/The organizer says this repeats weekly/);
    expect(src).not.toMatch(/weekly: 'Repeats weekly'/);
  });

  it('says the series is not on the calendar', () => {
    expect(src).toMatch(/only this date is on the calendar/);
  });
});

describe('PatchProfileGlimpses — the count and the list agree', () => {
  const src = source('components/PatchProfileGlimpses.svelte');

  it('derives what the capped list is not showing from the server count', () => {
    expect(src).toMatch(/let moreEvents = \$derived\(/);
    expect(src).toMatch(/node\?\.upcoming_event_count \?\? recentEvents\.length/);
  });

  it('links the remainder to the patch calendar instead of leaving it unsaid', () => {
    expect(src).toMatch(/\{#if moreEvents > 0\}/);
    expect(src).toMatch(/\{moreEvents\} more upcoming/);
    expect(src).toMatch(/href="\/patches\/\{slug\}\/events"/);
  });
});

/**
 * The same question, one rung up (docs/adr/105).
 *
 * The quilt's zone is the rung an event falls through to when neither it nor
 * its patch names one, so moving it changes what every inheriting event on
 * every patch says — and the people whose events those are are not in the
 * admin panel. docs/adr/101 named this as the clearest follow-up; this is it.
 */
describe('AdminQuiltSettings — moving the quilt asks the same question', () => {
  const src = source('pages/AdminQuiltSettings.svelte');

  it('checks the typed zone with the shared place rule, not with Intl alone', () => {
    expect(src).toMatch(/import \{ isPlaceZone \} from '\.\.\/lib\/datetime\.js'/);
    expect(src).toMatch(/return isPlaceZone\(tz\)/);
    expect(src).not.toMatch(/new Intl\.DateTimeFormat\('en-US', \{ timeZone: tz \}\)/);
  });

  it('catches the server 409 and holds the choice rather than toasting an error', () => {
    expect(src).toMatch(/e\?\.status === 409 && e\?\.data\?\.code === 'timezone_events_undecided'/);
    expect(src).toMatch(/zoneChoice = \{ \.\.\.e\.data, timezone: tz \}/);
  });

  it('counts the patches as well as the events, because that is the radius', () => {
    expect(src).toMatch(/zoneChoice\.patches_affected/);
    expect(src).toMatch(/zoneChoice\.events_affected/);
  });

  it('offers exactly the two honest answers, and neither is preselected', () => {
    expect(src).toMatch(/resolveZoneChange\('keep_clock'\)/);
    expect(src).toMatch(/resolveZoneChange\('keep_instant'\)/);
    expect(src).toMatch(/>Cancel</);
  });

  // The safety that makes the question answerable at all: a community that
  // said where it keeps time is not in the count and is told so.
  it('says which calendars it will not touch', () => {
    expect(src).toMatch(/Patches with a timezone of their own, events with one, and events from/);
  });

  it('re-sends the choice as timezone_events and reports what it did', () => {
    expect(src).toMatch(/body: \{ timezone: zoneChoice\.timezone, timezone_events: mode \}/);
    expect(src).toMatch(/saved\?\.timezone_change\?\.events_moved/);
  });
});
