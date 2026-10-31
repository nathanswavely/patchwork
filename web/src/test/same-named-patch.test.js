import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { resolve } from 'path';
import { normalizePatchName, worthChecking, findSameNamed } from '../lib/sameNamedPatch.js';

// F-033: a founder made her press twice, four minutes apart, and nothing
// said so. The create form now says, while she types, that the name is
// already on the quilt, and never refuses (docs/adr/034 accepts same-named
// patches).

describe('sameNamedPatch', () => {
  it('compares names the way a person reads them', () => {
    expect(normalizePatchName('  Loose   Threads Press ')).toBe('loose threads press');
    expect(normalizePatchName('LOOSE THREADS PRESS')).toBe(normalizePatchName('loose threads press'));
  });

  it('waits for something worth asking about', () => {
    expect(worthChecking('Lo')).toBe(false);
    expect(worthChecking('Loo')).toBe(true);
    expect(worthChecking('   ')).toBe(false);
  });

  it('finds an exact name, not a near one', () => {
    const items = [
      { slug: 'loose-threads', name: 'Loose Threads', status: 'active' },
      { slug: 'loose-threads-press', name: 'Loose Threads Press', status: 'active' },
    ];
    expect(findSameNamed('loose threads press', items)).toEqual({
      slug: 'loose-threads-press', name: 'Loose Threads Press', status: 'active', mine: false,
    });
    // The search matched on description and on substrings; only the name counts.
    expect(findSameNamed('Loose', items)).toBeNull();
  });

  it('prefers the caller\'s own patch, since "you already have this" is the likeliest truth', () => {
    const everyone = [{ slug: 'press-other', name: 'The Press', status: 'active' }];
    const mine = [{ slug: 'press-mine', name: 'The Press', status: 'active' }];
    expect(findSameNamed('the press', everyone, mine)).toMatchObject({ slug: 'press-mine', mine: true });
  });

  it('carries an unclaimed listing\'s status, so the form can say claim it', () => {
    const everyone = [{ slug: 'remnant', name: 'The Remnant Room', status: 'unclaimed' }];
    expect(findSameNamed('The Remnant Room', everyone)).toMatchObject({ status: 'unclaimed', mine: false });
  });

  it('answers null for nothing and survives missing lists', () => {
    expect(findSameNamed('', [{ name: '' }])).toBeNull();
    expect(findSameNamed('Anything', undefined, undefined)).toBeNull();
  });
});

describe('PatchForm wires the notice into creation only', () => {
  const src = readFileSync(resolve(__dirname, '../pages/PatchForm.svelte'), 'utf8');

  it('asks both listings, so a private patch of the caller\'s own is found too', () => {
    expect(src).toContain('api(`nodes?search=${q}&limit=20`)');
    expect(src).toContain('api(`nodes?scope=my&search=${q}&limit=20`)');
  });

  it('drops a stale answer for an earlier spelling', () => {
    expect(src).toMatch(/if \(seq !== sameNamedSeq\) return;/);
  });

  it('shows only in create mode, and warns rather than refusing', () => {
    expect(src).toContain("{#if mode === 'create' && sameNamed}");
    expect(src).toMatch(/if \(mode !== 'create' \|\| !worthChecking\(typed\)\)/);
    // Nothing gates the submit on it.
    expect(src).not.toMatch(/disabled=\{[^}]*sameNamed/);
  });

  // scope=my lists followed patches too, so "you already have this" would be
  // false for a follower. The match being theirs picks the link, not the words.
  it('never claims the patch is the founder\'s own', () => {
    expect(src).not.toMatch(/You already have a patch/);
    expect(src).not.toMatch(/\{#if sameNamed\.mine\}/);
  });

  it('opens the existing patch in a new tab so the half-filled form survives', () => {
    expect(src).toContain('<a href="/patches/{sameNamed.slug}" target="_blank" rel="noopener">Open it</a>');
  });
});
