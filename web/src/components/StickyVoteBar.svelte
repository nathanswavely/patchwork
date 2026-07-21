<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';

  let {
    proposalId = '',
    approveCount = 0,
    rejectCount = 0,
    abstainCount = 0,
    userVote = null,
    votingEndsAt = null,
    visible = false,
    onVote = () => {},
  } = $props();

  let voting = $state(false);

  let timeLeft = $derived.by(() => {
    if (!votingEndsAt) return '';
    const ms = new Date(votingEndsAt) - new Date();
    if (ms <= 0) return 'ended';
    const days = Math.floor(ms / 86400000);
    const hours = Math.floor((ms % 86400000) / 3600000);
    if (days > 0) return `${days}d`;
    return `${hours}h`;
  });

  async function castVote(value) {
    voting = true;
    try {
      await api(`proposals/${proposalId}/vote`, { method: 'POST', body: { value } });
      onVote(value);
    } catch (e) {
      showToast(e.message || 'Failed to vote', 'error');
    } finally {
      voting = false;
    }
  }
</script>

{#if visible}
  <div class="sticky-vote-bar">
    <div class="vote-buttons">
      <button
        class="vote-btn approve"
        class:active={userVote === 'approve'}
        onclick={() => castVote('approve')}
        disabled={voting}
      >Approve</button>
      <button
        class="vote-btn reject"
        class:active={userVote === 'reject'}
        onclick={() => castVote('reject')}
        disabled={voting}
      >Reject</button>
      <button
        class="vote-btn abstain"
        class:active={userVote === 'abstain'}
        onclick={() => castVote('abstain')}
        disabled={voting}
      >Abstain</button>
    </div>
    <div class="vote-summary">
      <span class="count approve-count">{approveCount}&#10003;</span>
      <span class="count reject-count">{rejectCount}&#10007;</span>
      {#if abstainCount > 0}
        <span class="count abstain-count">{abstainCount}&mdash;</span>
      {/if}
      {#if timeLeft}
        <span class="time-left">{timeLeft}</span>
      {/if}
    </div>
  </div>
{/if}

<style>
  .sticky-vote-bar {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    background: var(--color-surface);
    border-top: 1px solid var(--color-border);
    box-shadow: 0 -2px 8px var(--color-shadow);
    padding: 0.6rem 2rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    z-index: 100;
    animation: slideUp 200ms ease;
  }

  @keyframes slideUp {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }

  .vote-buttons {
    display: flex;
    gap: 0.5rem;
  }

  .vote-btn {
    padding: 0.4rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.82rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 150ms ease;
  }

  .vote-btn:hover:not(:disabled) {
    border-color: var(--color-primary);
  }

  .vote-btn.approve.active {
    background: var(--color-success);
    border-color: var(--color-success);
    color: var(--color-on-success);
  }

  .vote-btn.reject.active {
    background: var(--color-error);
    border-color: var(--color-error);
    color: var(--color-on-error);
  }

  .vote-btn.abstain.active {
    background: var(--color-text-muted);
    border-color: var(--color-text-muted);
    color: var(--color-on-muted);
  }

  .vote-summary {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.82rem;
  }

  .count {
    font-weight: 600;
  }

  .approve-count { color: var(--color-success); }
  .reject-count { color: var(--color-error); }
  .abstain-count { color: var(--color-text-muted); }

  .time-left {
    color: var(--color-text-muted);
    font-size: 0.78rem;
  }

  @media (max-width: 640px) {
    .sticky-vote-bar {
      padding: 0.5rem 1rem;
    }

    .vote-btn {
      padding: 0.4rem 0.75rem;
      font-size: 0.78rem;
    }
  }
</style>
