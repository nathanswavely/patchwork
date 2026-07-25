/**
 * The unread badge used to move only when the bell's 60-second poll came
 * back, so reading a notification left the badge sitting there — long enough
 * that it read as broken (issue #55). Reading is a local fact; the poll is
 * only reconciliation.
 *
 * The store carries $state, which needs the Svelte compiler, so it's imported
 * from a .svelte.js module under vitest's svelte plugin. The surfaces that
 * call into it are asserted against source text, matching the approach in
 * about-lining-routes.test.js.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

vi.mock('../lib/api.js', () => ({
  api: vi.fn(),
}));

describe('unread count store', () => {
  let store;
  let api;

  beforeEach(async () => {
    vi.resetModules();
    ({ api } = await import('../lib/api.js'));
    api.mockReset();
    store = await import('../stores/notifications.svelte.js');
  });

  it('refreshUnread takes the server count', async () => {
    api.mockResolvedValue({ unread: 7 });
    await store.refreshUnread();
    expect(store.getUnread()).toBe(7);
    expect(api).toHaveBeenCalledWith('notifications/count');
  });

  it('a failed refresh keeps the last known count rather than zeroing it', async () => {
    api.mockResolvedValue({ unread: 3 });
    await store.refreshUnread();
    api.mockRejectedValue(new Error('offline'));
    await store.refreshUnread();
    expect(store.getUnread()).toBe(3);
  });

  it('reading one notification drops the badge by one immediately', () => {
    store.setUnread(3);
    store.decrementUnread();
    expect(store.getUnread()).toBe(2);
  });

  it('never counts below zero', () => {
    store.setUnread(1);
    store.decrementUnread();
    store.decrementUnread();
    expect(store.getUnread()).toBe(0);
    store.setUnread(-5);
    expect(store.getUnread()).toBe(0);
  });

  it('clearUnread zeroes the count, not just the rows on screen', () => {
    // "Mark all read" clears the whole table server-side, so a count of 40
    // with 20 rows rendered must still land on zero.
    store.setUnread(40);
    store.clearUnread();
    expect(store.getUnread()).toBe(0);
  });
});

describe('every surface that marks a notification read updates the badge', () => {
  const bell = source('components/NotificationBell.svelte');
  const bar = source('components/GlobalBar.svelte');
  const page = source('pages/Notifications.svelte');

  it('the bell polls through the store instead of owning the fetch', () => {
    expect(bell).toContain('refreshUnread');
    // No second copy of the count-fetching logic to drift from the store.
    expect(bell).not.toContain("api('notifications/count')");
  });

  it('the desktop panel decrements on read and clears on mark-all-read', () => {
    expect(bar).toContain("import { clearUnread, decrementUnread } from '../stores/notifications.svelte.js'");
    expect(bar).toMatch(/notifications\/\$\{notif\.id\}\/read[\s\S]{0,200}decrementUnread\(\)/);
    expect(bar).toMatch(/notifications\/read-all[\s\S]{0,200}clearUnread\(\)/);
  });

  it('the notifications page decrements on read and clears on mark-all-read', () => {
    expect(page).toContain("import { clearUnread, decrementUnread } from '../stores/notifications.svelte.js'");
    expect(page).toMatch(/notifications\/\$\{notif\.id\}\/read[\s\S]{0,200}decrementUnread\(\)/);
    expect(page).toMatch(/notifications\/read-all[\s\S]{0,200}clearUnread\(\)/);
  });
});
