<script>
  /**
   * A panel from the side of the screen: Liquid Textile's drawer on a
   * native <dialog> (ltDialog in lib/lt.js), so focus stays inside, Escape
   * and the backdrop close it, and focus goes back afterwards. `side` is
   * the edge it opens from. Its content renders only while it is open.
   */
  import { X } from 'phosphor-svelte';
  import { ltDialog } from '../lib/lt.js';

  let { open = false, onClose = () => {}, title = '', side = 'left', children } = $props();
</script>

<dialog
  class="lt-drawer sidepanel"
  class:lt-drawer-start={side === 'left'}
  aria-label={title}
  use:ltDialog={{ open, onClose }}
>
  <div class="lt-drawer-head">
    <h2 class="sidepanel-title">{title}</h2>
    <button class="lt-dialog-close sidepanel-close" data-close aria-label="Close">
      <X size={18} weight="bold" />
    </button>
  </div>
  <div class="lt-drawer-body sidepanel-body">
    {#if open}
      {@render children()}
    {/if}
  </div>
</dialog>

<style>
  .sidepanel-title {
    margin: 0;
    font-size: 1rem;
    font-weight: 700;
  }

  /* What it holds runs to the edges (the notification rows). */
  .sidepanel-body {
    padding: 0;
  }
</style>
