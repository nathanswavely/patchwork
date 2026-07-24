<script>
  // Archived patches (docs/adr/034): the only way back from archived.
  // Restore is instance-admin-only and silent — the patch reappearing in
  // the quilt is the announcement. Rejected submissions never show here.
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  let nodes = $state([]);
  let loading = $state(true);

  async function load() {
    loading = true;
    try {
      const data = await api('admin/nodes?status=archived');
      nodes = data.nodes || [];
    } catch {
      nodes = [];
    } finally {
      loading = false;
    }
  }

  async function restore(node) {
    try {
      const res = await api(`admin/nodes/${node.id}/restore`, { method: 'POST' });
      showToast(`${node.name} restored`);
      nodes = nodes.filter((n) => n.id !== node.id);
      return res;
    } catch (e) {
      showToast(e.data?.error || 'Failed to restore patch', 'error');
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }

  $effect(() => { load(); });
</script>

<div class="admin-page">
  <h1>Archived Patches</h1>
  <p class="page-desc">
    Archived patches are hidden everywhere but nothing is erased. Members,
    events, and history stay intact. Restoring returns a patch to what it was
    before, without notifying anyone.
  </p>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if nodes.length === 0}
    <div class="empty-state">
      <p>No archived patches.</p>
    </div>
  {:else}
    <div class="node-list">
      {#each nodes as node (node.id)}
        <div class="node-item card">
          <div class="node-info">
            <span class="node-name">{node.name}</span>
            <span class="node-meta">
              /{node.slug}
              · archived {formatDate(node.archived_at)}
              · {node.member_count} {node.member_count === 1 ? 'member' : 'members'},
              {node.follower_count} {node.follower_count === 1 ? 'follower' : 'followers'}
              {#if node.restores_to === 'unclaimed'}
                · restores as unclaimed
              {/if}
            </span>
          </div>
          <ConfirmAction
            label="Restore"
            confirmLabel="Yes, restore this patch"
            onConfirm={() => restore(node)}
          />
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

  .node-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .node-item {
    display: flex;
    align-items: center;
    gap: 0.9rem;
  }

  .node-info {
    flex: 1;
    min-width: 0;
  }

  .node-name {
    display: block;
    font-weight: 700;
    font-size: 0.9rem;
  }

  .node-meta {
    display: block;
    font-size: 0.82rem;
    color: var(--color-text-muted);
  }

  .empty-state {
    text-align: center;
    padding: 2rem 1rem;
    color: var(--color-text-muted);
  }
</style>
