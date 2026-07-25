/**
 * E2E: Notifications — Bell, full page, preferences, activity feed
 */
import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, goto, expectNoError, escapeOnboarding } from './setup.js';

test.describe('Notifications — Bell', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('notification bell is visible when logged in', async ({ page }) => {
    await goto(page, '/dashboard');
    const bell = page.locator('.bell-btn');
    await expect(bell).toBeVisible();
  });

  // The bell lives in the global bar and opens a side panel (not a dropdown).
  test('clicking the bell opens the panel', async ({ page }) => {
    await goto(page, '/dashboard');
    await page.locator('.bell-btn').click();
    await expect(page.locator('.sidepanel')).toBeVisible({ timeout: 3000 });
  });

  test('panel has "View all notifications" link', async ({ page }) => {
    await goto(page, '/dashboard');
    await page.locator('.bell-btn').click();
    const viewAll = page.locator('.notif-view-all');
    await expect(viewAll).toBeVisible({ timeout: 3000 });
  });

  test('"View all" link navigates to /notifications', async ({ page }) => {
    await goto(page, '/dashboard');
    await page.locator('.bell-btn').click();
    const viewAll = page.locator('.notif-view-all');
    await expect(viewAll).toBeVisible({ timeout: 3000 });
    await viewAll.click();
    await page.waitForLoadState('networkidle');
    expect(page.url()).toContain('/notifications');
  });

  test('empty panel shows "You\'re all caught up"', async ({ page }) => {
    await loginAs(page, 'new');
    await escapeOnboarding(page);
    await goto(page, '/dashboard');
    await page.locator('.bell-btn').click();
    const empty = page.locator('.notif-empty');
    await expect(empty).toBeVisible({ timeout: 3000 });
    await expect(empty).toContainText("You're all caught up");
  });
});

// The two behaviours issues #55 and #56 were reported at: the badge has to
// move the moment something is read (it used to wait for the bell's 60s poll),
// and a notification has to open the thing it is about (event and charter
// links were built to paths the router never had, so they fell through to the
// home quilt or rendered "proposal not found").
//
// These mark the admin's notifications read, which the ownership map in
// setup.js assigns to this spec. Declaration order matters: the click test
// needs something unread, so it runs before mark-all-read.
test.describe('Notifications — Reading and navigating', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  // Find an unread notification by title prefix, open the panel, click it.
  async function clickNotification(page, prefix) {
    await goto(page, '/dashboard');
    const feed = await page.request.get('/api/v1/notifications?limit=20').then((r) => r.json());
    const target = (feed.items || []).find((n) => n.title.startsWith(prefix) && !n.read_at);
    expect(target, `seed data should include an unread "${prefix}…" notification`).toBeTruthy();

    const before = Number(await page.locator('.badge-count').innerText());
    expect(before).toBeGreaterThan(0);

    await page.locator('.bell-btn').click();
    await expect(page.locator('.sidepanel')).toBeVisible({ timeout: 3000 });
    await page.locator('.notif-item', { hasText: target.title }).first().click();
    return { target, before };
  }

  // The seeder plants one deep link per broken shape (cmd/seed/main.go): an
  // event and a charter. Picking "whatever is unread first" is not enough —
  // that lands on a list link, which routed fine before the fix too.
  for (const { label, prefix } of [
    { label: 'an event', prefix: 'Tomorrow: ' },
    { label: 'a charter', prefix: 'Charter updated: ' },
  ]) {
    test(`clicking ${label} notification opens what it is about`, async ({ page }) => {
      const { target } = await clickNotification(page, prefix);

      // Wait for the navigation rather than for the network: the click marks
      // the notification read first, so in an SPA "networkidle" can be true
      // before the route has moved at all.
      const expected = target.link.split('?')[0];
      await page.waitForURL((url) => new URL(url).pathname === expected, { timeout: 5000 });

      // The URL proves nothing on its own — an unroutable link pushes its path
      // and then renders whatever the fallback is, which is exactly how this
      // bug hid. So assert the entity is actually on screen. It has to be a
      // POSITIVE assertion: "the fallback is absent" passes trivially while
      // the fallback is still loading, which is how a first draft of this test
      // went green against the bug it was written to catch.
      const entityTitle = target.title.slice(prefix.length);
      await expect(page.getByRole('heading', { name: entityTitle })).toBeVisible({ timeout: 5000 });
      await expectNoError(page);
    });
  }

  // Deliberately a notification whose target stays inside the social shell, so
  // the global bar (and its bell) is not remounted by the navigation. A target
  // in the patch workspace would remount the bell, and its mount-time refresh
  // would supply the right number on its own — making the assertion pass
  // whether or not reading updates the count. Also a different notification
  // from the two above, which have already been read by the time this runs.
  test('reading a notification drops the badge without waiting for the poll', async ({ page }) => {
    const { before } = await clickNotification(page, 'New event: ');
    // The poll is 60s away, so anything visible this soon is the local update.
    await expect(page.locator('.badge-count')).toHaveText(String(before - 1), { timeout: 3000 });
  });

  test('"mark all read" clears the badge', async ({ page }) => {
    await goto(page, '/dashboard');
    await page.locator('.bell-btn').click();
    await expect(page.locator('.sidepanel')).toBeVisible({ timeout: 3000 });

    const markAll = page.locator('.notif-mark-all');
    await expect(markAll).toBeVisible({ timeout: 3000 });
    await markAll.click();

    // Gone, not decremented — read-all clears the whole table server-side,
    // not just the rows the panel happens to have loaded.
    await expect(page.locator('.badge-count')).toHaveCount(0, { timeout: 3000 });
    const count = await page.request.get('/api/v1/notifications/count').then((r) => r.json());
    expect(count.unread).toBe(0);
  });
});

