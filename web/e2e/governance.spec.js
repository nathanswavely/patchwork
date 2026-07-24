/**
 * E2E: Governance — Documents, Proposals, Voting, Amendments
 * Tests the full proposal lifecycle from document editing to vote resolution.
 */
import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, goto, expectNoError } from './setup.js';

const PATCH_SLUG = 'lancaster-arts-district';
const PATCH_URL = `/patches/${PATCH_SLUG}`;

test.describe('Governance — Document List', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('governance docs page loads with documents', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/docs`);
    await expectNoError(page);
    // Should show at least one governance document
    const docItems = page.locator('.card, [class*="doc"]').first();
    await expect(docItems).toBeVisible({ timeout: 5000 });
  });

  test('governance overview loads with sidebar nav', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance`);
    await expectNoError(page);
    // Sidebar should show governance sections (scope to the sidebar —
    // the same words appear elsewhere on the page).
    const sidebar = page.locator('.settings-sidebar, .settings-nav').first();
    if (await sidebar.isVisible()) {
      await expect(sidebar.getByText('Overview').first()).toBeVisible();
      await expect(sidebar.getByText('Documents').first()).toBeVisible();
      await expect(sidebar.getByText('Proposals').first()).toBeVisible();
    }
  });
});

test.describe('Governance — Proposal List', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('proposal list loads with filter chips', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/proposals`);
    await expectNoError(page);
    // Should have status filter chips (scoped — statuses also appear on
    // the proposal cards themselves).
    await expect(page.locator('.chip', { hasText: 'Open' }).first()).toBeVisible();
    await expect(page.locator('.chip', { hasText: 'Approved' }).first()).toBeVisible();
    await expect(page.locator('.chip', { hasText: 'All' }).first()).toBeVisible();
  });

  test('clicking filter chip changes displayed proposals', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const allChip = page.locator('.chip', { hasText: 'All' });
    await allChip.click();
    await page.waitForLoadState('networkidle');
    // Should not error
    await expectNoError(page);
  });

  test('"New Proposal" button visible for members', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const newBtn = page.locator('a, button', { hasText: 'New Proposal' });
    await expect(newBtn).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Governance — Create General Proposal', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('proposal form loads for general proposals', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/new`);
    await expectNoError(page);
    await expect(page.locator('input#title')).toBeVisible();
    await expect(page.locator('textarea#body')).toBeVisible();
  });

  test('proposal form has type selector without Amendment option', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/new`);
    // Should have Action, Membership — but NOT Amendment.
    await expect(page.locator('.type-radio-label', { hasText: 'Action' })).toBeVisible();
    await expect(page.locator('.type-radio-label', { hasText: 'Membership' })).toBeVisible();
    const amendmentOption = page.locator('.type-radio-label', { hasText: 'Amendment' });
    await expect(amendmentOption).not.toBeVisible();
  });

  test('can submit a general proposal', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/new`);
    await page.locator('input#title').fill('Test Action Proposal ' + Date.now());
    await page.locator('textarea#body').fill('This is a test proposal for E2E testing.');
    await page.locator('button[type="submit"]').click();
    await page.waitForLoadState('networkidle');
    // Should redirect to the proposal detail page
    expect(page.url()).toMatch(/\/governance\//);
    await expectNoError(page);
  });
});

test.describe('Governance — Proposal Detail', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('proposal detail loads with tabs', async ({ page }) => {
    // Navigate to proposals list and click the first one
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const firstProposal = page.locator('.proposal-card').first();
    if (await firstProposal.isVisible()) {
      await firstProposal.click();
      await page.waitForLoadState('networkidle');
      await expectNoError(page);
      // Should have tabs
      await expect(page.locator('.tab', { hasText: 'Overview' })).toBeVisible();
      await expect(page.locator('.tab', { hasText: 'Discussion' })).toBeVisible();
    }
  });

  test('proposal detail shows status banner', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const firstProposal = page.locator('.proposal-card').first();
    if (await firstProposal.isVisible()) {
      await firstProposal.click();
      await page.waitForLoadState('networkidle');
      const banner = page.locator('.status-banner');
      await expect(banner).toBeVisible({ timeout: 5000 });
    }
  });

  test('tab switching works on proposal detail', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const firstProposal = page.locator('.proposal-card').first();
    if (await firstProposal.isVisible()) {
      await firstProposal.click();
      await page.waitForLoadState('networkidle');

      // Click Discussion tab
      const discussionTab = page.locator('.tab', { hasText: 'Discussion' });
      if (await discussionTab.isVisible()) {
        await discussionTab.click();
        // Should show comment thread
        await page.waitForTimeout(500);
        await expectNoError(page);
      }

      // Click History tab
      const historyTab = page.locator('.tab', { hasText: 'History' });
      if (await historyTab.isVisible()) {
        await historyTab.click();
        await page.waitForTimeout(500);
        await expectNoError(page);
      }
    }
  });
});

