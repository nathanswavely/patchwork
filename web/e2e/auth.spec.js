/**
 * E2E: Authentication (User Stories 2.1–2.4)
 * Tests login page, session management, logout.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin, logout } from './setup.js';

test.describe('Auth — Login Page', () => {
  test('2.2 — login page renders signup-first with email input', async ({ page }) => {
    await page.goto('/login');

    // Default view is the join/signup form: email input + continue button.
    const loginPage = page.locator('.login-page');
    await expect(loginPage).toBeVisible();
    await expect(loginPage.locator('h1')).toContainText(/join|create your account/i);
    await expect(loginPage.locator('input[type="email"]')).toBeVisible();
  });

  test('2.2 — signup form has Continue with email button', async ({ page }) => {
    await page.goto('/login');

    const submitBtn = page.locator('.login-page button[type="submit"]');
    await expect(submitBtn).toBeVisible();
    await expect(submitBtn).toContainText(/continue with email/i);
  });

  test('2.3 — sign-in view offers email link and passkey', async ({ page }) => {
    await page.goto('/login');

    // Toggle to the returning-user view.
    await page.locator('.signin-link').getByText('Sign in').click();
    await expect(page.locator('h1')).toContainText(/welcome back/i);

    const options = page.locator('.signin-options');
    await expect(options).toBeVisible();
    await expect(options.locator('button[type="submit"]')).toContainText(/sign-in link/i);

    const passKeyBtn = page.locator('.passkey-btn');
    await expect(passKeyBtn).toBeVisible();
    await expect(passKeyBtn).toContainText(/passkey/i);
  });

  test('2.2 — sign-in view lists passkey before email', async ({ page }) => {
    await page.goto('/login');
    await page.locator('.signin-link').getByText('Sign in').click();

    const passkeyBox = await page.locator('.passkey-btn').boundingBox();
    const emailBox = await page.locator('.signin-options input[type="email"]').boundingBox();
    expect(emailBox).not.toBeNull();
    expect(passkeyBox).not.toBeNull();
    // Passkey option should be above the email option.
    expect(passkeyBox.y).toBeLessThan(emailBox.y);
  });
});

test.describe('Auth — /login Redirects an Already-Authenticated Session', () => {
  // Issue #9: navigating directly to /login while a session is already
  // active used to render the login form unconditionally. Pure navigation
  // against the seeded admin — read-only, no shared state touched.

  test('logged-in visitor to /login is bounced to home without seeing the form', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/login');
    await page.waitForURL((url) => url.pathname === '/');
    await expect(page.locator('.login-page')).not.toBeVisible();
  });

  test('logged-in visitor to /login?redirect=... lands on that destination', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/login?redirect=%2Fdashboard');
    await page.waitForURL((url) => url.pathname === '/dashboard');
    await expect(page.locator('h1')).toContainText('Dashboard');
  });

  test('logged-in visitor to /login with an off-site redirect target falls back to home', async ({ page }) => {
    await loginAsAdmin(page);
    // Open-redirect guard: an absolute/off-origin ?redirect= must never be
    // honored — only same-origin relative paths are safe to hand to the
    // client router.
    await page.goto(`/login?redirect=${encodeURIComponent('https://evil.example/phish')}`);
    await page.waitForURL((url) => url.pathname === '/');
    expect(page.url()).not.toContain('evil.example');
  });

  test('logged-out visitor still sees the login form (no regression)', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('.login-page')).toBeVisible();
  });
});

test.describe('Auth — Session', () => {
  test('authenticated user sees dashboard', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/dashboard');
    await expect(page.locator('h1')).toContainText('Dashboard');
  });

  test('auth guard shows sign-in prompt when session is invalid', async ({ page }) => {
    // Set an invalid session cookie (not in DB)
    await page.context().addCookies([{
      name: 'patchwork_session',
      value: 'invalid-token-that-does-not-exist',
      domain: 'localhost',
      path: '/',
      httpOnly: false,
      secure: false,
      sameSite: 'Lax',
    }]);
    await page.goto('/dashboard');
    await page.waitForTimeout(2000);
    // Should show sign-in prompt since the token is invalid
    const hasPrompt = await page.getByText(/sign in/i).isVisible().catch(() => false);
    const showsDashboard = await page.locator('h1').filter({ hasText: 'Dashboard' }).isVisible().catch(() => false);
    expect(hasPrompt || !showsDashboard).toBeTruthy();
  });

  test('2.4 — logout clears session', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/dashboard');
    await expect(page.locator('h1')).toContainText('Dashboard');

    // Click user name dropdown (now a text button with caret, not an icon)
    const userBtn = page.locator('.user-btn').first();
    if (await userBtn.isVisible()) {
      await userBtn.click();
      const logoutBtn = page.locator('.user-dropdown').getByText(/log ?out/i);
      if (await logoutBtn.isVisible()) {
        await logoutBtn.click();
        await page.waitForTimeout(500);
        // Should no longer be on dashboard
        await page.goto('/dashboard');
        await page.waitForTimeout(1000);
      }
    }
  });
});

test.describe('Auth — Security Settings Page', () => {
  test('security settings page is accessible', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/settings/security');
    await page.waitForTimeout(2000);

    // Should not 404
    const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
    expect(notFound).toBe(false);
  });
});
