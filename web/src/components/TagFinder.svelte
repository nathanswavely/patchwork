<script>
  /**
   * Every tag in the vocabulary, with a field to find one by typing: the
   * body of the filter sheet on a phone. (Desktop grows its chip row into a
   * card instead, FilterChips, with the same find rule from lib/tagFind.)
   * A vocabulary past a couple of dozen words is found by typing, not by
   * reading, so the field is always there.
   *
   * Toggling applies at once, like a chip in the row: the canvas behind is
   * how a person sees what a tag does, so there is no Apply step to wait on.
   * Tags stay in usage order (getRankedTags), the same order the row uses.
   */
  import { MagnifyingGlass } from 'phosphor-svelte';
  import {
    getRankedTags,
    getSelectedTags,
    toggleTag,
    resetFilters,
    getActiveFilterCount,
  } from '../stores/quilt.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { findTags } from '../lib/tagFind.js';

  let { autofocus = false } = $props();

  let find = $state('');
  let needle = $derived(find.trim());
  let shown = $derived(findTags(getRankedTags(), find));
  let count = $derived(getActiveFilterCount());

  function focusOnMount(node) {
    if (autofocus) node.focus();
  }

  // Enter takes the first match, so typing "cer" and pressing Enter is the
  // whole act. The field empties so the next word starts fresh.
  function onFindKeydown(e) {
    if (e.key !== 'Enter' || !needle || !shown.length) return;
    e.preventDefault();
    toggleTag(shown[0]);
    find = '';
  }
</script>

<div class="finder">
  <label class="finder-field">
    <MagnifyingGlass size={14} weight="bold" />
    <input
      class="lt-field"
      type="text"
      placeholder="Find a tag"
      aria-label="Find a tag"
      bind:value={find}
      onkeydown={onFindKeydown}
      use:focusOnMount
    />
  </label>

  {#if shown.length}
    <div class="finder-tags lt-scroll-thin" role="group" aria-label="All tags">
      {#each shown as tag (tag)}
        {@const active = getSelectedTags().includes(tag)}
        <button
          class="lt-chip lt-resin"
          style="--lt-resin-color: {colorForTag(tag)}; {active ? `background: ${colorForTag(tag)}; border-color: ${colorForTag(tag)}; color: ${textOnColor(colorForTag(tag))};` : `border-color: ${colorForTag(tag)}40;`}"
          aria-pressed={active}
          onclick={() => toggleTag(tag)}
        >{tag}</button>
      {/each}
    </div>
  {:else}
    <p class="lt-muted finder-none">No tag matches “{find.trim()}”.</p>
  {/if}

  {#if count > 0}
    <div class="finder-foot">
      <button class="finder-clear" onclick={resetFilters}>Clear</button>
    </div>
  {/if}
</div>

<style>
  /* Fills a bounded column (the popover, the sheet) and lets only the tag
     list scroll inside it. */
  .finder {
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-height: 0;
  }

  .finder-field {
    position: relative;
    display: flex;
    align-items: center;
    color: var(--color-text-muted);
  }

  .finder-field :global(svg) {
    position: absolute;
    left: 10px;
    pointer-events: none;
  }

  .finder-field input {
    width: 100%;
    padding-left: 30px;
  }

  .finder-tags {
    display: flex;
    flex-wrap: wrap;
    align-content: flex-start;
    gap: 6px;
    min-height: 0;
    overflow-y: auto;
  }

  .lt-chip {
    padding: 5px 12px;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--color-text);
    cursor: pointer;
    white-space: nowrap;
  }

  .finder-none {
    margin: 0;
    font-size: 0.85rem;
  }

  .finder-foot {
    display: flex;
    justify-content: flex-end;
  }

  .finder-clear {
    border: none;
    background: none;
    padding: 2px 6px;
    font-family: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-primary);
    cursor: pointer;
  }

  .finder-clear:hover {
    text-decoration: underline;
  }
</style>
