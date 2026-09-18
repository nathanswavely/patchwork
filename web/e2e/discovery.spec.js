/**
 * E2E: Discovery Mode (User Stories 1.1–1.8)
 * Tests the quilt canvas, top bar (search, workspace switcher, user menu),
 * sidebar nav, and patch/event browsing.
 */
import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './setup.js';

test.describe('Discovery — Quilt View', () => {
  test('1.1 — quilt renders with tiles on page load', async ({ page }) => {
    await page.goto('/');
    // Wait for SVG tiles to appear
    await page.locator('svg .tile').first().waitFor({ timeout: 10000 });
  });

  test('1.1 — canvas has woven background texture', async ({ page }) => {
    await page.goto('/');
    const container = page.locator('.canvas-container');
    await expect(container).toBeVisible();
    // lt-fill-canvas and lt-texture-grain classes
    await expect(container).toHaveClass(/lt-fill-canvas/);
    await expect(container).toHaveClass(/lt-texture-grain/);
  });

  test('1.3 — filter button opens chip strip', async ({ page }) => {
    await page.goto('/');
    // Quilt tiles are the page-ready signal; the filter button (if this
    // viewport shows one) only ever appears after them.
    await page.locator('svg .tile').first().waitFor({ timeout: 10000 });
    const filterBtn = page.locator('.filter-btn');
    if (await filterBtn.isVisible()) {
      await filterBtn.click();
      await expect(page.locator('.filter-chips .chip').first()).toBeVisible();
    }
  });

  test('1.4 — top bar search accepts text and opens the dropdown', async ({ page }) => {
    // The search is an autocomplete dropdown (docs/adr/033): typing shows
    // typed results plus the "Show matches on the quilt" action row.
    await page.goto('/');
    const searchInput = page.locator('.finder-input');
    await expect(searchInput).toBeVisible();
    await searchInput.click();
    await searchInput.fill('Lancaster');
    await expect(searchInput).toHaveValue('Lancaster');
    await expect(page.locator('.finder-results')).toBeVisible();
    await expect(page.locator('.finder-action')).toContainText('Show matches on the quilt');
  });

  test('1.8 — the Display menu switches theme between light and dark', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    // The lone Light/Dark toggle became the Display menu's Theme row when
    // docs/adr/112 added Colors beside it. Three choices now, not a flip —
    // `system` was always the store's default and no UI ever offered it back.
    await page.locator('.bar-avatar-btn').click();
    await expect(page.locator('.display-menu')).toBeVisible();

    await page.locator('.display-row', { hasText: 'Theme' }).getByText('Light', { exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    // The dropdown stays open, so the next choice is one click away.
    await page.locator('.display-row', { hasText: 'Theme' }).getByText('Dark', { exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });

  test('1.8b — the Display menu switches the quilt between default and muted', async ({ page }) => {
    // docs/adr/112. Held per browser rather than on the account, so this
    // asserts localStorage rather than anything the server knows.
    await loginAsAdmin(page);
    await page.goto('/');

    await page.locator('.bar-avatar-btn').click();
    const colors = page.locator('.display-row', { hasText: 'Colors' });
    await expect(colors.getByText('Default', { exact: true })).toHaveAttribute('aria-pressed', 'true');

    await colors.getByText('Muted', { exact: true }).click();
    await expect(colors.getByText('Muted', { exact: true })).toHaveAttribute('aria-pressed', 'true');
    expect(await page.evaluate(() => localStorage.getItem('patchwork-colors'))).toBe('muted');

    await colors.getByText('Default', { exact: true }).click();
    expect(await page.evaluate(() => localStorage.getItem('patchwork-colors'))).toBe('default');
  });

  test('1.8c — a signed-out reader gets the Display menu in the same slot', async ({ page }) => {
    // docs/adr/112: the control must not move when somebody joins, and an
    // anonymous reader previously had no theme control at all.
    await page.goto('/');

    await expect(page.locator('.bar-avatar-btn')).toHaveCount(0);
    await page.locator('.bar-display-btn').click();
    await expect(page.locator('.display-menu')).toBeVisible();
    await expect(page.locator('.display-row', { hasText: 'Colors' })).toBeVisible();
  });
});

test.describe('Discovery — Sidebar Navigation', () => {
  test('1.5 — Patches nav item shows the quilt', async ({ page }) => {
    await page.goto('/events');

    await page.locator('.rail-item', { hasText: 'Patches' }).click();
    await page.locator('svg .tile').first().waitFor({ timeout: 10000 });
    expect(new URL(page.url()).pathname).toBe('/');
  });

  test('1.6 — Events nav item shows the event list', async ({ page }) => {
    await page.goto('/');

    await page.locator('.rail-item', { hasText: 'Events' }).click();
    await page.waitForURL(/\/events/);
    expect(page.url()).toContain('/events');
  });

  test('9.2 — notification panel opens from bell and closes', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    await page.locator('.bar-bell .bell-btn').click();
    const panel = page.locator('.sidepanel');
    await expect(panel).toBeVisible({ timeout: 2000 });

    await page.locator('.sidepanel .close-btn, .sidepanel-close').first().click();
    await expect(panel).not.toBeVisible({ timeout: 2000 });
  });
});

test.describe('Discovery — Search keyboard behaviour', () => {
  test('Enter takes the first result when there is one', async ({ page }) => {
    await page.goto('/');
    const searchInput = page.locator('.finder-input');
    await searchInput.click();
    await searchInput.fill('Lancaster');
    const first = page.locator('.finder-item').first();
    await expect(first).toHaveClass(/active/);
    await first.locator('.finder-item-label').textContent();
    await searchInput.press('Enter');
    await expect(page.locator('.finder-results')).toBeHidden();
    expect(new URL(page.url()).pathname).not.toBe('/');
  });

  test('Enter on a zero-result query stays put — the suggest row is opt-in', async ({ page }) => {
    // The suggest row is never preselected: Enter is how people submit a
    // search, and it used to leave for the submission form unannounced.
    await page.goto('/');
    const searchInput = page.locator('.finder-input');
    await searchInput.click();
    await searchInput.fill('zzzzqqqq');
    await expect(page.locator('.finder-empty')).toContainText('No matches');
    const suggest = page.locator('.finder-action');
    await expect(suggest).toContainText('Suggest');
    await expect(suggest).not.toHaveClass(/active/);

    await searchInput.press('Enter');
    // Enter on a zero-result query is handled synchronously (activeIndex
    // stays -1, so onKeydown returns before touching navigation) — no
    // navigation is ever in flight to wait out.
    await expect(page).toHaveURL('/');

    // Still reachable deliberately.
    await suggest.click();
    await expect(page).toHaveURL(/\/submit\?name=zzzzqqqq/);
  });
});

test.describe('Discovery — Mobile search takeover', () => {
  test.use({ viewport: { width: 390, height: 780 } });

  test('the shelf search button opens the panel on the first tap', async ({ page }) => {
    // The tap that mounts the takeover is still bubbling toward window when
    // Svelte flushes the finder in; it must not close the panel autofocus
    // just opened, or typing looks dead until you tap the field again.
    await page.goto('/');
    await page.locator('.rail-search').click();
    const input = page.locator('.mobile-search-bar .finder-input');
    await expect(input).toBeFocused();
    await input.pressSequentially('Lanc');
    await expect(page.locator('.mobile-search-bar .finder-results')).toBeVisible();
  });

  test('tapping away closes the takeover', async ({ page }) => {
    await page.goto('/events');
    await page.locator('.rail-search').click();
    await expect(page.locator('.mobile-search-bar')).toBeVisible();
    await page.locator('.date-filter-btn').click();
    await expect(page.locator('.mobile-search-bar')).toBeHidden();
    expect(new URL(page.url()).pathname).toBe('/events');
  });
});

test.describe('Discovery — Workspace Switcher', () => {
  test('logged out — switcher shows instance only, no My Quilt option', async ({ page }) => {
    await page.goto('/');

    await page.locator('.scope-btn').click();
    const dropdown = page.locator('.scope-dropdown');
    await expect(dropdown).toBeVisible();
    await expect(dropdown.locator('.scope-option')).toHaveCount(1);
  });

  test('logged in — switcher offers instance and My Quilt', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    await page.locator('.scope-btn').click();
    const options = page.locator('.scope-dropdown .scope-option');
    await expect(options).toHaveCount(2);
    await expect(options.nth(1)).toContainText('My Quilt');
  });

  // Scope lives in the URL (docs/adr/035): / is the whole quilt for
  // everyone — the old logged-in default to My Quilt is gone.
  test('logged in — / is the whole quilt, never My Quilt', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    // Ground the negative assertion on the label actually being mounted —
    // otherwise a not-yet-rendered element (empty text) would pass trivially.
    await expect(page.locator('.scope-btn .logo-label')).toBeVisible();
    await expect(page.locator('.scope-btn .logo-label')).not.toContainText('My Quilt');
  });

  test('logged in — /my is My Quilt scope', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/my');

    await expect(page.locator('.scope-btn .logo-label')).toContainText('My Quilt');
  });

  test('logged in — selecting My Quilt moves to /my', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    await page.locator('.scope-btn').click();
    await page.locator('.scope-dropdown .scope-option').nth(1).click();
    await expect(page.locator('.scope-btn .logo-label')).toContainText('My Quilt');
    await expect(page).toHaveURL(/\/my$/);
  });

  test('logged in — selecting the instance switches scope back', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/my');

    await page.locator('.scope-btn').click();
    await page.locator('.scope-dropdown .scope-option').first().click();
    await expect(page.locator('.scope-btn .logo-label')).not.toContainText('My Quilt');
  });
});

