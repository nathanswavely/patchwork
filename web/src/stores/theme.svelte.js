/**
 * Theme store — manages dark/light/system preference.
 * Persists to localStorage. Applies data-theme attribute to <html>.
 */

const STORAGE_KEY = 'patchwork-theme';

// 'dark' | 'light' | 'system' — default follows the OS so light-preferring
// first-time visitors don't land in dark mode.
let preference = $state(localStorage.getItem(STORAGE_KEY) || 'system');

// Resolved theme: always 'dark' or 'light'.
let resolved = $state(resolveTheme(preference));

function resolveTheme(pref) {
  if (pref === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return pref;
}

// Apply to DOM immediately.
function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
}

// Initialize on load.
applyTheme(resolved);

// Listen for system preference changes.
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (preference === 'system') {
    resolved = resolveTheme('system');
    applyTheme(resolved);
  }
});

export function getResolvedTheme() {
  return resolved;
}

export function setTheme(pref) {
  preference = pref;
  resolved = resolveTheme(pref);
  localStorage.setItem(STORAGE_KEY, pref);
  applyTheme(resolved);
}

export function toggleTheme() {
  const next = resolved === 'dark' ? 'light' : 'dark';
  setTheme(next);
}
