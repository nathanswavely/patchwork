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
   *
   * The new-word field appears only where a word will actually go somewhere,
   * and the caller says where:
   *
   *   onSuggest         sends it now (Patch Settings). Suggesting there used
   *                     to wait for Save tags, so a word suggested and then
   *                     left behind never reached the queue, while its chip
   *                     already read as waiting on an admin.
   *   deferSuggestions  collects it in `suggested` for a form that sends it
   *                     on submit (patch creation).
   *
   * An instance admin curates the vocabulary, so for them the field creates
   * the tag outright and picks it, rather than asking themselves and then
   * approving their own request.
   */
  import { onMount } from 'svelte';
  import { getTagVocabulary, loadTags } from '../stores/quilt.svelte.js';
  import { isAdmin } from '../stores/auth.svelte.js';
  import { api } from '../lib/api.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { MOTIFS } from '../lib/patchIcons.js';
  import { X, Plus } from 'phosphor-svelte';

  let {
    selected = $bindable([]),
    suggested = $bindable([]),
    pending = [],
    onWithdraw = null,
    onSuggest = null,
    deferSuggestions = false,
    disabled = false,
  } = $props();

  // An instance admin adds to the vocabulary; anyone else asks for a word.
  let creates = $derived(isAdmin());
  let showNewWord = $derived(creates || !!onSuggest || deferSuggestions);

  onMount(() => {
    if (getTagVocabulary().length === 0) loadTags();
  });

  let vocabulary = $derived(getTagVocabulary());
  let available = $derived(vocabulary.filter(t => !selected.includes(t.name)));

  let draft = $state('');
  let draftError = $state('');
  let sending = $state(false);

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

  async function addNewWord() {
    if (disabled || sending) return;
    const name = normalize(draft);
    draftError = '';
    if (!name) {
      draftError = creates ? 'Enter a tag name.' : 'Enter a word to suggest.';
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
    if (creates) {
      sending = true;
      try {
        // CreateTag approves a pending or declined row of the same name
        // rather than colliding, so this also settles anyone else's
        // suggestion of the word.
        const tag = await api('admin/tags', { method: 'POST', body: { name } });
        await loadTags();
        add(tag?.name || name);
        draft = '';
      } catch (e) {
        draftError = e.message || 'Failed to add tag';
      } finally {
        sending = false;
      }
      return;
    }
    if (pending.includes(name) || suggested.includes(name)) {
      draftError = `${name} is already waiting for review.`;
      return;
    }
    if (onSuggest) {
      sending = true;
      try {
        await onSuggest(name);
        draft = '';
      } catch (e) {
        draftError = e.message || 'Failed to suggest tag';
      } finally {
        sending = false;
      }
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

  {#if showNewWord}
    <div class="suggest-row">
      <input
        class="lt-field"
        type="text"
        placeholder={creates ? 'New tag' : 'Suggest a tag'}
        bind:value={draft}
        disabled={disabled || sending}
        maxlength="32"
        aria-label={creates ? 'Add a new tag to the quilt' : 'Suggest a tag that is not on the list'}
        onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addNewWord(); } }}
      />
      <button
        type="button"
        class="lt-btn lt-btn-secondary lt-btn-sm"
        onclick={addNewWord}
        disabled={disabled || sending || !draft.trim()}
      ><Plus size={12} weight="bold" /> {creates ? 'Add tag' : 'Suggest'}</button>
    </div>
    {#if draftError}
      <p class="lt-muted suggest-hint error-hint">{draftError}</p>
    {:else if creates}
      <p class="lt-muted suggest-hint">A tag added here joins the quilt's tags right away.</p>
    {:else if onSuggest}
      <p class="lt-muted suggest-hint">
        A suggestion is sent for review right away. Until an admin adds it to
        the quilt's tags, only this patch's admins see it.
      </p>
    {:else}
      <p class="lt-muted suggest-hint">
        Suggestions are sent for review with this form. Until an admin adds
        one to the quilt's tags, only this patch's admins see it.
      </p>
    {/if}
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
