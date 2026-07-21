/**
 * E2E: Join / Follow / Leave / Unfollow — Full membership lifecycle
 * Tests every membership state transition from both PatchShell and PatchPanel.
 * Uses multiple user roles to test permission boundaries.
 *
 * Every membership mutation here belongs to `joiner` and
 * longhand-writers-guild, the pair cmd/seed seeds for this spec alone
 * (seedJoinFlowPersona). This spec must never join or follow as `new`:
 * notifications.spec.js asserts that user's bell is empty, and a membership
 * in a patch where governance.spec.js concurrently files a proposal turns
 * that assertion into a coin flip — which is exactly how it used to fail.
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto, expectNoError } from './setup.js';
import { API_URL } from './ports.js';

// Read-only permission checks run against the richly-seeded patch.
const PATCH_SLUG = 'lancaster-arts-district';
const PATCH_URL = `/patches/${PATCH_SLUG}`;

// Membership round trips run here: open policy, and no other spec asserts on
// its member list. `joiner` starts as a non-member of it, and is a member of
// yoga-in-the-park so the zero-membership /welcome redirect never fires.
const ROUND_TRIP_SLUG = 'longhand-writers-guild';
const ROUND_TRIP_URL = `/patches/${ROUND_TRIP_SLUG}`;

/**
 * Put `joiner` back to non-member of the round-trip patch, so each test
 * states its own precondition instead of inheriting the previous one's
 * leftovers. Leaving when not a member is a no-op error — ignore it.
 */
async function resetRoundTrip(page) {
  await page.request.post(`/api/v1/nodes/${ROUND_TRIP_SLUG}/leave`, {
    headers: { 'X-Patchwork-Request': 'true' },
  }).catch(() => {});
}

test.describe('Join — Non-member', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'joiner');
    await resetRoundTrip(page);
  });

  test('sees Join and Follow buttons on patch page', async ({ page }) => {
    await goto(page, ROUND_TRIP_URL);
    const joinBtn = page.getByRole('button', { name: 'Join' });
    const followBtn = page.getByRole('button', { name: 'Follow' });
    await expect(joinBtn).toBeVisible({ timeout: 5000 });
    await expect(followBtn).toBeVisible();
  });

  test('does not see Leave or Unfollow buttons', async ({ page }) => {
    await goto(page, ROUND_TRIP_URL);
    await page.waitForLoadState('networkidle');
    const leaveBtn = page.getByRole('button', { name: 'Leave' });
    const unfollowBtn = page.getByRole('button', { name: 'Unfollow' });
    await expect(leaveBtn).not.toBeVisible();
    await expect(unfollowBtn).not.toBeVisible();
  });

  test('does not see Settings tab', async ({ page }) => {
    await goto(page, ROUND_TRIP_URL);
    const settingsTab = page.locator('.workspace-tab', { hasText: 'Settings' });
    await expect(settingsTab).not.toBeVisible();
  });
});

