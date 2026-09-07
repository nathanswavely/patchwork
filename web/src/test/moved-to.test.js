/**
 * The moved-to pointer, in the interface (docs/adr/090).
 *
 * patchLink.js is a pure module and is tested as one. The rest is asserted
 * against source text — there is no Svelte render library in this project
 * (see patch-profile-window.test.js), so these tests can say a control is
 * wired and cannot say it renders. The browser pass is what checks that.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { parsePatchLink, patchLinkPath, linkHost } from '../lib/patchLink.js';
import { eventPostingRight } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('a pasted patch link is recognised in one place', () => {
  it('reads host and slug out of a patch URL', () => {
    expect(parsePatchLink('https://their.example/patches/gallery-row')).toEqual({
      host: 'their.example',
      slug: 'gallery-row',
    });
    // Trailing slash and surrounding whitespace are how a paste actually
    // arrives.
    expect(parsePatchLink('  http://their.example/patches/gallery-row/  ')).toEqual({
      host: 'their.example',
      slug: 'gallery-row',
    });
  });

  it('refuses anything that is not a patch', () => {
    for (const url of [
      'https://their.example',
      'https://their.example/users/somebody',
      'https://their.example/patches/gallery-row/events',
      'javascript:alert(1)',
      '',
      null,
    ]) {
      expect(parsePatchLink(url)).toBeNull();
    }
  });

  it('sends a link home to the patch and a link away to the remote card', () => {
    expect(patchLinkPath('https://here.example/patches/gallery-row', 'here.example')).toBe(
      '/patches/gallery-row'
    );
    expect(patchLinkPath('https://their.example/patches/gallery-row', 'here.example')).toBe(
      '/quilts/their.example/patches/gallery-row'
    );
    expect(patchLinkPath('https://their.example/about', 'here.example')).toBe('');
  });

  it('names the host of any link, for saying where one goes', () => {
    expect(linkHost('https://www.their.example/anything?x=1')).toBe('their.example');
    expect(linkHost('nonsense')).toBe('');
  });

  // The shape used to be written out at three call sites. Three copies is a
  // rule that disagrees with itself the first time somebody widens it.
  it('is the only copy of the shape', () => {
    for (const file of [
      'components/SocialShell.svelte',
      'components/EventLinks.svelte',
      'pages/QuiltsSettings.svelte',
    ]) {
      expect(source(file)).not.toMatch(/\/patches\\\/\(\[a-z0-9-\]/);
      expect(source(file)).toMatch(/from '\.\.\/lib\/patchLink\.js'/);
    }
  });
});

describe('the notice says where, and offers the in-app door when it can', () => {
  const src = source('components/MovedNotice.svelte');

  it('names the two subjects', () => {
    expect(src).toMatch(/This person has moved to/);
    expect(src).toMatch(/This patch has moved to/);
  });

  it('links out with rel="noopener" and in through the remote patch card', () => {
    expect(src).toMatch(/<a href=\{url\} target="_blank" rel="noopener">\{host\}<\/a>/);
    expect(src).toMatch(/patchLinkPath\(url, window\.location\.host\)/);
    expect(src).toMatch(/\{#if cardPath\}/);
  });
});

describe('the banner is at the top of the patch page', () => {
  const src = source('pages/PatchProfile.svelte');

  it('renders the notice from the node it loaded', () => {
    expect(src).toMatch(/<MovedNotice url=\{node\.moved_to\} subject="patch" \/>/);
  });

  // Above the description, so a reader who landed on a patch the community
  // has left gets the forwarding address before the blurb.
  it('puts it ahead of the description', () => {
    expect(src.indexOf('<MovedNotice')).toBeLessThan(src.indexOf('class="profile-desc"'));
  });
});

describe('the discovery card wears a chip', () => {
  const src = source('pages/Discover.svelte');

  it('marks a moved patch in both lists', () => {
    const chips = src.match(/<span class="moved-chip">Moved<\/span>/g) || [];
    expect(chips.length).toBe(2);
    expect(src).toMatch(/\{#if patch\.moved_to\}/);
  });
});

describe('the profile carries a person\'s own pointer', () => {
  const src = source('pages/UserProfile.svelte');

  it('renders it from the profile payload', () => {
    expect(src).toMatch(/<MovedNotice url=\{profile\.moved_to\} subject="person" \/>/);
  });
});

describe('a moved patch offers no rung it cannot honour', () => {
  // docs/adr/042's rule, applied again: an absent door beats a 403 at the
  // end of a ceremony. The server is still the authority — it declines both
  // acts — and the banner directly above the row has already said why.
  it('drops Follow and Become a member, and keeps standing', () => {
    const src = source('components/PatchRelationship.svelte');
    expect(src).toMatch(/let hasMoved = \$derived\(!!node\?\.moved_to\);/);
    expect(src).toMatch(/\{:else if !hasMoved\}\s*\n\s*<button class="btn btn-primary \{btnSize\}" onclick=\{handleFollow\}/);
    expect(src).toMatch(/canBecomeMember = \$derived\(\s*\n\s*!hasMoved &&/);
    // Standing itself is untouched: somebody already in the patch keeps the
    // control that lets them leave.
    expect(src).toMatch(/\{#if standing\}/);
  });

  it('offers no event door to an outsider and keeps one for a member', () => {
    const base = { signedIn: true, submissionsEnabled: true, acceptSuggestions: true, hasMoved: true };
    expect(eventPostingRight(base)).toBe('none');
    expect(eventPostingRight({ ...base, isMemberOrAdmin: true })).toBe('direct');
    expect(eventPostingRight({ ...base, isInstanceAdmin: true })).toBe('direct');
    // And nothing changes for a patch that has not moved.
    expect(eventPostingRight({ ...base, hasMoved: false })).toBe('suggest');
  });

  it('is asked for on both surfaces that draw the door', () => {
    for (const file of ['pages/PatchProfile.svelte', 'pages/PatchEvents.svelte']) {
      expect(source(file)).toMatch(/hasMoved: !!node\?\.moved_to,/);
    }
  });
});

describe('both settings pages can set and clear it', () => {
  it('patch settings PATCHes the node and offers a way back', () => {
    const src = source('pages/PatchSettingsInfo.svelte');
    expect(src).toMatch(/We have moved/);
    expect(src).toMatch(/body: \{ moved_to: value \}/);
    // Clearing is a real act: the field empties and the same save runs.
    expect(src).toMatch(/Clear the\s+field to undo it\./);
  });

  it('account settings PATCHes the person', () => {
    const src = source('pages/AccountSettings.svelte');
    expect(src).toMatch(/I have moved/);
    expect(src).toMatch(/body: \{ moved_to: movedTo\.trim\(\) \}/);
    expect(src).toMatch(/Clear the field to undo it\./);
  });
});
