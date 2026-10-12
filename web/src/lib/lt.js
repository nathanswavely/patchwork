/**
 * Liquid Textile's behaviours, for Patchwork. The one module that imports
 * the vendored behaviors.js, so no component reaches into vendor/ and the
 * day Liquid Textile becomes a package only this import moves
 * (docs/adr/2026-10-11-the-design-system-is-liquid-textile-vendored.md).
 *
 * Step 4b brought the viewport: the media conditions Liquid Textile's CSS
 * is written against, so script decides "narrow" exactly where the
 * stylesheet does. Step 5 turns its behaviours into Svelte actions, one
 * per behaviour, each rebuilt when its options change:
 *
 *   <button use:ltConfirm={{ onConfirm, confirmLabel, tone }}>
 */
import { confirmAction, dialog } from '../vendor/liquid-textile/behaviors.js';

export { media, viewport, toast } from '../vendor/liquid-textile/behaviors.js';

/** A behaviour whose function takes (node, options) and returns { destroy }. */
function action(make) {
  return (node, opts) => {
    let handle = make(node, opts);
    return {
      update(next) { handle.destroy(); handle = make(node, next); },
      destroy() { handle.destroy(); },
    };
  };
}

/** A two-step button for an act you cannot take back (confirmAction). */
export const ltConfirm = action((button, opts) => confirmAction(button, opts));

/**
 * A native <dialog class="lt-dialog | lt-drawer | lt-sheet"> run by
 * dialog(): the browser holds focus inside, closes on Escape and makes the
 * page inert; dialog() adds the backdrop click, [data-close] and handing
 * focus back. It sits in the top layer, above every z-index.
 *
 *   <dialog class="lt-dialog" use:ltDialog={{ open, onClose }}>
 *
 * `open` opens and closes it. onClose runs when the person closes it
 * (Escape, the backdrop, a [data-close] button), not when `open` went
 * false, so a page's own close never echoes back as a second one.
 */
export function ltDialog(el, opts) {
  let current = opts;
  const handle = dialog(el, { onClose: () => { if (current.open) current.onClose?.(); } });
  if (current.open) handle.open();
  return {
    update(next) {
      current = next;
      if (next.open) handle.open(); else handle.close();
    },
    destroy() {
      current = { open: false };
      handle.close();
    },
  };
}
