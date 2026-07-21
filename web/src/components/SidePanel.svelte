<script>
  import { X } from 'phosphor-svelte';
  let { open = false, onClose = () => {}, title = '', side = 'left', children } = $props();

  function handleBackdrop(e) {
    if (e.target === e.currentTarget) onClose();
  }

  function handleKeydown(e) {
    if (open && e.key === 'Escape') onClose();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <div class="sidepanel-backdrop" onclick={handleBackdrop}>
    <div class="sidepanel" class:from-left={side === 'left'} class:from-right={side === 'right'}>
      <div class="sidepanel-header">
        <h2 class="sidepanel-title">{title}</h2>
        <button class="sidepanel-close" onclick={onClose}>
          <X size={18} weight="bold" />
        </button>
      </div>
      <div class="sidepanel-body">
        {@render children()}
      </div>
    </div>
  </div>
{/if}

<style>
  .sidepanel-backdrop {
    position: fixed;
    inset: 0;
    background: var(--color-scrim);
    z-index: 200;
    animation: fadeIn 150ms ease;
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  .sidepanel {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 360px;
    max-width: 85vw;
    background: var(--color-surface);
    box-shadow: 4px 0 24px var(--color-shadow);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .sidepanel.from-left {
    left: 0;
    animation: slideInLeft 250ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  .sidepanel.from-right {
    right: 0;
    box-shadow: -4px 0 24px var(--color-shadow);
    animation: slideInRight 250ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  @keyframes slideInLeft {
    from { transform: translateX(-100%); }
    to { transform: translateX(0); }
  }

  @keyframes slideInRight {
    from { transform: translateX(100%); }
    to { transform: translateX(0); }
  }

  .sidepanel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .sidepanel-title {
    font-size: 1rem;
    font-weight: 700;
  }

  .sidepanel-close {
    width: 32px;
    height: 32px;
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

  .sidepanel-close:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }

  .sidepanel-body {
    flex: 1;
    overflow-y: auto;
  }
</style>
