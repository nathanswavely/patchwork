<script>
  import {
    anchorsFor,
    facesForCell,
    facesForDraft,
    polygonCentroid,
    pointInPolygon,
    SEAM_BUDGET,
    MAX_GRID,
  } from '../lib/draftGeometry.js';
  import BundlePicker from './BundlePicker.svelte';

  // draft: {grid, seams, colors} in quarter-cell units (docs/adr/029).
  // bundle: 1-6 hex fabrics off the wall; slot 0 is the identity color.
  let { draft = $bindable(), bundle = $bindable() } = $props();

  const CANVAS = 336;

  let tool = $state('sew'); // 'sew' | 'color' | 'unpick'
  let selectedSlot = $state(0);
  let pendingAnchor = $state(null); // first anchor of a seam being sewn

  let unit = $derived(CANVAS / (4 * draft.grid));
  let anchors = $derived(anchorsFor(draft.grid));
  let cells = $derived(facesForDraft(draft));
  let seamsLeft = $derived(SEAM_BUDGET - draft.seams.length);

  function slotColor(i) {
    return bundle[i] ?? bundle[i % bundle.length] ?? '#cccccc';
  }

  function pieceFill(r, c, i) {
    return slotColor(draft.colors[`${r},${c}`]?.[i] ?? 0);
  }

  /**
   * Rebuild the colors map for a new seam set: each new piece inherits
   * the fabric of the old piece containing its centroid, so sewing and
   * unpicking never scramble what's already colored.
   */
  function reassignColors(newSeams) {
    const colors = {};
    for (let r = 0; r < draft.grid; r++) {
      for (let c = 0; c < draft.grid; c++) {
        const key = `${r},${c}`;
        const oldFaces = facesForCell(draft.seams, r, c);
        const oldSlots = draft.colors[key] || [];
        const newFaces = facesForCell(newSeams, r, c);
        const slots = newFaces.map((f) => {
          const [cx, cy] = polygonCentroid(f);
          const idx = oldFaces.findIndex((of) => pointInPolygon(of, cx, cy));
          return idx >= 0 ? (oldSlots[idx] ?? 0) : 0;
        });
        if (slots.some((s) => s !== 0)) colors[key] = slots;
      }
    }
    return colors;
  }

  function clickAnchor(a) {
    if (tool !== 'sew') return;
    if (!pendingAnchor) {
      pendingAnchor = a;
      return;
    }
    if (pendingAnchor[0] === a[0] && pendingAnchor[1] === a[1]) {
      pendingAnchor = null; // clicked the same anchor: cancel
      return;
    }
    if (draft.seams.length >= SEAM_BUDGET) return;
    const seam = [pendingAnchor[0], pendingAnchor[1], a[0], a[1]];
    const newSeams = [...draft.seams, seam];
    draft.colors = reassignColors(newSeams);
    draft.seams = newSeams;
    pendingAnchor = null;
  }

  function clickPiece(r, c, i) {
    if (tool !== 'color') return;
    const key = `${r},${c}`;
    const faceCount = facesForCell(draft.seams, r, c).length;
    const slots = [...(draft.colors[key] || [])];
    while (slots.length < faceCount) slots.push(0);
    slots[i] = selectedSlot;
    if (slots.some((s) => s !== 0)) {
      draft.colors[key] = slots;
    } else {
      delete draft.colors[key];
    }
  }

  function unpickSeam(idx) {
    if (tool !== 'unpick') return;
    const newSeams = draft.seams.filter((_, i) => i !== idx);
    draft.colors = reassignColors(newSeams);
    draft.seams = newSeams;
  }

  function changeGrid(e) {
    const g = +e.target.value;
    if (g === draft.grid) return;
    if (draft.seams.length && !confirm('Changing the grid clears your seams (colors of surviving cells stay). Continue?')) {
      e.target.value = draft.grid;
      return;
    }
    // Anchors are a function of grid size: seams can't survive, cell
    // colors can (collapsed to the cell's first piece's fabric).
    const colors = {};
    for (const [key, slots] of Object.entries(draft.colors)) {
      const [r, c] = key.split(',').map(Number);
      if (r < g && c < g && slots[0]) colors[key] = [slots[0]];
    }
    pendingAnchor = null;
    draft = { grid: g, seams: [], colors };
  }

