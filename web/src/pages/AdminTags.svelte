<script>
  /**
   * Admin vocabulary page (docs/adr/021): the instance-curated tag list.
   * Patch admins pick from this vocabulary; only instance admins change it.
   * Each tag optionally carries a motif — the mark patches wearing it show
   * when they chose no explicit motif.
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { loadTags } from '../stores/quilt.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { MOTIFS, MOTIF_KEYS } from '../lib/patchIcons.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  let tags = $state([]);
  let loading = $state(true);
  let error = $state('');

  // The suggested-tag queue (docs/adr/114). Approving may rename, and a
  // rename may merge onto a word that already exists, so the name is an
  // editable field rather than a fixed label.
  let suggestions = $state([]);
  let editedNames = $state({});
  let deciding = $state('');

  let newName = $state('');
  let creating = $state(false);

  // Tag id whose motif picker is open.
  let pickingFor = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      tags = await api('tags');
    } catch (e) {
      error = e.message;
    }
    try {
      suggestions = await api('admin/tag-suggestions');
      editedNames = Object.fromEntries(suggestions.map(s => [s.id, s.name]));
    } catch (e) {
      // The vocabulary still lists; a queue that fails to load is not a
      // reason to blank the page.
      suggestions = [];
    }
    loading = false;
  }

  async function decide(suggestion, action) {
    deciding = suggestion.id;
    try {
      const body = { action };
      const edited = (editedNames[suggestion.id] || '').trim();
      if (action === 'approve' && edited && edited !== suggestion.name) body.name = edited;
      await api(`admin/tag-suggestions/${suggestion.id}`, { method: 'PATCH', body });
      showToast(action === 'approve' ? 'Tag added to the vocabulary' : 'Suggestion declined', 'success');
      await load();
      loadTags();
    } catch (e) {
      showToast(e.message || 'Failed to decide', 'error');
    } finally {
      deciding = '';
    }
  }

  $effect(() => { load(); });

  async function createTag() {
    const name = newName.trim().toLowerCase();
    if (!name) return;
    creating = true;
    try {
      await api('admin/tags', { method: 'POST', body: { name } });
      newName = '';
      showToast('Tag created', 'success');
      await load();
      loadTags(); // refresh the shared vocabulary (pickers, motif resolver)
    } catch (e) {
      showToast(e.message || 'Failed to create tag', 'error');
    } finally {
      creating = false;
    }
  }

  async function setMotif(tag, motifKey) {
    try {
      await api(`admin/tags/${tag.id}`, { method: 'PATCH', body: { motif: motifKey } });
      pickingFor = '';
      showToast(motifKey ? 'Motif set' : 'Motif cleared', 'success');
      await load();
      loadTags();
    } catch (e) {
      showToast(e.message || 'Failed to update tag', 'error');
    }
  }

  async function deleteTag(tag) {
    try {
      await api(`admin/tags/${tag.id}`, { method: 'DELETE' });
      showToast('Tag deleted', 'success');
      await load();
      loadTags();
    } catch (e) {
      showToast(e.message || 'Failed to delete tag', 'error');
    }
  }
</script>

<div class="admin-tags">
  <h1>Tags</h1>
  <p class="muted intro">
    The vocabulary patch admins tag their patches with. Tags power discovery
    and pull patches with shared tags together on the quilt. A tag's motif
    is the mark shown for patches that chose no motif of their own.
    Deleting a tag removes it from every patch wearing it.
  </p>

  <!-- Always rendered, empty or not (docs/adr/114). A queue that disappears
       when nothing is in it leaves an admin no way to tell "nobody has asked"
       from "this instance does not have the feature", and those want opposite
       copy. -->
  <section class="suggestions">
    <h2>Suggested tags</h2>
    <p class="muted intro">
      Words patch admins asked for. A suggestion sits privately on the
      patch that asked until you add it here. Approving publishes it on
      every patch waiting on it; declining removes it from them and spends
      the word, though you can still add it yourself later.
    </p>
    {#if suggestions.length === 0}
      <p class="muted empty">Nothing waiting for review.</p>
    {:else}
    <ul class="suggestion-list">
        {#each suggestions as s (s.id)}
          <li class="suggestion-row">
            <div class="suggestion-main">
              <input
                class="suggestion-name"
                type="text"
                bind:value={editedNames[s.id]}
                maxlength="32"
                aria-label="Tag name, editable before approving"
                disabled={deciding === s.id}
              />
              <span class="muted suggestion-meta">
                {#if s.suggested_by?.username}
                  by {s.suggested_by.display_name || s.suggested_by.username}
                {:else}
                  by {s.suggested_by?.display_name || 'Deleted account'}
                {/if}
                {#if s.patches?.length}
                  {' · '}on {s.patches.map(p => p.name).join(', ')}
                {/if}
              </span>
            </div>
            <div class="suggestion-actions">
              <button
                class="btn btn-primary btn-sm"
                onclick={() => decide(s, 'approve')}
                disabled={deciding === s.id}
              >Approve</button>
              <ConfirmAction
                label="Decline"
                confirmLabel="Decline"
                variant="danger"
                onConfirm={() => decide(s, 'reject')}
              />
            </div>
          </li>
        {/each}
    </ul>
    {/if}
  </section>

  <form class="create-row" onsubmit={(e) => { e.preventDefault(); createTag(); }}>
    <input
      type="text"
      placeholder="New tag name (e.g. music)"
      bind:value={newName}
      disabled={creating}
    />
    <button type="submit" class="btn btn-primary" disabled={creating || !newName.trim()}>
      {creating ? 'Adding…' : 'Add tag'}
    </button>
  </form>

  {#if loading}
    <Skeleton rows={5} />
  {:else if error}
    <ErrorState message={error} onRetry={load} />
  {:else if tags.length === 0}
    <p class="muted">No tags yet. Add the first one above.</p>
  {:else}
    <ul class="tag-list">
      {#each tags as tag (tag.id)}
        {@const color = colorForTag(tag.name)}
        {@const motif = tag.motif && MOTIFS[tag.motif] ? MOTIFS[tag.motif] : null}
        <li class="tag-row">
          <span class="chip" style="background: {color}; color: {textOnColor(color)};">{tag.name}</span>
          <div class="motif-cell">
            {#if motif}
              {@const MotifIcon = motif.component}
              <button
                type="button"
                class="motif-current"
                onclick={() => { pickingFor = pickingFor === tag.id ? '' : tag.id; }}
                title="Change motif"
              >
                <MotifIcon size={16} weight="fill" />
                <span>{motif.name}</span>
              </button>
            {:else}
              <button
                type="button"
                class="motif-current none"
                onclick={() => { pickingFor = pickingFor === tag.id ? '' : tag.id; }}
              >
                No motif
              </button>
            {/if}
          </div>
          <ConfirmAction
            label="Delete"
            confirmLabel="Delete"
            variant="danger"
            onConfirm={() => deleteTag(tag)}
          />
        </li>
        {#if pickingFor === tag.id}
          <li class="motif-picker-row">
            <div class="motif-grid">
              {#each MOTIF_KEYS as key (key)}
                {@const m = MOTIFS[key]}
                {@const MotifIcon = m.component}
                <button
                  type="button"
                  class="motif-swatch"
                  class:selected={tag.motif === key}
                  onclick={() => setMotif(tag, key)}
                  title={m.name}
                  aria-label={m.name}
                >
                  <MotifIcon size={18} weight="fill" />
                </button>
              {/each}
            </div>
            {#if tag.motif}
              <button type="button" class="btn btn-secondary btn-sm" onclick={() => setMotif(tag, '')}>
                Clear motif
              </button>
            {/if}
          </li>
        {/if}
      {/each}
    </ul>
  {/if}
</div>

<style>
  .suggestions {
    margin: 1rem 0 1.5rem;
    padding: 0.85rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-surface);
  }

  .suggestions .empty {
    font-size: 0.85rem;
    margin: 0.5rem 0 0;
  }

  .suggestions h2 {
    font-size: 1rem;
    margin: 0 0 0.35rem;
  }

  .suggestion-list {
    list-style: none;
    padding: 0;
    margin: 0.75rem 0 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .suggestion-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
    justify-content: space-between;
  }

  .suggestion-main {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
    flex: 1;
  }

  .suggestion-name {
    max-width: 18rem;
    font-weight: 500;
  }

  .suggestion-meta {
    font-size: 0.75rem;
  }

  .suggestion-actions {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .admin-tags {
    max-width: var(--pw-measure);
  }

  h1 {
    margin-bottom: 0.25rem;
  }

  .intro {
    font-size: 0.85rem;
    margin-bottom: 1.25rem;
  }

  .create-row {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1.25rem;
  }

  .create-row input {
    flex: 1;
  }

  .tag-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .tag-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    padding: 0.22rem 0.55rem;
    border-radius: 999px;
    font-size: 0.78rem;
    font-weight: 500;
  }

  .motif-cell {
    flex: 1;
  }

  .motif-current {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    padding: 0.3rem 0.55rem;
    font-size: 0.8rem;
    cursor: pointer;
  }

  .motif-current:hover {
    border-color: var(--color-primary);
  }

  .motif-current.none {
    color: var(--color-text-muted);
    font-style: italic;
  }

  .motif-picker-row {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.6rem 0 0.8rem;
    border-bottom: 1px solid var(--color-border);
  }

  .motif-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
    gap: 0.35rem;
  }

  .motif-swatch {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.45rem;
    border: 2px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    cursor: pointer;
  }

  .motif-swatch:hover {
    border-color: var(--color-text-muted);
  }

  .motif-swatch.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
  }

  .btn-sm {
    align-self: flex-start;
  }
</style>
