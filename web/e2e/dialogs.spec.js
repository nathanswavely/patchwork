/**
 * E2E: the four dialogs on Liquid Textile's native <dialog> (step 5b).
 *
 * A modal <dialog> holds focus inside, closes on Escape, and hands focus
 * back; its component then has to hear about the close (ltDialog's
 * onClose), or it stays mounted and the next open does nothing. Source-text
 * tests cannot see any of that, so each dialog here is opened, closed with
 * Escape, and opened again. Read-only: nothing here changes the seed.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto, openOverflow } from './setup.js';

const PATCH_SLUG = 'lancaster-arts-district';

test.describe('Dialogs on native <dialog>', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'admin');
  });

  test('the template preview drawer closes on Escape and opens again', async ({ page }) => {
    await goto(page, '/patches/new');
    await page.locator('.fork-card').first().click();

    const previews = page.locator('.preview-link');
    await previews.first().click();
    const drawer = page.locator('dialog.template-drawer');
    await expect(drawer).toBeVisible();
    expect(await drawer.evaluate((d) => d.matches(':modal'))).toBe(true);

    await page.keyboard.press('Escape');
    // Closed and unmounted: PatchForm heard the close and cleared it.
    await expect(drawer).toHaveCount(0);
    await expect(previews.first()).toBeFocused();

    await previews.nth(1).click();
    await expect(page.locator('dialog.template-drawer')).toBeVisible();
  });

  // Not a <dialog> any more: the bell opens a dropdown under it, which is
  // not modal, so the page behind it stays usable. It still has to close
  // on Escape, hand focus back, and open again.
  test('the notifications dropdown closes on Escape and hands focus to the bell', async ({ page }) => {
    await goto(page, '/');
    const bell = page.locator('.bar-bell .bell-btn');
    await bell.click();
    const dropdown = page.locator('.notif-dropdown');
    await expect(dropdown).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(dropdown).toHaveCount(0);
    await expect(bell).toBeFocused();

    await bell.click();
    await expect(dropdown).toBeVisible();
  });

  test('a modal is centred on the screen, and closes on Escape', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await goto(page, `/patches/${PATCH_SLUG}`);
    await openOverflow(page);
    await page.locator('.overflow-menu > *', { hasText: 'Subscribe' }).click();

    const modal = page.locator('dialog.modal[open]');
    await expect(modal).toBeVisible();
    const box = await modal.boundingBox();
    // Centred: a * { margin: 0 } reset once pinned it to the top-left.
    expect(Math.abs(box.x + box.width / 2 - 640)).toBeLessThan(2);
    expect(Math.abs(box.y + box.height / 2 - 400)).toBeLessThan(2);

    await page.keyboard.press('Escape');
    await expect(page.locator('dialog.modal[open]')).toHaveCount(0);
  });

  test("a phone's patch sheet closes on Escape and opens again", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await goto(page, `/patches/${PATCH_SLUG}/settings/info`);
    const name = page.locator('.patch-bar-name');
    await name.click();
    const sheet = page.locator('dialog.patch-sheet');
    await expect(sheet).toBeVisible();
    await expect(sheet.getByText('View the public profile')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(sheet).not.toBeVisible();

    await name.click();
    await expect(sheet).toBeVisible();
  });
});
