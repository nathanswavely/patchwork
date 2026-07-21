/**
 * E2E: Patch map location (issue #4).
 *
 * Walks the placement flow: a patch admin opens the picker, clicks the map to
 * drop a marker, saves explicitly, sees the patch is now on the map, then
 * removes it.
 *
 * DATA OWNERSHIP: this spec creates its own uniquely-named patch via the API
 * as `admin` (who becomes its admin) and only ever mutates that patch's
 * latitude/longitude. No assertions on seed data.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

test.describe.configure({ mode: 'serial' });

let slug = '';

test.describe('Map location placement', () => {
  test('admin creates a patch to place', async ({ page }) => {
    await loginAs(page, 'admin');
    await page.goto('/');
    const unique = `Map Place Venue ${Date.now()}`;
    const res = await page.request.post('/api/v1/nodes', {
      data: { name: unique, description: 'placement fixture' },
      headers: { 'X-Patchwork-Request': 'true' },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    slug = body.slug;
    expect(slug).toBeTruthy();
  });

  test('the info page starts off the map', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/settings/info`);
    await expect(page.getByText('Not on the map.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Set map location' })).toBeVisible();
  });

  test('placing a marker and saving puts the patch on the map', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/settings/info`);

    await page.getByRole('button', { name: 'Set map location' }).click();

    // The picker map renders. Click its center to drop the marker
    // (a click stands in for a drag — drag is flaky in Leaflet e2e).
    const map = page.locator('.picker-map');
    await expect(map).toBeVisible();
    const box = await map.boundingBox();
    await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);

    // Save is only enabled once a marker exists; a coordinate readout shows.
    const save = page.getByRole('button', { name: 'Save location' });
    await expect(save).toBeEnabled();
    await expect(page.locator('.picker-coords')).toBeVisible();
    await save.click();

    // Back on the info page, the patch now reads as on the map.
    await expect(page.getByText('On the map at')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Adjust map location' })).toBeVisible();
  });

  test('removing the marker takes the patch off the map', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `/patches/${slug}/settings/info`);

    // ConfirmAction is a two-step button: click to arm, click again to confirm.
    await page.getByRole('button', { name: 'Remove from map' }).click();
    await page.getByRole('button', { name: 'Remove', exact: true }).click();

    await expect(page.getByText('Not on the map.')).toBeVisible();
  });
});
