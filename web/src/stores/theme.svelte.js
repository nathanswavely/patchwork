/**
 * The theme: a reactive shell over Liquid Textile's initTheme (lib/lt.js),
 * the way the viewport store wraps viewport(). It sets data-theme on
 * <html> from the reader's choice, else the OS, and keeps following the OS
 * until they choose. Held per browser under 'patchwork-theme'.
 *
 * index.html's pre-paint script applies the same rule before the bundle
 * loads, so the first frame is already in the right theme; keep the two
 * reading the same key.
 *
 *   getResolvedTheme()    what is showing: 'light' or 'dark'
 *   getThemePreference()  what the reader chose: 'light', 'dark' or
 *                         'system', so the Display menu can keep "System"
 *                         lit while the OS swings it
 *   setTheme(pref)        'system' forgets the choice
 */
import { initTheme } from '../lib/lt.js';

const theme = initTheme({ key: 'patchwork-theme' });

let resolved = $state(theme.get());
let preference = $state(theme.preference());

// What is showing changes under the store too, when the OS swings and the
// reader has not chosen; data-theme is the one place both changes land.
new MutationObserver(() => { resolved = theme.get(); })
  .observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });

export function getResolvedTheme() {
  return resolved;
}

export function getThemePreference() {
  return preference;
}

export function setTheme(pref) {
  theme.set(pref);
  preference = theme.preference();
  resolved = theme.get();
}
