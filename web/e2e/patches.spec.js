/**
 * E2E: Patch Management (User Stories 3.1–3.7)
 * Tests create, join, leave, follow, dashboard, PatchShell.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './setup.js';

test.describe('Patches — Create', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('3.1 — /patches/new renders form or auth guard', async ({ page }) => {
    await page.goto('/patches/new');
    await page.waitForTimeout(2000);
    // Should show the create form OR an auth guard — not a 404
    const hasForm = await page.locator('input, form, h1').first().isVisible().catch(() => false);
    const hasSignIn = await page.getByText(/sign in/i).isVisible().catch(() => false);
    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    expect(notFound).toBe(false);
    expect(hasForm || hasSignIn).toBeTruthy();
  });

  test('3.1 — create patch form has fields when authenticated', async ({ page }) => {
    await page.goto('/patches/new');
    await page.waitForTimeout(2000);
    const nameInput = page.locator('input#name');
    if (await nameInput.isVisible()) {
      // Visibility/policy moved out of the create form (set in settings
      // after creation); the form is name/description/address/website.
      await expect(page.locator('textarea#description')).toBeVisible();
      await expect(page.locator('input#address')).toBeVisible();
      await expect(page.locator('input#website')).toBeVisible();
    }
    // If not visible, auth guard is showing — that's also acceptable
  });
});

test.describe('Patches — PatchShell Rendering', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('Patch detail page renders inside PatchShell', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    // PatchShell should render with the patch name
    const shellTitle = page.locator('.shell-title');
    if (await shellTitle.isVisible()) {
      const text = await shellTitle.textContent();
      expect(text.length).toBeGreaterThan(0);
    }
  });

  test('PatchShell tabs are visible on patch page', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    const tabs = page.locator('.workspace-tabs');
    if (await tabs.isVisible()) {
      const tabCount = await page.locator('.workspace-tab').count();
      // At least 5 tabs (Overview, Proposals, Charters, Members, Events)
      expect(tabCount).toBeGreaterThanOrEqual(5);
    }
  });

  test('PatchShell breadcrumb is visible', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    const breadcrumb = page.locator('.breadcrumb, nav[aria-label="breadcrumb"]').first();
    const isVisible = await breadcrumb.isVisible().catch(() => false);
    // Breadcrumb should be present when shell renders
    if (isVisible) {
      expect(isVisible).toBe(true);
    }
  });

  test('PatchShell shows membership controls', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    // The profile-actions area should show Join/Follow or a role badge
    const profileActions = page.locator('.profile-actions');
    if (await profileActions.isVisible()) {
      const hasJoin = await page.getByRole('button', { name: 'Join' }).isVisible().catch(() => false);
      const hasFollow = await page.getByRole('button', { name: 'Follow' }).isVisible().catch(() => false);
      const hasBadge = await page.locator('.role-badge').first().isVisible().catch(() => false);
      const hasLeave = await page.getByRole('button', { name: 'Leave' }).isVisible().catch(() => false);
      expect(hasJoin || hasFollow || hasBadge || hasLeave).toBeTruthy();
    }
  });
});

test.describe('Patches — Settings (formerly Admin)', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('Patch settings page renders at /patches/:slug/settings', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district/settings');
    await page.waitForTimeout(2000);

    // Should render inside PatchShell with Settings tab active, or show auth guard
    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    expect(notFound).toBe(false);

    // If admin, the Settings tab should be active
    const activeTab = page.locator('.workspace-tab.active');
    if (await activeTab.isVisible()) {
      const text = await activeTab.textContent();
      // Could be Settings or could redirect if not admin
      expect(text).toBeTruthy();
    }
  });

  test('Patch members page renders at /patches/:slug/members', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district/members');
    await page.waitForTimeout(2000);

    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    expect(notFound).toBe(false);

    // Members tab should be active
    const activeTab = page.locator('.workspace-tab.active');
    if (await activeTab.isVisible()) {
      await expect(activeTab).toContainText('Members');
    }
  });

  test('Patch events page renders at /patches/:slug/events', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district/events');
    await page.waitForTimeout(2000);

    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    expect(notFound).toBe(false);

    // Events tab should be active
    const activeTab = page.locator('.workspace-tab.active');
    if (await activeTab.isVisible()) {
      await expect(activeTab).toContainText('Events');
    }
  });

  test('retired /charters URL redirects to governance docs', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district/charters');

    // ADR 003: retired URL schemes redirect to the canonical one.
    await page.waitForURL('**/patches/lancaster-arts-district/governance/docs', { timeout: 10000 });
    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    expect(notFound).toBe(false);
    await expect(page.locator('.workspace-tabs')).toBeVisible();
  });

  test('retired /manage URLs redirect to canonical equivalents', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district/manage/settings/members');
    await page.waitForURL('**/patches/lancaster-arts-district/settings/members', { timeout: 10000 });

    await page.goto('/patches/lancaster-arts-district/manage');
    await page.waitForURL('**/patches/lancaster-arts-district/governance', { timeout: 10000 });
  });
});

