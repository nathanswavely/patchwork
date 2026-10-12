/**
 * E2E: tabs and segmented controls on Liquid Textile's tabs() and
 * segmented() (step 5d). Both give a row one tab stop and let the arrows
 * move and choose; neither is visible to a source-text test. Read-only.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

test.describe('Tabs and segmented controls', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'admin');
  });

  test('report tabs choose with the arrows, one tab stop for the row', async ({ page }) => {
    await goto(page, '/admin/review/reports');
    const row = page.getByRole('tablist', { name: 'Reports' });
    const tabs = row.getByRole('tab');
    await expect(tabs.first()).toHaveAttribute('aria-selected', 'true');
    // One tab stop: only the chosen tab is in the tab order.
    await expect(tabs.nth(1)).toHaveAttribute('tabindex', '-1');

    await tabs.first().focus();
    await page.keyboard.press('ArrowRight');
    await expect(tabs.nth(1)).toBeFocused();
    await expect(tabs.nth(1)).toHaveAttribute('aria-selected', 'true');
    await expect(tabs.first()).toHaveAttribute('aria-selected', 'false');

    await page.keyboard.press('Home');
    await expect(tabs.first()).toHaveAttribute('aria-selected', 'true');
  });

  test("the admin shell marks the section it is on", async ({ page }) => {
    await goto(page, '/admin/users');
    await expect(page.locator('.workspace-tab[aria-current="page"]')).toHaveText(/Users/);
  });

  test('a segmented control is a radio group the arrows move through', async ({ page }) => {
    await goto(page, '/admin/settings/usage');
    const group = page.getByRole('radiogroup', { name: 'Window' });
    const radios = group.getByRole('radio');
    const checked = group.locator('[aria-checked="true"]');
    await expect(checked).toHaveCount(1);

    await checked.focus();
    const before = await checked.textContent();
    await page.keyboard.press('ArrowRight');
    await expect(group.locator('[aria-checked="true"]')).not.toHaveText(before);
    await expect(group.locator('[aria-checked="true"]')).toBeFocused();
    expect(await radios.count()).toBeGreaterThan(1);
  });
});
