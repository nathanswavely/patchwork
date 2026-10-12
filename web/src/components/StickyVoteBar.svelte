<script>
  import { api } from '../lib/api.js';
  import { timeLeft as timeLeftFor, timeLeftShort } from '../lib/datetime.js';
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
    const left = timeLeftFor(votingEndsAt);
    if (!left) return '';
    if (left.ended) return 'ended';
    return timeLeftShort(left);
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
  <!-- The labels follow the vote, the way VoteSection's do. A CSS `active`
       class was the only thing marking the cast vote here, so the bar read
       "Approve" two inches under a panel reading "Approved": "that bottom
       bar says Approve even on the rules proposal where I've already
       voted" (F-050). -->
  <div class="lt-action-bar sticky-vote-bar">
    <div class="vote-buttons">
      <button
        class="vote-btn approve"
        class:active={userVote === 'approve'}
        onclick={() => castVote('approve')}
        disabled={voting}
      >{userVote === 'approve' ? 'Approved' : 'Approve'}</button>
      <button
        class="vote-btn reject"
        class:active={userVote === 'reject'}
        onclick={() => castVote('reject')}
        disabled={voting}
      >{userVote === 'reject' ? 'Rejected' : 'Reject'}</button>
      <button
        class="vote-btn abstain"
        class:active={userVote === 'abstain'}
        onclick={() => castVote('abstain')}
        disabled={voting}
      >{userVote === 'abstain' ? 'Abstained' : 'Abstain'}</button>
    </div>
    <div class="vote-summary">
      <span class="count approve-count">{approveCount}&#10003;</span>
      <span class="count reject-count">{rejectCount}&#10007;</span>
      {#if abstainCount > 0}
        <span class="count abstain-count">{abstainCount} abstain</span>
      {/if}
      {#if timeLeft}
        <span class="time-left">{timeLeft}</span>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* Liquid Textile's action bar: fixed to the foot, surface, top edge,
     shadow, the slide-up and its reduced-motion opt-out. It sits on
     --lt-chrome-bottom, which is Patchwork's tab bar, the patch
     workspace's or the quilt's (0px where there is none). Above the rail
     and the global bar, as it always was. */
  .sticky-vote-bar {
    z-index: 100;
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

  /* A phone: the tally on a line of its own above three buttons that
     share the width at a finger's height. Side by side, the tally ran
     past the right edge and the buttons were 31px. */
  @media (max-width: 640px) {
    .sticky-vote-bar {
      flex-direction: column-reverse;
      align-items: stretch;
      gap: 0.4rem;
      padding: 0.5rem var(--pw-gutter);
    }

    .vote-buttons {
      gap: 0.4rem;
    }

    .vote-btn {
      flex: 1;
      min-height: 44px;
      padding: 0 0.5rem;
      font-size: 0.85rem;
    }

    .vote-summary {
      justify-content: center;
    }
  }
</style>
