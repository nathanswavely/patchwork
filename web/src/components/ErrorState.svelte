<script>
  // `onRetry` is the name two admin pages passed; it was never read, so
  // their Try Again button never showed. Both names work.
  let { message = 'Something went wrong.', retry = null, onRetry = null } = $props();
  let again = $derived(retry ?? onRetry);
</script>

<!-- A load that failed: Liquid Textile's empty state, its title in the
     error tone, announced as it appears. -->
<div class="lt-empty" role="alert">
  <p class="lt-empty-title lt-tone-error">{message}</p>
  {#if again}
    <div class="lt-empty-actions">
      <button class="lt-btn lt-btn-secondary" onclick={again}>Try Again</button>
    </div>
  {/if}
</div>
