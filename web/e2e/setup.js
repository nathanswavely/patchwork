/**
 * E2E test helpers.
 * Sets up dev session cookies for different user roles.
 * Tokens are created by `cmd/seed/main.go`.
 *
 * DATA OWNERSHIP — spec files share one backend and one seeded SQLite
 * database, and run in parallel workers (one worker per file). A spec file
 * that MUTATES seed data owns what it mutates; no other spec may assert on
 * that state. Current ownership map:
 *
 *   join-follow.spec.js      owns the `joiner` user and membership in
 *                            longhand-writers-guild (join/leave round trips).
 *                            Every membership mutation in that spec uses that
 *                            pair — it must never join or follow as `new`.
 *   card-actions.spec.js     owns the `lurker` user's follow edges
 *   appearance.spec.js       owns code-and-coffee's appearance column
 *   proposal-voting.spec.js  owns the admin's votes on lancaster-arts-district
 *                            proposals (specifically "Create youth mentorship
 *                            program")
 *   governance.spec.js       owns proposal creation in lancaster-arts-district
 *   notifications.spec.js    owns the admin's notification preferences and
 *                            code-and-coffee's notification config; also owns
 *                            the `new` user, whose bell it asserts is empty.
 *                            The ONLY membership `new` may ever be given is
 *                            the escapeOnboarding() follow of sowe-garden,
 *                            where no spec creates notifiable activity. A
 *                            membership anywhere a proposal or event lands
 *                            makes that assertion a coin flip — join-follow
 *                            used to do exactly that, and the empty-bell test
 *                            failed intermittently (once in four runs when it
 *                            was caught on 2026-07-20).
 *   reporting.spec.js        inserts content_reports rows but asserts only on
 *                            request/response and modal state, never on report
 *                            counts or the admin queue — owns no shared state
 *   claims.spec.js           creates its own uniquely-named unclaimed patch
 *                            and claims it as `active` (claim never completes,
 *                            so no membership changes) — owns only that patch
 *   event-form.spec.js       creates its own uniquely-named patch as `admin`
 *                            (auto-admin membership) and posts one event to
 *                            it — owns only that patch and event
 *
 * Everything else must treat seed data as read-only, and assertions on shared
 * entities (e.g. lancaster-arts-district lists) must be keyed to specific
 * seeded names rather than exact counts, so concurrent owned mutations can't
 * break them.
 */

const DEV_TOKENS = {
  admin: 'dev-admin-token',       // Site admin, member of many patches
  organizer: 'dev-organizer-token', // Runs 6 patches
  active: 'dev-active-token',     // Member of many patches
  lurker: 'dev-lurker-token',     // Follows lots, joins none
  new: 'dev-new-token',           // Brand new, no memberships — read-only!
  joiner: 'dev-joiner-token',     // Owned by join-follow.spec.js round trips
};

/**
 * Authenticate as a specific role by setting the dev session cookie.
 * @param {import('@playwright/test').Page} page
 * @param {'admin'|'organizer'|'active'|'lurker'|'new'} role
 */
export async function loginAs(page, role = 'admin') {
  const token = DEV_TOKENS[role];
  if (!token) throw new Error(`Unknown test role: ${role}`);
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

/** Shorthand for loginAs(page, 'admin') */
export async function loginAsAdmin(page) {
  return loginAs(page, 'admin');
}

/** Clear session cookie for unauthenticated tests. */
export async function logout(page) {
  await page.context().clearCookies();
}

/**
 * Give a membership-less user one follower membership so the app's
 * first-run onboarding redirect (App.svelte: zero memberships -> /welcome)
 * doesn't intercept navigation. Follows a patch unrelated to the ones the
 * membership tests exercise.
 */
export async function escapeOnboarding(page, slug = 'sowe-garden') {
  await page.request.post(`/api/v1/nodes/${slug}/join`, {
    data: { role: 'follower' },
    headers: { 'X-Patchwork-Request': 'true' },
  });
}

/**
 * Navigate and wait for the page to settle (network idle + no loading spinners).
 * Replaces the pattern of goto + waitForTimeout.
 */
export async function goto(page, path) {
  await page.goto(path);
  await page.waitForLoadState('networkidle');
}

/**
 * Assert that no "Page not found" or error state is visible.
 */
export async function expectNoError(page) {
  const notFound = await page.getByText('Page not found').isVisible().catch(() => false);
  const error500 = await page.getByText('Internal Server Error').isVisible().catch(() => false);
  if (notFound) throw new Error(`Page shows "Page not found" at ${page.url()}`);
  if (error500) throw new Error(`Page shows 500 error at ${page.url()}`);
}
