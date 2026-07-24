<script>
  let { message = 'Something went wrong.', retry = null } = $props();
</script>

<div class="error-state">
  <p class="error-message">{message}</p>
  {#if retry}
    <button class="btn btn-secondary" onclick={retry}>Try Again</button>
  {/if}
</div>

<style>
  .error-state {
    text-align: center;
    padding: 2rem 1rem;
  }

  .error-message {
    color: var(--color-error);
    margin-bottom: 1rem;
    font-size: 0.95rem;
  }
</style>
