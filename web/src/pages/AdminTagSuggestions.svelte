<script>
  /**
   * The suggested-tag queue (docs/adr/114), a Review section of the admin
   * panel (docs/adr/118). It used to sit at the top of the Tags page;
   * deciding a word is a queue's work, and curating the vocabulary is a
   * setting's, so the two now live under different tabs and link to each
   * other.
   *
   * Approving may rename, and a rename may merge onto a word that already
   * exists, so the name is an editable field rather than a fixed label.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { loadTags } from '../stores/quilt.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  let suggestions = $state([]);
  let editedNames = $state({});
  let loading = $state(true);
  let error = $state('');
  let deciding = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      suggestions = await api('admin/tag-suggestions');
      editedNames = Object.fromEntries(suggestions.map(s => [s.id, s.name]));
    } catch (e) {
      error = e.message;
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

  function handleNav(e, path) {
    e.preventDefault();
    navigate(path);
  }

  $effect(() => { load(); });
</script>

<div class="admin-tag-suggestions">
  <h1>Suggested tags</h1>
  <p class="muted intro">
    Words patch admins asked for. A suggestion sits privately on the
    patch that asked until you add it to the vocabulary. Approving
    publishes it on every patch waiting on it; declining removes it from
    them and spends the word, though you can still add it yourself later
    under
    <a href="/admin/settings/tags" onclick={(e) => handleNav(e, '/admin/settings/tags')}>Settings</a>.
  </p>

  {#if loading}
    <Skeleton rows={3} />
  {:else if error}
    <ErrorState message={error} onRetry={load} />
  {:else if suggestions.length === 0}
    <p class="muted">No suggested tags waiting.</p>
  {:else}
    <ul class="suggestion-list">
      {#each suggestions as s (s.id)}
        <li class="suggestion-row card">
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
</div>

<style>
  .admin-tag-suggestions {
    max-width: var(--pw-measure);
  }

  h1 {
    margin-bottom: 0.25rem;
  }

  .intro {
    font-size: 0.85rem;
    margin-bottom: 1.25rem;
  }

  .suggestion-list {
    list-style: none;
    padding: 0;
    margin: 0;
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
</style>
