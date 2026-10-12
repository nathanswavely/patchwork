<script>
  let { lines = 3, width = '100%', height = '1rem' } = $props();
</script>

<!-- Liquid Textile's skeleton: it spaces the lines and shortens the last
     one itself, and its shimmer is drawn from theme tokens, where this used
     to hard-code a light stripe that ran pale across dark mode. -->
<div style:width={width}>
  {#each Array(lines) as _, i (i)}
    <span class="lt-skeleton" style:height={height}></span>
  {/each}
</div>
