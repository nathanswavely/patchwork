/**
 * Regression tests for issues #45 and #46.
 *
 * These cover the two places the frontend and backend disagreed:
 *   #46 — patch pages used the key "location"; the column is "address", and
 *         "location" belongs to events. The field rendered blank and 400'd.
 *   #45 — the map fetched patches with no scope param, so "My Quilt" was
 *         silently ignored there while the quilt and cards honoured it.
 *
 * There is no Svelte render library in this project, so the component wiring
 * is asserted against the source text. That is enough to catch the exact
 * regression: both bugs were a wrong key/missing param in the source.
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

describe('#46: patch address uses the address key', () => {
  it('creating a patch posts address, never location', async () => {
    const spy = mockFetch({ id: 'p1', slug: 'the-selvage', address: '44 W King St' });

    await api('nodes', {
      method: 'POST',
      body: { name: 'The Selvage', address: '44 W King St' },
    });

    const [, opts] = spy.mock.calls[0];
    const body = JSON.parse(opts.body);
    expect(body.address).toBe('44 W King St');
    expect(body).not.toHaveProperty('location');
  });

  it('updating a patch patches address, never location', async () => {
    const spy = mockFetch({ id: 'p1', address: '123 N Queen St' });

    await api('nodes/the-selvage', {
      method: 'PATCH',
      body: { address: '123 N Queen St' },
    });

    const [url, opts] = spy.mock.calls[0];
    expect(url).toBe('/api/v1/nodes/the-selvage');
    const body = JSON.parse(opts.body);
    expect(body.address).toBe('123 N Queen St');
    expect(body).not.toHaveProperty('location');
  });

  it('PatchForm binds and submits address', () => {
    const src = source('pages/PatchForm.svelte');
    expect(src).toMatch(/address:\s*address\.trim\(\)/);
    // "location" must not appear as a submitted key on a patch.
    expect(src).not.toMatch(/location:\s*location\.trim\(\)/);
  });

  it('PatchSettingsInfo reads and saves address', () => {
    const src = source('pages/PatchSettingsInfo.svelte');
    expect(src).toContain("saveField('address'");
    expect(src).toContain('node?.address');
    expect(src).not.toContain("saveField('location'");
    expect(src).not.toContain('node?.location');
  });
});

describe('#45: the map honours the quilt scope', () => {
  it('requests scoped nodes when the scope is My Quilt', async () => {
    const spy = mockFetch({ items: [] });

    await api('nodes?limit=500&scope=my');

    const [url] = spy.mock.calls[0];
    expect(url).toContain('scope=my');
  });

  it('SocialHome passes the scope to the map fetch and refetches on change', () => {
    const src = source('pages/SocialHome.svelte');
    // The map request is built with the scope param.
    expect(src).toMatch(/scopeParam\s*=\s*quiltScope === 'my' \? '&scope=my' : ''/);
    expect(src).toMatch(/api\(`nodes\?limit=500\$\{scopeParam\}`\)/);
    // The map effect depends on quiltScope, not only showMap.
    const mapEffect = src.match(/\$effect\(\(\) => \{[^}]*loadMapData\(\);\s*\}\);/s);
    expect(mapEffect, 'map $effect not found').toBeTruthy();
    expect(mapEffect[0]).toContain('quiltScope');
  });
});
