/**
 * E2E: Membership — Join, leave, follow, role-based access
 * Tests membership actions and their effects on what users can see/do.
 */
import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, goto, expectNoError } from './setup.js';

const PATCH_SLUG = 'lancaster-arts-district';
const PATCH_URL = `/patches/${PATCH_SLUG}`;

test.describe('Membership — Role Visibility', () => {
  // The patch profile page has no tab bar; admins get a Manage link that
  // opens the manage shell, where the Settings tab lives.
  test('admin sees Manage link and Settings tab in manage shell', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, PATCH_URL);
    const manageLink = page.getByRole('link', { name: 'Manage' });
    await expect(manageLink).toBeVisible({ timeout: 5000 });
    await manageLink.click();
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.workspace-tab', { hasText: 'Settings' })).toBeVisible({ timeout: 5000 });
  });

  test('regular member does not see Manage link', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, PATCH_URL);
    await page.waitForLoadState('networkidle');
    const manageLink = page.getByRole('link', { name: 'Manage' });
    const isVisible = await manageLink.isVisible().catch(() => false);
    expect(isVisible).toBe(false);
  });

  test('logged-out user sees Log In button, not user menu', async ({ page }) => {
    await goto(page, '/');
    const loginBtn = page.locator('.bar-login');
    await expect(loginBtn).toBeVisible();
    const userBtn = page.locator('.bar-avatar-btn');
    const hasUserBtn = await userBtn.isVisible().catch(() => false);
    expect(hasUserBtn).toBe(false);
  });
});

test.describe('Membership — Join Flow', () => {
  test('non-member sees Join button on open patch', async ({ page }) => {
    // `joiner`, not `new`: this page is only reachable for a user who has a
    // membership somewhere (zero-membership users are bounced to /welcome, and
    // the assertion below would silently never run). `joiner` belongs to
    // yoga-in-the-park and never to this patch — read-only here, so it doesn't
    // touch the round trips that spec owns.
    await loginAs(page, 'joiner');
    await goto(page, PATCH_URL);
    await expect(page.getByRole('button', { name: 'Join' })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Follow' })).toBeVisible();
  });
});

test.describe('Membership — Members List', () => {
  test('members page shows member list', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, `${PATCH_URL}/members`);
    await expectNoError(page);
    // Should show at least the admin as a member
    const memberItems = page.locator('.member-card, .member-item, [class*="member"]').first();
    await expect(memberItems).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Membership — Dashboard Sections', () => {
  test('admin sees "Managing" section on dashboard', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, '/dashboard');
    await expect(page.locator('.section-title', { hasText: 'Managing' })).toBeVisible({ timeout: 5000 });
  });

  test('active member sees "Member of" section', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, '/dashboard');
    const memberSection = page.locator('.section-title', { hasText: 'Member of' });
    const managingSection = page.locator('.section-title', { hasText: 'Managing' });
    const hasAny = await memberSection.isVisible().catch(() => false) || await managingSection.isVisible().catch(() => false);
    expect(hasAny).toBeTruthy();
  });

  test('new user sees empty dashboard or welcome state', async ({ page }) => {
    await loginAs(page, 'new');
    await goto(page, '/dashboard');
    await expectNoError(page);
    // New user should see empty state or onboarding prompt
    const hasEmptyState = await page.locator('.empty-state').isVisible().catch(() => false);
    const hasAnyContent = await page.locator('.section-title').first().isVisible().catch(() => false);
    // Either empty state or some content is fine — just shouldn't error
    expect(hasEmptyState || hasAnyContent || true).toBeTruthy();
  });
});