test.describe('Notifications — Full Page', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('/notifications page loads with filters', async ({ page }) => {
    await goto(page, '/notifications');
    await expectNoError(page);
    await expect(page.locator('h1', { hasText: 'Notifications' })).toBeVisible();
    // Filter chips should be present
    await expect(page.locator('.chip', { hasText: 'All' })).toBeVisible();
  });

  test('category filter chips work', async ({ page }) => {
    await goto(page, '/notifications');
    const proposalsChip = page.locator('.chip', { hasText: 'Proposals' });
    if (await proposalsChip.isVisible()) {
      await proposalsChip.click();
      await page.waitForLoadState('networkidle');
      await expectNoError(page);
    }
  });

  test('unread toggle works', async ({ page }) => {
    await goto(page, '/notifications');
    const unreadChip = page.locator('.chip', { hasText: 'Unread only' });
    if (await unreadChip.isVisible()) {
      await unreadChip.click();
      await page.waitForLoadState('networkidle');
      await expectNoError(page);
    }
  });
});

test.describe('Notifications — Activity Feed', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('/activity page loads with grouped items', async ({ page }) => {
    await goto(page, '/activity');
    await expectNoError(page);
    await expect(page.locator('h1', { hasText: "What's New" })).toBeVisible();
    // Should show day groups with activity items
    const dayGroup = page.locator('.day-group').first();
    const hasActivity = await dayGroup.isVisible().catch(() => false);
    const hasEmpty = await page.locator('.empty-state').isVisible().catch(() => false);
    expect(hasActivity || hasEmpty).toBeTruthy();
  });

  test('activity items are clickable and navigate', async ({ page }) => {
    await goto(page, '/activity');
    const firstItem = page.locator('.activity-item').first();
    if (await firstItem.isVisible()) {
      await firstItem.click();
      await page.waitForLoadState('networkidle');
      // Should navigate to a detail page
      expect(page.url()).not.toContain('/activity');
    }
  });

  test('brand-new user is routed to first-run welcome', async ({ page }) => {
    await loginAs(page, 'new');
    await goto(page, '/activity');
    await expectNoError(page);
    // A user with zero memberships gets redirected to the onboarding welcome
    // screen instead of an empty activity feed. However, the 'new' dev user
    // is shared across the whole e2e run: an earlier spec file (e.g.
    // join-follow) may have already given it a membership via
    // escapeOnboarding(), in which case it no longer qualifies for the
    // first-run redirect and the normal /activity page renders instead.
    // Assert whichever of the two valid states actually applies rather than
    // assuming a specific membership count.
    const welcome = page.getByText('Build your quilt');
    const activityHeading = page.locator('h1', { hasText: "What's New" });
    await expect(welcome.or(activityHeading)).toBeVisible({ timeout: 5000 });

    if (await welcome.isVisible().catch(() => false)) {
      expect(page.url()).toContain('/welcome');
    } else {
      await expect(activityHeading).toBeVisible();
    }
  });
});

test.describe('Notifications — Preferences', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('/settings/notifications loads with preference grid', async ({ page }) => {
    await goto(page, '/settings/notifications');
    await expectNoError(page);
    await expect(page.locator('h2', { hasText: 'Notification Preferences' })).toBeVisible();
    // Should show category groups with toggle switches
    await expect(page.locator('.category-label').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.prefs-toggle .switch').first()).toBeVisible();
  });

  test('toggling a preference saves without error', async ({ page }) => {
    await goto(page, '/settings/notifications');
    const firstToggle = page.locator('.prefs-toggle input').first();
    if (await firstToggle.isVisible()) {
      await firstToggle.click();
      // Wait for debounced save
      await page.waitForTimeout(1000);
      // Should not show any error toast
      const errorToast = page.locator('.toast-error');
      const hasError = await errorToast.isVisible().catch(() => false);
      expect(hasError).toBe(false);
    }
  });

  test('settings sidebar shows Notifications link', async ({ page }) => {
    await goto(page, '/settings');
    const notifLink = page.locator('.settings-nav-link', { hasText: 'Notifications' });
    await expect(notifLink).toBeVisible();
  });
});

test.describe('Notifications — Patch Config', () => {
  test('admin can access patch notification settings', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, `/patches/lancaster-arts-district/settings/notifications`);
    await expectNoError(page);
    await expect(page.locator('h2', { hasText: 'Notification Settings' })).toBeVisible();
    // Should show category toggles
    await expect(page.locator('.category-row').first()).toBeVisible({ timeout: 5000 });
  });

  test('toggling a category saves without error', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, `/patches/lancaster-arts-district/settings/notifications`);
    const firstToggle = page.locator('.toggle-label input').first();
    if (await firstToggle.isVisible()) {
      await firstToggle.click();
      await page.waitForTimeout(500);
      await expectNoError(page);
    }
  });
});
