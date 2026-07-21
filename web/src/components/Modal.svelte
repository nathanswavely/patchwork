<script>
  import { X } from 'phosphor-svelte';
  let { open = false, onClose = () => {}, label = 'Dialog', children } = $props();

  let contentEl = $state(null);

  const FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  // Move focus into the dialog on open, restore it on close.
  $effect(() => {
    if (!open || !contentEl) return;
    const previous = document.activeElement;
    const first = contentEl.querySelector(FOCUSABLE);
    (first ?? contentEl).focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  });

  function handleBackdrop(e) {
    if (e.target === e.currentTarget) onClose();
  }

  function handleKeydown(e) {
    if (!open) return;
    if (e.key === 'Escape') {
      onClose();
      return;
    }
    if (e.key !== 'Tab' || !contentEl) return;
    const focusables = [...contentEl.querySelectorAll(FOCUSABLE)];
    if (focusables.length === 0) {
      e.preventDefault();
      contentEl.focus();
      return;
    }
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (e.shiftKey && (document.activeElement === first || document.activeElement === contentEl)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <div class="modal-backdrop" onclick={handleBackdrop}>
    <div
      class="modal-content"
      role="dialog"
      aria-modal="true"
      aria-label={label}
      tabindex="-1"
      bind:this={contentEl}
    >
      <button class="modal-close" aria-label="Close" onclick={onClose}>
        <X size={16} weight="bold" />
      </button>
      {@render children()}
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--color-scrim);
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
  }

  .modal-content {
    background: var(--color-surface);
    border-radius: var(--radius);
    box-shadow: 0 8px 32px var(--color-shadow);
    max-width: 560px;
    width: 100%;
    max-height: 80vh;
    overflow-y: auto;
    padding: 1.5rem;
    position: relative;
  }

  .modal-content:focus {
    outline: none;
  }

  .modal-close {
    position: absolute;
    top: 12px;
    right: 12px;
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
    border-radius: var(--radius);
    transition: background 100ms ease, color 100ms ease;
  }

  .modal-close:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }
</style>
