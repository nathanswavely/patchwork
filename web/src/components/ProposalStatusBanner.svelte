<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';

  let {
    state: propState = '',
    status = '',
    isAdmin = false,
    isAuthor = false,
    proposalId = '',
    votingEndsAt = null,
    approveCount = 0,
    rejectCount = 0,
    onStateChange = () => {},
  } = $props();

  // Compute effective state from both state and legacy status fields.
  let effectiveState = $derived(propState || (status === 'open' ? 'voting' : status === 'passed' || status === 'approved' ? 'approved' : status));

  let timeLeft = $derived.by(() => {
    if (!votingEndsAt) return '';
    const ms = new Date(votingEndsAt) - new Date();
    if (ms <= 0) return 'Voting ended';
    const days = Math.floor(ms / 86400000);
    const hours = Math.floor((ms % 86400000) / 3600000);
    if (days > 0) return `${days} day${days > 1 ? 's' : ''} left`;
    return `${hours} hour${hours > 1 ? 's' : ''} left`;
  });

  let applying = $state(false);
  let submittingForVote = $state(false);

  async function handleApply() {
    applying = true;
    try {
      await api(`proposals/${proposalId}/apply`, { method: 'POST' });
      showToast('Change is now in effect', 'success');
      onStateChange('in_effect');
    } catch (e) {
      showToast(e.message || 'Failed to apply', 'error');
    } finally {
      applying = false;
    }
  }

  async function handleWithdraw() {
    try {
      await api(`proposals/${proposalId}`, { method: 'DELETE' });
      showToast('Proposal withdrawn', 'info');
      onStateChange('withdrawn');
    } catch (e) {
      showToast(e.message || 'Failed to withdraw', 'error');
    }
  }

  async function handleSubmitForVoting() {
    submittingForVote = true;
    try {
      await api(`proposals/${proposalId}`, { method: 'PATCH', body: { state: 'voting' } });
      showToast('Submitted for voting', 'success');
      onStateChange('voting');
    } catch (e) {
      showToast(e.message || 'Failed to submit', 'error');
    } finally {
      submittingForVote = false;
    }
  }
</script>

{#if effectiveState === 'draft'}
  <div class="status-banner draft">
    <p>This is a draft. Only you can see it.</p>
    {#if isAuthor}
      <div class="banner-actions">
        <button class="btn btn-primary btn-sm" onclick={handleSubmitForVoting} disabled={submittingForVote}>
          {submittingForVote ? 'Submitting...' : 'Submit for voting'}
        </button>
      </div>
    {/if}
  </div>

{:else if effectiveState === 'discussion'}
  <div class="status-banner discussion">
    <p>Open for discussion. Voting begins soon. Members can comment and the author can revise.</p>
  </div>

{:else if effectiveState === 'voting'}
  <div class="status-banner voting">
    <p>Voting is open. {timeLeft}. Cast your vote below.</p>
    {#if isAuthor}
      <button class="banner-link" onclick={handleWithdraw}>Withdraw this proposal</button>
    {/if}
  </div>

{:else if effectiveState === 'approved'}
  <div class="status-banner approved-pending">
    <p>The community approved this change. An admin needs to make it official.</p>
    {#if isAdmin}
      <div class="banner-actions">
        <button class="btn btn-primary" onclick={handleApply} disabled={applying}>
          {applying ? 'Applying...' : 'Make this official'}
        </button>
      </div>
    {/if}
  </div>

{:else if effectiveState === 'in_effect' || effectiveState === 'passed'}
  <div class="status-banner in-effect">
    <p>Approved. This change is now in effect.</p>
  </div>

{:else if effectiveState === 'rejected'}
  <div class="status-banner rejected">
    <p>This proposal did not pass. {approveCount} approved, {rejectCount} rejected.</p>
  </div>

{:else if effectiveState === 'withdrawn'}
  <div class="status-banner withdrawn">
    <p>Withdrawn by the author.</p>
  </div>
{/if}

<style>
  .status-banner {
    padding: 0.75rem 1rem;
    border-radius: var(--radius);
    margin-bottom: 1.25rem;
    font-size: 0.88rem;
    line-height: 1.5;
  }

  .status-banner p {
    margin: 0;
  }

  .draft {
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    color: var(--color-text-muted);
  }

  .discussion {
    background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface));
    border: 1px solid color-mix(in srgb, var(--color-primary) 25%, var(--color-border));
    color: var(--color-text);
  }

  .voting {
    background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface));
    border: 1px solid var(--color-primary);
    color: var(--color-text);
  }

  .approved-pending {
    background: color-mix(in srgb, var(--color-accent) 8%, var(--color-surface));
    border: 1px solid var(--color-accent);
    color: var(--color-text);
  }

  .in-effect {
    background: color-mix(in srgb, var(--color-success) 10%, var(--color-surface));
    border: 1px solid var(--color-success);
    color: var(--color-text);
  }

  .rejected {
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    color: var(--color-text-muted);
  }

  .withdrawn {
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    color: var(--color-text-muted);
  }

  .banner-actions {
    margin-top: 0.5rem;
  }

  .banner-link {
    border: none;
    background: none;
    color: var(--color-text-muted);
    font-size: 0.78rem;
    cursor: pointer;
    padding: 0;
    margin-top: 0.25rem;
    text-decoration: underline;
  }

  .banner-link:hover {
    color: var(--color-text);
  }
</style>
