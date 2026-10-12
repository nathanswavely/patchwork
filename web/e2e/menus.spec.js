/**
 * E2E: the menus on Liquid Textile's menu() (step 5c).
 *
 * menu() opens a list on ArrowDown with focus on its first item, moves with
 * the arrows, closes on Escape and hands focus back to its button, and
 * closes on a click elsewhere. None of that is visible to a source-text
 * test. Read-only: no item here is chosen except to navigate.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

const PATCH = '/patches/lancaster-arts-district';

test.describe('Menus on menu()', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'admin');
  });

  test('the overflow menu opens from the keyboard and Escape hands focus back', async ({ page }) => {
    await goto(page, PATCH);
    const trigger = page.getByRole('button', { name: 'More actions' });
    await trigger.focus();
    await page.keyboard.press('ArrowDown');

    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();
    await expect(trigger).toHaveAttribute('aria-expanded', 'true');
    await expect(menu.getByRole('menuitem').first()).toBeFocused();

    await page.keyboard.press('ArrowDown');
    await expect(menu.getByRole('menuitem').nth(1)).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
    await expect(trigger).toHaveAttribute('aria-expanded', 'false');
  });

  test('the New menu closes on a click elsewhere, and an item navigates', async ({ page }) => {
    await goto(page, '/');
    const trigger = page.getByRole('button', { name: 'Create new' });
    await trigger.click();
    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();

    await page.mouse.click(5, 400);
    await expect(menu).toBeHidden();

    await trigger.click();
    await page.getByRole('menuitem', { name: 'New event' }).click();
    await page.waitForURL(/\/events\/new/);
    await expect(page.getByRole('menu')).toBeHidden();
  });

  test('the scope menu marks the current scope', async ({ page }) => {
    await goto(page, '/');
    await page.locator('.scope-btn').click();
    const menu = page.locator('.scope-dropdown');
    await expect(menu).toBeVisible();
    await expect(menu.locator('[aria-current="true"]')).toHaveCount(1);
    await page.keyboard.press('Escape');
    await expect(menu).toBeHidden();
  });
});
