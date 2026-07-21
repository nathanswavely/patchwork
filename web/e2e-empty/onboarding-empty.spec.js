/**
 * First-run onboarding on a completely empty instance (zero patches).
 *
 * Regression coverage for the launch-day softlock: the SPA redirects any
 * zero-membership user to /welcome, and "Skip" used to navigate to '/' —
 * where the redirect immediately bounced back. On an empty instance there is
 * nothing to follow, so onboarding was inescapable. The seeded suite could
 * never hit this (escapeOnboarding() in e2e/setup.js follows a seeded patch).
 *
 * The first user here was created by global-setup.js through the real
 * magic-link flow and is the bootstrapped instance admin.
 */
import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

async function loginAsFirstUser(page) {
  const { token } = JSON.parse(readFileSync(path.join(__dirname, '.session.json'), 'utf8'));
  await page.context().addCookies([{
    name: 'patchwork_session',
    value: token,
    domain: 'localhost',
    path: '/',
    httpOnly: false,
    secure: false,
    sameSite: 'Lax',
  }]);
}

// Serial and ordered: the last test creates the instance's first patch,
// which ends the zero-patch state the earlier tests depend on.
test.describe.configure({ mode: 'serial' });

test('zero-membership user is redirected to first-run onboarding', async ({ page }) => {
  await loginAsFirstUser(page);
  await page.goto('/');
  await expect(page).toHaveURL(/\/welcome/);
  await expect(page.getByRole('button', { name: /build your quilt/i })).toBeVisible();
});

test('skip genuinely exits onboarding on an empty instance', async ({ page }) => {
  await loginAsFirstUser(page);
  await page.goto('/welcome');
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: /build your quilt/i }).click();

  // Empty instance: no tags, so the interests step is bypassed, and with no
  // patches to follow the create-first-patch state shows.
  await expect(page.getByRole('heading', { name: /first one here/i })).toBeVisible();

  // The skip affordance ("I'll explore on my own" — Welcome.svelte's
  // handleSkip, not a button literally labelled Skip).
  await page.getByRole('button', { name: /explore on my own/i }).click();
  await expect(page).not.toHaveURL(/\/welcome/);

  // The old bug: the zero-membership redirect fired again and bounced the
  // user straight back. Give the app time to (wrongly) redirect, then check.
  await page.waitForTimeout(1000);
  await expect(page).not.toHaveURL(/\/welcome/);

  // Dismissal persists across a full reload.
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await expect(page).not.toHaveURL(/\/welcome/);
});

test('empty instance steers the first user to create a patch', async ({ page }) => {
  await loginAsFirstUser(page);
  await page.goto('/welcome');
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: /build your quilt/i }).click();

  await expect(page.getByRole('heading', { name: /first one here/i })).toBeVisible();
  await page.getByRole('button', { name: /create the first patch/i }).click();

  await expect(page).toHaveURL(/\/patches\/new/);
  await expect(page.getByRole('heading', { name: 'Create Patch' })).toBeVisible();

  // Filling the form must not get intercepted by the onboarding redirect.
  await page.waitForTimeout(1000);
  await expect(page).toHaveURL(/\/patches\/new/);

  await page.getByLabel('Name').fill('First Patch');
  await page.getByRole('button', { name: 'Create Patch' }).click();

  // Landed on the new patch's profile, not back in onboarding.
  await expect(page).toHaveURL(/\/patches\/(?!new)[a-z0-9-]+/);
  await expect(page.getByText('First Patch').first()).toBeVisible();

  // With a membership (creator becomes admin), the redirect no longer fires.
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await expect(page).not.toHaveURL(/\/welcome/);
});
