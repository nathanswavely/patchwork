import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// docs/adr/090 decision 5. Patchwork tells nobody an event is coming, so
// this control and subscribing to the patch are the whole of how an event
// reaches somebody who asked for it. These assert against source text,
// which is all this project's frontend suite can do — the control was also
// checked in a browser, where the earlier ordering bug actually showed up.
const source = readFileSync(
  resolve(__dirname, '../pages/EventDetail.svelte'),
  'utf8',
);

describe('an event can be taken away as a calendar file', () => {
  it('links straight at the endpoint, with no script in the way', () => {
    expect(source).toContain('href="/api/v1/events/{event.id}/event.ics"');
    expect(source).toContain('Add to calendar');
  });

  it('is a plain anchor, so the download is the browser’s job', () => {
    const anchor = source.slice(source.indexOf('calendar-actions'));
    const control = anchor.slice(0, anchor.indexOf('</div>'));
    expect(control).not.toContain('onclick');
  });

  it('is withheld while the event is still in the review queue', () => {
    // The endpoint 404s on a pending submission, and offering a file that
    // cannot exist is worse than offering none.
    expect(source).toContain("{#if event.status !== 'pending_review'}");
  });

  it('sits after the description, where an action belongs', () => {
    expect(source.indexOf('class="description"')).toBeLessThan(
      source.indexOf('class="calendar-actions"'),
    );
  });
});
