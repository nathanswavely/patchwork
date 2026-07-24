<script>
  import { api } from '../lib/api.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let entries = $state([]);
  let loading = $state(true);
  let error = $state('');
  let nextCursor = $state('');
  let actionFilter = $state('');
  let userSearch = $state('');
  let dateFrom = $state('');
  let dateTo = $state('');
  let expandedIds = $state(new Set());
  let dateRangeWarning = $derived(
    dateFrom && dateTo && dateFrom > dateTo
      ? 'The "From" date must be before the "To" date.'
      : ''
  );

  $effect(() => {
    void actionFilter;
    void userSearch;
    void dateFrom;
    void dateTo;
    loadEntries();
  });

  async function loadEntries(append = false) {
    if (!append) {
      loading = true;
      entries = [];
      nextCursor = '';
    }
    error = '';
    try {
      const params = new URLSearchParams();
      if (actionFilter) params.set('action', actionFilter);
      if (userSearch) params.set('user_id', userSearch);
      if (dateFrom) params.set('from', dateFrom);
      if (dateTo) params.set('to', dateTo);
      if (append && nextCursor) params.set('after', nextCursor);
      const data = await api(`admin/audit-log?${params}`);
      if (append) {
        entries = [...entries, ...data.items];
      } else {
        entries = data.items;
      }
      nextCursor = data.next_cursor || '';
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  function toggleExpand(id) {
    const next = new Set(expandedIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    expandedIds = next;
  }

  function formatTimestamp(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleString(undefined, {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }

  function formatMetadata(meta) {
    try {
      return JSON.stringify(JSON.parse(meta), null, 2);
    } catch {
      return meta;
    }
  }

  function handleApplyFilters() {
    loadEntries();
  }
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>Audit Log</h1>
  </div>

  <div class="filters">
    <div class="filter-row">
      <label class="filter-field">
        <span class="filter-label">Action</span>
        <select bind:value={actionFilter}>
          <option value="">All</option>
          <option value="node.create">node.create</option>
          <option value="node.update">node.update</option>
          <option value="node.delete">node.delete</option>
          <option value="event.create">event.create</option>
          <option value="event.update">event.update</option>
          <option value="event.delete">event.delete</option>
          <option value="report.create">report.create</option>
          <option value="report.resolve">report.resolve</option>
          <option value="admin.user_update">admin.user_update</option>
          <option value="membership.join">membership.join</option>
          <option value="auth.login">auth.login</option>
        </select>
      </label>
      <label class="filter-field">
        <span class="filter-label">User</span>
        <input type="text" placeholder="Search by user ID..." bind:value={userSearch} />
      </label>
      <label class="filter-field">
        <span class="filter-label">From</span>
        <input type="date" bind:value={dateFrom} />
      </label>
      <label class="filter-field">
        <span class="filter-label">To</span>
        <input type="date" bind:value={dateTo} />
      </label>
    </div>
    {#if dateRangeWarning}
      <p class="date-warning">{dateRangeWarning}</p>
    {/if}
  </div>

  {#if loading}
    <Skeleton lines={6} />
  {:else if error}
    <ErrorState message={error} retry={() => loadEntries()} />
  {:else if entries.length === 0}
    <p class="muted" style="text-align: center; padding: 2rem 0;">No audit entries found.</p>
  {:else}
    <div class="table-wrapper">
      <table class="data-table">
        <thead>
          <tr>
            <th>Time</th>
            <th>User</th>
            <th>Action</th>
            <th>Target</th>
            <th>Details</th>
          </tr>
        </thead>
        <tbody>
          {#each entries as entry (entry.id)}
            <tr>
              <td class="muted nowrap">{formatTimestamp(entry.created_at)}</td>
              <td>{entry.display_name || entry.username || entry.user_id || '--'}</td>
              <td><span class="badge">{entry.action}</span></td>
              <td class="muted">{entry.entity_type}/{entry.entity_id?.substring(0, 8)}...</td>
              <td>
                {#if entry.metadata && entry.metadata !== '{}'}
                  <button class="expand-btn" onclick={() => toggleExpand(entry.id)}>
                    {expandedIds.has(entry.id) ? 'Hide' : 'Show'}
                  </button>
                  {#if expandedIds.has(entry.id)}
                    <pre class="metadata-json">{formatMetadata(entry.metadata)}</pre>
                  {/if}
                {:else}
                  <span class="muted">--</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if nextCursor}
      <div style="text-align: center; padding: 1rem 0;">
        <button class="btn btn-secondary" onclick={() => loadEntries(true)}>Load More</button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .page-header {
    padding: 1.5rem 0 1rem;
  }

  .filters {
    margin-bottom: 1rem;
    padding: 0.75rem 1rem;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
  }

  .filter-row {
    display: flex;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .filter-field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .filter-label {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .filter-field select,
  .filter-field input {
    padding: 0.35rem 0.5rem;
    font-size: 0.85rem;
  }

  .date-warning {
    margin: 0.5rem 0 0;
    font-size: 0.8rem;
    color: var(--color-error);
    font-weight: 500;
  }

  .table-wrapper {
    overflow-x: auto;
  }

  .data-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }

  .data-table th,
  .data-table td {
    padding: 0.5rem 0.6rem;
    text-align: left;
    border-bottom: 1px solid var(--color-border);
    vertical-align: top;
  }

  .data-table th {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .nowrap {
    white-space: nowrap;
  }

  .expand-btn {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.8rem;
    padding: 0;
    cursor: pointer;
    text-decoration: underline;
  }

  .metadata-json {
    margin-top: 0.4rem;
    padding: 0.5rem;
    background: var(--color-bg);
    border-radius: var(--radius);
    font-size: 0.75rem;
    overflow-x: auto;
    white-space: pre-wrap;
    word-break: break-all;
  }
</style>
