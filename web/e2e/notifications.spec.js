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
    await expect(page.locator('.toggle-track').first()).toBeVisible();
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
