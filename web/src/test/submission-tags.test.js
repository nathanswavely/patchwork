/**
 * Regression coverage for issue #5: community patch suggestions can't carry
 * tags.
 *
 * Before this change, SubmitPatch.svelte collected no tags and the
 * /api/v1/submissions request body never carried them, so a
 * community-suggested patch landed invisible to discovery/onboarding/quilt
 * placement until an admin tagged it by hand.
 *
 * There is no Svelte render library in this project (see
 * patch-address-and-scope.test.js), so component wiring is asserted against
 * the source text, and the request-body contract is asserted with a mocked
 * fetch — matching the existing pattern in this file set.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { api } from '../lib/api.js';

function mockFetch(data, status = 200) {
  return vi.spyOn(global, 'fetch').mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    json: async () => data,
  });
}

// Vitest runs with the web/ project root as cwd.
function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe('#5: the suggest-a-patch form carries tags', () => {
  it('posts a tags array when tags are picked', async () => {
    const spy = mockFetch({ status: 'pending_review', id: 'sub1' }, 201);

    await api('submissions', {
      method: 'POST',
      body: { name: 'Gallery Row', tags: ['music', 'craft'] },
    });

    const [url, opts] = spy.mock.calls[0];
    expect(url).toBe('/api/v1/submissions');
    const body = JSON.parse(opts.body);
    expect(body.tags).toEqual(['music', 'craft']);
  });

  it('omits tags entirely when none are picked, rather than posting []', async () => {
    const spy = mockFetch({ status: 'pending_review', id: 'sub1' }, 201);

    await api('submissions', {
      method: 'POST',
      body: { name: 'Gallery Row', tags: [].length > 0 ? [] : undefined },
    });

    const [, opts] = spy.mock.calls[0];
    const body = JSON.parse(opts.body);
    expect(body).not.toHaveProperty('tags');
  });

  it('SubmitPatch.svelte imports the shared curated-vocabulary TagPicker', () => {
    const src = source('pages/SubmitPatch.svelte');
    expect(src).toContain("import TagPicker from '../components/TagPicker.svelte'");
    expect(src).toMatch(/<TagPicker\s+bind:selected=\{tags\}/);
  });

  it('SubmitPatch.svelte submits tags only when at least one is picked', () => {
    const src = source('pages/SubmitPatch.svelte');
    expect(src).toMatch(/tags:\s*tags\.length > 0 \? tags : undefined/);
  });

  it('SubmitPatch.svelte does not require tags to submit', () => {
    const src = source('pages/SubmitPatch.svelte');
    // The only required-field guard is the name check; tags carries no such guard.
    const requiredChecks = src.match(/if \(!\w+(\.trim\(\))?\)\s*\{[^}]*error\s*=/g) || [];
    expect(requiredChecks.some((c) => c.includes('name'))).toBe(true);
    expect(requiredChecks.some((c) => c.includes('tags'))).toBe(false);
  });

  it('AdminSubmissions.svelte renders the tags submitted with each patch', () => {
    const src = source('pages/AdminSubmissions.svelte');
    expect(src).toMatch(/sub\.tags/);
  });
});

describe('#5: the reviewer can correct tags before approval', () => {
  it('posts the corrected tags with the approve action', async () => {
    const spy = mockFetch({ status: 'ok' });

    await api('admin/submissions/sub1', {
      method: 'PATCH',
      body: { action: 'approve', verification_domain: '', tags: ['theater', 'craft'] },
    });

    const [url, opts] = spy.mock.calls[0];
    expect(url).toBe('/api/v1/admin/submissions/sub1');
    const body = JSON.parse(opts.body);
    expect(body.action).toBe('approve');
    expect(body.tags).toEqual(['theater', 'craft']);
  });

  it('AdminSubmissions.svelte edits tags with the shared curated-vocabulary TagPicker', () => {
    const src = source('pages/AdminSubmissions.svelte');
    expect(src).toContain("import TagPicker from '../components/TagPicker.svelte'");
    expect(src).toMatch(/<TagPicker\s+bind:selected=\{tagInputs\[sub\.id\]\}/);
  });

  it('AdminSubmissions.svelte seeds the picker from the submitted tags', () => {
    const src = source('pages/AdminSubmissions.svelte');
    expect(src).toMatch(/tags\[sub\.id\]\s*=\s*sub\.tags \|\| \[\]/);
  });

  it('AdminSubmissions.svelte sends the picker state only on approve', () => {
    const src = source('pages/AdminSubmissions.svelte');
    // tags ride the same approve-only branch as the verification domain.
    const approveBranch = src.match(/if \(action === 'approve'\) \{[^}]*\}/s)?.[0] || '';
    expect(approveBranch).toContain("body.tags = tagInputs[id] || []");
  });
});
