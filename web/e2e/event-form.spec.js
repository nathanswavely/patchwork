/**
 * E2E: Event creation — the "select a patch" dropdown on /events/new.
 *
 * Regression coverage for issue #8: EventForm.svelte read the wrong field
 * names from GET /api/v1/me/nodes. That endpoint returns membership rows
 * shaped {id: <membership id>, node_id, node_name, node_slug, ...}, but the
 * form rendered `node.name` (undefined — blank option labels) and submitted
 * `node.id` (the *membership* row id, not the patch id) as the event's
 * node_id. With foreign_keys=ON that id doesn't match any row in `nodes`,
 * so the old code either 500s on submit or, worse, files the event under
 * whatever patch the stray id happens to collide with.
 *
 * DATA OWNERSHIP (see setup.js): creates its own fresh patch (unique name)
 * via the admin API and posts an event to it as `admin` — no assertions on
 * seed data, no mutation of any other spec's owned state. `admin` already
 * has many memberships, so a populated, multi-option dropdown is itself
 * part of the regression check (a single-option dropdown wouldn't catch a
 * wrong-id submission landing on the "right" patch by luck).
 */
import { test, expect } from '@playwright/test';
import { loginAs, goto } from './setup.js';

test.describe.configure({ mode: 'serial' });

let slug = '';
let patchName = '';

test.describe('Event creation — patch dropdown', () => {
  test('admin creates a fresh patch to host the event', async ({ page }) => {
    await loginAs(page, 'admin');
    await page.goto('/');
    patchName = `Event Form Test Patch ${Date.now()}`;
    const res = await page.request.post('/api/v1/nodes', {
      data: { name: patchName, visibility: 'public', membership_policy: 'open' },
      headers: { 'X-Patchwork-Request': 'true' },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    slug = body.slug || body.node?.slug;
    expect(slug).toBeTruthy();
  });

  test('dropdown shows real patch names and submits the selected patch', async ({ page }) => {
    await loginAs(page, 'admin');
    await goto(page, '/events/new');

    const select = page.locator('#node');
    await expect(select).toBeVisible();

    // The bug rendered every <option> with a blank label (node.name was
    // undefined on the membership shape). Assert the option carries the
    // real patch name, not an empty label.
    const option = select.locator('option', { hasText: patchName });
    await expect(option).toHaveCount(1);

    await select.selectOption({ label: patchName });

    await page.locator('#title').fill('Dropdown Regression Test Event');
    await page.locator('#starts-at').fill('2027-01-01T18:00');
    await page.getByRole('button', { name: 'Create Event' }).click();

    // Old bug: submits the *membership* id as node_id, which fails the
    // node_id foreign key (or, if it happened to collide, files the event
    // under the wrong patch) instead of navigating to the new event.
    await page.waitForURL(/\/events\/[^/]+$/, { timeout: 5000 });
    await expect(page.locator('h1', { hasText: 'Dropdown Regression Test Event' })).toBeVisible();
    await expect(page.getByText(`Hosted by ${patchName}`)).toBeVisible();

    // And it landed on the exact patch that was selected, not just some
    // patch — proves the *value* submitted was the node id, not the
    // membership id.
    await expect(page.locator('.host-link')).toHaveAttribute('href', `/patches/${slug}`);
  });
});
