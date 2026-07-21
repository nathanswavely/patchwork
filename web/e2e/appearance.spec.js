/**
 * E2E: Patch Appearance — palette/block/rotation/motif picker and the
 * block drafter (docs/adr/029). The site admin admins code-and-coffee;
 * freewheelery is used for API-level checks. Seed data pins
 * lancaster-arts-district to liberalAnimation/logCabin and gives
 * the-selvage a drafted block (Economy on a 4x4) with a punch/chambray
 * bundle.
 *
 * OWNS (see setup.js): code-and-coffee's appearance column (saved, then
 * reset to null). No other spec may assert on that patch's colors.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin, goto, expectNoError } from './setup.js';

const PATCH = 'code-and-coffee';

test.describe('Patch Appearance — Picker', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('appearance section renders with preview, palettes, and blocks', async ({ page }) => {
    await goto(page, `/patches/${PATCH}/settings/appearance`);
    await expectNoError(page);

    // Canonical URL — no /manage mirror (ADR 003).
    expect(page.url()).toContain(`/patches/${PATCH}/settings/appearance`);

    await expect(page.locator('.preview-tile')).toBeVisible();
    await expect(page.locator('.palette-swatch')).toHaveCount(8);
    await expect(page.locator('.block-thumb')).toHaveCount(12);
    await expect(page.locator('.motif-swatch')).toHaveCount(34);
    // Exactly one of each is selected (the effective appearance).
    await expect(page.locator('.palette-swatch.selected')).toHaveCount(1);
    await expect(page.locator('.block-thumb.selected')).toHaveCount(1);
    await expect(page.locator('.motif-swatch.selected')).toHaveCount(1);
  });

  test('save pins palette, block, rotation, and motif; reset returns to hash', async ({ page }) => {
    await goto(page, `/patches/${PATCH}/settings/appearance`);

    const saveBtn = page.getByRole('button', { name: 'Save appearance' });

    // Pick a specific palette + block + motif, rotate once.
    await page.locator('.palette-swatch', { hasText: 'Adolescents' }).click();
    await page.locator('.block-thumb', { hasText: 'Pinwheel' }).click();
    await page.locator('.motif-swatch[aria-label="Guitar"]').click();
    await page.getByRole('button', { name: /Rotate/ }).click();
    await expect(saveBtn).toBeEnabled();
    await saveBtn.click();
    await expect(page.getByText('Appearance saved')).toBeVisible();

    // Stored appearance matches the picks. A palette pick is a pre-cut
    // bundle, so the bundle rides along (one color system — ADR 029).
    const detail = await page.request.get(`/api/v1/nodes/${PATCH}`).then(r => r.json());
    expect(detail.node.appearance.palette).toBe('adolescents');
    expect(detail.node.appearance.block).toBe('pinwheel');
    expect(detail.node.appearance.icon).toBe('guitar');
    expect([0, 90, 180, 270]).toContain(detail.node.appearance.rotation);
    expect(detail.node.appearance.bundle[0].toLowerCase()).toBe('#039be6');

    // Reset clears appearance entirely (back to hash-assigned).
    await page.getByRole('button', { name: 'Reset appearance' }).click();
    await expect(page.getByText('Appearance reset')).toBeVisible();
    const cleared = await page.request.get(`/api/v1/nodes/${PATCH}`).then(r => r.json());
    expect(cleared.node.appearance ?? null).toBeNull();
  });

  test('drafting a block: sew, color, save, reload, reset', async ({ page }) => {
    await goto(page, `/patches/${PATCH}/settings/appearance`);

    // Open the drafter.
    await page.getByRole('tab', { name: 'Draft your own' }).click();
    await expect(page.locator('.drafter-canvas')).toBeVisible();
    await expect(page.getByText('0 of 24 seams')).toBeVisible();

    // Sew one seam: first anchor to last anchor (corner to corner).
    // dispatchEvent: corner anchors sit exactly on the SVG edge, where
    // Playwright's hit-testing sees the canvas instead of the circle.
    await page.locator('.anchor').first().dispatchEvent('click');
    await page.locator('.anchor').last().dispatchEvent('click');
    await expect(page.getByText('1 of 24 seams')).toBeVisible();

    // Color a piece with fabric 2.
    await page.getByRole('button', { name: 'Color', exact: true }).click();
    await page.locator('.bundle-slots .slot').nth(1).click();
    await page.locator('.piece').first().dispatchEvent('click');

    // Save; the stored appearance is a draft object plus the bundle.
    await page.getByRole('button', { name: 'Save appearance' }).click();
    await expect(page.getByText('Appearance saved')).toBeVisible();
    const detail = await page.request.get(`/api/v1/nodes/${PATCH}`).then(r => r.json());
    expect(detail.node.appearance.block.grid).toBe(3);
    expect(detail.node.appearance.block.seams).toHaveLength(1);
    expect(detail.node.appearance.bundle.length).toBeGreaterThan(0);

    // Reload: the drafter restores from the stored draft.
    await goto(page, `/patches/${PATCH}/settings/appearance`);
    await expect(page.locator('.drafter-canvas')).toBeVisible();
    await expect(page.getByText('1 of 24 seams')).toBeVisible();

    // Cleanup: back to hash-assigned (this spec owns the column).
    await page.getByRole('button', { name: 'Reset appearance' }).click();
    await expect(page.getByText('Appearance reset')).toBeVisible();
    const cleared = await page.request.get(`/api/v1/nodes/${PATCH}`).then(r => r.json());
    expect(cleared.node.appearance ?? null).toBeNull();
  });

  test('selecting a palette updates the preview and block thumbnails', async ({ page }) => {
    await goto(page, `/patches/${PATCH}/settings/appearance`);

    // Adolescents primary is #039BE6.
    await page.locator('.palette-swatch', { hasText: 'Adolescents' }).click();
    const fills = await page.locator('.block-thumb svg [fill]').evaluateAll(
      els => els.map(el => el.getAttribute('fill').toLowerCase())
    );
    expect(fills).toContain('#039be6');
  });
});

test.describe('Patch Appearance — API validation', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('rejects malformed appearance values', async ({ page }) => {
    await goto(page, '/');
    const bad = [
      { appearance: 'anthem' },
      { appearance: { palette: 'has spaces' } },
      { appearance: { icon: 'has spaces' } },
      { appearance: { rotation: 45 } },
      { appearance: { palette: 'anthem', extra: true } },
      { appearance: { theme: 'anthem' } },
      // Drafted-block violations (docs/adr/029).
      { appearance: { block: { grid: 0 } } },
      { appearance: { block: { grid: 11 } } },
      { appearance: { block: { grid: 2, seams: Array(25).fill([0, 0, 8, 8]) } } },
      { appearance: { block: { grid: 2, seams: [[1, 1, 8, 8]] } } }, // interior anchor
      { appearance: { block: { grid: 6, seams: [[1, 0, 24, 24]] } } }, // quarter anchor on coarse grid
      { appearance: { block: { grid: 2, colors: { '0,0': [6] } } } }, // slot out of range
      { appearance: { bundle: ['red'] } },
      { appearance: { bundle: ['#111111', '#222222', '#333333', '#444444', '#555555', '#666666', '#777777'] } },
    ];
    for (const body of bad) {
      const resp = await page.request.patch(`/api/v1/nodes/${PATCH}`, {
        data: body,
        headers: { 'X-Patchwork-Request': 'true' }, // CSRF custom-header check
      });
      expect(resp.status(), JSON.stringify(body)).toBe(400);
    }
  });

  test('appearance flows through the tree endpoint', async ({ page }) => {
    await goto(page, '/');
    const tree = await page.request.get('/api/v1/nodes/tree').then(r => r.json());
    const lad = tree.tree.children.find(c => c.slug === 'lancaster-arts-district');
    expect(lad.appearance.palette).toBe('liberalAnimation');
    expect(lad.appearance.block).toBe('logCabin');

    // A drafted block rides the same column untouched (docs/adr/029).
    const selvage = tree.tree.children.find(c => c.slug === 'the-selvage');
    expect(selvage.appearance.block.grid).toBe(4);
    expect(selvage.appearance.block.seams).toHaveLength(4);
    expect(selvage.appearance.bundle[0]).toBe('#DA0956');
  });
});

test.describe('Patch Appearance — Identity color', () => {
  test('card banner uses the patch palette primary', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, '/');

    // lancaster-arts-district pins liberalAnimation (primary #952117).
    const card = page.locator('.patch-card', { hasText: 'Lancaster Arts District' });
    await expect(card).toBeVisible();
    const bg = await card.locator('.card-image').evaluate(el => getComputedStyle(el).backgroundColor);
    expect(bg).toBe('rgb(149, 33, 23)');
  });

  test('a drafted patch takes its identity color from bundle slot 0', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, '/');

    // the-selvage's seeded bundle leads with punch (#DA0956).
    const card = page.locator('.patch-card', { hasText: 'The Selvage' });
    await expect(card).toBeVisible();
    const bg = await card.locator('.card-image').evaluate(el => getComputedStyle(el).backgroundColor);
    expect(bg).toBe('rgb(218, 9, 86)');
  });
});
