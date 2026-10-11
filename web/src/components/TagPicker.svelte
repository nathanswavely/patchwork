<script>
  /**
   * Tag picker — a patch admin picks tags from the instance-curated
   * vocabulary (docs/adr/021). Order matters: the list order is the stored
   * priority order, and the first motif-bearing tag derives the patch's
   * motif when no explicit motif is chosen.
   *
   * A word that is not in the vocabulary can be suggested (docs/adr/114).
   * Suggestions are kept in their own array rather than mixed into the
   * picked ones, because they travel on their own request field: the server
   * still rejects an unknown name in `tags`, so a typo stays a typo instead
   * of quietly coining vocabulary.
   *
   * `pending` are suggestions already awaiting review on this patch. They are
   * shown so an admin can see what they asked for, and withdrawn through
   * `onWithdraw` rather than by editing the array, because taking a
   * suggestion back is an explicit act.
   */
  import { onMount } from 'svelte';
  import { getTagVocabulary, loadTags } from '../stores/quilt.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { MOTIFS } from '../lib/patchIcons.js';
  import { X, Plus } from 'phosphor-svelte';

  let {
    selected = $bindable([]),
    suggested = $bindable([]),
    pending = [],
    onWithdraw = null,
    disabled = false,
  } = $props();

  onMount(() => {
    if (getTagVocabulary().length === 0) loadTags();
  });

  let vocabulary = $derived(getTagVocabulary());
  let available = $derived(vocabulary.filter(t => !selected.includes(t.name)));

  let draft = $state('');
  let draftError = $state('');

  /**
   * Mirrors normalizeTagName in internal/handler/tags.go. The server
   * normalizes regardless; doing it here means the chip says what will
   * actually be saved.
   */
  function normalize(raw) {
    return (raw || '')
      .normalize('NFC')
      .trim()
      .replace(/[\s_-]+/g, '-')
      .replace(/[^\p{L}\p{N}-]/gu, '')
      .toLowerCase()
      .replace(/^-+|-+$/g, '')
      .slice(0, 32)
      .replace(/-+$/g, '');
  }

  function add(name) {
    if (disabled || selected.includes(name)) return;
    selected = [...selected, name];
  }

  function remove(name) {
    if (disabled) return;
    selected = selected.filter(t => t !== name);
  }

  function addSuggestion() {
    if (disabled) return;
    const name = normalize(draft);
    draftError = '';
    if (!name) {
      draftError = 'Enter a word to suggest.';
      return;
    }
    const known = vocabulary.find(t => t.name === name);
    if (known) {
      // Already in the vocabulary, so it is an ordinary pick and never
      // reaches the queue.
      add(known.name);
      draft = '';
      return;
    }
    if (pending.includes(name) || suggested.includes(name)) {
      draftError = `${name} is already waiting for review.`;
      return;
    }
    suggested = [...suggested, name];
    draft = '';
  }

  function removeSuggestion(name) {
    if (disabled) return;
    suggested = suggested.filter(t => t !== name);
  }

  function motifFor(name) {
    const tag = vocabulary.find(t => t.name === name);
    return tag?.motif && MOTIFS[tag.motif] ? MOTIFS[tag.motif] : null;
  }
</script>

<div class="tag-picker">
  {#if selected.length > 0}
    <div class="lt-chips selected-chips" role="list" aria-label="Selected tags, in priority order">
      {#each selected as name (name)}
        {@const color = colorForTag(name)}
        {@const motif = motifFor(name)}
        <span class="lt-chip" role="listitem" style="background: {color}; color: {textOnColor(color)};">
          {#if motif}
            {@const MotifIcon = motif.component}
            <MotifIcon size={12} weight="fill" />
          {/if}
          {name}
          <button
            type="button"
            class="lt-chip-remove"
            onclick={() => remove(name)}
            {disabled}
            title="Remove"
            aria-label="Remove {name}"
          ><X size={12} weight="bold" /></button>
        </span>
      {/each}
    </div>
  {/if}

  {#if pending.length > 0 || suggested.length > 0}
    <div class="lt-chips" role="list" aria-label="Suggested tags awaiting review">
      {#each pending as name (name)}
        <span class="lt-chip lt-chip-pending" role="listitem">
          {name}
          <span class="pending-note">waiting</span>
          {#if onWithdraw}
            <button
              type="button"
              class="lt-chip-remove"
              onclick={() => onWithdraw(name)}
              {disabled}
              title="Withdraw"
              aria-label="Withdraw {name}"
            ><X size={12} weight="bold" /></button>
          {/if}
        </span>
      {/each}
      {#each suggested as name (name)}
        <span class="lt-chip lt-chip-pending" role="listitem">
          {name}
          <span class="pending-note">to suggest</span>
          <button
            type="button"
            class="lt-chip-remove"
            onclick={() => removeSuggestion(name)}
            {disabled}
            title="Remove"
            aria-label="Remove {name}"
          ><X size={12} weight="bold" /></button>
        </span>
      {/each}
    </div>
  {/if}

  {#if available.length > 0}
    <div class="lt-chips" role="list" aria-label="Available tags">
      {#each available as tag (tag.name)}
        <button
          type="button"
          class="lt-chip"
          role="listitem"
          onclick={() => add(tag.name)}
          {disabled}
        >
          {tag.name}
        </button>
      {/each}
    </div>
  {:else if vocabulary.length === 0}
    <p class="lt-muted empty-hint">No tags yet. The instance admin curates the list.</p>
  {/if}

  <div class="suggest-row">
    <input
      type="text"
      placeholder="Suggest a tag"
      bind:value={draft}
      {disabled}
      maxlength="32"
      aria-label="Suggest a tag that is not on the list"
      onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addSuggestion(); } }}
    />
    <button
      type="button"
      class="lt-btn lt-btn-secondary lt-btn-sm"
      onclick={addSuggestion}
      disabled={disabled || !draft.trim()}
    ><Plus size={12} weight="bold" /> Suggest</button>
  </div>
  {#if draftError}
    <p class="lt-muted suggest-hint error-hint">{draftError}</p>
  {:else}
    <p class="lt-muted suggest-hint">
      A suggested tag stays on this patch privately until an admin adds it to
      the quilt's tags.
    </p>
  {/if}
</div>

<style>
  .tag-picker {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .selected-chips .lt-chip {
    border-color: transparent;
    cursor: default;
    font-weight: 500;
  }

  .selected-chips {
    padding-bottom: 0.35rem;
    border-bottom: 1px dashed var(--color-border);
  }

  /* Dashed and unfilled, the same way an unconfirmed map marker is drawn:
     provisional, not yet a fact about the patch. */

  .pending-note {
    font-size: 0.68rem;
    opacity: 0.75;
    font-style: italic;
  }

  .empty-hint {
    font-size: 0.8rem;
  }

  .suggest-row {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .suggest-row input {
    flex: 1;
    min-width: 0;
  }

  .suggest-hint {
    font-size: 0.75rem;
    margin: 0;
  }

  .error-hint {
    color: var(--color-error);
  }
</style>
