<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';

  import { isLoggedIn } from '../stores/auth.svelte.js';
  import VocabLabel from '../components/VocabLabel.svelte';
  import GovernanceShell from '../components/GovernanceShell.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let membershipRole = $derived(patch.value.membershipRole);
  let proposals = $state([]);
  let loading = $state(true);
  let followerPermissions = $derived(patch.value.followerPermissions);
  let permissionDenied = $derived(membershipRole === 'follower' && followerPermissions?.proposals === false);
  let statusFilter = $state('open');

  $effect(() => {
    if (slug) {
      loadProposals();
    }
  });

  async function loadProposals() {
    loading = true;
    try {
      const params = statusFilter && statusFilter !== 'all' ? `?status=${encodeURIComponent(statusFilter)}` : '';
      const data = await api(`nodes/${slug}/proposals${params}`);
      proposals = data.items || data || [];
    } catch {
      proposals = [];
    } finally {
      loading = false;
    }
  }

  function filterByStatus(status) {
    statusFilter = status;
    loadProposals();
  }

  function statusClass(status) {
    if (status === 'approved') return 'status-approved';
    if (status === 'rejected') return 'status-rejected';
    if (status === 'open') return 'status-open';
    if (status === 'withdrawn') return 'status-withdrawn';
    return '';
  }

  function typeLabel(type) {
    const labels = { amendment: 'Amendment', membership: 'Membership', action: 'Action', other: 'Other' };
    return labels[type] || type;
  }

  function timeRemaining(votingEndsAt) {
    if (!votingEndsAt) return '';
    const now = new Date();
    const end = new Date(votingEndsAt);
    const diff = end - now;
    if (diff <= 0) return 'Voting ended';
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const days = Math.floor(hours / 24);
    if (days > 0) return `${days}d ${hours % 24}h left`;
    if (hours > 0) return `${hours}h left`;
    const mins = Math.floor(diff / (1000 * 60));
    return `${mins}m left`;
  }

  function voteTallyPercent(approve, reject) {
    const total = approve + reject;
    if (total === 0) return 50;
    return Math.round((approve / total) * 100);
  }
</script>

