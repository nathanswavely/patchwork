<script>
  /**
   * Tag picker — a patch admin picks tags from the instance-curated
   * vocabulary (docs/adr/021). Order matters: the list order is the stored
   * priority order, and the first motif-bearing tag derives the patch's
   * motif when no explicit motif is chosen.
   */
  import { onMount } from 'svelte';
  import { getTagVocabulary, loadTags } from '../stores/quilt.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { MOTIFS } from '../lib/patchIcons.js';
  import { CaretLeft, X } from 'phosphor-svelte';

  let { selected = $bindable([]), disabled = false } = $props();

  onMount(() => {
    if (getTagVocabulary().length === 0) loadTags();
  });

  let vocabulary = $derived(getTagVocabulary());
  let available = $derived(vocabulary.filter(t => !selected.includes(t.name)));

  function add(name) {
    if (disabled || selected.includes(name)) return;
    selected = [...selected, name];
  }

  function remove(name) {
    if (disabled) return;
    selected = selected.filter(t => t !== name);
  }

  function moveEarlier(index) {
    if (disabled || index === 0) return;
    const next = [...selected];
    [next[index - 1], next[index]] = [next[index], next[index - 1]];
    selected = next;
  }

  function motifFor(name) {
    const tag = vocabulary.find(t => t.name === name);
    return tag?.motif && MOTIFS[tag.motif] ? MOTIFS[tag.motif] : null;
  }
</script>

<div class="tag-picker">
  {#if selected.length > 0}
    <div class="chips selected-chips" role="list" aria-label="Selected tags, in priority order">
      {#each selected as name, i (name)}
        {@const color = colorForTag(name)}
        {@const motif = motifFor(name)}
        <span class="chip" role="listitem" style="background: {color}; color: {textOnColor(color)};">
          {#if i > 0}
            <button
              type="button"
              class="chip-btn"
              onclick={() => moveEarlier(i)}
              {disabled}
              title="Move earlier"
              aria-label="Move {name} earlier"
            ><CaretLeft size={12} weight="bold" /></button>
          {/if}
          {#if motif}
            {@const MotifIcon = motif.component}
            <MotifIcon size={12} weight="fill" />
          {/if}
          {name}
          <button
            type="button"
            class="chip-btn"
            onclick={() => remove(name)}
            {disabled}
            title="Remove"
            aria-label="Remove {name}"
          ><X size={12} weight="bold" /></button>
        </span>
      {/each}
    </div>
  {/if}

  {#if available.length > 0}
    <div class="chips" role="list" aria-label="Available tags">
      {#each available as tag (tag.name)}
        <button
          type="button"
          class="chip chip-available"
          role="listitem"
          onclick={() => add(tag.name)}
          {disabled}
        >
          {tag.name}
        </button>
      {/each}
    </div>
  {:else if vocabulary.length === 0}
    <p class="muted empty-hint">No tags yet — the instance admin curates the tag list.</p>
  {/if}
</div>

<style>
  .tag-picker {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .selected-chips {
    padding-bottom: 0.35rem;
    border-bottom: 1px dashed var(--color-border);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.22rem 0.55rem;
    border-radius: 999px;
    font-size: 0.78rem;
    font-weight: 500;
    border: none;
  }

  .chip-available {
    background: var(--color-surface);
    color: var(--color-text);
    border: 1px solid var(--color-border);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .chip-available:hover:not(:disabled) {
    border-color: var(--color-primary);
  }

  .chip-btn {
    display: inline-flex;
    align-items: center;
    border: none;
    background: none;
    color: inherit;
    padding: 0;
    cursor: pointer;
    opacity: 0.75;
  }

  .chip-btn:hover:not(:disabled) {
    opacity: 1;
  }

  .empty-hint {
    font-size: 0.8rem;
  }
</style>
