/**
 * E2E: rows and the audit log on Liquid Textile (step 7b). A row wraps its
 * actions under the name by its own width (.lt-item), and the audit log is
 * a stacked table under 40rem (.lt-table-stack); neither is visible to a
 * source-text test. Read-only.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

const PHONE = { width: 375, height: 812 };

test.describe('Rows and the stacked audit log', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'admin');
  });

  test('the audit log on a phone reads top to bottom, a label beside each value', async ({ page }) => {
    await page.setViewportSize(PHONE);
    await goto(page, '/admin/audit');
    const row = page.locator('table.lt-table-stack tbody tr').first();
    await expect(row).toBeVisible();
    await expect(page.locator('table.lt-table-stack thead')).toHaveCSS('position', 'absolute');

    const time = row.locator('td[data-label="Time"]');
    expect(await time.evaluate((td) => getComputedStyle(td, '::before').content)).toBe('"Time"');
    // One rule per entry, not one under every cell.
    await expect(time).toHaveCSS('border-bottom-width', '0px');
    // The action keeps its own width rather than the column's.
    const tag = await row.locator('td[data-label="Action"] .lt-tag').boundingBox();
    const cell = await row.locator('td[data-label="Action"]').boundingBox();
    expect(tag.width).toBeLessThan(cell.width * 0.6);
  });

  test('a wide audit log is still a table', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await goto(page, '/admin/audit');
    await expect(page.locator('table.lt-table-stack thead')).toBeVisible();
  });

  test("on a phone My Patches' Leave sits apart from the two that tune the row", async ({ page }) => {
    await page.setViewportSize(PHONE);
    await goto(page, '/settings/patches');
    const row = page.locator('.patch-row', { has: page.getByRole('button', { name: 'Leave' }) }).first();
    const leave = await row.getByRole('button', { name: 'Leave' }).boundingBox();
    const box = await row.boundingBox();
    expect(Math.abs(leave.x + leave.width - (box.x + box.width))).toBeLessThan(2);
    // Under the name, which keeps its line.
    const name = await row.locator('.patch-name').boundingBox();
    expect(leave.y).toBeGreaterThan(name.y + name.height - 1);
  });

  // A field and its button share a phone's line: the field shrinks rather
  // than pushing the button off the screen or onto two lines (7c, once
  // app.css stopped making every field 100% wide).
  for (const [route, row] of [['/submit', '.link-row'], ['/admin/settings/label', '.steward-add']]) {
    test(`a field and its button fit a phone's line on ${route}`, async ({ page }) => {
      await page.setViewportSize(PHONE);
      await goto(page, route);
      const box = await page.locator(row).first().boundingBox();
      for (const el of await page.locator(`${row} > *`).all()) {
        const b = await el.boundingBox();
        expect(b.x + b.width).toBeLessThanOrEqual(box.x + box.width + 1);
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(PHONE.width);
      const button = page.locator(`${row} button`).first();
      if (await button.count()) {
        const h = await button.evaluate((b) => b.getBoundingClientRect().height / parseFloat(getComputedStyle(b).lineHeight || '20'));
        expect(h).toBeLessThan(2.5); // one line of words, not two
      }
    });
  }
});
