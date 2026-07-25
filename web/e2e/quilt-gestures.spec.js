/**
 * E2E: Quilt gestures on a touch screen.
 *
 * A finger that lands on a name badge pans the quilt, because a badge is a DOM
 * element sitting above the svg and d3's zoom never sees the press (see
 * QuiltCanvas, LABEL DRAG GESTURES). The interesting part is that it must keep
 * panning: every event of a touch sequence is delivered to the element that
 * received the touchstart, and each pan frame rebuilds the whole label layer —
 * so a badge that doesn't outlive its own rebuild takes the rest of the gesture
 * down with it. That shipped once: the pan moved a single frame and froze.
 *
 * Playwright's touchscreen only taps, so the drags here go through CDP.
 * Read-only: nothing in this file mutates seed data.
 */
import { test, expect } from '@playwright/test';
import { goto } from './setup.js';

test.use({ hasTouch: true, viewport: { width: 390, height: 800 } });

/** The zoom transform d3 writes onto the canvas's outermost <g>. */
async function quiltTransform(page) {
  const attr = await page.evaluate(() => {
    const g = document.querySelector('.canvas-container svg > g');
    return g?.getAttribute('transform') || '';
  });
  const move = attr.match(/translate\(([-\d.]+)[, ]+([-\d.]+)\)/);
  const scale = attr.match(/scale\(([-\d.]+)/);
  if (!move) return null;
  return { x: parseFloat(move[1]), y: parseFloat(move[2]), k: scale ? parseFloat(scale[1]) : 1 };
}

/** One finger down, a run of moves, then up. */
async function touchDrag(page, startX, startY, steps) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart', touchPoints: [{ x: startX, y: startY, id: 1 }],
  });
  for (const [dx, dy] of steps) {
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove', touchPoints: [{ x: startX + dx, y: startY + dy, id: 1 }],
    });
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await cdp.detach();
}

/** Center of the first name badge on the quilt. */
async function firstLabelCenter(page) {
  const label = page.locator('.patch-label').first();
  await expect(label).toBeVisible({ timeout: 15000 });
  const box = await label.boundingBox();
  return { label, x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

test('a finger dragging from a name badge keeps panning the quilt', async ({ page }) => {
  await goto(page, '/');
  const { x, y } = await firstLabelCenter(page);

  const before = await quiltTransform(page);
  expect(before).not.toBeNull();

  // Eight frames of travel, the way a real drag arrives. A single frame would
  // pass even with the freeze, so the assertion below is about the total.
  const TRAVEL = 120;
  const steps = [];
  for (let i = 1; i <= 8; i++) steps.push([(TRAVEL / 8) * i, 0]);
  await touchDrag(page, x, y, steps);

  const after = await quiltTransform(page);
  // The first TAP_SLOP px are held back while the press resolves into a tap or
  // a drag, so the quilt owes us the rest of the travel.
  expect(after.x - before.x).toBeGreaterThan(TRAVEL - 25);
  expect(after.x - before.x).toBeLessThan(TRAVEL + 5);

  // The badge kept alive for the gesture is gone once the finger is up — it was
  // an address, not a label, and hidden ones must not pile up in the layer.
  expect(await page.locator('.patch-label.gesture-holdover').count()).toBe(0);
});

test('a pan moves the badges rather than rebuilding them', async ({ page }) => {
  await goto(page, '/');
  const { x, y } = await firstLabelCenter(page);

  const badges = await page.locator('.patch-label').count();
  expect(badges).toBeGreaterThan(1);

  // Count badges added to the layer while the quilt pans. Each badge carries a
  // motif svg and six listeners, so rebuilding the set every frame is the most
  // expensive thing on a phone during a drag.
  await page.evaluate(() => {
    window.__badgesAdded = 0;
    new MutationObserver((records) => {
      for (const r of records) {
        for (const n of r.addedNodes) {
          if (n.classList?.contains('patch-label')) window.__badgesAdded++;
        }
      }
    }).observe(document.querySelector('.labels-layer'), { childList: true });
  });

  const FRAMES = 8;
  const steps = [];
  for (let i = 1; i <= FRAMES; i++) steps.push([i * 15, 0]);
  await touchDrag(page, x, y, steps);

  // A rebuild per frame would add FRAMES × badges. Reuse only adds the few that
  // newly earn a label as the quilt slides, so anything near one full set of
  // badges means the layer is being rebuilt again.
  const added = await page.evaluate(() => window.__badgesAdded);
  expect(added).toBeLessThan(badges);

  // The failure mode reuse introduces is bookkeeping drift: a patch ending up
  // with two badges, or a dropped one left parked in the layer.
  const names = await page.locator('.patch-label .label-name').allInnerTexts();
  expect(new Set(names).size).toBe(names.length);
  expect(await page.locator('.patch-label.gesture-holdover').count()).toBe(0);
});

test('two fingers landing on a name badge pinch the quilt', async ({ page }) => {
  await goto(page, '/');
  const { x, y } = await firstLabelCenter(page);

  const before = (await quiltTransform(page)).k;
  expect(before).toBeGreaterThan(0);

  const cdp = await page.context().newCDPSession(page);
  // Both fingers land on the badge, so the badge owns the gesture rather than
  // the svg underneath it — then they spread.
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: x - 8, y, id: 1 }, { x: x + 8, y, id: 2 }],
  });
  for (let i = 1; i <= 8; i++) {
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: x - 8 - i * 6, y, id: 1 }, { x: x + 8 + i * 6, y, id: 2 }],
    });
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await cdp.detach();

  expect((await quiltTransform(page)).k).toBeGreaterThan(before * 1.5);
});

test('a cursor dragging from a name badge pans the quilt', async ({ page }) => {
  await goto(page, '/');
  const { x, y } = await firstLabelCenter(page);

  const before = await quiltTransform(page);
  await page.mouse.move(x, y);
  await page.mouse.down();
  for (let i = 1; i <= 8; i++) await page.mouse.move(x + i * 15, y);
  await page.mouse.up();

  const after = await quiltTransform(page);
  expect(after.x - before.x).toBeGreaterThan(95);
});

test('a tap on a name badge still opens the patch', async ({ page }) => {
  await goto(page, '/');
  const { label } = await firstLabelCenter(page);
  const name = (await label.locator('.label-name').innerText()).trim();

  await label.tap();
  await expect(page).toHaveURL(/\/patches\/[^/]+$/);
  await expect(page.locator('h1')).toContainText(name);
});

test('a drag that starts on a name badge does not open the patch', async ({ page }) => {
  await goto(page, '/');
  const { x, y } = await firstLabelCenter(page);

  await touchDrag(page, x, y, [[20, 10], [45, 20], [70, 30], [95, 40]]);

  await expect(page).toHaveURL(/\/(\?.*)?$/);
});
