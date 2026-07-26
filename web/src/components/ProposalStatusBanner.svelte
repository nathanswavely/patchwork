<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from './ConfirmAction.svelte';

  let {
    state: propState = '',
    status = '',
    isAdmin = false,
    isAuthor = false,
    proposalId = '',
    votingEndsAt = null,
    approveCount = 0,
    rejectCount = 0,
    directChange = false,
    canVote = false,
    onStateChange = () => {},
  } = $props();

  // A proposal opens for voting when it is created (docs/adr/048), so there
  // are no `draft` or `discussion` branches here. The states exist in the
  // migration-016 column and nothing writes them; the "Submit for voting"
  // button that used to promote a draft PATCHed a field the handler drops,
  // and the handler now refuses it by name.

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

  // "Cast your vote below" only when there is a vote below. A viewer outside
  // the electorate has the buttons hidden, and an instruction pointing at
  // nothing is the same dead end one sentence over (docs/adr/044).
  let votingLine = $derived(
    `Voting is open. ${timeLeft}.` + (canVote ? ' Cast your vote below.' : '')
  );

  let applying = $state(false);

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
</script>

{#if effectiveState === 'voting'}
  <div class="status-banner voting">
    <p>{votingLine}</p>
    {#if isAuthor}
      <div class="banner-actions">
        <ConfirmAction
          label="Withdraw this proposal"
          confirmLabel="Withdraw"
          variant="danger"
          onConfirm={handleWithdraw}
        />
      </div>
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
    <p>{directChange ? 'This change is in effect.' : 'Approved. This change is now in effect.'}</p>
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
</style>
