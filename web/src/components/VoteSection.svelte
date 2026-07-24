<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';

  let {
    proposalId = '',
    approveCount = 0,
    rejectCount = 0,
    abstainCount = 0,
    memberCount = 0,
    quorumPercent = 0,
    threshold = 'majority',
    userVote = null,
    votingEndsAt = null,
    state: propState = 'voting',
    voters = [],
    canVote = false,
    onVote = () => {},
  } = $props();

  let voting = $state(false);
  let showVoters = $state(false);

  let totalVotes = $derived(approveCount + rejectCount + abstainCount);
  let approvePercent = $derived(totalVotes > 0 ? Math.round((approveCount / totalVotes) * 100) : 0);
  let rejectPercent = $derived(totalVotes > 0 ? Math.round((rejectCount / totalVotes) * 100) : 0);

  let quorumMet = $derived.by(() => {
    if (quorumPercent === 0) return true;
    if (memberCount === 0) return false;
    return (totalVotes / memberCount) * 100 >= quorumPercent;
  });

  let quorumNeeded = $derived(Math.ceil(memberCount * quorumPercent / 100));

  let timeLeft = $derived.by(() => {
    if (!votingEndsAt) return '';
    const ms = new Date(votingEndsAt) - new Date();
    if (ms <= 0) return 'Voting has ended';
    const days = Math.floor(ms / 86400000);
    const hours = Math.floor((ms % 86400000) / 3600000);
    if (days > 0) return `${days} day${days > 1 ? 's' : ''} remaining`;
    return `${hours} hour${hours > 1 ? 's' : ''} remaining`;
  });

  let thresholdExplain = $derived.by(() => {
    const explanations = {
      majority: 'Majority \u2014 more than half of votes must approve',
      supermajority: 'Supermajority \u2014 at least 2 out of 3 votes must approve',
      consensus: 'Consensus \u2014 no reject votes allowed',
    };
    return explanations[threshold] || threshold;
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

<div class="vote-section">
  <!-- Progress bar -->
  <div class="vote-bar">
    <div class="bar-fill approve-fill" style="width: {approvePercent}%"></div>
    <div class="bar-fill reject-fill" style="width: {rejectPercent}%"></div>
    {#if quorumPercent > 0}
      <div class="quorum-marker" style="left: {quorumPercent}%" title="Quorum: {quorumPercent}%"></div>
    {/if}
  </div>

  <!-- Counts -->
  <div class="vote-counts">
    <span class="approve-count">{approveCount} approve</span>
    <span class="sep">&middot;</span>
    <span class="reject-count">{rejectCount} reject</span>
    <span class="sep">&middot;</span>
    <span class="abstain-count">{abstainCount} abstain</span>
  </div>

  <!-- Quorum status -->
  <div class="quorum-status">
    {#if quorumPercent > 0}
      {#if quorumMet}
        <span class="quorum-met">Quorum met ({totalVotes} of {memberCount} voted, {quorumPercent}% needed)</span>
      {:else}
        <span class="quorum-unmet">Quorum not yet met ({totalVotes} of {quorumNeeded} needed)</span>
      {/if}
    {:else}
      <span class="quorum-none muted">No quorum required</span>
    {/if}
  </div>

  <!-- Time remaining -->
  {#if timeLeft && (propState === 'voting' || propState === 'open')}
    <div class="time-remaining muted">{timeLeft}</div>
  {/if}

  <!-- Vote buttons -->
  {#if canVote && (propState === 'voting' || propState === 'open')}
    <div class="vote-buttons">
      <button
        class="vote-btn approve"
        class:active={userVote === 'approve'}
        onclick={() => castVote('approve')}
        disabled={voting}
      >
        {#if voting}<span class="spinner-inline"></span>{:else}{userVote === 'approve' ? 'Approved' : 'Approve'}{/if}
      </button>
      <button
        class="vote-btn reject"
        class:active={userVote === 'reject'}
        onclick={() => castVote('reject')}
        disabled={voting}
      >
        {#if voting}<span class="spinner-inline"></span>{:else}{userVote === 'reject' ? 'Rejected' : 'Reject'}{/if}
      </button>
      <button
        class="vote-btn abstain"
        class:active={userVote === 'abstain'}
        onclick={() => castVote('abstain')}
        disabled={voting}
      >
        {#if voting}<span class="spinner-inline"></span>{:else}{userVote === 'abstain' ? 'Abstained' : 'Abstain'}{/if}
      </button>
    </div>
  {/if}

  <!-- Threshold explanation -->
  <div class="threshold-info muted">{thresholdExplain}</div>

  <!-- Voter list -->
  {#if voters.length > 0}
    <button class="toggle-voters" onclick={() => showVoters = !showVoters}>
      {showVoters ? 'Hide' : 'Show'} voters ({voters.length})
    </button>
    {#if showVoters}
      <ul class="voter-list">
        {#each voters as voter}
          <li>
            <span class="voter-name">{voter.display_name || voter.username}</span>
            <span class="voter-value badge {voter.value}">{voter.value}</span>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .vote-section {
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .vote-bar {
    position: relative;
    height: 8px;
    background: var(--color-overlay);
    border-radius: 4px;
    overflow: visible;
    display: flex;
    margin-bottom: 0.6rem;
  }

  .bar-fill {
    height: 100%;
    transition: width 300ms ease;
  }

  .approve-fill {
    background: var(--color-success);
    border-radius: 4px 0 0 4px;
  }

  .reject-fill {
    background: var(--color-error);
    border-radius: 0 4px 4px 0;
  }

  .quorum-marker {
    position: absolute;
    top: -3px;
    width: 2px;
    height: 14px;
    background: var(--color-text);
    opacity: 0.6;
    transform: translateX(-1px);
  }

  .vote-counts {
    display: flex;
    gap: 0.4rem;
    font-size: 0.82rem;
    margin-bottom: 0.4rem;
  }

  .approve-count { color: var(--color-success); font-weight: 500; }
  .reject-count { color: var(--color-error); font-weight: 500; }
  .abstain-count { color: var(--color-text-muted); }
  .sep { color: var(--color-text-muted); }

  .quorum-status {
    font-size: 0.78rem;
    margin-bottom: 0.25rem;
  }

  .quorum-met { color: var(--color-success); }
  .quorum-unmet { color: var(--color-accent); }

  .time-remaining {
    font-size: 0.78rem;
    margin-bottom: 0.75rem;
  }

  .vote-buttons {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }

  .vote-btn {
    flex: 1;
    padding: 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.88rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 150ms ease;
    text-align: center;
  }

  .vote-btn:hover:not(:disabled):not(.active) {
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

  .threshold-info {
    font-size: 0.75rem;
    margin-bottom: 0.5rem;
  }

  .toggle-voters {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.78rem;
    cursor: pointer;
    padding: 0;
  }

  .toggle-voters:hover {
    text-decoration: underline;
  }

  .voter-list {
    list-style: none;
    padding: 0;
    margin-top: 0.5rem;
    font-size: 0.82rem;
  }

  .voter-list li {
    display: flex;
    justify-content: space-between;
    padding: 0.25rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .voter-list li:last-child {
    border-bottom: none;
  }

  .voter-value.approve { color: var(--color-success); }
  .voter-value.reject { color: var(--color-error); }
  .voter-value.abstain { color: var(--color-text-muted); }

  .spinner-inline {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid var(--color-border);
    border-top-color: var(--color-text-muted);
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
