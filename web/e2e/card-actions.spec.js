/**
 * E2E: Patch card corner — the relationship indicator on patch-list cards.
 * admin → Manage chip (links to workspace); member → Member chip;
 * follower/none → working follow heart.
 *
 * OWNS (see setup.js): the lurker's follow edges (the round trip below
 * briefly unfollows one). Other specs may use the lurker but must tolerate
 * any individual follow being absent.
 */
import { test, expect } from '@playwright/test';
import { loginAs, logout, goto } from './setup.js';

async function myNodes(page) {
  const data = await page.request.get('/api/v1/me/nodes').then(r => r.json());
  return data.items || data;
}

function cardFor(page, name) {
  return page.locator('.patch-card', { hasText: name });
}

test.describe('Card corner — roles', () => {
  test('admin patches show a Manage chip that opens the workspace', async ({ page }) => {
    await loginAs(page, 'organizer');
    const adminOf = (await myNodes(page)).find(m => m.role === 'admin');
    expect(adminOf).toBeTruthy();

    await goto(page, '/');
    const card = cardFor(page, adminOf.node_name);
    await expect(card.locator('.card-manage-chip')).toBeVisible();
    await card.locator('.card-manage-chip').click();
    await page.waitForURL(`**/patches/${adminOf.node_slug}/governance**`);
  });

  test('member patches show a Member chip, not a heart', async ({ page }) => {
    await loginAs(page, 'active');
    const memberOf = (await myNodes(page)).find(m => m.role === 'member');
    expect(memberOf).toBeTruthy();

    await goto(page, '/');
    const card = cardFor(page, memberOf.node_name);
    await expect(card.locator('.card-member-chip')).toBeVisible();
    await expect(card.locator('.card-follow-btn')).toHaveCount(0);
  });
});

test.describe('Card corner — follow heart', () => {
  test('unfollow and re-follow round trip', async ({ page }) => {
    await loginAs(page, 'lurker');
    const followed = (await myNodes(page)).find(m => m.role === 'follower');
    expect(followed).toBeTruthy();
    const slug = followed.node_slug;

    await goto(page, '/');
    const heart = cardFor(page, followed.node_name).locator('.card-follow-btn');
    await expect(heart).toHaveClass(/following/);

    // Unfollow.
    await heart.click();
    await expect(heart).not.toHaveClass(/following/);
    let mine = await myNodes(page);
    expect(mine.find(m => m.node_slug === slug)).toBeUndefined();

    // Re-follow.
    await heart.click();
    await expect(heart).toHaveClass(/following/);
    mine = await myNodes(page);
    const back = mine.find(m => m.node_slug === slug);
    expect(back?.role).toBe('follower');
    expect(back?.status).toBe('active');
  });

  test('logged-out heart click goes to login', async ({ page }) => {
    await logout(page);
    await goto(page, '/');
    const heart = page.locator('.card-follow-btn').first();
    await expect(heart).toBeVisible();
    await heart.click();
    await page.waitForURL('**/login');
  });
});
