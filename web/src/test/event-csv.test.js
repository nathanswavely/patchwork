import { describe, it, expect } from 'vitest';
import { parseCsv, rowsToEvents, TEMPLATE_CSV } from '../lib/eventCsv.js';

describe('parseCsv', () => {
  it('handles quoted fields, escaped quotes, and CRLF', () => {
    const rows = parseCsv('a,"b,1","say ""hi"""\r\nc,d,e\r\n');
    expect(rows).toEqual([
      ['a', 'b,1', 'say "hi"'],
      ['c', 'd', 'e'],
    ]);
  });

  it('handles embedded newlines inside quotes and a BOM', () => {
    const rows = parseCsv('﻿title,notes\nShow,"line one\nline two"\n');
    expect(rows).toEqual([
      ['title', 'notes'],
      ['Show', 'line one\nline two'],
    ]);
  });
});

describe('rowsToEvents', () => {
  it('maps aliased headers and forgiving date formats', () => {
    const rows = parseCsv(
      'Event Name,Date,Start Time,Venue,Details\n' +
        'Opening Night,7/25/2026,7:00 PM,Main Stage,First show\n' +
        'Talk,2026-08-01,18:30,Gallery,\n'
    );
    const { events, errors } = rowsToEvents(rows);
    expect(errors).toEqual([]);
    expect(events).toHaveLength(2);
    expect(events[0].title).toBe('Opening Night');
    expect(events[0].location).toBe('Main Stage');
    expect(events[0].description).toBe('First show');
    // 7:00 PM local on July 25 2026 — round-trips through local tz.
    const d = new Date(events[0].starts_at);
    expect([d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(), d.getMinutes()]).toEqual([2026, 6, 25, 19, 0]);
    const d2 = new Date(events[1].starts_at);
    expect([d2.getHours(), d2.getMinutes()]).toEqual([18, 30]);
  });

  it('supports a combined start column and time-only ends', () => {
    const rows = parseCsv('title,start,end\nLate Show,7/25/2026 10:00 PM,1:00 AM\n');
    const { events, errors } = rowsToEvents(rows);
    expect(errors).toEqual([]);
    const start = new Date(events[0].starts_at);
    const end = new Date(events[0].ends_at);
    expect(end > start).toBe(true);
    expect(end.getDate()).toBe(start.getDate() + 1); // past-midnight show rolls to next day
  });

  it('reports row-numbered errors and skips blank lines', () => {
    const rows = parseCsv('title,date\nGood,2026-08-01\n,\nBad,not-a-date\n');
    const { events, errors } = rowsToEvents(rows);
    expect(events).toHaveLength(1);
    expect(errors).toHaveLength(1);
    expect(errors[0].row).toBe(3);
    expect(errors[0].message).toContain('not-a-date');
  });

  it('rejects files without a recognizable title or date column', () => {
    const { errors } = rowsToEvents(parseCsv('foo,bar\n1,2\n'));
    expect(errors.length).toBeGreaterThan(0);
  });

  it('the shipped template parses cleanly', () => {
    const { events, errors } = rowsToEvents(parseCsv(TEMPLATE_CSV));
    expect(errors).toEqual([]);
    expect(events).toHaveLength(2);
    expect(events[0].ends_at).toBeDefined();
    expect(events[1].ends_at).toBeUndefined();
  });
});
