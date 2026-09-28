/**
 * Whether the screen is narrow enough that a workspace drills down, one
 * level of navigation on screen at a time, instead of showing a tab row and
 * a sidebar together. The width is SettingsShell's own breakpoint, so the
 * CSS that stacks the sidebar and the script that decides whether a bare
 * tab URL redirects agree on what "narrow" means.
 */
export const NARROW_QUERY = '(max-width: 640px)';

const mq = typeof window !== 'undefined' && window.matchMedia
  ? window.matchMedia(NARROW_QUERY)
  : null;

let narrow = $state(mq ? mq.matches : false);

mq?.addEventListener('change', (e) => {
  narrow = e.matches;
});

export function isNarrow() {
  return narrow;
}
