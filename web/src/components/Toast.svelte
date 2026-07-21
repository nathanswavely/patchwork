<script>
  import { getToasts } from '../stores/toast.svelte.js';

  let toasts = $derived(getToasts());
</script>

{#if toasts.length > 0}
  <div class="toast-container">
    {#each toasts as toast (toast.id)}
      <div class="toast toast-{toast.type}">
        {toast.message}
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    bottom: 1.5rem;
    right: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    z-index: 1000;
    max-width: 360px;
  }

  .toast {
    padding: 0.75rem 1.25rem;
    border-radius: var(--radius);
    font-size: 0.9rem;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    box-shadow: 0 4px 12px var(--color-shadow);
    animation: slideUp 200ms ease;
  }

  .toast-success {
    border-left: 3px solid var(--color-success);
    color: var(--color-success);
  }

  .toast-error {
    border-left: 3px solid var(--color-error);
    color: var(--color-error);
  }

  .toast-info {
    border-left: 3px solid var(--color-primary);
    color: var(--color-text);
  }

  @keyframes slideUp {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (max-width: 640px) {
    .toast-container {
      left: 1rem;
      right: 1rem;
      max-width: none;
    }
  }
</style>
