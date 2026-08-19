import { describe, it, expect, vi, beforeEach } from 'vitest';
import { patchPickerProvider } from '../lib/finderProviders.js';

// The corpus behind the patch picker (CONTEXT.md "Patch picker"). The
// listing caps `limit` at 100 server-side, so the provider has to follow
// the cursor: a picker that truncates doesn't degrade, it lies — the
// missing patch reads as "not on this quilt".
function mockPages(pages) {
  return vi.spyOn(global, 'fetch').mockImplementation(async (url) => {
    const after = new URL(url, 'http://x').searchParams.get('after') || '';
    const page = pages[after];
    return { ok: true, status: 200, json: async () => page };
  });
}

describe('patchPickerProvider', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('follows next_cursor past the server-side limit cap', async () => {
    const page = (n, from) =>
      Array.from({ length: n }, (_, i) => ({
        name: `Patch ${from + i}`,
        slug: `patch-${from + i}`,
        status: 'active',
      }));
    const fetchSpy = mockPages({
      '': { items: page(100, 0), next_cursor: 'cur-1' },
      'cur-1': { items: page(100, 100), next_cursor: 'cur-2' },
      'cur-2': { items: page(7, 200), next_cursor: '' },
    });

    const items = await patchPickerProvider(() => ({ type: 'Patches' }));

    expect(fetchSpy).toHaveBeenCalledTimes(3);
    expect(items).toHaveLength(207);
    // The row a single capped request would have lost.
    expect(items.find((i) => i.slug === 'patch-206')).toBeTruthy();
  });

  it('stops on an empty cursor without a further request', async () => {
    const fetchSpy = mockPages({
      '': { items: [{ name: 'Only', slug: 'only', status: 'active' }], next_cursor: '' },
    });
    const items = await patchPickerProvider(() => ({ type: 'Patches' }));
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(items).toHaveLength(1);
  });

  it('carries label, slug and href, and lets decorate drop a row', async () => {
    mockPages({
      '': {
        items: [
          { name: 'Keep Me', slug: 'keep-me', status: 'unclaimed' },
          { name: 'Drop Me', slug: 'drop-me', status: 'active' },
        ],
        next_cursor: '',
      },
    });

    const items = await patchPickerProvider((n) =>
      n.slug === 'drop-me' ? null : { type: 'Unclaimed patches', disabled: true },
    );

    expect(items).toEqual([
      {
        label: 'Keep Me',
        href: '/patches/keep-me',
        slug: 'keep-me',
        type: 'Unclaimed patches',
        disabled: true,
      },
    ]);
  });

  it('returns the pages that did arrive when one fails', async () => {
    let call = 0;
    vi.spyOn(global, 'fetch').mockImplementation(async () => {
      call += 1;
      if (call === 1) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            items: [{ name: 'First', slug: 'first', status: 'active' }],
            next_cursor: 'cur-1',
          }),
        };
      }
      return { ok: false, status: 500, json: async () => ({}) };
    });

    const items = await patchPickerProvider(() => ({ type: 'Patches' }));
    expect(items).toHaveLength(1);
    expect(items[0].slug).toBe('first');
  });
});