test.describe('Governance — Amendment Flow', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('document detail has "Propose change" button for members', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/docs`);
    // Exclude the "New Charter" (/docs/new) and propose links — we want a
    // real document detail page.
    const firstDoc = page.locator('a[href*="/governance/docs/"]:not([href$="/new"]):not([href*="/propose"]):not([href*="/history"])').first();
    if (await firstDoc.isVisible()) {
      await firstDoc.click();
      await page.waitForLoadState('networkidle');
      const proposeBtn = page.locator('a, button', { hasText: 'Propose change' });
      await expect(proposeBtn).toBeVisible({ timeout: 5000 });
    }
  });

  test('amendment editor loads with full-file textarea', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/docs`);
    const firstDoc = page.locator('a[href*="/governance/docs/"]:not([href$="/new"]):not([href*="/propose"]):not([href*="/history"])').first();
    if (await firstDoc.isVisible()) {
      await firstDoc.click();
      await page.waitForLoadState('networkidle');
      const proposeBtn = page.locator('a, button', { hasText: 'Propose change' });
      if (await proposeBtn.isVisible()) {
        await proposeBtn.click();
        await page.waitForLoadState('networkidle');
        await expectNoError(page);
        // Should show a full-size textarea with document content
        const editor = page.locator('textarea.full-editor');
        await expect(editor).toBeVisible({ timeout: 5000 });
        const content = await editor.inputValue();
        expect(content.length).toBeGreaterThan(0);
      }
    }
  });

  test('amendment editor shows diff when content changes', async ({ page }) => {
    await goto(page, `${PATCH_URL}/governance/docs`);
    const firstDoc = page.locator('a[href*="/governance/docs/"]').first();
    if (await firstDoc.isVisible()) {
      await firstDoc.click();
      await page.waitForLoadState('networkidle');
      const proposeBtn = page.locator('a, button', { hasText: 'Propose change' });
      if (await proposeBtn.isVisible()) {
        await proposeBtn.click();
        await page.waitForLoadState('networkidle');
        const editor = page.locator('textarea.full-editor');
        if (await editor.isVisible()) {
          // Add text to trigger diff
          const current = await editor.inputValue();
          await editor.fill(current + '\n\n## New Section\nThis is a test addition.');
          // Click "Show diff preview"
          const diffToggle = page.locator('.diff-toggle');
          if (await diffToggle.isVisible()) {
            await diffToggle.click();
            // Diff view should appear
            await expect(page.locator('.diff-view')).toBeVisible({ timeout: 3000 });
          }
          // "Review & submit" should be enabled
          const reviewBtn = page.locator('button', { hasText: 'Review & submit' });
          await expect(reviewBtn).toBeEnabled();
        }
      }
    }
  });
});

test.describe('Governance — Voting', () => {
  test('member can see vote buttons on open proposal', async ({ page }) => {
    await loginAs(page, 'active');
    await goto(page, `${PATCH_URL}/governance/proposals`);
    // Find an open proposal
    const openChip = page.locator('.chip', { hasText: 'Open' });
    await openChip.click();
    await page.waitForLoadState('networkidle');
    const firstProposal = page.locator('.proposal-card').first();
    if (await firstProposal.isVisible()) {
      await firstProposal.click();
      await page.waitForLoadState('networkidle');
      // Vote buttons should be visible (Approve, Reject, Abstain)
      const approveBtn = page.locator('.vote-btn.approve');
      if (await approveBtn.isVisible()) {
        await expect(page.locator('.vote-btn.reject')).toBeVisible();
        await expect(page.locator('.vote-btn.abstain')).toBeVisible();
      }
    }
  });

  test('follower cannot see vote buttons', async ({ page }) => {
    await loginAs(page, 'lurker');
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const firstProposal = page.locator('.proposal-card').first();
    if (await firstProposal.isVisible()) {
      await firstProposal.click();
      await page.waitForLoadState('networkidle');
      // Vote buttons should NOT be visible for followers
      const approveBtn = page.locator('.vote-btn.approve');
      const isVoteVisible = await approveBtn.isVisible().catch(() => false);
      expect(isVoteVisible).toBe(false);
    }
  });
});

test.describe('Governance — Diff View', () => {
  test('diff view renders with split/unified toggle', async ({ page }) => {
    await loginAsAdmin(page);
    await goto(page, `${PATCH_URL}/governance/proposals`);
    const firstProposal = page.locator('.proposal-card').first();
    if (await firstProposal.isVisible()) {
      await firstProposal.click();
      await page.waitForLoadState('networkidle');
      // Switch to Changes tab if it exists (amendment proposals)
      const changesTab = page.locator('.tab', { hasText: 'Changes' });
      if (await changesTab.isVisible()) {
        await changesTab.click();
        await page.waitForTimeout(500);
        const diffView = page.locator('.diff-view');
        if (await diffView.isVisible()) {
          // Should have mode toggle
          const modeToggle = page.locator('.mode-toggle');
          await expect(modeToggle).toBeVisible();
        }
      }
    }
  });
});
