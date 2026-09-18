/**
 * The trust ask on the event form
 * (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar,
 * decision 7): offered exactly where the review cost is being paid — under
 * the notice that an event is about to queue on an unclaimed patch — with
 * that patch preselected in the shared TrustScopePicker. Answered, never
 * merely seen: pending, declined-with-cooldown and the open ask are three
 * different sentences, not one state that says "asked".
 *
 * Source text only — there is no Svelte render library in this project (see
 * event-form-hosting-patch.test.js).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const form = source('pages/EventForm.svelte');

describe('EventForm wires the trust ask', () => {
  it('imports the shared TrustScopePicker and binds its scope', () => {
    expect(form).toContain("import TrustScopePicker from '../components/TrustScopePicker.svelte'");
    expect(form).toMatch(/<TrustScopePicker[\s\S]{0,120}bind:all=\{trustAll\}/);
    expect(form).toMatch(/<TrustScopePicker[\s\S]{0,160}bind:selected=\{trustSelected\}/);
  });

  it('fetches the standing request only once the ask becomes relevant', () => {
    expect(form).toMatch(/if \(showTrustAsk && !trustRequestChecked\) loadTrustRequest\(\)/);
    expect(form).toMatch(/api\('users\/me\/trust-request'\)/);
    expect(form).toMatch(/trustRequestStatus = data\.request\?\.status \|\| ''/);
    expect(form).toMatch(/trustCanAskAgainAt = data\.can_ask_again_at \|\| null/);
  });

  it('posts the scope, node ids and optional message on send', () => {
    expect(form).toMatch(/api\('users\/me\/trust-request', \{ method: 'POST', body \}\)/);
    expect(form).toMatch(/scope: trustAll \? 'all' : 'patches'/);
    expect(form).toMatch(/node_ids: trustAll \? \[\] : trustSelected\.map\(\(n\) => n\.id\)/);
    expect(form).toMatch(/message: trustMessage\.trim\(\) \|\| undefined/);
  });

  it('preselects the chosen patch and clears scope-to-everywhere when the panel opens', () => {
    expect(form).toMatch(
      /trustSelected = hostingPatch\s*\n\s*\? \[\{ id: hostingPatch\.id, slug: hostingPatch\.slug, name: hostingPatch\.name \}\]/
    );
    expect(form).toMatch(/function openTrustPanel\(\)[\s\S]{0,300}trustAll = false;/);
  });

  it('shows the server’s error text from a 409 or 403 in the panel', () => {
    expect(form).toMatch(/trustError = e\.data\?\.error \|\| e\.message \|\| 'Failed to send that request'/);
    expect(form).toMatch(/\{#if trustError\}[\s\S]{0,40}\{trustError\}/);
  });
});

describe('EventForm states all three request outcomes verbatim', () => {
  it('the ask line, whose last sentence is the button that opens the panel', () => {
    expect(form).toContain(
      'An instance admin reviews events on unclaimed patches. Add them often?'
    );
    expect(form).toMatch(/onclick=\{openTrustPanel\}>Ask to be a trusted contributor\.</);
  });

  it('the pending line, once a request is waiting for an admin', () => {
    expect(form).toContain(
      'Your request to be a trusted contributor is waiting for an admin.'
    );
    expect(form).toMatch(/\{#if trustRequestStatus === 'pending'\}/);
  });

  it('the declined-with-cooldown line, using the project’s date formatter', () => {
    expect(form).toContain('Your last request was declined. You can ask again on');
    expect(form).toMatch(/\{formatDay\(trustCanAskAgainAt\)\}/);
    expect(form).toMatch(
      /import \{ toZonedInputValue, fromZonedInputValue, sameZoneAsViewer, isPlaceZone, formatDay \} from '\.\.\/lib\/datetime\.js'/
    );
  });

  it('falls back to the ask line once a decline’s cooldown has passed', () => {
    expect(form).toMatch(
      /\{:else if trustRequestStatus === 'declined' && !trustCanAskAgain\}/
    );
    // No third branch for approved/moot: they fall through the same else
    // as "never asked", which is the open-ask line.
    expect(form).toMatch(/\{:else if trustPanelOpen\}/);
  });
});

describe('the ask never appears off the one door it belongs to', () => {
  it('requires an unclaimed patch and willReview, so an active patch shows nothing', () => {
    expect(form).toMatch(
      /let showTrustAsk = \$derived\(\s*!isEdit && !!hostingPatch && hostingPatch\.status === 'unclaimed' && willReview\s*\)/
    );
  });

  it('is absent from editing — the ask is about events not yet posted', () => {
    // showTrustAsk itself excludes isEdit, so the pattern above already
    // covers this; this test guards against that clause being dropped.
    expect(form).toMatch(/showTrustAsk = \$derived\(\s*!isEdit &&/);
  });

  it('renders under both places the form already says "will be reviewed", and nowhere else', () => {
    const occurrences = (form.match(/\{@render trustAsk\(\)\}/g) || []).length;
    expect(occurrences).toBe(2);
  });
});