<GovernanceShell activeSection="proposals">
  {#snippet children()}
{#if permissionDenied}
  <div class="permission-notice">
    <p>This content is only visible to members.</p>
    <p class="muted">Become a member to access proposals.</p>
  </div>
{:else}
<div class="page-fade">
  <div>
      <div class="page-header">
        {#if isMember && membershipRole !== 'follower'}
          <a
            href="/patches/{slug}/governance/new"
            class="btn btn-primary"
            onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/new`); }}
          >
            New Proposal
          </a>
        {:else if membershipRole === 'follower'}
          <p class="role-prompt muted">Become a member to create proposals and vote.</p>
        {:else}
          <p class="role-prompt muted">Follow or join this patch to get started.</p>
        {/if}
      </div>

      <div class="status-filters">
        {#each [['open', 'Open'], ['approved', 'Approved'], ['rejected', 'Rejected'], ['all', 'All']] as [value, label]}
          <button
            class="chip"
            class:selected={statusFilter === value}
            onclick={() => filterByStatus(value)}
          >
            {label}
          </button>
        {/each}
      </div>

      {#if loading}
        <p class="muted" style="padding: 2rem 0; text-align: center;">Loading...</p>
      {:else if proposals.length === 0}
        <p class="muted" style="padding: 2rem 0; text-align: center;">
          No proposals{statusFilter && statusFilter !== 'all' ? ` with status "${statusFilter}"` : ''}.
        </p>
      {:else}
        <div class="proposal-list">
          {#each proposals as proposal (proposal.id)}
            <a
              href="/patches/{slug}/governance/{proposal.id}"
              class="proposal-card card"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/${proposal.id}`); }}
            >
              <div class="proposal-info">
                <div class="proposal-title-row">
                  <h3>{proposal.title}</h3>
                  {#if proposal.target_doc}
                    <span class="badge badge-amendment">Amendment: {proposal.target_doc}</span>
                  {:else if proposal.proposal_type && proposal.proposal_type !== 'other'}
                    <span class="type-badge">{typeLabel(proposal.proposal_type)}</span>
                  {/if}
                </div>
                <div class="proposal-meta">
                  {#if proposal.author_name}
                    <span class="muted">by {proposal.author_name}</span>
                  {/if}
                  {#if proposal.status === 'open' && proposal.voting_ends_at}
                    <span class="time-remaining">{timeRemaining(proposal.voting_ends_at)}</span>
                  {:else if proposal.status !== 'open'}
                    <span class="muted">{proposal.status}</span>
                  {/if}
                </div>
                {#if (proposal.approve_count || 0) + (proposal.reject_count || 0) > 0}
                  <div class="vote-bar-container">
                    <div class="vote-bar">
                      <div class="vote-bar-fill approve" style="width: {voteTallyPercent(proposal.approve_count || 0, proposal.reject_count || 0)}%"></div>
                    </div>
                    <div class="vote-bar-labels">
                      <span class="approve-label">{proposal.approve_count || 0}</span>
                      <span class="reject-label">{proposal.reject_count || 0}</span>
                    </div>
                  </div>
                {/if}
              </div>
              <span class="badge {statusClass(proposal.status)}">{proposal.status}</span>
            </a>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}
  {/snippet}
</GovernanceShell>

<style>
  .permission-notice {
    text-align: center;
    padding: 3rem 1rem;
  }

  .permission-notice p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .page-header {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 1.5rem;
  }

  .role-prompt {
    font-size: 0.85rem;
  }

  .status-filters {
    display: flex;
    gap: 0.4rem;
    margin-bottom: 1.5rem;
  }

  .chip {
    padding: 0.25rem 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    font-size: 0.8rem;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: all 150ms ease;
  }

  .chip:hover {
    border-color: var(--color-primary);
  }

  .chip.selected {
    background: var(--color-primary);
    border-color: var(--color-primary);
    color: var(--color-btn-on-primary);
  }

  .proposal-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .proposal-card {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    text-decoration: none;
    color: inherit;
    transition: border-color 150ms ease;
  }

  .proposal-card:hover {
    border-color: var(--color-primary);
    text-decoration: none;
  }

  .proposal-info {
    flex: 1;
    min-width: 0;
  }

  .proposal-title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.2rem;
  }

  .proposal-info h3 {
    font-size: 1rem;
    margin: 0;
  }

  .type-badge {
    font-size: 0.7rem;
    padding: 0.1rem 0.4rem;
    border-radius: 4px;
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
    color: var(--color-primary);
    white-space: nowrap;
  }

  .badge-amendment {
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
    font-size: 0.7rem;
    padding: 0.1rem 0.4rem;
    border-radius: 4px;
    white-space: nowrap;
  }

  .proposal-meta {
    display: flex;
    gap: 0.75rem;
    font-size: 0.8rem;
    margin-bottom: 0.4rem;
  }

  .time-remaining {
    color: var(--color-accent);
    font-weight: 500;
  }

  .vote-bar-container {
    margin-top: 0.3rem;
  }

  .vote-bar {
    width: 100%;
    max-width: 200px;
    height: 6px;
    background: color-mix(in srgb, var(--color-error) 10%, var(--color-surface));
    border-radius: 3px;
    overflow: hidden;
  }

  .vote-bar-fill.approve {
    height: 100%;
    background: var(--color-success);
    border-radius: 3px;
    transition: width 300ms ease;
  }

  .vote-bar-labels {
    display: flex;
    justify-content: space-between;
    max-width: 200px;
    font-size: 0.7rem;
    margin-top: 0.15rem;
  }

  .approve-label {
    color: var(--color-success);
  }

  .reject-label {
    color: var(--color-error);
  }

  .status-approved {
    background: color-mix(in srgb, var(--color-success) 10%, var(--color-surface));
    color: var(--color-success);
    border-color: color-mix(in srgb, var(--color-success) 25%, var(--color-border));
  }

  .status-rejected {
    background: color-mix(in srgb, var(--color-error) 10%, var(--color-surface));
    color: var(--color-error);
    border-color: color-mix(in srgb, var(--color-error) 25%, var(--color-border));
  }

  .status-open {
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
    color: var(--color-primary);
    border-color: color-mix(in srgb, var(--color-primary) 25%, var(--color-border));
  }

  .status-withdrawn {
    background: var(--color-bg);
    color: var(--color-text-muted);
  }

  @media (max-width: 640px) {
    .page-header {
      flex-direction: column;
      gap: 1rem;
    }

    .status-filters {
      flex-wrap: wrap;
    }
  }
</style>
