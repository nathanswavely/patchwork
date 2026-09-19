<script>
  /**
   * The moved-to pointer, rendered (docs/adr/090).
   *
   * ADR 012's third egress affordance: the old home saying where the new one
   * is, without needing anything from whoever runs this server. One component
   * for both levels because it is one sentence with two subjects, and because
   * the link handling is the part worth having in one place.
   *
   * A pointer at `https://host/patches/slug` also gets an in-app door to the
   * read-only remote patch card (docs/adr/024), which is the shape the
   * discovery search and Connected Quilts already recognise. Anything else is
   * a plain external link with rel="noopener" — this URL was typed by a patch
   * admin or by the person themselves, and it leaves the quilt.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { patchLinkPath, linkHost } from '../lib/patchLink.js';

  let { url = '', subject = 'patch' } = $props();

  let host = $derived(linkHost(url));
  let cardPath = $derived(
    typeof window === 'undefined' ? '' : patchLinkPath(url, window.location.host)
  );

  // A named handler rather than an inline arrow, and two {#if} branches
  // rather than one ternary, so every sentence a visitor reads is static
  // markup the copy ledger's extractor can see.
  function openCard(e) {
    e.preventDefault();
    navigate(cardPath);
  }
</script>

{#if url && host}
  <div class="moved-notice" class:line={subject === 'person'}>
    <p class="moved-line">
      {#if subject === 'person'}This person has moved to{:else}This patch has moved to{/if}
      <a href={url} target="_blank" rel="noopener">{host}</a>.
    </p>
    {#if cardPath}
      <a class="moved-card-link" href={cardPath} onclick={openCard}>Open the new patch here</a>
    {/if}
  </div>
{/if}

<style>
  .moved-notice {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.5rem;
    padding: 0.6rem 0.8rem;
    margin: 0 0 0.75rem;
    border: 1px solid var(--color-border);
    border-left: 3px solid var(--color-accent, var(--color-border));
    border-radius: 4px;
    background: var(--color-surface);
  }

  .moved-notice.line {
    border: none;
    border-left: none;
    background: none;
    padding: 0;
    justify-content: center;
  }

  .moved-line {
    margin: 0;
    font-size: 0.9rem;
  }

  .moved-card-link {
    font-size: 0.85rem;
  }
</style>
