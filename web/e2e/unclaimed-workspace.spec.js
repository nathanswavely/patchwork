/**
 * E2E: the instance admin's workspace for an unclaimed patch (issue #6).
 *
 * Before the fix, an unclaimed patch rendered zero workspace tabs and the
 * profile's only management entry pointed at a governance hub that doesn't
 * apply. This spec walks the fixed path: Manage → events workspace → the
 * pared-down tab row and settings sections, and the Verification section that
 * replaces Members/Notifications.
 *
 * DATA OWNERSHIP: creates its own uniquely-named unclaimed patch via the admin
 * API (like claims.spec.js) and never claims it, so no seed state is mutated
 * and the admin's memberships are untouched.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

test.describe.configure({ mode: 'serial' });

let slug = '';

test.describe('Unclaimed patch workspace', () => {
  test('admin creates an unclaimed patch with a website-derived domain', async ({ page }) => {
    await loginAs(page, 'admin');
    await page.goto('/');
    const unique = `Unclaimed WS Venue ${Date.now()}`;
    const res = await page.request.post('/api/v1/admin/unclaimed', {
      data: { name: unique, website: 'https://unclaimedws.example' },
      headers: { 'X-Patchwork-Request': 'true' },
    });
    expect(res.ok()).toBeTruthy();
    slug = (await res.json()).slug;
    expect(slug).toBeTruthy();
  });

  test('the profile Manage entry lands on the events workspace', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}`);

    await page.getByRole('link', { name: 'Manage' }).click();
    await page.waitForLoadState('networkidle');
    expect(page.url()).toContain(`/patches/${slug}/events`);

    // The events tab is live: the community-submitted calendar renders.
    await expect(page.getByText('Community-submitted').first()).toBeVisible();
  });

  test('the workspace shows only Events and Settings tabs', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/events`);

    const tabRow = page.locator('.workspace-tabs');
    await expect(tabRow.getByRole('link', { name: 'Events' })).toBeVisible();
    await expect(tabRow.getByRole('link', { name: 'Settings' })).toBeVisible();
    // Governance and Members don't apply to a patch nobody runs yet.
    await expect(tabRow.getByRole('link', { name: 'Governance' })).toHaveCount(0);
    await expect(tabRow.getByRole('link', { name: 'Members' })).toHaveCount(0);
  });

  test('settings offers Info, Appearance, Verification, Danger — not Members or Notifications', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/settings`);
    // Bare /settings redirects to /info.
    await expect(page).toHaveURL(new RegExp(`/patches/${slug}/settings/info`));

    const sidebar = page.locator('.settings-sidebar');
    await expect(sidebar.getByRole('link', { name: 'Patch Info' })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Appearance' })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Verification' })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Danger Zone' })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Members' })).toHaveCount(0);
    await expect(sidebar.getByRole('link', { name: 'Notifications' })).toHaveCount(0);
  });

  test('a stale Members link redirects back to Info', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/settings/members`);
    await expect(page).toHaveURL(new RegExp(`/patches/${slug}/settings/info`));
  });

  test('Verification shows the derived domain and can change it', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/settings/verification`);

    const input = page.locator('.v-input');
    await expect(input).toHaveValue('unclaimedws.example');

    await input.fill('changed.example');
    await page.getByRole('button', { name: 'Save' }).click();
    await page.waitForLoadState('networkidle');

    // Reload proves it persisted through the admin endpoint.
    await goto(page, `/patches/${slug}/settings/verification`);
    await expect(page.locator('.v-input')).toHaveValue('changed.example');
  });
});