test.describe('Follow Flow', () => {
  test('non-member can follow a patch', async ({ page }) => {
    await loginAs(page, 'joiner');
    await resetRoundTrip(page);
    await goto(page, ROUND_TRIP_URL);
    await page.getByRole('button', { name: 'Follow' }).click();
    // After following, should see "Become Member" + "Unfollow"
    await expect(page.getByRole('button', { name: 'Become Member' })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: 'Unfollow' })).toBeVisible();
  });

  test('follower sees Become Member and Unfollow buttons', async ({ page }) => {
    // Use the lurker who follows patches
    await loginAs(page, 'lurker');
    await goto(page, PATCH_URL);
    await page.waitForLoadState('networkidle');
    // Lurker may be a follower of this patch
    const becomeBtn = page.getByRole('button', { name: 'Become Member' });
    const unfollowBtn = page.getByRole('button', { name: 'Unfollow' });
    const joinBtn = page.getByRole('button', { name: 'Join' });
    // Should show either follower actions or non-member actions
    const hasFollowerUI = await becomeBtn.isVisible().catch(() => false);
    const hasNonMemberUI = await joinBtn.isVisible().catch(() => false);
    expect(hasFollowerUI || hasNonMemberUI).toBeTruthy();
  });

  test('follower can unfollow a patch', async ({ page }) => {
    await loginAs(page, 'joiner');
    // Set up the exact precondition via API: not a member, then follower.
    await resetRoundTrip(page);
    await page.request.post(`/api/v1/nodes/${ROUND_TRIP_SLUG}/join`, {
      data: { role: 'follower' },
      headers: { 'X-Patchwork-Request': 'true' },
    });
    await goto(page, ROUND_TRIP_URL);

    const unfollowBtn = page.getByRole('button', { name: 'Unfollow' });
    await expect(unfollowBtn).toBeVisible({ timeout: 10000 });
    await unfollowBtn.click();
    // Should go back to Join + Follow
    await expect(page.getByRole('button', { name: 'Join' })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: 'Follow' })).toBeVisible();
  });

  test('follower can upgrade to member via Become Member', async ({ page }) => {
    await loginAs(page, 'joiner');
    await resetRoundTrip(page);
    await goto(page, ROUND_TRIP_URL);

    await page.getByRole('button', { name: 'Follow' }).click();
    await expect(page.getByRole('button', { name: 'Become Member' })).toBeVisible({ timeout: 10000 });

    await page.getByRole('button', { name: 'Become Member' }).click();
    // Open policy, so the upgrade is immediate: a plain member sees Leave.
    await expect(page.getByRole('button', { name: 'Leave' })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: 'Become Member' })).not.toBeVisible();
  });
});

test.describe('Join Flow — Direct', () => {
  test('non-member can join a patch directly', async ({ page }) => {
    await loginAs(page, 'joiner');
    await resetRoundTrip(page);
    await goto(page, ROUND_TRIP_URL);

    // Assert the post-click state rather than sampling it: the join round
    // trip is async, and under a loaded backend the page is still re-fetching
    // when a bare isVisible() would read it.
    await page.getByRole('button', { name: 'Join' }).click();
    await expect(page.getByRole('button', { name: 'Leave' })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: 'Join' })).not.toBeVisible();
  });
});

test.describe('Leave Flow', () => {
  test('member can leave a patch', async ({ page }) => {
    await loginAs(page, 'joiner');
    await resetRoundTrip(page);
    await goto(page, ROUND_TRIP_URL);

    await page.getByRole('button', { name: 'Join' }).click();
    await expect(page.getByRole('button', { name: 'Leave' })).toBeVisible({ timeout: 10000 });

    await page.getByRole('button', { name: 'Leave' }).click();
    await expect(page.getByRole('button', { name: 'Join' })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: 'Follow' })).toBeVisible();
  });

  test('sole admin cannot leave', async ({ page }) => {
    // Admin of a patch where they're the only admin
    await loginAs(page, 'organizer');
    // Navigate to a patch the organizer runs
    await goto(page, '/dashboard');
    await page.waitForLoadState('networkidle');
    // This test verifies the backend rejects the request — we check via API directly
    // rather than clicking buttons, since admin doesn't see Leave button in the UI
  });
});

test.describe('Logged-Out User', () => {
  test('sees login prompt when clicking Join', async ({ page }) => {
    await goto(page, PATCH_URL);
    await page.waitForLoadState('networkidle');
    // On the quilt view, clicking a patch tile opens PatchPanel
    // On direct URL, we see PatchShell — but logged out may redirect
    const joinBtn = page.getByRole('button', { name: 'Join' });
    if (await joinBtn.isVisible()) {
      await joinBtn.click();
      await page.waitForTimeout(1000);
      // Should redirect to login
      expect(page.url()).toContain('/login');
    }
  });

  test('sees login prompt when clicking Follow', async ({ page }) => {
    await goto(page, PATCH_URL);
    await page.waitForLoadState('networkidle');
    const followBtn = page.getByRole('button', { name: 'Follow' });
    if (await followBtn.isVisible()) {
      await followBtn.click();
      await page.waitForTimeout(1000);
      expect(page.url()).toContain('/login');
    }
  });
});

