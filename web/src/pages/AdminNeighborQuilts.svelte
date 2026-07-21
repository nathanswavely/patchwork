<script>
  // Neighbor quilts (docs/adr/024): the instance's public statement of
  // adjacency, curated here, shown to every visitor in the quilt switcher.
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';

  let quilts = $state([]);
  let loading = $state(true);
  let newUrl = $state('');
  let busy = $state(false);
  let error = $state('');

  async function load() {
    loading = true;
    try {
      const data = await api('admin/neighbor-quilts');
      quilts = data.neighbor_quilts || [];
    } catch {
      quilts = [];
    } finally {
      loading = false;
    }
  }

  async function handleAdd() {
    if (!newUrl.trim() || busy) return;
    busy = true;
    error = '';
    try {
      // Validate reachability and capture the display name before saving.
      let name = '';
      try {
        const res = await fetch(`${newUrl.trim().replace(/\/+$/, '')}/api/v1/instance`);
        if (res.ok) name = (await res.json())?.name || '';
      } catch { /* unreachable is allowed — a neighbor can be pointed at anyway */ }
      await api('admin/neighbor-quilts', { method: 'POST', body: { url: newUrl.trim(), name } });
      showToast(name ? `Neighbored ${name}` : 'Neighbor quilt added');
      newUrl = '';
      await load();
    } catch (e) {
      error = e.data?.error || 'Failed to add neighbor quilt.';
    } finally {
      busy = false;
    }
  }

  async function handleRemove(quilt) {
    try {
      await api(`admin/neighbor-quilts/${quilt.id}`, { method: 'DELETE' });
      showToast('Neighbor removed');
      quilts = quilts.filter((q) => q.id !== quilt.id);
    } catch (e) {
      showToast(e.data?.error || 'Failed to remove', 'error');
    }
  }

  $effect(() => { load(); });
</script>

<div class="admin-page">
  <h1>Neighbor Quilts</h1>
  <p class="page-desc">
    Neighbor quilts appear in the quilt switcher for <strong>every</strong> visitor —
    including people without accounts. Neighboring is a public statement that
    your community stands beside theirs. People connect their own quilts
    privately on top of this list.
  </p>

  <form class="add-form" onsubmit={(e) => { e.preventDefault(); handleAdd(); }}>
    <input
      type="url"
      bind:value={newUrl}
      placeholder="https://other-patchwork.example.com"
      disabled={busy}
    />
    <button type="submit" class="btn btn-primary" disabled={busy || !newUrl.trim()}>
      {busy ? 'Adding…' : 'Add Neighbor'}
    </button>
  </form>
  {#if error}
    <p class="error-text">{error}</p>
  {/if}

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if quilts.length === 0}
    <div class="empty-state">
      <p>No neighbor quilts yet.</p>
      <p class="muted">Add another Patchwork's URL to publicly connect your communities.</p>
    </div>
  {:else}
    <div class="quilt-list">
      {#each quilts as quilt (quilt.id)}
        <div class="quilt-item card">
          <img
            class="quilt-icon"
            src={`${quilt.url}/api/v1/instance/icon`}
            alt=""
            width="32"
            height="32"
            loading="lazy"
            onerror={(e) => { e.target.style.display = 'none'; }}
          />
          <div class="quilt-info">
            {#if quilt.name}<span class="quilt-name">{quilt.name}</span>{/if}
            <span class="quilt-url">{quilt.url}</span>
          </div>
          <button class="btn btn-danger btn-sm" onclick={() => handleRemove(quilt)}>Remove</button>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .admin-page {
    max-width: 640px;
  }

  h1 {
    margin-bottom: 0.25rem;
  }

  .page-desc {
    color: var(--color-text-muted);
    margin-bottom: 1.5rem;
    line-height: 1.5;
  }

  .add-form {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .add-form input {
    flex: 1;
  }

  .quilt-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-top: 1rem;
  }

  .quilt-item {
    display: flex;
    align-items: center;
    gap: 0.9rem;
  }

  .quilt-icon {
    flex-shrink: 0;
    object-fit: cover;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
  }

  .quilt-info {
    flex: 1;
    min-width: 0;
  }

  .quilt-name {
    display: block;
    font-weight: 700;
    font-size: 0.9rem;
  }

  .quilt-url {
    display: block;
    font-family: monospace;
    font-size: 0.82rem;
    color: var(--color-text-muted);
    word-break: break-all;
  }

  .empty-state {
    text-align: center;
    padding: 2rem 1rem;
    color: var(--color-text-muted);
  }

  .error-text {
    color: var(--color-error);
    font-size: 0.85rem;
  }
</style>
