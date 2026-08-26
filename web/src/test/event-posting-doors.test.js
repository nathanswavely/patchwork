/**
 * The doors to posting one event (docs/adr/026).
 *
 * The gap this covers: the patch events page asked its own posting question
 * inside an else-if chain that bulk upload answered first, so an instance
 * admin visiting a patch they don't belong to — and a trusted contributor on
 * an unclaimed patch — were offered a CSV importer and no way at all to post
 * a single event. The patch profile had already been converted to the shared
 * helper; this page had not, so the two surfaces could disagree about whether
 * a person had a door.
 *
 * eventPostingRight itself is a pure function, tested in
 * patch-profile-window.test.js. What is asserted here is that both surfaces
 * ask it, and that they answer in the same words and link to the same place.
 * Source text, because there is no Svelte render library in this project.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { eventPostingRight } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const events = source('pages/PatchEvents.svelte');
const profile = source('pages/PatchProfile.svelte');

describe('the events page asks the shared question', () => {
  it('calls eventPostingRight rather than deriving its own answer', () => {
    expect(events).toContain("import { eventPostingRight } from '../lib/patchWorkspace.js'");
    expect(events).toMatch(/let postingRight = \$derived\(eventPostingRight\(\{/);
    // The page's own version of the question is gone.
    expect(events).not.toContain('canSuggest');
  });

  it('reads the role, not isMember, which the patch context sets for followers', () => {
    expect(events).toMatch(/isMemberOrAdmin: membershipRole === 'member' \|\| membershipRole === 'admin'/);
    expect(events).not.toMatch(/\$derived\(patch\.value\.isMember\)/);
  });
});

describe('bulk upload and posting one event are separate rights', () => {
  it('does not make them branches of one chain', () => {
    // The bug: `{:else if canBulkUpload}` swallowed the posting door for
    // anyone who could also upload a CSV.
    expect(events).not.toMatch(/\{:else if canBulkUpload\}/);
    // Upload stands on its own condition.
    expect(events).toMatch(/\{#if canBulkUpload\}[\s\S]{0,200}?Upload events/);
  });

  it('offers the one-event door whenever there is any right to post', () => {
    expect(events).toMatch(/\{#if postingRight !== 'none'\}/);
  });
});

describe('the two surfaces answer alike', () => {
  it('names the outcome in the same words', () => {
    const label = /postingRight === 'direct' \? 'New event' : 'Suggest an event'/;
    expect(events).toMatch(label);
    expect(profile).toMatch(label);
  });

  it('carries the patch into the form, so the door is never unscoped', () => {
    // Dropping ?node= lands people on a form asking which patch they meant.
    expect(events).toMatch(/href="\/events\/new\?node=\{slug\}"/);
    expect(profile).toMatch(/href="\/events\/new\?node=\{slug\}"/);
    expect(events).not.toMatch(/href="\/events\/new"/);
  });
});

describe('the rights the helper grants, at the boundaries this page cares about', () => {
  it('gives an instance admin a door on a patch they do not belong to', () => {
    expect(eventPostingRight({ signedIn: true, isInstanceAdmin: true })).toBe('direct');
  });

  it('gives a trusted contributor a door on an unclaimed patch', () => {
    expect(
      eventPostingRight({ signedIn: true, trustedContributor: true, isUnclaimed: true })
    ).toBe('direct');
  });

  it('still gives a follower none', () => {
    expect(
      eventPostingRight({ signedIn: true, isMemberOrAdmin: false, acceptSuggestions: false })
    ).toBe('none');
  });
});