</script>

{#snippet pieces(interactive)}
  {#each cells as cell (`${cell.r},${cell.c}`)}
    {#each cell.faces as poly, i}
      {#if interactive}
        <polygon
          points={poly.map(([x, y]) => `${x * unit},${y * unit}`).join(' ')}
          fill={pieceFill(cell.r, cell.c, i)}
          class="piece"
          class:colorable={tool === 'color'}
          role="button"
          tabindex="-1"
          onclick={() => clickPiece(cell.r, cell.c, i)}
          onkeydown={() => {}}
        />
      {:else}
        <polygon
          points={poly.map(([x, y]) => `${x * unit},${y * unit}`).join(' ')}
          fill={pieceFill(cell.r, cell.c, i)}
        />
      {/if}
    {/each}
  {/each}
{/snippet}

<div class="drafter">
  <div class="drafter-toolbar">
    <label class="grid-select">
      Grid
      <select value={draft.grid} onchange={changeGrid}>
        {#each Array.from({ length: MAX_GRID }, (_, i) => i + 1) as g}
          <option value={g}>{g}×{g}</option>
        {/each}
      </select>
    </label>
    <div class="tools" role="group" aria-label="Drafting tool">
      <button class:active={tool === 'sew'} onclick={() => { tool = 'sew'; }} title="Sew a seam between two anchors">
        Sew
      </button>
      <button class:active={tool === 'color'} onclick={() => { tool = 'color'; pendingAnchor = null; }} title="Color pieces with the selected fabric">
        Color
      </button>
      <button class:active={tool === 'unpick'} onclick={() => { tool = 'unpick'; pendingAnchor = null; }} title="Remove a seam">
        Unpick
      </button>
    </div>
    <span class="seam-budget" class:spent={seamsLeft === 0}>
      {draft.seams.length} of {SEAM_BUDGET} seams
    </span>
  </div>

  <div class="drafter-body">
    <svg
      class="drafter-canvas"
      viewBox="0 0 {CANVAS} {CANVAS}"
      width={CANVAS}
      height={CANVAS}
      role="img"
      aria-label="Block drafting canvas"
    >
      {@render pieces(true)}

      <!-- Cell walls, for orientation -->
      {#each Array.from({ length: draft.grid + 1 }, (_, i) => i) as i}
        <line x1={i * 4 * unit} y1="0" x2={i * 4 * unit} y2={CANVAS} class="gridline" />
        <line x1="0" y1={i * 4 * unit} x2={CANVAS} y2={i * 4 * unit} class="gridline" />
      {/each}

      <!-- Seams, drawn as stitching -->
      {#each draft.seams as s, i}
        <line
          x1={s[0] * unit} y1={s[1] * unit} x2={s[2] * unit} y2={s[3] * unit}
          class="seam-hit"
          class:unpickable={tool === 'unpick'}
          role="button"
          tabindex="-1"
          onclick={() => unpickSeam(i)}
          onkeydown={() => {}}
        />
        <line
          x1={s[0] * unit} y1={s[1] * unit} x2={s[2] * unit} y2={s[3] * unit}
          class="seam-line"
        />
      {/each}

      <!-- Anchors (sew mode) -->
      {#if tool === 'sew'}
        {#each anchors as a (a[0] + ':' + a[1])}
          <circle
            cx={a[0] * unit}
            cy={a[1] * unit}
            r={pendingAnchor && pendingAnchor[0] === a[0] && pendingAnchor[1] === a[1] ? 6 : 3.5}
            class="anchor"
            class:pending={pendingAnchor && pendingAnchor[0] === a[0] && pendingAnchor[1] === a[1]}
            role="button"
            tabindex="-1"
            onclick={() => clickAnchor(a)}
            onkeydown={() => {}}
          />
        {/each}
      {/if}
    </svg>

    <div class="drafter-side">
      <div class="mini-preview">
        <svg viewBox="0 0 {CANVAS} {CANVAS}" width="40" height="40" role="img" aria-label="Tile at quilt size">
          {@render pieces(false)}
        </svg>
        <span class="muted">at quilt size</span>
      </div>

      <BundlePicker
        bind:bundle
        bind:selectedSlot
        hint="Fabric {selectedSlot + 1} colors pieces you click. Pick its fabric from the wall:"
      />
    </div>
  </div>

  <p class="muted drafter-hint">
    {#if tool === 'sew'}
      Click an anchor, then a second anchor, to sew a seam. Seams split every piece they cross.
    {:else if tool === 'color'}
      Click a piece to color it with fabric {selectedSlot + 1}.
    {:else}
      Click a seam to unpick it. Pieces keep their fabric where they survive.
    {/if}
  </p>
</div>

<style>
  .drafter-toolbar {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
    margin-bottom: 0.6rem;
  }

  .grid-select {
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-text-muted);
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .grid-select select {
    padding: 0.2rem 0.35rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
  }

  .tools {
    display: flex;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .tools button {
    padding: 0.3rem 0.7rem;
    font-size: 0.78rem;
    font-weight: 600;
    background: var(--color-surface);
    color: var(--color-text-muted);
    border: none;
    cursor: pointer;
  }

  .tools button + button {
    border-left: 1px solid var(--color-border);
  }

  .tools button.active {
    background: var(--color-primary);
    color: #fff;
  }

  .seam-budget {
    font-size: 0.78rem;
    color: var(--color-text-muted);
    margin-left: auto;
  }

  .seam-budget.spent {
    color: var(--color-danger, #c0392b);
    font-weight: 600;
  }

  .drafter-body {
    display: flex;
    gap: 1rem;
    align-items: flex-start;
  }

  .drafter-canvas {
    border: 2px solid var(--lt-thread, var(--color-border));
    border-radius: 4px;
    flex-shrink: 0;
    background: var(--color-surface);
  }

  .piece.colorable {
    cursor: pointer;
  }

  /* Mouse-driven SVG controls: suppress the default focus ring (these are
     tabindex="-1" and have their own hover/selected affordances). */
  .piece:focus,
  .anchor:focus,
  .seam-hit:focus {
    outline: none;
  }

  .gridline {
    stroke: rgba(0, 0, 0, 0.18);
    stroke-width: 1;
    pointer-events: none;
  }

  .seam-line {
    stroke: rgba(0, 0, 0, 0.55);
    stroke-width: 1.5;
    stroke-dasharray: 5 3;
    pointer-events: none;
  }

  .seam-hit {
    stroke: transparent;
    stroke-width: 10;
  }

  .seam-hit.unpickable {
    cursor: pointer;
  }

  .seam-hit.unpickable:hover + .seam-line {
    stroke: var(--color-danger, #c0392b);
    stroke-width: 2.5;
  }

  .anchor {
    fill: var(--color-surface);
    stroke: rgba(0, 0, 0, 0.6);
    stroke-width: 1.25;
    cursor: pointer;
  }

  .anchor:hover {
    fill: var(--color-primary);
  }

  .anchor.pending {
    fill: var(--color-primary);
    stroke: #fff;
  }

  .drafter-side {
    min-width: 0;
    flex: 1;
  }

  .mini-preview {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }

  .mini-preview svg {
    border-radius: 2px;
    border: 1px solid var(--color-border);
  }

  .mini-preview .muted {
    font-size: 0.72rem;
  }

  .drafter-hint {
    font-size: 0.78rem;
    margin-top: 0.6rem;
  }

  @media (max-width: 720px) {
    .drafter-body {
      flex-direction: column;
    }

    .drafter-canvas {
      width: 100%;
      height: auto;
    }
  }
</style>
