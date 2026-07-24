<script>
  /**
   * Context crumb — the global bar's leading slot inside a takeover context:
   * quilt mark / context name. The mark always exits to the quilt; the name
   * goes to the context's root (workspace home or admin overview).
   */
  import { navigate } from '../stores/router.svelte.js';
  import { getInstanceIconUrl, getInstanceName } from '../stores/quilt.svelte.js';

  let { label = '', href = '/' } = $props();

  function handleNav(e, target) {
    e.preventDefault();
    navigate(target);
  }
</script>

<div class="context-crumb">
  <a
    href="/"
    class="crumb-mark"
    onclick={(e) => handleNav(e, '/')}
    title="Back to {getInstanceName()}"
    aria-label="Back to {getInstanceName()}"
  >
    {#if getInstanceIconUrl()}
      <img class="crumb-icon" src={getInstanceIconUrl()} alt="" width="20" height="20" />
    {:else}
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none">
        <rect x="2" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
        <rect x="13" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
        <rect x="2" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
        <rect x="13" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
      </svg>
    {/if}
  </a>
  <span class="crumb-sep">/</span>
  <a href={href} class="crumb-label" onclick={(e) => handleNav(e, href)}>{label}</a>
</div>

<style>
  .context-crumb {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 1;
    min-width: 0;
  }

  .crumb-mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: var(--radius);
    color: var(--color-primary);
    flex-shrink: 0;
    transition: background 150ms ease;
  }

  .crumb-mark:hover {
    background: var(--color-overlay);
  }

  .crumb-sep {
    color: var(--color-text-muted);
    font-size: 1rem;
    flex-shrink: 0;
  }

  .crumb-label {
    font-family: var(--font-display);
    font-variation-settings: 'BNCE' 20;
    font-weight: 700;
    font-size: 1.05rem;
    color: var(--color-text);
    text-decoration: none;
    line-height: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
    padding: 6px 8px;
    border-radius: var(--radius);
    transition: background 150ms ease;
  }

  .crumb-label:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  /* The quilt's own mark (docs/adr/014). Hard edges — textile surfaces
     have zero border-radius. */
  .crumb-icon {
    flex-shrink: 0;
    object-fit: cover;
    display: block;
  }
</style>
