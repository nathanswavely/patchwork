/**
 * Intro card dismissal (CONTEXT.md "Intro card", docs/adr/040).
 *
 * Anonymous, first-visit-only, dismissed once and gone forever. Unlike
 * lib/onboarding.js's dismissal (scoped per signed-in user id), there is no
 * account here to key on — an anonymous visitor is the browser, so one
 * unscoped key is correct and matches how the card itself is described:
 * a first-landing greeting, not a per-account preference.
 */
const KEY = 'patchwork_intro_dismissed';

export function isIntroDismissed() {
  try {
    return localStorage.getItem(KEY) === '1';
  } catch {
    return false;
  }
}

export function dismissIntro() {
  try {
    localStorage.setItem(KEY, '1');
  } catch {
    // Storage unavailable (private mode) — the card reappears next load,
    // which is a minor annoyance, not a broken flow.
  }
}