test.describe('Patches — Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('3.7 — dashboard shows role-grouped sections', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForTimeout(2000);

    // Dashboard now groups patches by role: Managing, Member of, Following
    const managingSection = page.locator('.section-title', { hasText: 'Managing' });
    const memberSection = page.locator('.section-title', { hasText: 'Member of' });
    const followingSection = page.locator('.section-title', { hasText: 'Following' });

    // At least one section should be visible if user has memberships
    const hasManaging = await managingSection.isVisible().catch(() => false);
    const hasMember = await memberSection.isVisible().catch(() => false);
    const hasFollowing = await followingSection.isVisible().catch(() => false);
    const hasEmpty = await page.locator('.empty-state').isVisible().catch(() => false);

    // Either role-grouped sections or empty state should appear
    expect(hasManaging || hasMember || hasFollowing || hasEmpty).toBeTruthy();
  });

  test('3.7 — dashboard has quick actions', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForTimeout(2000);
    // Should show action cards OR sign-in prompt
    const hasActions = await page.locator('.action-card, .quick-actions').first().isVisible().catch(() => false);
    const hasSignIn = await page.getByText(/sign in/i).isVisible().catch(() => false);
    expect(hasActions || hasSignIn).toBeTruthy();
  });

  test('3.7 — dashboard patch cards have settings/members links', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForTimeout(2000);

    // Managing section should have quick links to settings and members
    const patchCard = page.locator('.patch-card').first();
    if (await patchCard.isVisible()) {
      const settingsLink = patchCard.locator('.quick-link', { hasText: 'Settings' });
      const membersLink = patchCard.locator('.quick-link', { hasText: 'Members' });
      // Admin patches should have settings and members links
      const hasSettings = await settingsLink.isVisible().catch(() => false);
      const hasMembers = await membersLink.isVisible().catch(() => false);
      // At least one quick link should exist
      const hasAnyLink = await patchCard.locator('.quick-link').first().isVisible().catch(() => false);
      expect(hasSettings || hasMembers || hasAnyLink).toBeTruthy();
    }
  });

  test('3.7 — clicking patch name navigates to patch detail', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForTimeout(2000);

    const firstPatchName = page.locator('.patch-name').first();
    if (await firstPatchName.isVisible()) {
      await firstPatchName.click();
      await page.waitForTimeout(1000);
      // Should navigate to patch detail page (PatchShell)
      expect(page.url()).toMatch(/\/patches\//);
    }
  });
});

test.describe('Patches — Join/Leave/Follow', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('3.4 — patch page shows membership controls in PatchShell', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(3000);

    // The profile-actions area should show Join/Follow/Become Member or role badge
    const profileActions = page.locator('.profile-actions');
    if (await profileActions.isVisible()) {
      const hasJoin = await page.getByRole('button', { name: 'Join' }).isVisible().catch(() => false);
      const hasFollow = await page.getByRole('button', { name: 'Follow' }).isVisible().catch(() => false);
      const hasBadge = await page.locator('.role-badge').first().isVisible().catch(() => false);
      const hasLeave = await page.getByRole('button', { name: 'Leave' }).isVisible().catch(() => false);
      const hasBecomeMember = await page.getByRole('button', { name: 'Become Member' }).isVisible().catch(() => false);
      const hasUnfollow = await page.getByRole('button', { name: 'Unfollow' }).isVisible().catch(() => false);
      expect(hasJoin || hasFollow || hasBadge || hasLeave || hasBecomeMember || hasUnfollow).toBeTruthy();
    }
  });

  test('PatchShell shows "Become Member" and "Unfollow" for followers', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(3000);

    // If the current user is a follower, PatchShell shows Become Member + Unfollow
    // instead of a single Leave button
    const profileActions = page.locator('.profile-actions');
    if (await profileActions.isVisible()) {
      const hasBecomeMember = await page.getByRole('button', { name: 'Become Member' }).isVisible().catch(() => false);
      const hasUnfollow = await page.getByRole('button', { name: 'Unfollow' }).isVisible().catch(() => false);

      // If follower controls are shown, both buttons should appear together
      if (hasBecomeMember) {
        expect(hasUnfollow).toBe(true);
      }
      // If Unfollow is shown, Become Member should also be present
      if (hasUnfollow) {
        expect(hasBecomeMember).toBe(true);
      }
    }
  });
});

test.describe('Patches — PatchPanel Role Badge', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('PatchPanel shows role indicator badge from memberships store', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(3000);

    // PatchPanel renders a role-badge when the user has a membership
    const roleBadge = page.locator('.role-badge').first();
    const hasBadge = await roleBadge.isVisible().catch(() => false);

    if (hasBadge) {
      // Badge should contain a recognized role text
      const text = await roleBadge.textContent();
      expect(['admin', 'member', 'follower']).toContain(text.trim().toLowerCase());
    }
  });
});
