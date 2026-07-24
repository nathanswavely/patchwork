/**
 * E2E: Proposal voting — the "Review & vote" path and the vote flow.
 * Regression: /governance/:id used to register before /governance/proposals,
 * so the literal "proposals" segment was swallowed as a proposal id and the
 * list page rendered "proposal not found".
 * Seed: lancaster-arts-district has "Create youth mentorship program" open
 * for voting (3 approve / 0 reject) and the site admin has not voted.
 *
 * OWNS (see setup.js): the admin's votes on lancaster-arts-district
 * proposals. No other spec may vote as the admin or assert on this
 * proposal's tally.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin, goto } from './setup.js';

const SLUG = 'lancaster-arts-district';

test.describe('Proposal voting', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('proposals list renders at both legacy and canonical URLs', async ({ page }) => {
    // Canonical URL renders in place; the retired /manage mirror redirects
    // back to it (ADR 003).
    await goto(page, `/patches/${SLUG}/governance/proposals`);
    await expect(page.getByText('proposal not found')).not.toBeVisible();
    await expect(page.getByText('Create youth mentorship program')).toBeVisible();

    await goto(page, `/patches/${SLUG}/manage/governance/proposals`);
    await page.waitForURL(`**/patches/${SLUG}/governance/proposals`);
    await expect(page.getByText('Create youth mentorship program')).toBeVisible();
  });

  test('Review & vote banner leads to the proposals list', async ({ page }) => {
    await goto(page, `/patches/${SLUG}/governance`);
    const banner = page.getByText(/needs? your vote/);
    await expect(banner).toBeVisible();
    await page.getByText('Review & vote').click();
    await page.waitForURL('**/governance/proposals');
    await expect(page.getByText('Create youth mentorship program')).toBeVisible();
  });

  test('casting a vote updates the tally and persists', async ({ page }) => {
    await goto(page, `/patches/${SLUG}/governance/proposals`);
    await page.getByText('Create youth mentorship program').first().click();
    await page.waitForURL('**/governance/0*');

    await expect(page.getByText('Voting is open')).toBeVisible();

    // The seeded tally is rng-derived — read it, then assert our vote adds one.
    const tallyText = await page.getByText(/^\d+✓$/).first().textContent();
    const before = parseInt(tallyText, 10);

    // Both the inline vote section and the sticky bar offer Approve.
    await page.getByRole('button', { name: 'Approve', exact: true }).first().click();
    await expect(page.getByText(`${before + 1}✓`)).toBeVisible();

    // Survives a cold load.
    await page.reload();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText(`${before + 1}✓`)).toBeVisible();
    await expect(page.getByText('Create youth mentorship program')).toBeVisible();
  });
});
