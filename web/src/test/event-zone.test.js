import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  formatEventDate,
  formatEventDateLong,
  formatEventTime,
  sameZoneAsViewer,
} from '../lib/datetime.js';

// These cases read the reader's zone as UTC — the annotation exists to tell
// the two apart, so a test of it cannot be written without saying where the
// reader is. Nothing here inherits that from the machine: vite.config.js pins
// the suite to TZ=UTC, which is what makes these assertions mean the same
// thing in CI and on a laptop in Lancaster.

// A Lancaster 8pm show, stored as the instant it is. Under viewer-local
// rendering this reads as midnight *the next day* to anyone on UTC, which
// is the bug docs/adr/045 exists to end: both the events list and the patch
// profile lead with a date, so the visible symptom is the wrong day rather
// than a wrong clock.
const LANCASTER_8PM = '2026-07-23T00:00:00Z';

describe('an event renders in its own zone, not the reader’s', () => {
  it('keeps the wall clock the organizer meant', () => {
    expect(formatEventTime(LANCASTER_8PM, 'America/New_York')).toMatch(/^8:00 PM/);
  });

  it('keeps the day it is on, which is the half that reads as broken', () => {
    expect(formatEventDateLong(LANCASTER_8PM, 'America/New_York'))
      .toBe('Wednesday, July 22, 2026');
    // The same instant with no zone is the old behaviour, and for a UTC
    // reader it lands on the 23rd — the flyer says Wednesday and the
    // website says Thursday.
    expect(formatEventDateLong(LANCASTER_8PM)).toBe('Thursday, July 23, 2026');
  });

  it('renders one instant differently for two places', () => {
    expect(formatEventTime(LANCASTER_8PM, 'America/Los_Angeles')).toMatch(/^5:00 PM/);
    expect(formatEventDate(LANCASTER_8PM, 'America/Los_Angeles')).toBe('Wed, Jul 22');
    expect(formatEventDate(LANCASTER_8PM, 'Europe/Berlin')).toBe('Thu, Jul 23');
  });
});

describe('the zone is annotated only when it would surprise the reader', () => {
  it('says nothing to a reader already in the event’s zone', () => {
    // The suite is pinned to UTC, so UTC is "home" here.
    expect(formatEventTime(LANCASTER_8PM, 'UTC')).toBe('12:00 AM');
    expect(sameZoneAsViewer('UTC')).toBe(true);
  });

  it('names the zone when the event is somewhere else', () => {
    const out = formatEventTime(LANCASTER_8PM, 'America/New_York');
    expect(out).toBe('8:00 PM EDT');
    expect(sameZoneAsViewer('America/New_York')).toBe(false);
  });

  it('stays quiet for a different name that is the same clock', () => {
    // Etc/UTC and UTC are two spellings of one offset. Annotating that
    // would put a zone label on every event for no conversion at all.
    expect(sameZoneAsViewer('Etc/UTC')).toBe(true);
    expect(formatEventTime(LANCASTER_8PM, 'Etc/UTC')).toBe('12:00 AM');
  });

  it('falls back to the reader’s zone rather than throwing on a bad name', () => {
    // A stored zone can outlive a tzdata rename. Intl throws a RangeError
    // on an unknown timeZone, which would take the whole list down.
    expect(() => formatEventTime(LANCASTER_8PM, 'Mars/Olympus_Mons')).not.toThrow();
    expect(formatEventTime(LANCASTER_8PM, 'Mars/Olympus_Mons')).toBe('12:00 AM');
    expect(formatEventDate(LANCASTER_8PM, '')).toBe(formatEventDate(LANCASTER_8PM));
  });
});

// The formatters are only correct if the surfaces actually pass the zone.
// There is no render library here, so this asserts on source text — it
// catches a call site that was missed, not one that renders wrong.
describe('every event surface passes the event’s zone', () => {
  const surfaces = [
    'pages/EventsPage.svelte',
    'pages/EventDetail.svelte',
    'pages/PatchProfile.svelte',
    'pages/PatchEvents.svelte',
    'pages/Dashboard.svelte',
    'pages/AdminEventSubmissions.svelte',
  ];

  for (const rel of surfaces) {
    it(`${rel} renders no event time without one`, () => {
      const src = readFileSync(resolve(__dirname, '..', rel), 'utf8');
      // A call on an event's own starts_at/ends_at that stops at one
      // argument is a surface still showing the reader's clock.
      const zoneless = src.match(
        /format(?:Event)?(?:Date|Time|DateLong|DateStamped)\((\w+)\.(?:starts_at|ends_at)\)/g,
      );
      expect(zoneless, `zoneless event formatting in ${rel}`).toBeNull();
    });
  }
});

// Record dates are not event times and must not be given a zone: "joined
// on" is a fact about the reader's own account, and formatDay already
// handles a bare YYYY-MM-DD deliberately.
describe('record dates stay the reader’s', () => {
  it('leaves formatDay a one-argument function', () => {
    const src = readFileSync(resolve(__dirname, '../lib/datetime.js'), 'utf8');
    expect(src).toMatch(/export function formatDay\(iso\)/);
  });
});
