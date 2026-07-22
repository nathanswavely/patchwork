<script>
  let {
    label = 'Action',
    confirmLabel = 'Confirm',
    variant = 'default', // 'danger' | 'warning' | 'default'
    onConfirm = async () => {},
    autoRevertMs = 5000,
    disabled = false,
  } = $props();

  let confirming = $state(false);
  let executing = $state(false);
  let revertTimer;

  function startConfirm() {
    confirming = true;
    clearTimeout(revertTimer);
    revertTimer = setTimeout(() => { confirming = false; }, autoRevertMs);
  }

  function cancel() {
    confirming = false;
    clearTimeout(revertTimer);
  }

  async function doConfirm() {
    clearTimeout(revertTimer);
    executing = true;
    try {
      await onConfirm();
    } finally {
      executing = false;
      confirming = false;
    }
  }
</script>

{#if confirming}
  <span class="confirm-group">
    <span class="confirm-prompt">Are you sure?</span>
    <button
      class="btn btn-sm confirm-yes"
      class:btn-danger={variant === 'danger'}
      class:btn-warning={variant === 'warning'}
      class:btn-primary={variant === 'default'}
      onclick={doConfirm}
      disabled={executing}
    >
      {executing ? '...' : confirmLabel}
    </button>
    <button class="btn btn-sm btn-secondary" onclick={cancel} disabled={executing}>
      Cancel
    </button>
  </span>
{:else}
  <button
    class="btn btn-sm"
    class:btn-danger={variant === 'danger'}
    class:btn-warning={variant === 'warning'}
    class:btn-secondary={variant === 'default'}
    onclick={startConfirm}
    {disabled}
  >
    {label}
  </button>
{/if}

<style>
  .confirm-group {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .confirm-prompt {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    font-weight: 500;
  }

  .confirm-yes {
    font-weight: 600;
  }

  .btn-warning {
    background: var(--color-accent);
    color: var(--color-on-accent);
    border-color: var(--color-accent);
  }
</style>
