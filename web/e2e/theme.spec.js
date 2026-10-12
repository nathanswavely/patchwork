/**
 * E2E: the theme on Liquid Textile's initTheme (step 6). A choice is kept
 * per browser and applied before first paint; "System" forgets it and then
 * follows the OS while staying lit. Signed out, so it touches no account.
 */
import { test, expect } from '@playwright/test';

const THEME_KEY = 'patchwork-theme';

async function openDisplay(page) {
  await page.locator('.bar-display-btn').click();
  return page.locator('.display-row', { hasText: 'Theme' });
}

test.describe('Theme', () => {
  test('a choice is kept, and is the first frame after a reload', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' });
    await page.goto('/');
    const row = await openDisplay(page);
    await row.getByText('Dark', { exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(row.getByText('Dark', { exact: true })).toHaveAttribute('aria-checked', 'true');
    expect(await page.evaluate((k) => localStorage.getItem(k), THEME_KEY)).toBe('dark');

    // Before the bundle runs: index.html's pre-paint script, not the store.
    await page.route('**/src/main.js', (route) => route.abort());
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('.bar-display-btn')).toHaveCount(0); // the app never ran
  });

  test('System forgets the choice and follows the OS, still lit', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' });
    await page.goto('/');
    const row = await openDisplay(page);
    await row.getByText('Dark', { exact: true }).click();
    await row.getByText('System', { exact: true }).click();
    expect(await page.evaluate((k) => localStorage.getItem(k), THEME_KEY)).toBeNull();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    await page.emulateMedia({ colorScheme: 'dark' });
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(row.getByText('System', { exact: true })).toHaveAttribute('aria-checked', 'true');
  });

  test("an older browser's stored 'system' is no choice", async ({ page }) => {
    await page.addInitScript((k) => localStorage.setItem(k, 'system'), THEME_KEY);
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.goto('/');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    const row = await openDisplay(page);
    await expect(row.getByText('System', { exact: true })).toHaveAttribute('aria-checked', 'true');
  });
});
