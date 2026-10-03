import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const shell = shipped('../components/PatchShell.svelte');
const head = shipped('../components/PatchProfileHead.svelte');
const mapper = shipped('../lib/patchError.js');
const myPatches = shipped('../pages/UserSettingsPatches.svelte');

// F-123: "Patch not found / node not found" — two registers stacked, one of
// them a word CLAUDE.md says never reaches the UI. F-124: the patch behind
// that refusal was one the reader ran.

describe('what a person is told when a patch will not load (F-123)', () => {
  // Both components printed e.message straight under their own heading, so
  // whatever word the API used became product copy.
  it('neither component prints the raw API message any more', () => {
    for (const src of [shell, head]) {
      expect(src).not.toContain("e.message || 'Failed to load patch'");
      expect(src).toContain('patchLoadError(e)');
    }
  });

  // One mapping, because two copies of it is how the two pages came to
  // disagree in the first place.
  it('both read the same mapping', () => {
    for (const src of [shell, head]) {
      expect(src).toContain("from '../lib/patchError.js'");
    }
  });

  // A 404's heading says everything there is to say; a second line under it
  // was only ever the server talking to itself.
  it('says nothing further under a plain not-found', () => {
    expect(mapper).toContain("if (e?.status === 404) return '';");
  });

  it('keeps the server text for a failure, which is not a vocabulary', () => {
    expect(mapper).toContain("return e?.message ||");
  });

  // Found in the browser, not here: a plain not-found deliberately has no
  // message, and the error branch was testing the message string, so an
  // empty string is falsy and the page sat on its loading skeleton for
  // ever. Whether something failed and what to print about it are two
  // questions, and the branch now asks the first one.
  it('branches on whether the load failed, not on whether it has words', () => {
    for (const src of [shell, head]) {
      expect(src).toContain('let loadFailed = $state(false);');
      expect(src).toMatch(/\{:else if loadFailed/);
    }
    expect(shell).not.toContain('{:else if error && !node}');
    expect(head).not.toMatch(/\{:else if error\}/);
  });
});

describe('a patch its own admin cannot open (F-124)', () => {
  it('names the patch in the heading rather than denying it exists', () => {
    for (const src of [shell, head]) {
      expect(src).toContain('is archived`');
    }
  });

  it('says who can bring it back', () => {
    expect(mapper).toContain('An instance admin can restore it.');
  });

  it('lists the archived patches a person runs, asked for by name', () => {
    expect(myPatches).toContain("api('me/nodes?status=archived')");
    expect(myPatches).toContain('<h3 class="section-heading">Archived</h3>');
  });

  // No link: every slug route refuses an archived patch, so a link here
  // would be a door onto the refusal the section exists to explain.
  it('offers no door onto the refusal', () => {
    const start = myPatches.indexOf('<h3 class="section-heading">Archived</h3>');
    const section = myPatches.slice(start, myPatches.indexOf('{/if}', start));
    expect(section).not.toContain('href=');
    expect(section).toContain('class="patch-name plain"');
  });
});
