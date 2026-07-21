<script>
  let { lines = 3, width = '100%', height = '1rem' } = $props();
</script>

<div class="skeleton-container" style="width: {width};">
  {#each Array(lines) as _, i (i)}
    <div
      class="skeleton-line"
      style="height: {height}; width: {i === lines - 1 && lines > 1 ? '60%' : '100%'};"
    ></div>
  {/each}
</div>

<style>
  .skeleton-container {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .skeleton-line {
    background: linear-gradient(
      90deg,
      var(--color-border) 25%,
      #f0ece8 50%,
      var(--color-border) 75%
    );
    background-size: 200% 100%;
    border-radius: var(--radius);
    animation: shimmer 1.5s ease-in-out infinite;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }
</style>
