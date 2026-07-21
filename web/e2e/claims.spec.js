/**
 * E2E: Claiming an unclaimed patch (docs/adr/030).
 *
 * The regression that motivated the redesign: a user opened a claim, changed
 * their mind, reloaded — and was locked out ("a claim is already open").
 * This spec walks the fixed flow: open claim → reload survives → withdraw →
 * choose a different method → submit again.
 *
 * DATA OWNERSHIP: this spec creates its own unclaimed patch (unique name)
 * via the admin API and claims it as `active` — no assertions on seed data.
 * The claim never completes (no ownership transfer), so `active`'s
 * memberships are untouched.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

// One worker mutates this spec's own patch; run serially within the file.
test.describe.configure({ mode: 'serial' });

let slug = '';

test.describe('Claim flow', () => {
  test('admin creates an unclaimed patch with a verified domain', async ({ page }) => {
    await loginAs(page, 'admin');
    await page.goto('/');
    const unique = `Claim Flow Venue ${Date.now()}`;
    const res = await page.request.post('/api/v1/admin/unclaimed', {
      data: { name: unique, website: 'https://claimflow.example' },
      headers: { 'X-Patchwork-Request': 'true' },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    slug = body.slug;
    expect(slug).toBeTruthy();
  });

  test('claim page offers methods, none preselected, email disabled without SMTP', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, `/patches/${slug}/claim`);

    await expect(page.getByText('DNS Verification')).toBeVisible();
    await expect(page.getByText('Website Meta Tag')).toBeVisible();
    await expect(page.getByText('Admin Review')).toBeVisible();

    // Nothing preselected — a silent default is how the original bug user
    // ended up in a claim they didn't want.
    expect(await page.locator('input[name="method"]:checked').count()).toBe(0);
    await expect(page.getByRole('button', { name: 'Submit Claim' })).toBeDisabled();

    // Dev instance has no SMTP: email is visibly inert, not a trap — and
    // the reason names the actual missing prerequisite (this patch HAS a
    // verified domain, so the text must blame mail, not the domain).
    await expect(page.locator('input[name="method"][value="email"]')).toBeDisabled();
    await expect(page.getByText("This quilt can't send email")).toBeVisible();
    // DNS/meta are live because the admin's website derived a verified domain.
    await expect(page.locator('input[name="method"][value="meta_tag"]')).toBeEnabled();
  });

  test('open a meta-tag claim, reload, the claim survives', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, `/patches/${slug}/claim`);

    await page.locator('input[name="method"][value="meta_tag"]').check();
    await page.getByRole('button', { name: 'Submit Claim' }).click();
    await expect(page.getByText('Your claim is open')).toBeVisible();
    await expect(page.getByText('patchwork-verify')).toBeVisible();

    // The original bug: reload forgot the claim and the form 409'd.
    await page.reload();
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Your claim is open')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Withdraw Claim' })).toBeVisible();
  });

  test('patch profile shows the claim in progress', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, `/patches/${slug}`);
    await expect(page.getByText('Claim in progress')).toBeVisible();
  });

  test('withdraw, then claim again with a different method', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, `/patches/${slug}/claim`);

    await page.getByRole('button', { name: 'Withdraw Claim' }).click();
    // Back to the picker.
    await expect(page.getByText('DNS Verification')).toBeVisible();
    expect(await page.locator('input[name="method"]:checked').count()).toBe(0);

    // The user's actual goal: a different kind of claim.
    await page.locator('input[name="method"][value="admin"]').check();
    await page.locator('#evidence').fill('I run this venue — call me.');
    await page.getByRole('button', { name: 'Submit Claim' }).click();
    await expect(page.getByText('Your claim is open')).toBeVisible();
    await expect(page.getByText(/admin.*review/i).first()).toBeVisible();
  });
});
