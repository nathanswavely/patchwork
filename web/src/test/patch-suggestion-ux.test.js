/**
 * docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar
 *
 * Decision 1 forks patch creation before any field so nobody reads past a
 * muted sentence into admin of a place that hasn't admitted them. Decision 6
 * lets a suggestion carry a calendar. Decision 8 tells the suggester what
 * happened to it.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text, matching submission-tags.test.js.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('SubmitPatch.svelte: heading and intro match every link into it', () => {
  const src = source('pages/SubmitPatch.svelte');

  it('heads with "Suggest a patch"', () => {
    expect(src).toContain('<h1>Suggest a patch</h1>');
  });

  it('states the unclaimed-listing outcome in the intro line', () => {
    expect(src).toContain(
      "Know a place or group that should be on the map? Suggest it here and the real owner can claim it later."
    );
  });
});

describe('SubmitPatch.svelte: a suggestion may carry a feed (decision 6)', () => {
  const src = source('pages/SubmitPatch.svelte');

  it('has an optional Events feed field bound to feedUrl', () => {
    expect(src).toMatch(/let feedUrl = \$state\(''\)/);
    expect(src).toMatch(/<label for="feed-url">Events feed/);
    expect(src).toMatch(/<input id="feed-url" type="url" bind:value=\{feedUrl\}/);
  });

  it('hints at what is checked, and when', () => {
    expect(src).toContain(
      'A calendar Patchwork can read: an ICS address, a Squarespace events page, or a page with event markup. Checked when you submit.'
    );
  });

  it('sends feed_url on submit only when filled in', () => {
    expect(src).toMatch(/feed_url:\s*feedUrl\.trim\(\)\s*\|\|\s*undefined/);
  });

  it('shows a 400 from the API as the form error text, using the server message', () => {
    // The only special-cased status is 409 (name collision); everything
    // else, including a feed Patchwork can't read, falls through to the
    // server's own message rather than a generic one.
    const catchBlock = src.match(/\} catch \(e\) \{([\s\S]*?)\} finally/);
    expect(catchBlock, 'catch block not found').toBeTruthy();
    expect(catchBlock[1]).toMatch(/e\.status === 409/);
    expect(catchBlock[1]).toMatch(/error = e\.message \|\| 'Something went wrong\.'/);
  });
});

describe('SubmitPatch.svelte: who reviews a suggestion is stated, and differs by who is looking (decision 4)', () => {
  const src = source('pages/SubmitPatch.svelte');

  it('reads trusted-contributor status from the auth store', () => {
    expect(src).toContain("import { isTrustedContributor } from '../stores/auth.svelte.js'");
    expect(src).toMatch(/let trustedQuiltWide = \$derived\(isTrustedContributor\(\)\)/);
  });

  it('tells a quilt-wide trusted contributor their suggestion lands at once', () => {
    expect(src).toContain('As a trusted contributor, your suggestion joins the quilt at once.');
  });

  it('tells everyone else an instance admin reviews, and what approval unlocks', () => {
    expect(src).toContain(
      "An instance admin reviews suggestions. Once it's approved, you can add its events without review until its owner claims it."
    );
  });

  it('does not add a trusted-contributor ask here — that lives on the event form', () => {
    expect(src).not.toMatch(/ask to become a trusted contributor/i);
    expect(src).not.toMatch(/trust-?request/i);
  });
});

describe('SubmitPatch.svelte: the toasts match the two review outcomes (decision 8)', () => {
  const src = source('pages/SubmitPatch.svelte');

  it('the reviewed path tells the suggester they will hear back', () => {
    expect(src).toContain("Suggested. An admin will review it and you'll be notified.");
  });

  it('the auto-approved (trusted, quilt-wide) path keeps its existing toast', () => {
    expect(src).toContain("Patch added to the quilt!");
  });
});

describe('Dashboard and Discover no longer promise creation before the fork', () => {
  it('Dashboard’s quick-action card reads "Add a patch"', () => {
    const src = source('pages/Dashboard.svelte');
    expect(src).not.toMatch(/>Create Patch</);
    expect(src).toMatch(/href="\/patches\/new"[\s\S]{0,200}>Add a patch</);
  });

  it('Discover’s empty-quilt call to action reads "Add a patch" and does not promise creation in its subtitle', () => {
    const src = source('pages/Discover.svelte');
    expect(src).not.toMatch(/>\s*Create a patch\s*</);
    expect(src).toMatch(/navigate\('\/patches\/new'\)\}>\s*Add a patch\s*</);
    expect(src).not.toContain('Make one for your group');
    expect(src).toContain(
      'No patches on this quilt so far. Add one, and the next person who\n          comes looking will have something to find.'
    );
  });

  it('GlobalBar’s New menu keeps "New patch" (it sits under New and promises nothing)', () => {
    const src = source('components/GlobalBar.svelte');
    expect(src).toContain('>New patch</a>');
  });
});
