/**
 * Colors — the viewer's standing choice between the quilt's two registers
 * (docs/adr/112). "default" draws what patches chose; "muted" carries their
 * hue, normalizes lightness across every tile, and caps chroma.
 *
 * Held per browser, like theme.svelte.js, and deliberately not on the
 * account: the Display menu has to reach a reader with no account, and there
 * is nothing to hang a column on for them. It also means this preference
 * never becomes user data — no endpoint, no migration, and nothing for the
 * seamrip boundary or the personal export to decide about.
 *
 * Default is the default on every instance, and no instance setting moves
 * it. Loud is the intent; muted is the accommodation.
 */

const STORAGE_KEY = 'patchwork-colors';

export const COLOR_MODES = ['default', 'muted'];

function read() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return COLOR_MODES.includes(stored) ? stored : 'default';
  } catch {
    // Private windows and blocked site data both throw here. A reader who
    // cannot be remembered still gets a working quilt.
    return 'default';
  }
}

let mode = $state(read());

/**
 * The register to draw in. Read this inside an effect or a $derived and the
 * caller re-runs when it changes — which is how quiltTheme.js gives every
 * card, marker and chip its reactivity without any of them knowing this
 * store exists.
 */
export function getColorMode() {
  return mode;
}

export function setColorMode(next) {
  if (!COLOR_MODES.includes(next)) return;
  mode = next;
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Not remembering the choice is survivable; refusing to make it is not.
  }
}
