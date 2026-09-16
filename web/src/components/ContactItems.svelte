<script>
  /**
   * The shared items, rendered once (docs/adr/083 decision 7 — one rendering
   * of a person). The person card and the profile both show the same items to
   * the same audience; they differ only in how much room they have, which is
   * a density setting and not a second rendering.
   *
   * The kind leads, because what a value *is* matters before what it says — a
   * bare string of digits reads as nothing. It leads as an icon *and* a word:
   * the icon alone was sr-only-worded in the card, which meant a sighted
   * low-vision reader on a touch device got a 14px glyph and no name for it.
   *
   * This component never says which patch an item came through. The granting
   * membership may be private or hidden, and docs/adr/006 keeps those off the
   * profile — so nothing here takes a patch, and there is no prop to pass one.
   * The server decided who may see these; this only draws them.
   */
  import { CONTACT_KIND_WORD, CONTACT_KIND_MARK, contactHref } from '../lib/contactItems.js';

  let { items = [], compact = false } = $props();
</script>

<ul class="contact-items" class:compact>
  {#each items as item (item.id)}
    {@const Mark = CONTACT_KIND_MARK[item.kind]}
    <li>
      <span class="contact-kind muted">
        {#if Mark}<Mark size={14} aria-hidden="true" />{/if}
        <span>{CONTACT_KIND_WORD[item.kind] || item.kind}</span>
      </span>
      {#if contactHref(item)}
        <a class="contact-value" href={contactHref(item)}>{item.value}</a>
      {:else}
        <span class="contact-value">{item.value}</span>
      {/if}
      {#if item.label}<span class="muted">{' · '}{item.label}</span>{/if}
    </li>
  {/each}
</ul>

<style>
  .contact-items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .contact-items li {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .contact-kind {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.85rem;
    /* A column at full size, so values line up down the page. The card has no
       width to spend on one. */
    min-width: 5.5rem;
  }

  .contact-value {
    word-break: break-word;
  }

  .contact-items.compact {
    gap: 0.3rem;
    font-size: 0.9rem;
  }

  .contact-items.compact .contact-kind {
    min-width: 0;
    font-size: 0.8rem;
  }
</style>