test.describe('Discovery — Top Bar Identity', () => {
  test('logged out — shows "Log In" button instead of user icon', async ({ page }) => {
    await page.goto('/');

    const loginBtn = page.locator('.bar-login');
    await expect(loginBtn).toBeVisible();
    await expect(loginBtn).toContainText('Log In');
  });

  test('logged out — Log In button navigates to login page', async ({ page }) => {
    await page.goto('/');

    await page.locator('.bar-login').click();
    await page.waitForURL(/\/login/);
    expect(page.url()).toContain('/login');
  });

  test('logged in — shows avatar button with user menu', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    const avatarBtn = page.locator('.bar-avatar-btn');
    await expect(avatarBtn).toBeVisible();

    await avatarBtn.click();
    const dropdown = page.locator('.user-dropdown');
    await expect(dropdown).toBeVisible({ timeout: 2000 });
    // Dropdown is headed by the user's name
    await expect(dropdown.locator('.user-dropdown-name')).not.toBeEmpty();
  });

  test('logged in — user dropdown shows Settings and Log Out', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    await page.locator('.bar-avatar-btn').click();
    const dropdown = page.locator('.user-dropdown');
    await expect(dropdown).toBeVisible({ timeout: 2000 });

    await expect(dropdown.getByText('Settings')).toBeVisible();
    await expect(dropdown.getByText('Log Out')).toBeVisible();
  });

  test('logged in — user dropdown Admin link visible for admin users', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    await page.locator('.bar-avatar-btn').click();
    const dropdown = page.locator('.user-dropdown');
    await expect(dropdown).toBeVisible({ timeout: 2000 });
    await expect(dropdown.locator('a[href="/admin"]')).toBeVisible();
  });

  test('logged in — notification bell is visible in the top bar', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/');

    await expect(page.locator('.bar-bell')).toBeVisible();
  });
});
