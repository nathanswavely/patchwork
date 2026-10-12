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

  // A target as far as this log knows it: its type and the tail of its id.
  // The tail, because ids are UUIDv7 and begin with a timestamp, so every
  // row written in the same stretch shared the same first eight characters
  // and the column told them apart not at all.
  function formatTarget(entry) {
    if (!entry.entity_id) return entry.entity_type || '--';
    return `${entry.entity_type}/…${entry.entity_id.slice(-8)}`;
  }

  function handleApplyFilters() {
    loadEntries();
  }
</script>

{#snippet details(entry)}
  {#if entry.metadata && entry.metadata !== '{}'}
    <button class="lt-btn-link expand-btn" onclick={() => toggleExpand(entry.id)}>
      {expandedIds.has(entry.id) ? 'Hide' : 'Show'}
    </button>
    {#if expandedIds.has(entry.id)}
      <pre class="metadata-json">{formatMetadata(entry.metadata)}</pre>
    {/if}
  {:else}
    <span class="lt-muted">--</span>
  {/if}
{/snippet}

<div class="lt-fade-in">
  <div class="page-header">
    <h1>Audit log</h1>
  </div>

  <div class="filters">
    <div class="filter-row">
      <label class="filter-field">
        <span class="filter-label">Action</span>
        <select class="lt-field" data-density="compact" bind:value={actionFilter}>
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
          <option value="admin.user_email_set">admin.user_email_set</option>
          <option value="membership.join">membership.join</option>
          <option value="auth.login">auth.login</option>
        </select>
      </label>
      <label class="filter-field">
        <span class="filter-label">User</span>
        <input class="lt-field" data-density="compact" type="text" placeholder="Search by user ID..." bind:value={userSearch} />
      </label>
      <label class="filter-field">
        <span class="filter-label">From</span>
        <input class="lt-field" data-density="compact" type="date" bind:value={dateFrom} />
      </label>
      <label class="filter-field">
        <span class="filter-label">To</span>
        <input class="lt-field" data-density="compact" type="date" bind:value={dateTo} />
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
    <p class="lt-muted" style="text-align: center; padding: 2rem 0;">No audit entries found.</p>
  {:else}
    <!-- Liquid Textile's stacked table: in a box under 40rem (a phone)
         each entry reads top to bottom, a label beside each value. As a
         table the details button sat 660px to the right, off the screen. -->
    <div class="lt-table-wrap">
      <table class="lt-table lt-table-stack">
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
              <td class="lt-muted nowrap" data-label="Time">{formatTimestamp(entry.created_at)}</td>
              <td data-label="User">{entry.display_name || entry.username || entry.user_id || '--'}</td>
              <td data-label="Action"><span class="lt-tag">{entry.action}</span></td>
              <td class="lt-muted" data-label="Target">{formatTarget(entry)}</td>
              <td data-label="Details">{@render details(entry)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if nextCursor}
      <div style="text-align: center; padding: 1rem 0;">
        <button class="lt-btn lt-btn-secondary" onclick={() => loadEntries(true)}>Load more</button>
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

  .date-warning {
    margin: 0.5rem 0 0;
    font-size: 0.8rem;
    color: var(--color-error);
    font-weight: 500;
  }

  /* The audit log's own denser reading of Liquid Textile's table: rows
     are scanned in bulk, and an expanded metadata block has to sit under
     its own row. The cell padding only while it is a table; stacked, the
     cells take the stack's. */
  .lt-table {
    font-size: 0.85rem;
  }

  .lt-table td {
    vertical-align: top;
  }

  @container (width > 40rem) {
    .lt-table th,
    .lt-table td {
      padding: 0.5rem 0.6rem;
    }
  }

  .lt-table th {
    font-size: 0.75rem;
  }

  .nowrap {
    white-space: nowrap;
  }

  /* Liquid Textile's text button: its target reaches --lt-target with
     padding the negative margin hands back, so the row keeps its height. */
  .expand-btn {
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

  /* The four filters two by two, instead of four full-width rows above
     the first entry. */
  @media (max-width: 640px) {
    .filters {
      padding: 0.75rem;
    }

    .filter-row {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 0.6rem;
    }

    .filter-field {
      min-width: 0;
    }
  }

  @media (pointer: coarse) {
    .expand-btn {
      padding-inline: 0.25rem;
    }
  }
</style>
