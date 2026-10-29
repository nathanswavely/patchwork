<script>
  /**
   * The "Made with AI" tag (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md,
   * decision 2). Only 'generated' is shown; 'assisted' is recorded and never
   * worn, and an unanswered question shows nothing. `image` names the image
   * rather than the text, which carries its own answer (decision 3).
   */
  let { value = null, image = false } = $props();
</script>

{#if value === 'generated'}
  <span class="lt-pill made-with-ai">{image ? 'Image made with AI' : 'Made with AI'}</span>
{/if}

<style>
  .made-with-ai {
    white-space: nowrap;
  }
</style>
