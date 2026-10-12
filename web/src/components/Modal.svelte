<script>
  /**
   * A modal: Liquid Textile's dialog on a native <dialog> (ltDialog in
   * lib/lt.js). The browser holds focus inside it, closes it on Escape and
   * makes the page behind inert; the backdrop and the close button close
   * it too, and focus goes back where it was. It sits in the top layer,
   * above every z-index, so it needs no portal out of a positioned box.
   * Its content renders only while it is open.
   */
  import { X } from 'phosphor-svelte';
  import { ltDialog } from '../lib/lt.js';

  let { open = false, onClose = () => {}, label = 'Dialog', children } = $props();
</script>

<dialog class="lt-dialog modal" aria-label={label} use:ltDialog={{ open, onClose }}>
  <button class="lt-dialog-close" data-close aria-label="Close">
    <X size={16} weight="bold" />
  </button>
  {#if open}
    {@render children()}
  {/if}
</dialog>

<style>
  /* Liquid Textile's dialog sizes to its content, up to 32rem. These hold
     forms, which want the whole of it. */
  .modal {
    width: 100%;
  }
</style>
