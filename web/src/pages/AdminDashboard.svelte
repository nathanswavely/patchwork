<script>
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let stats = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    loadStats();
  });

  async function loadStats() {
    loading = true;
    error = '';
    try {
      stats = await api('admin/stats');
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  function handleNav(e, path) {
    e.preventDefault();
    navigate(path);
  }

  function proposalBarWidth(count) {
    if (!stats) return '0%';
    const total = stats.open_proposals + stats.passed_proposals + stats.rejected_proposals;
    if (total === 0) return '0%';
    return Math.round((count / total) * 100) + '%';
  }
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>Admin Dashboard</h1>
  </div>

  {#if loading}
    <Skeleton lines={5} />
  {:else if error}
    <ErrorState message={error} retry={loadStats} />
  {:else if stats}
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{stats.total_users}</div>
        <div class="stat-label">Total Users</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{stats.active_users_30d}</div>
        <div class="stat-label">Active (30d)</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{stats.total_nodes}</div>
        <div class="stat-label">Patches</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{stats.total_events}</div>
        <div class="stat-label">Events</div>
      </div>
      <div class="stat-card highlight">
        <div class="stat-value">{stats.pending_reports}</div>
        <div class="stat-label">Pending Reports</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{stats.recent_signups_7d}</div>
        <div class="stat-label">Signups (7d)</div>
      </div>
    </div>

    <div class="section">
      <h2>Proposals</h2>
      <div class="proposal-bars">
        <div class="bar-row">
          <span class="bar-label">Open</span>
          <div class="bar-track">
            <div class="bar-fill bar-open" style="width: {proposalBarWidth(stats.open_proposals)}"></div>
          </div>
          <span class="bar-count">{stats.open_proposals}</span>
        </div>
        <div class="bar-row">
          <span class="bar-label">Passed</span>
          <div class="bar-track">
            <div class="bar-fill bar-passed" style="width: {proposalBarWidth(stats.passed_proposals)}"></div>
          </div>
          <span class="bar-count">{stats.passed_proposals}</span>
        </div>
        <div class="bar-row">
          <span class="bar-label">Rejected</span>
          <div class="bar-track">
            <div class="bar-fill bar-rejected" style="width: {proposalBarWidth(stats.rejected_proposals)}"></div>
          </div>
          <span class="bar-count">{stats.rejected_proposals}</span>
        </div>
      </div>
    </div>

    <div class="section">
      <h2>Quick Links</h2>
      <div class="quick-links">
        <a href="/admin/reports" class="quick-link card" onclick={(e) => handleNav(e, '/admin/reports')}>
          <strong>Reports Queue</strong>
          <span class="muted">Review and resolve content reports</span>
        </a>
        <a href="/admin/users" class="quick-link card" onclick={(e) => handleNav(e, '/admin/users')}>
          <strong>User Management</strong>
          <span class="muted">Search, suspend, and manage roles</span>
        </a>
        <a href="/admin/audit" class="quick-link card" onclick={(e) => handleNav(e, '/admin/audit')}>
          <strong>Audit Log</strong>
          <span class="muted">View system activity history</span>
        </a>
        <!-- Export is passkey-gated (docs/adr/017), so it lives on the Quilt
             Settings page where the confirmation flow and the "you need a
             passkey" notice are. A direct link here would just 403. -->
        <a href="/admin/quilt" class="quick-link card" onclick={(e) => handleNav(e, '/admin/quilt')}>
          <strong>Export Data (Seamrip)</strong>
          <span class="muted">Download all community data as a zip: patches, members, events, governance</span>
        </a>
      </div>
    </div>
  {/if}
</div>

<style>
  .page-header {
    padding: 1.5rem 0 1rem;
  }

  .stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 0.75rem;
    margin-bottom: 2rem;
  }

  .stat-card {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    padding: 1rem;
    text-align: center;
  }

  .stat-card.highlight {
    border-color: var(--color-accent);
  }

  .stat-value {
    font-size: 1.75rem;
    font-weight: 700;
    line-height: 1.2;
  }

  .stat-label {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    margin-top: 0.25rem;
  }

  .section {
    margin-bottom: 2rem;
  }

  .section h2 {
    font-size: 1.1rem;
    margin-bottom: 0.75rem;
  }

  .proposal-bars {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .bar-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .bar-label {
    width: 70px;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .bar-track {
    flex: 1;
    height: 20px;
    background: var(--color-bg);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .bar-fill {
    height: 100%;
    border-radius: var(--radius);
    transition: width 300ms ease;
    min-width: 2px;
  }

  .bar-open {
    background: var(--color-primary);
  }

  .bar-passed {
    background: var(--color-success);
  }

  .bar-rejected {
    background: var(--color-error);
  }

  .bar-count {
    width: 30px;
    text-align: right;
    font-size: 0.85rem;
    font-weight: 600;
  }

  .quick-links {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 0.75rem;
  }

  .quick-link {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    text-decoration: none;
    color: var(--color-text);
    transition: border-color 150ms ease;
  }

  .quick-link:hover {
    border-color: var(--color-primary);
    text-decoration: none;
  }
</style>
