/**
 * Moves a node to <body> so a fixed overlay clears every stacking context
 * between its component and the root.
 *
 * A modal renders where it was mounted, and a mount point inside a
 * positioned box with a z-index — the cards pane (10), the docked panel in
 * it, the quilt pane (0) — pins the modal's scrim to that box's layer, so
 * the bar (60), the rail (55) and the filter chips all drew on top of it
 * while the dialog they were meant to be behind sat in the middle. The
 * join sheet shipped that way from the docked profile.
 *
 * The node must be the only node of its block: Svelte tears a block down
 * by walking siblings from its first node to its last, and a moved node's
 * siblings are body's (QuiltCanvas records the same hazard for a moved last
 * node). Modal keeps the backdrop as the sole child of its {#if}.
 */
export function portal(node) {
  document.body.appendChild(node);
  return {
    destroy() {
      node.remove();
    },
  };
}
