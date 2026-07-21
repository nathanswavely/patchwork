<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { getParams } from '../stores/router.svelte.js';
  import { isLoggedIn, getUser } from '../stores/auth.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import DiffView from '../components/DiffView.svelte';
  import CommentThread from '../components/CommentThread.svelte';
  import RevisionHistory from '../components/RevisionHistory.svelte';
  import StructuredRulesDiff from '../components/StructuredRulesDiff.svelte';
  import ProposalStatusBanner from '../components/ProposalStatusBanner.svelte';
  import VoteSection from '../components/VoteSection.svelte';
  import StickyVoteBar from '../components/StickyVoteBar.svelte';
  const patch = getContext('patch');
  let patchIsAdmin = $derived(patch.value.isAdmin);
  let patchIsMember = $derived(patch.value.isMember);
  let membershipRole = $derived(patch.value.membershipRole);
  let proposalId = $derived(getParams().id || '');

  let proposal = $state(null);
  let loading = $state(true);
  let error = $state('');

  let user = $derived(getUser());
  let isAuthor = $derived(user && proposal && proposal.author_id === user.id);
  let canVote = $derived(isLoggedIn() && patchIsMember && membershipRole !== 'follower');

  let effectiveState = $derived(
    proposal?.state || (proposal?.status === 'open' ? 'voting' : proposal?.status === 'passed' ? 'in_effect' : proposal?.status)
  );
  let isVoting = $derived(effectiveState === 'voting' || (effectiveState === 'open' && proposal?.status === 'open'));

  let isRulesChange = $derived(
    proposal?.target_doc === 'governance-rules.json' || proposal?.target_doc === 'Governance Rules'
  );
  let hasAmendment = $derived(
    proposal?.target_doc && proposal?.current_doc_content != null && proposal?.proposed_body != null
  );

  // Tabs.
  let activeTab = $state('overview');

  let tabs = $derived.by(() => {
    const t = [{ id: 'overview', label: 'Overview' }];
    if (hasAmendment) t.push({ id: 'changes', label: 'Changes' });
    t.push({ id: 'discussion', label: 'Discussion', count: proposal?.comment_count || 0 });
    t.push({ id: 'history', label: 'History' });
    return t;
  });

  $effect(() => {
    if (proposalId) loadProposal();
  });

  $effect(() => {
    if (proposal) {
      patch.value.setBreadcrumbExtra?.([{ label: proposal.title }]);
    }
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  async function loadProposal() {
    loading = true;
    error = '';
    try {
      proposal = await api(`proposals/${proposalId}`);
    } catch (e) {
      error = e.message || 'Failed to load proposal';
      proposal = null;
    } finally {
      loading = false;
    }
  }

  function handleVote(value) {
    const prev = proposal.my_vote;
    proposal.my_vote = value;
    proposal.approve_count = (proposal.approve_count || 0) + (value === 'approve' ? 1 : 0) - (prev === 'approve' ? 1 : 0);
    proposal.reject_count = (proposal.reject_count || 0) + (value === 'reject' ? 1 : 0) - (prev === 'reject' ? 1 : 0);
    proposal.abstain_count = (proposal.abstain_count || 0) + (value === 'abstain' ? 1 : 0) - (prev === 'abstain' ? 1 : 0);
  }

  function handleStateChange(newState) {
    if (proposal) {
      proposal.state = newState;
      proposal.status = newState;
    }
    loadProposal();
  }
</script>

<div class="proposal-page page-fade">
  {#if loading}
    <div style="padding: 2rem 0;">
      <Skeleton lines={1} height="2rem" width="60%" />
      <div style="margin-top: 1rem;">
        <Skeleton lines={4} height="0.9rem" />
      </div>
    </div>
  {:else if error}
    <ErrorState message={error} retry={loadProposal} />
  {:else if proposal}

    <!-- Status banner — always visible -->
    <ProposalStatusBanner
      state={effectiveState}
      status={proposal.status}
      isAdmin={patchIsAdmin}
      isAuthor={isAuthor}
      proposalId={proposal.id}
      votingEndsAt={proposal.voting_ends_at}
      approveCount={proposal.approve_count || 0}
      rejectCount={proposal.reject_count || 0}
      onStateChange={handleStateChange}
    />

    <!-- Title + meta — always visible -->
    <div class="proposal-header">
      <h1>{proposal.title}</h1>
      <div class="proposal-meta">
        {#if proposal.proposal_type && proposal.proposal_type !== 'other'}
          <span class="type-badge">{proposal.proposal_type === 'amendment' ? 'Amendment' : proposal.proposal_type}</span>
        {/if}
        {#if proposal.target_doc}
          <span class="target-badge">to {proposal.target_doc.replace('.json', '').replace(/-/g, ' ')}</span>
        {/if}
        <span class="muted">Proposed by {proposal.author_name || 'unknown'}</span>
        <span class="muted">{new Date(proposal.created_at).toLocaleDateString()}</span>
      </div>
    </div>

    <!-- Tab bar -->
    <div class="tab-bar">
      {#each tabs as tab}
        <button
          class="tab"
          class:active={activeTab === tab.id}
          onclick={() => activeTab = tab.id}
        >
          {tab.label}
          {#if tab.count > 0}<span class="tab-count">{tab.count}</span>{/if}
        </button>
      {/each}
    </div>

    <!-- Tab content -->
    <div class="tab-content">

      {#if activeTab === 'overview'}
        <!-- Body / rationale -->
        {#if proposal.body}
          <section class="proposal-section">
            <h2>Why this change</h2>
            <div class="content-card">
              <MarkdownRenderer content={proposal.body} />
            </div>
          </section>
        {/if}

        <!-- Inline diff summary for amendments (compact, links to Changes tab) -->
        {#if hasAmendment}
          <section class="proposal-section">
            <div class="changes-summary">
              <span class="muted">This proposal modifies <strong>{proposal.target_doc.replace('.json', '').replace(/-/g, ' ')}</strong></span>
              <button class="btn-link" onclick={() => activeTab = 'changes'}>View changes</button>
            </div>
          </section>
        {/if}

        <!-- Vote section -->
        {#if isVoting || effectiveState === 'approved' || effectiveState === 'in_effect' || effectiveState === 'rejected' || effectiveState === 'passed'}
          <section class="proposal-section">
            <h2>Vote</h2>
            <VoteSection
              proposalId={proposal.id}
              approveCount={proposal.approve_count || 0}
              rejectCount={proposal.reject_count || 0}
              abstainCount={proposal.abstain_count || 0}
              memberCount={proposal.member_count || 0}
              quorumPercent={proposal.quorum_percent || 0}
              threshold={proposal.amendment_threshold || proposal.decision_method || 'majority'}
              userVote={proposal.my_vote}
              votingEndsAt={proposal.voting_ends_at}
              state={effectiveState}
              voters={proposal.voters || []}
              {canVote}
              onVote={handleVote}
            />
          </section>
        {/if}

      {:else if activeTab === 'changes'}
        <section class="proposal-section changes-tab">
          {#if isRulesChange}
            <StructuredRulesDiff
              currentRules={JSON.parse(proposal.current_doc_content)}
              proposedRules={JSON.parse(proposal.proposed_body)}
            />
          {:else}
            <div class="doc-version-label muted">
              {proposal.target_doc.replace(/-/g, ' ')}
            </div>
            <DiffView
              oldText={proposal.current_doc_content}
              newText={proposal.proposed_body}
              oldLabel="Current"
              newLabel="Proposed"
            />
          {/if}
        </section>

      {:else if activeTab === 'discussion'}
        <section class="proposal-section">
          <CommentThread proposalId={proposal.id} isMember={!!patchIsMember} isAdmin={!!patchIsAdmin} />
        </section>

      {:else if activeTab === 'history'}
        <section class="proposal-section">
          <RevisionHistory proposalId={proposal.id} />
        </section>
      {/if}
    </div>

  {/if}
</div>

<!-- Sticky vote bar — only on overview tab during voting -->
{#if proposal && isVoting && canVote && activeTab === 'overview'}
  <StickyVoteBar
    proposalId={proposal.id}
    approveCount={proposal.approve_count || 0}
    rejectCount={proposal.reject_count || 0}
    abstainCount={proposal.abstain_count || 0}
    userVote={proposal.my_vote}
    votingEndsAt={proposal.voting_ends_at}
    visible={true}
    onVote={handleVote}
  />
{/if}

<style>
  .proposal-page {
    max-width: 1000px;
  }

  .proposal-header {
    margin-bottom: 1rem;
  }

  .proposal-header h1 {
    font-size: 1.4rem;
    margin-bottom: 0.4rem;
  }

  .proposal-meta {
    display: flex;
    gap: 0.6rem;
    align-items: center;
    font-size: 0.82rem;
    flex-wrap: wrap;
  }

  .type-badge {
    font-size: 0.72rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
    color: var(--color-primary);
    font-weight: 500;
    text-transform: capitalize;
  }

  .target-badge {
    font-size: 0.72rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    background: color-mix(in srgb, var(--color-accent) 10%, var(--color-surface));
    color: var(--color-accent);
    font-weight: 500;
    text-transform: capitalize;
  }

  /* Tab bar */
  .tab-bar {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--color-border);
    margin-bottom: 1.25rem;
  }

  .tab {
    padding: 0.6rem 1rem;
    border: none;
    background: none;
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    transition: color 150ms ease, border-color 150ms ease;
  }

  .tab:hover {
    color: var(--color-text);
  }

  .tab.active {
    color: var(--color-primary);
    border-bottom-color: var(--color-primary);
    font-weight: 600;
  }

  .tab-count {
    display: inline-block;
    margin-left: 0.35rem;
    padding: 0.05rem 0.4rem;
    border-radius: 999px;
    background: var(--color-overlay);
    font-size: 0.7rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  /* Tab content */
  .tab-content {
    padding-bottom: 2rem;
  }

  .proposal-section {
    padding: 0.75rem 0;
  }

  .proposal-section + .proposal-section {
    border-top: 1px solid var(--color-border);
    padding-top: 1.25rem;
  }

  .proposal-section h2 {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--color-text-muted);
    margin-bottom: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .content-card {
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.92rem;
    line-height: 1.7;
  }

  .content-card :global(p:first-child) { margin-top: 0; }
  .content-card :global(p:last-child) { margin-bottom: 0; }

  .changes-summary {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.6rem 0.85rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.85rem;
  }

  .changes-summary .btn-link {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.82rem;
    cursor: pointer;
    padding: 0;
  }

  .changes-summary .btn-link:hover {
    text-decoration: underline;
  }

  .changes-tab {
    padding-top: 0;
  }

  .doc-version-label {
    padding: 0.5rem 0.75rem;
    font-size: 0.78rem;
    text-transform: capitalize;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg);
  }

  @media (max-width: 640px) {
    .proposal-page {
      padding-bottom: 100px;
    }

    .proposal-header h1 {
      font-size: 1.2rem;
    }

    .tab {
      padding: 0.5rem 0.65rem;
      font-size: 0.8rem;
    }
  }
</style>
