/**
 * E2E: Settings — User settings, patch settings, admin pages
 * Tests that settings pages render correctly and forms work.
 */
import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, goto, expectNoError } from './setup.js';

test.describe('User Settings — Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('all settings sidebar links work', async ({ page }) => {
    const links = [
      { label: 'Profile', url: '/settings' },
      { label: 'Notifications', url: '/settings/notifications' },
      { label: 'Security', url: '/settings/security' },
      { label: 'My Patches', url: '/settings/patches' },
      { label: 'My Quilts', url: '/settings/quilts' },
    ];

    await goto(page, '/settings');
    for (const link of links) {
      const navLink = page.locator('.settings-nav-link', { hasText: link.label });
      if (await navLink.isVisible()) {
        await navLink.click();
        await page.waitForLoadState('networkidle');
        expect(page.url()).toContain(link.url);
        await expectNoError(page);
      }
    }
  });
});

test.describe('User Settings — Security', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  // Read-only: asserts the session manager renders and flags the current
  // session. Deliberately does NOT revoke anything — the dev session tokens
  // are the shared login mechanism for every spec and worker (see the data
  // ownership note in setup.js), so revoking one here would sign the whole
  // suite out. Mutation paths are covered by the Go handler tests.
  test('security page lists active sessions and marks the current one', async ({ page }) => {
    await goto(page, '/settings/security');
    await expectNoError(page);
    await expect(page.getByRole('heading', { name: 'Active sessions' })).toBeVisible();
    await expect(page.getByText('This session').first()).toBeVisible();
  });
});

test.describe('User Settings — Profile', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('profile page shows display name field', async ({ page }) => {
    await goto(page, '/settings');
    await expectNoError(page);
    const nameField = page.locator('input[name="display_name"], input#display_name, input#displayName').first();
    const hasNameField = await nameField.isVisible().catch(() => false);
    // Should have some editable fields
    const hasAnyInput = await page.locator('input[type="text"]').first().isVisible().catch(() => false);
    expect(hasNameField || hasAnyInput).toBeTruthy();
  });
});

test.describe('Patch Settings — Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('all patch settings sidebar links work', async ({ page }) => {
    const slug = 'lancaster-arts-district';
    const links = [
      { label: 'Patch Info', url: '/settings/info' },
      { label: 'Members', url: '/settings/members' },
      { label: 'Notifications', url: '/settings/notifications' },
      { label: 'Danger Zone', url: '/settings/danger' },
    ];

    await goto(page, `/patches/${slug}/settings/info`);
    for (const link of links) {
      const navLink = page.locator('.settings-nav-link', { hasText: link.label });
      if (await navLink.isVisible()) {
        await navLink.click();
        await page.waitForLoadState('networkidle');
        expect(page.url()).toContain(link.url);
        await expectNoError(page);
      }
    }
  });
});

test.describe('Admin — Pages Load', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  const adminPages = [
    { path: '/admin', title: 'Overview' },
    { path: '/admin/users', title: 'Users' },
    { path: '/admin/reports', title: 'Reports' },
    { path: '/admin/audit', title: 'Audit' },
    { path: '/admin/submissions', title: 'Submissions' },
    { path: '/admin/claims', title: 'Claims' },
  ];

  for (const p of adminPages) {
    test(`${p.path} loads without error`, async ({ page }) => {
      await goto(page, p.path);
      await expectNoError(page);
    });
  }
});

/**
 * OWNS: the instance's icon design (instance_settings.icon_design). The
 * test drafts one and resets it in the same run, so every other spec
 * still sees the starter block assigned from the quilt's name. No other
 * spec may assert on the quilt icon.
 */
test.describe('Admin — Quilt Icon Designer', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('drafts the quilt icon from a starter block and resets it', async ({ page }) => {
    await goto(page, '/admin/quilt');
    await expectNoError(page);

    // No upload path survives (docs/adr/042).
    await expect(page.locator('input[type="file"]')).toHaveCount(0);

    const preview = page.locator('.icon-preview');
    await expect(preview).toBeVisible();
    await expect(preview.locator('polygon').first()).toBeVisible();
    await expect(page.locator('.starter-option')).not.toHaveCount(0);
    await expect(page.locator('.drafter-canvas')).toBeVisible();

    const servedBefore = await page.evaluate(() =>
      fetch('/api/v1/instance/icon?e2e=' + Date.now()).then((r) => r.text()));

    // Nothing to save until something changes.
    const save = page.getByRole('button', { name: /Save icon/ });
    await expect(save).toBeDisabled();

    await page.locator('.starter-option', { hasText: 'Flying Geese' }).click();
    await expect(save).toBeEnabled();
    await save.click();

    await expect(page.locator('.icon-kind')).toContainText('Drafted for this quilt');
    const servedAfter = await page.evaluate(() =>
      fetch('/api/v1/instance/icon?e2e=' + Date.now()).then((r) => r.text()));
    expect(servedAfter).not.toBe(servedBefore);
    expect(servedAfter).toContain('<polygon');

    await page.getByRole('button', { name: 'Reset icon' }).click();
    await expect(page.locator('.icon-kind')).toContainText('Assigned from the quilt');
    const servedReset = await page.evaluate(() =>
      fetch('/api/v1/instance/icon?e2e=' + Date.now()).then((r) => r.text()));
    expect(servedReset).toBe(servedBefore);
  });
});

test.describe('Admin — Non-Admin Access', () => {
  test('non-admin user cannot access admin pages', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, '/admin');
    // Should show access denied or redirect
    const hasContent = await page.locator('h1').isVisible().catch(() => false);
    // Should NOT show admin dashboard content
    const hasAdminContent = await page.locator('.admin-stats, .admin-dashboard').isVisible().catch(() => false);
    expect(hasAdminContent).toBe(false);
  });
});
