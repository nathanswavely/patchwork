<script>
  import { getContext, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  let sources = $state([]);
  let loading = $state(true);
  let newUrl = $state('');
  let adding = $state(false);
  let syncing = $state({});
  let pollTimer = null;

  $effect(() => {
    if (slug) loadSources();
  });

  onDestroy(() => clearTimeout(pollTimer));

  async function loadSources() {
    loading = true;
    try {
      const data = await api(`nodes/${slug}/event-sources`);
      sources = data.items || [];
      schedulePoll();
    } catch (e) {
      showToast('Failed to load event sources', 'error');
    } finally {
      loading = false;
    }
  }

  // A just-attached source syncs in the background; poll until it
  // settles so the row's status updates without a manual refresh.
  function schedulePoll() {
    clearTimeout(pollTimer);
    if (sources.some((s) => s.status === 'pending')) {
      pollTimer = setTimeout(async () => {
        try {
          const data = await api(`nodes/${slug}/event-sources`);
          sources = data.items || [];
        } catch (e) {
          // Next poll or manual action will surface it.
        }
        schedulePoll();
      }, 2000);
    }
  }

  async function addSource(e) {
    e.preventDefault();
    if (!newUrl.trim()) return;
    adding = true;
    try {
      await api(`nodes/${slug}/event-sources`, { method: 'POST', body: { url: newUrl.trim() } });
      newUrl = '';
      await loadSources();
    } catch (err) {
      showToast(err.message || 'Failed to attach feed', 'error');
    } finally {
      adding = false;
    }
  }

  async function removeSource(id) {
    try {
      await api(`nodes/${slug}/event-sources/${id}`, { method: 'DELETE' });
      showToast('Event source removed', 'info');
      await loadSources();
    } catch (e) {
      showToast('Failed to remove event source', 'error');
    }
  }

  async function syncNow(id) {
    syncing = { ...syncing, [id]: true };
    try {
      const updated = await api(`nodes/${slug}/event-sources/${id}/sync`, { method: 'POST' });
      sources = sources.map((s) => (s.id === id ? updated : s));
      if (updated.status === 'ok') {
        showToast('Synced', 'info');
      }
    } catch (err) {
      showToast(err.message || 'Sync failed', 'error');
      loadSources();
    } finally {
      syncing = { ...syncing, [id]: false };
    }
  }

  function hostOf(url) {
    try {
      return new URL(url).host;
    } catch (e) {
      return url;
    }
  }

  function relTime(iso) {
    if (!iso) return null;
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return null;
    const mins = Math.round((Date.now() - then) / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.round(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.round(hours / 24)}d ago`;
  }
</script>

<div class="page-fade">
  <h2>Event Sources</h2>
  <p class="muted subtitle">
    Calendar feeds this patch pulls events from. Paste an ICS address — a
    Google Calendar's secret address, the calendar feed from your website —
    or an events page from Squarespace, Humanitix, and similar sites.
    Imported events publish directly and stay in step with the feed.
  </p>

  {#if loading}
    <p class="muted" style="padding: 2rem 0;">Loading...</p>
  {:else}
    {#if sources.length > 0}
      <ul class="source-list">
        {#each sources as source (source.id)}
          <li class="source-row">
            <div class="source-info">
              <span class="source-host">{hostOf(source.url)}</span>
              <span class="source-url muted">{source.url}</span>
              <span class="source-status">
                {#if source.status === 'pending'}
                  <span class="muted">First sync in progress…</span>
                {:else if source.status === 'error'}
                  <span class="status-error">Sync failed{source.last_error ? `: ${source.last_error}` : ''}</span>
                {:else}
                  <span class="muted">
                    {source.event_count} {source.event_count === 1 ? 'event' : 'events'}
                    {#if relTime(source.last_success_at)}· synced {relTime(source.last_success_at)}{/if}
                  </span>
                {/if}
              </span>
            </div>
            <div class="source-actions">
              <button
                class="btn-small"
                disabled={syncing[source.id] || source.status === 'pending'}
                onclick={() => syncNow(source.id)}
              >{syncing[source.id] ? 'Syncing…' : 'Sync now'}</button>
              <ConfirmAction
                label="Remove"
                confirmLabel="Remove — future imported events go with it"
                variant="danger"
                onConfirm={() => removeSource(source.id)}
              />
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="muted empty">No event sources yet.</p>
    {/if}

    <form class="add-form" onsubmit={addSource}>
      <input
        type="url"
        placeholder="https://calendar.google.com/calendar/ical/…/basic.ics"
        bind:value={newUrl}
        disabled={adding}
      />
      <button class="btn-primary" type="submit" disabled={adding || !newUrl.trim()}>
        {adding ? 'Attaching…' : 'Attach feed'}
      </button>
    </form>
    <p class="muted hint">
      Removing a source keeps past events and removes upcoming imported ones.
    </p>
  {/if}
</div>

<style>
  h2 {
    font-size: 1.2rem;
    margin-bottom: 0.25rem;
  }

  .subtitle {
    font-size: 0.85rem;
    margin-bottom: 1.5rem;
  }

  .source-list {
    list-style: none;
    padding: 0;
    margin: 0 0 1.5rem;
  }

  .source-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .source-info {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .source-host {
    font-size: 0.92rem;
    font-weight: 500;
  }

  .source-url {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 340px;
  }

  .source-status {
    font-size: 0.8rem;
  }

  .status-error {
    color: var(--color-danger, #c0392b);
  }

  .source-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .btn-small {
    font-size: 0.8rem;
    padding: 0.3rem 0.6rem;
  }

  .add-form {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .add-form input {
    flex: 1;
    min-width: 0;
  }

  .hint {
    font-size: 0.78rem;
    margin-top: 0.5rem;
  }

  .empty {
    padding: 1rem 0;
  }
</style>
