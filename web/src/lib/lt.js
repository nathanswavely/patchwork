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
import { confirmAction, dialog, menu, segmented, tabs } from '../vendor/liquid-textile/behaviors.js';

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
 * A menu of actions under a button (menu()): click or ArrowDown opens it on
 * its first item, arrows, Home and End move, Escape closes it and hands
 * focus back, and choosing an item, Tab or a click elsewhere closes it. It
 * flips above the button when there is no room below.
 *
 *   <button use:ltMenu={{ list: listEl, align: 'end' }}>
 *   <div class="lt-popover lt-menu" bind:this={listEl}>
 *     <button class="lt-menu-item">…</button>
 *
 * The list is always rendered; menu() hides and shows it, so it must not
 * set a display of its own. It is built once the list is bound, and an item
 * that appears later (an {#if} inside the list) is made a menu item as it
 * does. The list's stacking level is --lt-z-menu, set on its container.
 */
export const ltMenu = action((trigger, { list, ...opts } = {}) => {
  if (!list) return { destroy() {} };
  const handle = menu(trigger, list, opts);
  const enlist = () => {
    for (const el of list.querySelectorAll('.lt-menu-item:not([role])')) {
      el.setAttribute('role', 'menuitem');
      el.tabIndex = -1;
    }
  };
  const watch = new MutationObserver(enlist);
  watch.observe(list, { childList: true, subtree: true });
  return { destroy() { watch.disconnect(); handle.destroy(); } };
});

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

/**
 * A few mutually exclusive choices side by side (segmented()): a radio
 * group of its .lt-segment buttons, one tab stop, arrows move and choose,
 * aria-checked marks the choice. A segment's value is its data-value.
 *
 *   <div class="lt-segmented" use:ltSegmented={{ value, onChange }}>
 *
 * The page owns the value: a new `value` is shown without calling onChange.
 */
export function ltSegmented(group, opts) {
  let current = opts;
  const handle = segmented(group, { value: opts.value, onChange: (v) => current.onChange?.(v) });
  return {
    update(next) {
      current = next;
      if (next.value !== handle.value) handle.value = next.value;
    },
  };
}

/**
 * A row of .lt-tab in .lt-tabs (tabs()). Buttons with role="tab" and
 * aria-selected switch in place: one tab stop, Left, Right, Home and End
 * move and choose, and onSelect(tab) hands the chosen tab to the page. Links
 * with aria-current="page" just scroll into view. Either way the row keeps
 * data-overflow, which fades an edge with more tabs beyond it.
 *
 *   <div class="lt-tabs" role="tablist" use:ltTabs={{ onSelect }}>
 */
export function ltTabs(row, opts = {}) {
  let current = opts;
  const handle = tabs(row, { onSelect: (t) => current.onSelect?.(t) });
  return {
    update(next) { current = next || {}; },
    destroy() { handle.destroy(); },
  };
}
