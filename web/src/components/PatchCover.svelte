<script>
  /**
   * A patch's block run as a cover band — the same block, palette, and
   * rotation the quilt draws, repeated across a wide strip like yardage.
   * A single square sliced into a banner loses most of the block; repeating
   * it keeps whole blocks at full height however wide the band gets.
   * Absolutely positioned; the parent provides position and stacking.
   */
  import * as d3 from 'd3';
  import { renderBlock } from '../lib/quiltBlocks.js';
  import { paletteForPatch } from '../lib/quiltTheme.js';

  let { patch, size = 120, repeat = 6 } = $props();

  let svgEl = $state(null);

  $effect(() => {
    if (!svgEl) return;
    const palette = paletteForPatch(patch.id, patch.appearance);
    const svg = d3.select(svgEl);
    svg.selectAll('*').remove();
    for (let i = 0; i < repeat; i++) {
      const g = svg.append('g').attr('transform', `translate(${i * size}, 0)`);
      renderBlock(g, size, patch.id, palette, patch.appearance);
    }
  });
</script>

<svg
  bind:this={svgEl}
  class="patch-cover"
  viewBox="0 0 {size * repeat} {size}"
  preserveAspectRatio="xMidYMid slice"
  aria-hidden="true"
></svg>

<style>
  .patch-cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: block;
  }
</style>
