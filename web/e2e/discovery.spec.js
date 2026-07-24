/**
 * E2E: Discovery Mode (User Stories 1.1–1.8)
 * Tests the quilt canvas, top bar (search, workspace switcher, user menu),
 * sidebar nav, and patch/event browsing.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './setup.js';

test.describe('Discovery — Quilt View', () => {
  test('1.1 — quilt renders with tiles on page load', async ({ page }) => {
    await page.goto('/');
    // Wait for SVG tiles to appear
    await page.locator('svg .tile').first().waitFor({ timeout: 10000 });
  });

  test('1.1 — canvas has woven background texture', async ({ page }) => {
    await page.goto('/');
    const container = page.locator('.canvas-container');
    await expect(container).toBeVisible();
    // lt-fill-canvas and lt-texture-grain classes
    await expect(container).toHaveClass(/lt-fill-canvas/);
    await expect(container).toHaveClass(/lt-texture-grain/);
  });

  test('1.3 — filter button opens chip strip', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);
    const filterBtn = page.locator('.filter-btn');
    if (await filterBtn.isVisible()) {
      await filterBtn.click();
      await expect(page.locator('.filter-chips .chip').first()).toBeVisible();
    }
  });

  test('1.4 — top bar search accepts text and opens the dropdown', async ({ page }) => {
    // The search is an autocomplete dropdown (docs/adr/033): typing shows
    // typed results plus the "Show matches on the quilt" action row.
    await page.goto('/');
    const searchInput = page.locator('.finder-input');
    await expect(searchInput).toBeVisible();
    await searchInput.click();
    await searchInput.fill('Lancaster');
    await expect(searchInput).toHaveValue('Lancaster');
    await expect(page.locator('.finder-results')).toBeVisible();
    await expect(page.locator('.finder-action')).toContainText('Show matches on the quilt');
  });

  test('1.8 — theme toggle in user menu switches between light and dark', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    const initialTheme = await page.locator('html').getAttribute('data-theme');

    // Theme toggle lives in the user menu
    await page.locator('.bar-avatar-btn').click();
    await page.locator('.user-dropdown button', { hasText: /mode/i }).click();
    const newTheme = await page.locator('html').getAttribute('data-theme');
    expect(newTheme).not.toBe(initialTheme);

    // Toggle back — the dropdown stays open after toggling
    await page.locator('.user-dropdown button', { hasText: /mode/i }).click();
    const revertedTheme = await page.locator('html').getAttribute('data-theme');
    expect(revertedTheme).toBe(initialTheme);
  });
});

test.describe('Discovery — Sidebar Navigation', () => {
  test('1.5 — Patches nav item shows the quilt', async ({ page }) => {
    await page.goto('/events');
    await page.waitForTimeout(500);

    await page.locator('.rail-item', { hasText: 'Patches' }).click();
    await page.locator('svg .tile').first().waitFor({ timeout: 10000 });
    expect(new URL(page.url()).pathname).toBe('/');
  });

  test('1.6 — Events nav item shows the event list', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.rail-item', { hasText: 'Events' }).click();
    await page.waitForTimeout(500);
    expect(page.url()).toContain('/events');
  });

  test('9.2 — notification panel opens from bell and closes', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.bar-bell .bell-btn').click();
    const panel = page.locator('.sidepanel');
    await expect(panel).toBeVisible({ timeout: 2000 });

    await page.locator('.sidepanel .close-btn, .sidepanel-close').first().click();
    await expect(panel).not.toBeVisible({ timeout: 2000 });
  });
});

test.describe('Discovery — Workspace Switcher', () => {
  test('logged out — switcher shows instance only, no My Quilt option', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.scope-btn').click();
    const dropdown = page.locator('.scope-dropdown');
    await expect(dropdown).toBeVisible();
    await expect(dropdown.locator('.scope-option')).toHaveCount(1);
  });

  test('logged in — switcher offers instance and My Quilt', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.scope-btn').click();
    const options = page.locator('.scope-dropdown .scope-option');
    await expect(options).toHaveCount(2);
    await expect(options.nth(1)).toContainText('My Quilt');
  });

  // Scope lives in the URL (docs/adr/035): / is the whole quilt for
  // everyone — the old logged-in default to My Quilt is gone.
  test('logged in — / is the whole quilt, never My Quilt', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(1000);

    await expect(page.locator('.scope-btn .logo-label')).not.toContainText('My Quilt');
  });

  test('logged in — /my is My Quilt scope', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/my');
    await page.waitForTimeout(1000);

    await expect(page.locator('.scope-btn .logo-label')).toContainText('My Quilt');
  });

  test('logged in — selecting My Quilt moves to /my', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(1000);

    await page.locator('.scope-btn').click();
    await page.locator('.scope-dropdown .scope-option').nth(1).click();
    await expect(page.locator('.scope-btn .logo-label')).toContainText('My Quilt');
    await expect(page).toHaveURL(/\/my$/);
  });

  test('logged in — selecting the instance switches scope back', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/my');
    await page.waitForTimeout(1000);

    await page.locator('.scope-btn').click();
    await page.locator('.scope-dropdown .scope-option').first().click();
    await expect(page.locator('.scope-btn .logo-label')).not.toContainText('My Quilt');
  });
});

test.describe('Discovery — Top Bar Identity', () => {
  test('logged out — shows "Log In" button instead of user icon', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    const loginBtn = page.locator('.bar-login');
    await expect(loginBtn).toBeVisible();
    await expect(loginBtn).toContainText('Log In');
  });

  test('logged out — Log In button navigates to login page', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.bar-login').click();
    await page.waitForTimeout(500);
    expect(page.url()).toContain('/login');
  });

  test('logged in — shows avatar button with user menu', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    const avatarBtn = page.locator('.bar-avatar-btn');
    await expect(avatarBtn).toBeVisible();

    await avatarBtn.click();
    const dropdown = page.locator('.user-dropdown');
    await expect(dropdown).toBeVisible({ timeout: 2000 });
    // Dropdown is headed by the user's name
    await expect(dropdown.locator('.user-dropdown-name')).not.toBeEmpty();
  });

  test('logged in — user dropdown shows Settings and Log Out', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.bar-avatar-btn').click();
    const dropdown = page.locator('.user-dropdown');
    await expect(dropdown).toBeVisible({ timeout: 2000 });

    await expect(dropdown.getByText('Settings')).toBeVisible();
    await expect(dropdown.getByText('Log Out')).toBeVisible();
  });

  test('logged in — user dropdown Admin link visible for admin users', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    await page.locator('.bar-avatar-btn').click();
    const dropdown = page.locator('.user-dropdown');
    await expect(dropdown).toBeVisible({ timeout: 2000 });
    await expect(dropdown.locator('a[href="/admin"]')).toBeVisible();
  });

  test('logged in — notification bell is visible in the top bar', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');
    await page.waitForTimeout(500);

    await expect(page.locator('.bar-bell')).toBeVisible();
  });
});
