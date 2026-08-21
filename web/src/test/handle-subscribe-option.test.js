/**
 * The handle is a subscribe option (docs/adr/059).
 *
 * Asserted against source text — there is no Svelte render library here, so
 * these confirm the markup exists and cannot confirm anyone can read it.
 * The row was also read in a browser; see e2e.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('SubscribeFeeds carries the patch handle', () => {
  const src = source('components/SubscribeFeeds.svelte');

  it('offers the handle beside the calendar and the feed', () => {
    expect(src).toContain('Mastodon');
    expect(src).toMatch(/`@\$\{slug\}@\$\{getInstanceDomain\(\)\}`/);
  });

  it('gates the handle on the quilt actually federating', () => {
    // With federation off the /ap/* routes are not mounted, so the address
    // would resolve to nothing.
    expect(src).toMatch(
      /handleAvailable = \$derived\(getInstanceFederation\(\) && !!getInstanceDomain\(\)\)/
    );
  });

  it('says replies do not reach the patch', () => {
    // APNodeInbox answers 202 to unhandled activities, so a reply is
    // accepted and discarded. Publishing the address without saying so
    // ships a silent drop to the newcomers this is for.
    expect(src).toMatch(/Replies don't\s+reach the patch/);
  });

  it('says the address belongs to the quilt, not the patch', () => {
    // docs/adr/060: ap_followers and ap_id stay behind on a seamrip.
    expect(src).toMatch(/doesn't travel if this community moves to another quilt/);
  });

  it('never says "fediverse"', () => {
    expect(src).not.toMatch(/fediverse/i);
  });
});

describe('both subscribe surfaces render the same component', () => {
  it('the patch profile overflow', () => {
    expect(source('components/PatchOverflow.svelte')).toContain('<SubscribeFeeds {slug} />');
  });

  it('the workspace events tab', () => {
    expect(source('pages/PatchEvents.svelte')).toContain('<SubscribeFeeds {slug} />');
  });
});

describe('the quilt store carries the federation flag', () => {
  const src = source('stores/quilt.svelte.js');

  it('reads it from GET /api/v1/instance', () => {
    expect(src).toMatch(/if \(data\?\.federation !== undefined\) instanceFederation = data\.federation;/);
    expect(src).toContain('export function getInstanceFederation()');
  });

  it('defaults to off, so a stale store never advertises a dead address', () => {
    expect(src).toMatch(/let instanceFederation = \$state\(false\);/);
  });
});
