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
import { confirmAction } from '../vendor/liquid-textile/behaviors.js';

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
