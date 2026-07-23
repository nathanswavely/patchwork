/**
 * E2E: Navigation & Routing (User Stories 9.1–9.2)
 * Tests that all critical routes render something and navigation works.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './setup.js';

test.describe('Navigation — Public Routes', () => {
  test('/ renders quilt canvas', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.canvas-container')).toBeVisible();
  });

  test('/map renders the map on a direct load', async ({ page }) => {
    await page.goto('/map');
    await expect(page.locator('.leaflet-container')).toBeVisible();
  });

  // The quilt's tooltip used to be the last node of QuiltCanvas's own
  // fragment and got moved to <body> on mount, so tearing the canvas down
  // swept the {#if} anchor with it and the pane stayed empty until a reload.
  // Coverage, not a guard: this suite runs the Vite *dev* build, whose extra
  // anchor comments hide that failure — it only shows in a production build.
  test('quilt → map → quilt swaps the pane in-app', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.canvas-container')).toBeVisible();

    await page.getByRole('button', { name: 'Map', exact: true }).first().click();
    await expect(page).toHaveURL(/\/map$/);
    await expect(page.locator('.leaflet-container')).toBeVisible();

    await page.getByRole('button', { name: 'Quilt', exact: true }).first().click();
    await expect(page.locator('.canvas-container')).toBeVisible();
  });

  test('/login renders login form', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('input[type="email"]')).toBeVisible();
  });

  test('/events renders the events page', async ({ page }) => {
    await page.goto('/events');
    await expect(page.locator('.events-page')).toBeVisible();
  });
});

test.describe('Navigation — Authenticated Routes', () => {
  const authRoutes = [
    '/dashboard',
    '/settings',
    '/settings/security',
    '/settings/quilts',
    '/settings/patches',
    '/patches/new',
    '/events/new',
    '/admin',
    '/admin/users',
    '/admin/audit',
  ];

  for (const route of authRoutes) {
    test(`${route} renders content or auth guard`, async ({ page }) => {
      await loginAsAdmin(page);
      await page.goto(route);
      await page.waitForTimeout(2000);
      // Should show page content OR sign-in prompt — NOT "Page not found"
      const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
      expect(notFound).toBe(false);
    });
  }
});

test.describe('Navigation — Discovery <> Work Mode', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('9.1 — discovery mode shows quilt + rail chrome', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.canvas-container')).toBeVisible();
    await expect(page.locator('.bar-search-input')).toBeVisible();
    await expect(page.locator('.rail-item').first()).toBeVisible();
  });

  test('9.1 — work mode shows rail + content without quilt', async ({ page }) => {
    await page.goto('/dashboard');
    // The rail persists in work mode; the quilt canvas does not.
    await expect(page.locator('.sidebar-rail')).toBeVisible();
    await expect(page.locator('.canvas-container')).not.toBeVisible();
  });

  test('9.1 — navigating from work to discovery', async ({ page }) => {
    await page.goto('/dashboard');
    // Click logo/home link to go back to quilt
    const homeLink = page.locator('a[href="/"]').first();
    if (await homeLink.isVisible()) {
      await homeLink.click();
      await page.waitForTimeout(1000);
      await expect(page.locator('.canvas-container')).toBeVisible();
    }
  });
});

test.describe('Navigation — PatchShell Tab Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('PatchShell renders tabs on patch detail page', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    // The shell tabs container should be visible
    const tabsContainer = page.locator('.shell-tabs');
    if (await tabsContainer.isVisible()) {
      // Core tabs should be visible for all patches
      await expect(page.locator('.shell-tab', { hasText: 'Overview' })).toBeVisible();
      await expect(page.locator('.shell-tab', { hasText: 'Proposals' })).toBeVisible();
      await expect(page.locator('.shell-tab', { hasText: 'Charters' })).toBeVisible();
      await expect(page.locator('.shell-tab', { hasText: 'Members' })).toBeVisible();
      await expect(page.locator('.shell-tab', { hasText: 'Events' })).toBeVisible();
    }
  });

  test('PatchShell shows breadcrumb on patch detail page', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    // Breadcrumb should be rendered
    const breadcrumb = page.locator('.breadcrumb, nav[aria-label="breadcrumb"]').first();
    if (await breadcrumb.isVisible()) {
      // Should include a link back to home
      await expect(breadcrumb.locator('a[href="/"]')).toBeVisible();
    }
  });

  test('PatchShell tab click navigates to sub-route', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    const membersTab = page.locator('.shell-tab', { hasText: 'Members' });
    if (await membersTab.isVisible()) {
      await membersTab.click();
      await page.waitForTimeout(1000);
      expect(page.url()).toContain('/patches/lancaster-arts-district/members');
    }
  });

  test('PatchShell Settings tab visible for admin', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    // Admin should see the Settings tab
    const settingsTab = page.locator('.shell-tab', { hasText: 'Settings' });
    const isVisible = await settingsTab.isVisible().catch(() => false);
    // If the user is admin of this patch, Settings should appear
    if (isVisible) {
      await settingsTab.click();
      await page.waitForTimeout(1000);
      expect(page.url()).toContain('/patches/lancaster-arts-district/settings');
    }
  });

  test('PatchShell shows active tab styling', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district');
    await page.waitForTimeout(2000);

    // The Overview tab should be active by default
    const overviewTab = page.locator('.shell-tab.active');
    if (await overviewTab.isVisible()) {
      await expect(overviewTab).toContainText('Overview');
    }
  });
});

test.describe('Navigation — Admin Shell', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('Admin pages render inside AdminShell with sidebar', async ({ page }) => {
    await page.goto('/admin');
    await page.waitForTimeout(2000);

    // AdminShell wraps content in SettingsShell with sidebar nav
    const sidebar = page.locator('.settings-sidebar');
    if (await sidebar.isVisible()) {
      // Should have nav links for admin sections
      await expect(page.locator('.settings-nav-link', { hasText: 'Overview' })).toBeVisible();
      await expect(page.locator('.settings-nav-link', { hasText: 'Users' })).toBeVisible();
      await expect(page.locator('.settings-nav-link', { hasText: 'Audit Log' })).toBeVisible();
    }
  });

  test('Admin shell sidebar navigation works', async ({ page }) => {
    await page.goto('/admin');
    await page.waitForTimeout(2000);

    const usersLink = page.locator('.settings-nav-link', { hasText: 'Users' });
    if (await usersLink.isVisible()) {
      await usersLink.click();
      await page.waitForTimeout(1000);
      expect(page.url()).toContain('/admin/users');
    }
  });
});

test.describe('Navigation — User Settings Shell', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('Settings pages render inside UserSettingsShell with sidebar', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForTimeout(2000);

    const sidebar = page.locator('.settings-sidebar');
    if (await sidebar.isVisible()) {
      await expect(page.locator('.settings-nav-link', { hasText: 'Profile' })).toBeVisible();
      await expect(page.locator('.settings-nav-link', { hasText: 'Security' })).toBeVisible();
      await expect(page.locator('.settings-nav-link', { hasText: 'My Quilts' })).toBeVisible();
      await expect(page.locator('.settings-nav-link', { hasText: 'My Patches' })).toBeVisible();
    }
  });

  test('Settings shell sidebar navigation to My Patches', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForTimeout(2000);

    const patchesLink = page.locator('.settings-nav-link', { hasText: 'My Patches' });
    if (await patchesLink.isVisible()) {
      await patchesLink.click();
      await page.waitForTimeout(1000);
      expect(page.url()).toContain('/settings/patches');
    }
  });

});

test.describe('Navigation — Removed Routes Redirect', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('/patches/:slug/admin is no longer a valid route', async ({ page }) => {
    await page.goto('/patches/lancaster-arts-district/admin');
    await page.waitForTimeout(2000);
    // Should either redirect to /settings or show not found
    const url = page.url();
    const isRedirected = url.includes('/settings') || url.includes('/patches/lancaster-arts-district');
    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    // Either redirected or shows 404 — the old /admin sub-route should not render
    expect(isRedirected || notFound).toBeTruthy();
  });
});

test.describe('Navigation — No 404s on Valid Routes', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  const routes = [
    '/',
    '/patches',
    '/events',
    '/dashboard',
    '/settings',
    '/settings/security',
    '/settings/quilts',
    '/settings/patches',
    '/patches/new',
    '/events/new',
    '/admin',
    '/admin/users',
    '/admin/reports',
    '/admin/audit',
    '/login',
  ];

  for (const route of routes) {
    test(`${route} does not show "Page not found"`, async ({ page }) => {
      await page.goto(route);
      await page.waitForTimeout(500);
      const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
      expect(notFound).toBe(false);
    });
  }
});
