/**
 * The screen, as Liquid Textile measures it: a reactive shell over its
 * viewport() (lib/lt.js), the same way the theme store wraps initTheme.
 * The conditions are the ones its CSS is written against, so the script
 * that decides whether a bare tab URL redirects, or whether a tap
 * previews, agrees with the stylesheet that stacks the sidebar or docks
 * the tab bar.
 *
 *   isNarrow()             a phone layout: stack, drill down, tables to
 *                          lists (640px; SettingsShell's split)
 *   isHandheld()           the app's chrome goes mobile (768px): the bottom
 *                          tab bar, notifications as a page
 *   isCoarse()             a finger
 *   canHover()             a pointer that can rest without pressing
 *   prefersReducedMotion() the person asked for less movement
 *
 * Each is read reactively. ADR 094's per-gesture pointerType check stays
 * where it is: it asks what this touch was, not what the screen is.
 */
import { media, viewport } from '../lib/lt.js';

export const NARROW_QUERY = media.narrow;

const live = typeof window !== 'undefined' && typeof window.matchMedia === 'function';

let now = $state({ narrow: false, handheld: false, coarse: false, hover: true, reducedMotion: false });

if (live) {
  const vp = viewport((next) => { now = next; });
  now = Object.fromEntries(Object.keys(media).map((name) => [name, vp[name]]));
}

export function isNarrow() {
  return now.narrow;
}

export function isHandheld() {
  return now.handheld;
}

export function isCoarse() {
  return now.coarse;
}

export function canHover() {
  return now.hover;
}

export function prefersReducedMotion() {
  return now.reducedMotion;
}
