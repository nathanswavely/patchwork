/**
 * First-run onboarding dismissal.
 *
 * The app redirects zero-membership users to /welcome (App.svelte). Skipping
 * onboarding must genuinely exit it — on an empty instance there is nothing
 * to follow, so without a persisted dismissal the redirect loops forever.
 *
 * Stored in localStorage keyed by user id: per-browser, and scoped so a
 * different account on a shared machine still gets its own first run.
 */
const PREFIX = 'patchwork_onboarding_dismissed:';

export function isOnboardingDismissed(userId) {
  if (!userId) return false;
  try {
    return localStorage.getItem(PREFIX + userId) === '1';
  } catch {
    return false;
  }
}

export function dismissOnboarding(userId) {
  if (!userId) return;
  try {
    localStorage.setItem(PREFIX + userId, '1');
  } catch {
    // Storage unavailable (private mode) — the redirect will fire again next
    // load, but skip still works within this session via in-page navigation.
  }
}
