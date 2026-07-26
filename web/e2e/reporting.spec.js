/**
 * E2E: Content reporting — the ReportButton → modal → POST /api/v1/reports
 * flow that lets a signed-in person flag a patch to the instance admins.
 *
 * Files a report as the lurker (a follower who is never a patch admin, so the
 * control always renders). Asserts only on the request/response and the modal
 * lifecycle — never on report counts or the admin queue — so this spec owns no
 * shared seed state and can't collide with other workers.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto, openOverflow } from './setup.js';

// On a patch profile the trigger is a row in the overflow (docs/adr/042);
// the modal itself is mounted outside that menu, so opening it can close
// the menu without destroying the dialog.
const reportRow = (page) => page.getByRole('menuitem', { name: 'Report' });

test.describe('Content reporting', () => {
  // Reporting is rare and consequential, so it lives in the profile's
  // overflow rather than in the relationship row (docs/adr/042).
  test('a signed-in member can report a patch', async ({ page }) => {
    await loginAs(page, 'lurker');
    await goto(page, '/patches/gallery-row');

    await openOverflow(page);
    await reportRow(page).click();
    await expect(page.getByRole('heading', { name: 'Report this patch' })).toBeVisible();

    await page.getByRole('combobox').selectOption('Spam or scam');
    await page.getByPlaceholder(/helps an admin/i).fill('Automated test report.');

    const [resp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().endsWith('/api/v1/reports') && r.request().method() === 'POST'
      ),
      page.getByRole('button', { name: 'Submit report' }).click(),
    ]);
    expect(resp.status()).toBe(201);

    // The modal closes on success.
    await expect(page.getByRole('heading', { name: 'Report this patch' })).not.toBeVisible();
  });

  // Signed out there is nothing to report with: the overflow still opens
  // (the calendar feeds are public), but it carries no report row.
  test('the report control is hidden when signed out', async ({ page }) => {
    await goto(page, '/patches/gallery-row');
    await openOverflow(page);
    await expect(reportRow(page)).toHaveCount(0);
  });
});
