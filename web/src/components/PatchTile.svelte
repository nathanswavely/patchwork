<script>
  /**
   * A patch's quilt tile as a fill: the same block, palette, and rotation
   * the quilt draws, rendered into a square SVG that covers its container
   * (preserveAspectRatio "slice" clips the overflow). Used anywhere a patch
   * deserves its tile rather than a flat identity color — card covers, etc.
   * Absolutely positioned; the parent provides position and stacking.
   */
  import * as d3 from 'd3';
  import { renderBlock } from '../lib/quiltBlocks.js';
  import { paletteForPatch } from '../lib/quiltTheme.js';

  let { patch, size = 120 } = $props();

  let svgEl = $state(null);

  $effect(() => {
    if (!svgEl) return;
    const palette = paletteForPatch(patch.id, patch.appearance);
    const svg = d3.select(svgEl);
    svg.selectAll('*').remove();
    renderBlock(svg.append('g'), size, patch.id, palette, patch.appearance);
  });
</script>

<svg
  bind:this={svgEl}
  class="patch-tile"
  viewBox="0 0 {size} {size}"
  preserveAspectRatio="xMidYMid slice"
  aria-hidden="true"
></svg>

<style>
  .patch-tile {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: block;
  }
</style>
