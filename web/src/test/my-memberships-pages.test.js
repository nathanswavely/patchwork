import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { resolve, join, relative } from 'node:path';

/**
 * `GET /api/v1/me/nodes` is paged like every other list: twenty rows unless
 * asked, a hundred at most, the cursor back as `after`. Every client read
 * one page. On lancasterpatchwork.org an account with 23 relationships got
 * 20 rows, and the three it lost, all follows, showed "Follow" on their My
 * Quilt cards, because SocialHome's relationship chip reads
 * getMembershipRoles(). The event form's patch picker and PatchShell's
 * pending state read the same store.
 *
 * The store runs here against a stubbed api, as membership-standing does.
 */

function row(i, role = 'follower') {
  return { id: `m${String(i).padStart(3, '0')}`, node_slug: `patch-${i}`, role, status: 'active' };
}

const firstPage = Array.from({ length: 100 }, (_, i) => row(i + 1));
const secondPage = [row(101), row(102, 'member')];

function pagedApi(calls) {
  return async (path) => {
    calls.push(path);
    if (path === 'users/me/invitations') return { items: [] };
    const q = new URLSearchParams(path.split('?')[1] || '');
    if (!path.startsWith('me/nodes')) return { items: [] };
    if (q.get('after') === 'm100') return { items: secondPage, next_cursor: '' };
    return { items: firstPage, next_cursor: 'm100' };
  };
}

describe('the membership store holds every page of me/nodes', () => {
  let store;
  let calls;

  beforeEach(async () => {
    calls = [];
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({ api: pagedApi(calls) }));
    store = await import('../stores/memberships.svelte.js');
    await store.loadMemberships();
  });

  it('keeps rows from both pages in the role map', () => {
    const roles = store.getMembershipRoles();
    expect(roles.get('patch-1')).toBe('follower');
    expect(roles.get('patch-100')).toBe('follower');
    // Past the first page: the follows that used to say "Follow".
    expect(roles.get('patch-101')).toBe('follower');
    expect(roles.get('patch-102')).toBe('member');
    expect(roles.size).toBe(102);
    expect(store.getMemberships()).toHaveLength(102);
  });

  it('asks for the largest page and sends the cursor back as `after`', () => {
    const mine = calls.filter((p) => p.startsWith('me/nodes'));
    expect(mine).toEqual(['me/nodes?limit=100', 'me/nodes?limit=100&after=m100']);
  });
});

describe('allPages stops on a cursor that does not move', () => {
  it('a server that ignores the cursor costs two requests, not a loop', async () => {
    const calls = [];
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({
      api: async (path) => {
        calls.push(path);
        return { items: [row(calls.length)], next_cursor: 'stuck' };
      },
    }));
    const { allMyMemberships } = await import('../lib/pages.js');
    const rows = await allMyMemberships();
    expect(calls).toHaveLength(2);
    expect(rows).toHaveLength(2);
  });

  it('is capped even when the cursor keeps moving', async () => {
    let n = 0;
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({
      api: async () => ({ items: [row(++n)], next_cursor: `c${n}` }),
    }));
    const { allPages } = await import('../lib/pages.js');
    await allPages('me/nodes', {}, { maxPages: 5 });
    expect(n).toBe(5);
  });

  it('asks for archived patches by status, on every page', async () => {
    const calls = [];
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({
      api: async (path) => {
        calls.push(path);
        return calls.length === 1 ? { items: [row(1)], next_cursor: 'm001' } : { items: [row(2)] };
      },
    }));
    const { allMyMemberships } = await import('../lib/pages.js');
    expect(await allMyMemberships({ status: 'archived' })).toHaveLength(2);
    expect(calls).toEqual(['me/nodes?limit=100&status=archived', 'me/nodes?limit=100&status=archived&after=m001']);
  });
});

function shipped(path) {
  return readFileSync(path, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (name === 'test' || name === 'vendor') continue;
      walk(p, out);
    } else if (/\.(svelte|js)$/.test(name)) {
      out.push(p);
    }
  }
  return out;
}

describe('no client reads one page of me/nodes', () => {
  const src = resolve(__dirname, '..');

  // A bare api('me/nodes…') is one page. Every caller goes through
  // allMyMemberships, which walks them all.
  it('every caller goes through allMyMemberships', () => {
    const offenders = walk(src)
      .filter((p) => /api\(\s*[`'"]me\/nodes/.test(shipped(p)))
      .map((p) => relative(src, p));
    expect(offenders).toEqual([]);
  });

  it('the dashboard and Settings → Patches use it', () => {
    const dashboard = shipped(resolve(src, 'pages/Dashboard.svelte'));
    const settings = shipped(resolve(src, 'pages/UserSettingsPatches.svelte'));
    expect(dashboard).toContain('memberships = await allMyMemberships()');
    expect(settings).toContain('patches = await allMyMemberships()');
    expect(settings).toContain("archivedPatches = await allMyMemberships({ status: 'archived' })");
  });
});