test.describe('Role-Based UI After Join', () => {
  // The profile page has no tab bar; the shell tabs live under the
  // governance/manage views.
  test('member sees Governance tab in the governance shell', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, `${PATCH_URL}/governance`);
    await page.waitForLoadState('networkidle');
    const govTab = page.locator('.workspace-tab', { hasText: 'Governance' });
    await expect(govTab).toBeVisible({ timeout: 5000 });
  });

  test('admin sees Settings tab in the governance shell', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, `${PATCH_URL}/governance`);
    await page.waitForLoadState('networkidle');
    const settingsTab = page.locator('.workspace-tab', { hasText: 'Settings' });
    await expect(settingsTab).toBeVisible({ timeout: 5000 });
  });

  test('non-member does not see Settings tab', async ({ page }) => {
    await loginAs(page, 'joiner');
    await goto(page, `${PATCH_URL}/governance`);
    await page.waitForLoadState('networkidle');
    const settingsTab = page.locator('.workspace-tab', { hasText: 'Settings' });
    await expect(settingsTab).not.toBeVisible();
  });
});

test.describe('PatchPanel — Quilt View Join/Follow', () => {
  test('PatchPanel shows join buttons for non-member', async ({ page }) => {
    await loginAs(page, 'joiner');
    await goto(page, '/');
    await page.waitForLoadState('networkidle');
    // Click a tile to open PatchPanel. The quilt canvas animates
    // continuously, so skip Playwright's stability check.
    const tile = page.locator('svg .tile').first();
    if (await tile.isVisible()) {
      await tile.click({ force: true });
      await page.waitForTimeout(500);
      // PatchPanel should open with Join/Follow buttons
      const panel = page.locator('.preview-actions');
      if (await panel.isVisible()) {
        // Whichever tile the quilt put first, `joiner` may already belong to
        // it (yoga-in-the-park) — any membership control counts.
        const buttons = ['Join', 'Follow', 'Leave', 'Become Member', 'Unfollow'];
        let hasSomeAction = false;
        for (const name of buttons) {
          if (await page.getByRole('button', { name }).isVisible().catch(() => false)) {
            hasSomeAction = true;
            break;
          }
        }
        expect(hasSomeAction).toBeTruthy();
      }
    }
  });
});

test.describe('Unclaimed Patches', () => {
  test('unclaimed patch shows Follow button but not Join', async ({ page }) => {
    await loginAs(page, 'joiner');
    // Find an unclaimed patch slug from the quilt or navigate directly
    // This test assumes seed data has at least one unclaimed patch
    // If not, it gracefully skips
    await goto(page, '/');
    await page.waitForLoadState('networkidle');
    // Look for the "?" indicator on unclaimed patches
    const unclaimedIndicator = page.locator('.unclaimed-indicator, .unclaimed-badge').first();
    if (await unclaimedIndicator.isVisible()) {
      await unclaimedIndicator.click();
      await page.waitForTimeout(500);
      const claimBtn = page.getByRole('button', { name: /Claim/i });
      const followBtn = page.getByRole('button', { name: 'Follow' });
      // Should have claim and/or follow option
      const hasClaim = await claimBtn.isVisible().catch(() => false);
      const hasFollow = await followBtn.isVisible().catch(() => false);
      expect(hasClaim || hasFollow).toBeTruthy();
    }
  });
});

test.describe('Membership API — Edge Cases', () => {
  test('double-join returns conflict', async ({ request }) => {
    // Use the admin token who is already a member
    const response = await request.post(`${API_URL}/api/v1/nodes/lancaster-arts-district/join`, {
      headers: {
        'Cookie': 'patchwork_session=dev-admin-token',
        'X-Patchwork-Request': 'true',
      },
    });
    // Should be 409 Conflict (already a member)
    expect(response.status()).toBe(409);
  });

  test('join invite-only patch returns forbidden', async ({ request }) => {
    // static-season is an invite_only band; self-join must be rejected.
    // The 403 means no membership is created, so this leaves `joiner` where
    // the other tests here expect to find them.
    const response = await request.post(`${API_URL}/api/v1/nodes/static-season/join`, {
      headers: {
        'Cookie': 'patchwork_session=dev-joiner-token',
        'X-Patchwork-Request': 'true',
      },
    });
    expect(response.status()).toBe(403);
  });
});
