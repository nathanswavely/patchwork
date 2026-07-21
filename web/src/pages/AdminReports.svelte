<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  let reports = $state([]);
  let loading = $state(true);
  let error = $state('');
  let activeTab = $state('pending');
  let nextCursor = $state('');
  let resolvingId = $state(null);
  let resolutionNote = $state('');
  let selectedAction = $state('dismiss');

  const tabs = [
    { key: 'pending', label: 'Pending' },
    { key: 'reviewed', label: 'Reviewed' },
    { key: 'resolved', label: 'Resolved' },
    { key: 'dismissed', label: 'Dismissed' },
  ];

  $effect(() => {
    void activeTab;
    loadReports();
  });

  async function loadReports(append = false) {
    if (!append) {
      loading = true;
      reports = [];
      nextCursor = '';
    }
    error = '';
    try {
      const cursor = append ? nextCursor : '';
      const params = new URLSearchParams({ status: activeTab });
      if (cursor) params.set('after', cursor);
      const data = await api(`admin/reports?${params}`);
      if (append) {
        reports = [...reports, ...data.items];
      } else {
        reports = data.items;
      }
      nextCursor = data.next_cursor || '';
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  function openResolve(reportId) {
    resolvingId = reportId;
    resolutionNote = '';
    selectedAction = 'dismiss';
  }

  function cancelResolve() {
    resolvingId = null;
  }

  async function submitResolve() {
    if (!resolvingId) return;
    try {
      const statusMap = {
        dismiss: 'dismissed',
        warn: 'resolved',
        remove_content: 'resolved',
        suspend_user: 'resolved',
        reset_appearance: 'resolved',
      };
      await api(`admin/reports/${resolvingId}`, {
        method: 'PATCH',
        body: {
          status: statusMap[selectedAction],
          resolution_note: resolutionNote,
          action: selectedAction,
        },
      });
      showToast('Report resolved', 'success');
      resolvingId = null;
      loadReports();
    } catch (e) {
      showToast(e.message, 'error');
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>Reports Queue</h1>
  </div>

  <div class="tabs">
    {#each tabs as tab}
      <button
        class="tab-btn"
        class:active={activeTab === tab.key}
        onclick={() => { activeTab = tab.key; }}
      >
        {tab.label}
      </button>
    {/each}
  </div>

  {#if loading}
    <Skeleton lines={4} />
  {:else if error}
    <ErrorState message={error} retry={() => loadReports()} />
  {:else if reports.length === 0}
    <p class="muted" style="text-align: center; padding: 2rem 0;">No {activeTab} reports.</p>
  {:else}
    <div class="report-list">
      {#each reports as report (report.id)}
        <div class="card report-card">
          <div class="report-header">
            <span class="badge">{report.entity_type}</span>
            <span class="report-date muted">{formatDate(report.created_at)}</span>
          </div>
          <div class="report-body">
            <div class="report-meta">
              <span>Reported by <strong>{report.reporter_name || 'Unknown'}</strong></span>
              {#if report.target_name}
                <span class="muted"> &middot; Target: <strong>{report.target_name}</strong></span>
              {/if}
            </div>
            <p class="report-reason">{report.reason}</p>
            {#if report.details}
              <p class="report-details muted">{report.details}</p>
            {/if}
          </div>

          {#if activeTab === 'pending'}
            {#if resolvingId === report.id}
              <div class="resolve-form">
                <label class="form-label">
                  Action
                  <select bind:value={selectedAction}>
                    <option value="dismiss">Dismiss</option>
                    <option value="warn">Warn</option>
                    <option value="remove_content">Remove Content</option>
                    {#if report.entity_type === 'node'}
                      <option value="reset_appearance">Reset Appearance</option>
                    {/if}
                    <option value="suspend_user">Suspend User</option>
                  </select>
                </label>
                <label class="form-label">
                  Resolution Note
                  <textarea bind:value={resolutionNote} rows="2" placeholder="Optional note..."></textarea>
                </label>
                {#if selectedAction === 'suspend_user'}
                  <p class="warning-text">This will suspend the reported user's account.</p>
                {:else if selectedAction === 'reset_appearance'}
                  <p class="warning-text">
                    This clears the patch's chosen tile — the quilt decides its look again.
                    The patch itself is untouched.
                  </p>
                {/if}
                <div class="resolve-actions">
                  {#if selectedAction === 'suspend_user'}
                    <ConfirmAction
                      label="Confirm"
                      confirmLabel="Yes, suspend user"
                      variant="danger"
                      onConfirm={submitResolve}
                    />
                  {:else}
                    <button class="btn btn-primary" onclick={submitResolve}>Confirm</button>
                  {/if}
                  <button class="btn btn-secondary" onclick={cancelResolve}>Cancel</button>
                </div>
              </div>
            {:else}
              <div class="report-actions">
                <button class="btn btn-secondary" onclick={() => openResolve(report.id)}>Resolve</button>
              </div>
            {/if}
          {/if}

          {#if report.resolution_note}
            <div class="resolution-note muted">
              Resolution: {report.resolution_note}
            </div>
          {/if}
        </div>
      {/each}
    </div>

    {#if nextCursor}
      <div style="text-align: center; padding: 1rem 0;">
        <button class="btn btn-secondary" onclick={() => loadReports(true)}>Load More</button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .page-header {
    padding: 1.5rem 0 1rem;
  }

  .tabs {
    display: flex;
    gap: 0.25rem;
    margin-bottom: 1.5rem;
    border-bottom: 1px solid var(--color-border);
    padding-bottom: 0;
  }

  .tab-btn {
    padding: 0.5rem 1rem;
    border: none;
    background: none;
    font-size: 0.9rem;
    color: var(--color-text-muted);
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }

  .tab-btn.active {
    color: var(--color-primary);
    border-bottom-color: var(--color-primary);
  }

  .tab-btn:hover {
    color: var(--color-text);
  }

  .report-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .report-card {
    padding: 1rem;
  }

  .report-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .report-date {
    font-size: 0.8rem;
  }

  .report-meta {
    font-size: 0.85rem;
    margin-bottom: 0.4rem;
  }

  .report-reason {
    font-size: 0.9rem;
    margin-bottom: 0.25rem;
  }

  .report-details {
    font-size: 0.85rem;
  }

  .report-actions {
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--color-border);
  }

  .resolve-form {
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .form-label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .resolve-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.25rem;
  }

  .warning-text {
    font-size: 0.85rem;
    color: var(--color-error);
    font-weight: 500;
    margin: 0;
  }

  .resolution-note {
    margin-top: 0.5rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--color-border);
    font-size: 0.85rem;
  }
</style>
