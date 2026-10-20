/**
 * What a Copy button falls back to when the clipboard refuses (F-031).
 *
 * "Copy failed. Select the address instead" was only honest where the
 * address was on screen whole. Every caller now shows the text unclamped
 * and selects it, so a phone's long-press, or Ctrl+C, finishes the job.
 * SecuritySettings' recovery codes are the model: the text is already
 * there to take.
 */

/** Select every character inside `node`. */
export function selectContents(node) {
  if (!node || typeof window === 'undefined') return;
  const selection = window.getSelection?.();
  if (!selection) return;
  const range = document.createRange();
  range.selectNodeContents(node);
  selection.removeAllRanges();
  selection.addRange(range);
}

/**
 * Svelte action: select the node's text whenever `active` turns true.
 * `use:selectWhen={failed === value}`.
 */
export function selectWhen(node, active) {
  if (active) selectContents(node);
  return {
    update(next) {
      if (next) selectContents(node);
    },
  };
}
