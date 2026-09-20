import { describe, it, expect, afterEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { viewingAsVisitor, signedInForPatchView } from '../lib/preview.js';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const app = shipped('../App.svelte');
const apiSrc = shipped('../lib/api.js');
const overflow = shipped('../components/PatchOverflow.svelte');
const glimpses = shipped('../components/PatchProfileGlimpses.svelte');

function setSearch(s) {
  window.history.replaceState({}, '', s ? `/?${s}` : '/');
}
afterEach(() => setSearch(''));

// F-126. "Let me look at my own page the way a stranger looks at it without
// signing out, because signing out is how I lost my account this
// afternoon."

describe('the preview reads the URL, not a variable', () => {
  it('is off by default and on with the parameter', () => {
    setSearch('');
    expect(viewingAsVisitor()).toBe(false);
    setSearch('as=visitor');
    expect(viewingAsVisitor()).toBe(true);
  });

  it('ignores any other value', () => {
    setSearch('as=admin');
    expect(viewingAsVisitor()).toBe(false);
    setSearch('as=');
    expect(viewingAsVisitor()).toBe(false);
  });

  // The first version set a flag from an effect in App.svelte. Effects run
  // after the components under them have mounted and fetched, so the
  // profile head asked before the flag existed and came back holding admin
  // standing: a Settings door over a visitor's view of the rooms below it.
  it('is asked at call time, so nothing can ask before it is set', () => {
    expect(apiSrc).toContain("if (method === 'GET' && viewingAsVisitor())");
    expect(app).not.toContain('setViewingAsVisitor');
  });

  // Only GETs. A preview must not change what a write is allowed to do.
  it('never rides on a write', () => {
    expect(apiSrc).toContain("method === 'GET' &&");
  });
});

describe('the client agrees with the server about who is reading', () => {
  it('treats the reader as signed out while previewing', () => {
    setSearch('as=visitor');
    expect(signedInForPatchView(true)).toBe(false);
    setSearch('');
    expect(signedInForPatchView(true)).toBe(true);
    expect(signedInForPatchView(false)).toBe(false);
  });

  // Found by putting the preview beside a real signed-out page: the
  // server was answering as the public while the page still offered
  // "Suggest an event" and "Report", neither of which a signed-out
  // visitor is shown.
  it('the event door and the report door both go through it', () => {
    expect(glimpses).toContain('signedIn: signedInForPatchView(isLoggedIn())');
    expect(glimpses).toContain('isInstanceAdmin: isInstanceAdmin() && !viewingAsVisitor()');
    expect(overflow).toContain('signedInForPatchView(isLoggedIn())');
  });
});

describe('getting in and out', () => {
  it('offers the way in to the people who set what is published', () => {
    expect(overflow).toContain('let canPreview = $derived(isAdmin && !previewing);');
    expect(overflow).toContain('>View as a visitor</a>');
  });

  // A real link, not navigate(): the components that fetch a patch key
  // their effects on its slug, which does not change when only the query
  // does, so a soft navigation left half the page holding admin answers.
  it('enters with a page load rather than a soft navigation', () => {
    expect(overflow).toContain('<a role="menuitem" href={previewHref}>View as a visitor</a>');
    expect(overflow).not.toContain('navigate(previewHref)');
  });

  it('leaves the same way', () => {
    expect(app).toContain('window.location.assign(getPath());');
  });

  // A preview you cannot tell you are in is worse than no preview.
  it('says so on screen the whole time, with the way out', () => {
    expect(app).toContain('{#if previewingAsVisitor}');
    expect(app).toContain('You are seeing this patch as a visitor sees it.');
    expect(app).toContain('Leave preview');
  });
});
