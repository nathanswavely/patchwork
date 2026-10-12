/**
 * E2E: the workspace tab bar, folds and inline edits on Liquid Textile
 * (step 5e). What the foot of a phone screen reserves now depends on which
 * bar is on the page, a fold is held open by script, and an inline edit
 * builds its field on Edit; a source-text test sees none of it. Read-only:
 * the one save tried here is turned away before it is sent.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

const PATCH_SLUG = 'lancaster-arts-district';
const PHONE = { width: 375, height: 812 };

/** What the page reserves at the foot of the screen, in px. The token is
 *  a calc() that reads back as text, so a probe resolves it. */
const chromeBottom = (page) => page.evaluate(() => {
  const probe = document.createElement('div');
  probe.style.cssText = 'position:absolute;height:var(--lt-chrome-bottom)';
  document.body.append(probe);
  const px = probe.getBoundingClientRect().height;
  probe.remove();
  return Math.round(px);
});

test.describe('The shells on Liquid Textile', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'admin');
  });

  test("a phone's workspace tabs are a bar at the foot that the page clears", async ({ page }) => {
    await page.setViewportSize(PHONE);
    await goto(page, `/patches/${PATCH_SLUG}/governance`);
    const bar = page.locator('nav.workspace-tabs');
    await expect(bar).toHaveClass(/lt-tabbar/);
    const box = await bar.boundingBox();
    expect(Math.round(box.y + box.height)).toBe(PHONE.height);
    expect(Math.round(box.height)).toBe(51);
    expect(await chromeBottom(page)).toBe(51);

    await bar.getByRole('link', { name: /Events/ }).click();
    await expect(page).toHaveURL(/\/events$/);
    await expect(bar.locator('[aria-current="page"]')).toHaveText(/Events/);
  });

  test('a wide workspace has its tab row under the bar and reserves nothing', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await goto(page, `/patches/${PATCH_SLUG}/governance`);
    const bar = page.locator('nav.workspace-tabs');
    expect((await bar.boundingBox()).y).toBeLessThan(120);
    expect(await chromeBottom(page)).toBe(0);
  });

  test("the quilt's rail is the phone's bar there; the admin shell has none", async ({ page }) => {
    await page.setViewportSize(PHONE);
    await goto(page, '/events');
    expect(await chromeBottom(page)).toBe(51);
    const rail = await page.locator('nav.sidebar-rail').boundingBox();
    expect(Math.round(rail.height)).toBe(51);

    await goto(page, '/admin/users');
    expect(await chromeBottom(page)).toBe(0);
  });

  test('a fold lies flat on a wide screen and folds on a phone', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await goto(page, '/settings');
    const moved = page.locator('details.lt-fold', { has: page.getByRole('heading', { name: 'I have moved' }) });
    await expect(moved).toHaveAttribute('open', '');
    await expect(moved).toHaveAttribute('data-held', '');
    await moved.locator('summary').click();
    await expect(moved).toHaveAttribute('open', '');

    await page.setViewportSize(PHONE);
    await goto(page, '/admin/settings/usage');
    const counted = page.locator('details.counted-fold');
    await expect(counted).not.toHaveAttribute('open', '');
    await counted.locator('summary').click();
    await expect(counted).toHaveAttribute('open', '');
  });

  test('an inline edit takes focus, gives it back, and keeps a value the page turned away', async ({ page }) => {
    await goto(page, `/patches/${PATCH_SLUG}/settings/info`);
    const zone = page.locator('.lt-inline-edit', { hasText: 'Timezone' });
    const shown = await zone.locator('[data-value]').textContent();

    await zone.getByRole('button', { name: 'Edit' }).click();
    const field = zone.getByRole('textbox', { name: 'Timezone' });
    await expect(field).toBeFocused();
    await expect(field).toHaveAttribute('placeholder', /.+/);
    await page.keyboard.press('Escape');
    await expect(zone.getByRole('button', { name: 'Edit' })).toBeFocused();

    // Not a place: the page refuses it with a toast and saves nothing.
    await zone.getByRole('button', { name: 'Edit' }).click();
    await field.fill('Nowhere/Atlantis');
    await page.keyboard.press('Enter');
    await expect(page.getByText('That is not a place this quilt keeps time in.', { exact: false })).toBeVisible();
    await expect(zone.locator('[data-value]')).toHaveText(shown);
  });
});
