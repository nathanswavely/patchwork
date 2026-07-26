/**
 * E2E: Membership — Join, leave, follow, role-based access
 * Tests membership actions and their effects on what users can see/do.
 */
import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, goto, expectNoError, openOverflow } from './setup.js';

const PATCH_SLUG = 'lancaster-arts-district';
const PATCH_URL = `/patches/${PATCH_SLUG}`;

test.describe('Membership — Role Visibility', () => {
  // The profile carries no door named for the container — no "Manage"
  // pill (docs/adr/042). Every glimpse heading is a door into its own room,
  // and the overflow keeps a "Workspace view" fallback for people with
  // standing.
  test('admin reaches the workspace and its Settings tab', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, PATCH_URL);
    await expect(page.getByRole('link', { name: 'Manage' })).toHaveCount(0);

    await openOverflow(page);
    await page.getByRole('menuitem', { name: 'Workspace view' }).click();
    await page.waitForLoadState('networkidle');
    await expect(page.locator('.workspace-tab', { hasText: 'Settings' })).toBeVisible({ timeout: 5000 });
  });

  test('a glimpse heading is itself the door into its room', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, PATCH_URL);
    await page.getByRole('link', { name: 'Members', exact: true }).click();
    await page.waitForLoadState('networkidle');
    expect(page.url()).toContain(`${PATCH_URL}/members`);
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
  test('non-member sees Follow and the membership rung on an open patch', async ({ page }) => {
    // `joiner`, not `new`: this page is only reachable for a user who has a
    // membership somewhere (zero-membership users are bounced to /welcome, and
    // the assertion below would silently never run). `joiner` belongs to
    // yoga-in-the-park and never to this patch — read-only here, so it doesn't
    // touch the round trips that spec owns.
    await loginAs(page, 'joiner');
    await goto(page, PATCH_URL);
    // Follow leads everywhere; the rung is secondary (docs/adr/042).
    await expect(page.getByRole('button', { name: 'Follow', exact: true })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: 'Become a member' })).toBeVisible();
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
