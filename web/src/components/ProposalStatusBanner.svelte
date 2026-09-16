<script>
  import { api } from '../lib/api.js';
  import { timeLeft as timeLeftFor, timeLeftPhrase, formatDay } from '../lib/datetime.js';
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
    abstainCount = 0,
    directChange = false,
    canVote = false,
    // The maintainer's patch (docs/adr/092). `advisory` says any tally on
    // this proposal is advice and the maintainer decides; `canDecide` is the
    // server's answer to whether this viewer is that maintainer, right now;
    // `declinedBy` names who said no, when somebody did.
    advisory = false,
    canDecide = false,
    declinedBy = '',
    // An election's phase, empty on every other proposal (docs/adr/051). An
    // election row carries state 'voting' from the moment it is created, so
    // without this the banner announces an open vote through the whole
    // nomination window — over a panel correctly saying nominations close in
    // two weeks, and with an empty time-left leaving "Voting is open. .".
    electionPhase = '',
    // When standing shuts. A different deadline from votingEndsAt, and the
    // only one that matters while nominations are open (docs/adr/106).
    nominationsCloseAt = null,
    onStateChange = () => {},
  } = $props();

  // An election has no author in the ordinary sense — the calendar opened it,
  // and `systemAuthorFor` named an admin because the record needed a name. So
  // it is never withdrawable, and the server refuses it too.
  let mayWithdraw = $derived(isAuthor && !electionPhase);

  // A proposal opens for voting when it is created (docs/adr/048), so there
  // are no `draft` or `discussion` branches here. The states exist in the
  // migration-016 column and nothing writes them; the "Submit for voting"
  // button that used to promote a draft PATCHed a field the handler drops,
  // and the handler now refuses it by name.
  //
  // `elsewhere` is not one of those. It is not a stage ahead of a vote that
  // will come — it is a patch that holds its votes somewhere else, and
  // nothing promotes out of it (docs/adr/053).

  // Compute effective state from both state and legacy status fields.
  let effectiveState = $derived(propState || (status === 'open' ? 'voting' : status === 'passed' || status === 'approved' ? 'approved' : status));

  let timeLeft = $derived.by(() => {
    const left = timeLeftFor(votingEndsAt);
    if (!left) return '';
    if (left.ended) return 'Voting ended';
    return `${timeLeftPhrase(left)} left`;
  });

  // "Cast your vote below" only when there is a vote below. A viewer outside
  // the electorate has the buttons hidden, and an instruction pointing at
  // nothing is the same dead end one sentence over (docs/adr/044).
  //
  // `timeLeft` is guarded too: an election has no voting_ends_at until
  // nominations close, and the unguarded template rendered "Voting is open. ."
  let votingLine = $derived(
    (timeLeft ? `Voting is open. ${timeLeft}.` : 'Voting is open.') +
      (canVote ? ' Cast your vote below.' : '')
  );

  // An advisory vote says so in the first breath (docs/adr/092): a member
  // watching a bar fill toward a majority that has no force is the lie the
  // whole feature exists to end.
  let advisoryLine = $derived(
    (timeLeft ? `Advisory vote. ${timeLeft}. ` : 'Advisory vote. ') +
      'The maintainer decides.' +
      (canVote ? ' Cast your vote below.' : '')
  );

  let hasBallots = $derived(approveCount + rejectCount + abstainCount > 0);
  // The members can be asked once. A window that has run carries its
  // advice; the server refuses a second one and the button is not offered.
  let mayAskMembers = $derived(canDecide && !hasBallots && !votingEndsAt);

  let applying = $state(false);
  let deciding = $state(false);

  async function handleDecide(decision) {
    deciding = true;
    try {
      await api(`proposals/${proposalId}/decide`, { method: 'POST', body: { decision } });
      showToast(decision === 'approve' ? 'Approved. The change is in effect' : 'Declined', decision === 'approve' ? 'success' : 'info');
      onStateChange(decision === 'approve' ? 'in_effect' : 'rejected');
    } catch (e) {
      showToast(e.message || 'Failed to decide', 'error');
    } finally {
      deciding = false;
    }
  }

  async function handleAskMembers() {
    deciding = true;
    try {
      await api(`proposals/${proposalId}/open-vote`, { method: 'POST', body: {} });
      showToast('Advisory vote opened', 'success');
      onStateChange('voting');
    } catch (e) {
      showToast(e.message || 'Failed to open the vote', 'error');
    } finally {
      deciding = false;
    }
  }

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

{#if electionPhase === 'nominating'}
  <!-- An election is born carrying state 'voting' because that is where it is
       headed, not where it is (docs/adr/051). Until nominations close there is
       no ballot and no clock, so the phase decides this branch, not the
       state. -->
  <div class="status-banner voting">
    <!-- With the date (docs/adr/106). This said only that nominations were
         open; the one number on the page was the voting countdown, which is
         a different deadline. Three members of a co-op arrived to stand after
         the window had shut, and the person who told them when it shut had
         read the wrong line. A window is its closing date. -->
    <p>
      {#if nominationsCloseAt}
        Nominations are open until {formatDay(nominationsCloseAt)}. Voting starts then.
      {:else}
        Nominations are open. Voting starts when they close.
      {/if}
    </p>
    {#if mayWithdraw}
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

{:else if effectiveState === 'voting' && advisory}
  <!-- The maintainer asked the members (docs/adr/092). The vote runs on
       the ordinary ballot and the ordinary clock, and decides nothing: the
       maintainer may approve or decline at any time, and when the window
       closes the proposal comes back to them with the tally attached. -->
  <div class="status-banner voting advisory">
    <p>{advisoryLine}</p>
    {#if canDecide || mayWithdraw}
      <div class="banner-actions">
        {#if canDecide}
          <button class="btn btn-primary" onclick={() => handleDecide('approve')} disabled={deciding}>
            {deciding ? 'Working...' : 'Approve'}
          </button>
          <ConfirmAction
            label="Decline"
            confirmLabel="Decline this proposal"
            variant="danger"
            onConfirm={() => handleDecide('decline')}
          />
        {/if}
        {#if mayWithdraw}
          <ConfirmAction
            label="Withdraw this proposal"
            confirmLabel="Withdraw"
            variant="danger"
            onConfirm={handleWithdraw}
          />
        {/if}
      </div>
    {/if}
  </div>

{:else if effectiveState === 'voting'}
  <div class="status-banner voting">
    <p>{votingLine}</p>
    {#if mayWithdraw}
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

{:else if effectiveState === 'awaiting_admin'}
  <!-- Waiting on the maintainer (docs/adr/092). Open, discussable, and
       carrying no ballot: a member's proposal is born here, and an
       advisory vote lands back here when its window closes. No clock ends
       it; only an admin's decision or the author withdrawing does. -->
  <div class="status-banner awaiting">
    <p>
      {#if hasBallots}
        The members have been asked and the vote has closed. The maintainer decides, with the tally below.
      {:else}
        Waiting on the maintainer. This patch's admins decide its proposals, and may ask the members first.
      {/if}
    </p>
    {#if canDecide || mayWithdraw}
      <div class="banner-actions">
        {#if canDecide}
          <button class="btn btn-primary" onclick={() => handleDecide('approve')} disabled={deciding}>
            {deciding ? 'Working...' : 'Approve'}
          </button>
          <ConfirmAction
            label="Decline"
            confirmLabel="Decline this proposal"
            variant="danger"
            onConfirm={() => handleDecide('decline')}
          />
          {#if mayAskMembers}
            <button class="btn btn-secondary" onclick={handleAskMembers} disabled={deciding}>
              Ask the members
            </button>
          {/if}
        {/if}
        {#if mayWithdraw}
          <ConfirmAction
            label="Withdraw this proposal"
            confirmLabel="Withdraw"
            variant="danger"
            onConfirm={handleWithdraw}
          />
        {/if}
      </div>
    {/if}
  </div>

{:else if effectiveState === 'elsewhere'}
  <!-- This patch decides its proposals somewhere else (docs/adr/053). The
       proposal is open — it can be discussed, revised and withdrawn — and has
       no ballot, because a patch with both would let an admin who disliked
       where a tally was heading record a meeting result instead. -->
  <div class="status-banner elsewhere">
    <p>
      Open for discussion. This patch decides at meetings, not here, and what
      it decides gets recorded on the charter afterwards.
    </p>
    {#if mayWithdraw}
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

{:else if (effectiveState === 'in_effect' || effectiveState === 'passed') && electionPhase}
  <!-- A settled election (docs/adr/109). It fell through to the amendment
       branch below and read "Approved. This change is now in effect." over a
       council: no winner, no chair, no term, and a community's leadership
       called "this change". The `unsettled` sibling has had a sentence of its
       own since docs/adr/097; the outcome a year of contests is actually for
       had none. Who was seated is in the panel below, which is why this does
       not repeat it. -->
  <div class="status-banner in-effect">
    <p>This election has closed and the council below is seated.</p>
  </div>

{:else if effectiveState === 'in_effect' || effectiveState === 'passed'}
  <div class="status-banner in-effect">
    <p>
      {#if directChange}
        This change is in effect.
      {:else if advisory}
        The maintainer approved this. It is in effect.
      {:else}
        Approved. This change is now in effect.
      {/if}
    </p>
  </div>

{:else if effectiveState === 'lapsed'}
  <!-- The window closed under quorum (docs/adr/097). Nobody decided
       anything, so this is worded as a vote that did not happen rather than
       one that failed; the tally below is who turned up. -->
  <div class="status-banner lapsed">
    <p>Voting ended without reaching quorum. This proposal lapsed and was not decided.</p>
  </div>

{:else if effectiveState === 'unsettled'}
  <!-- An election that seated nobody: no candidates, quorum unmet, or
       nobody approved (docs/adr/051). The election-shaped sibling of a
       lapse — holdover, not a rejection — so it never wears the tally
       sentence below, which would read "0 approved, 0 rejected" over a
       contest the community simply did not settle. -->
  <div class="status-banner unsettled">
    <!-- Not "the council continues" (docs/adr/106): this banner outlives the
         council it would be describing, and on a patch with no admins it was
         the same comfortable lie the record was telling. The seats it names
         are the ones this contest was for (docs/adr/103). -->
    <p>This election settled nothing; nobody was seated, and the seats it was for are unchanged.</p>
  </div>
{:else if effectiveState === 'rejected'}
  <div class="status-banner rejected">
    <!-- A decline is one person's decision (docs/adr/092), and the tally, if
         any, was advice — so it is never worded as a vote that failed. -->
    <p>
      {#if declinedBy}
        Declined by {declinedBy}.
      {:else}
        This proposal did not pass. {approveCount} approved, {rejectCount} rejected.
      {/if}
    </p>
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

  /* Advice, not a decision: the vote's own tint, dashed, so it reads as a
     vote with something missing (docs/adr/092). */
  .voting.advisory {
    border-style: dashed;
  }

  .awaiting {
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    color: var(--color-text);
  }

  .elsewhere {
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
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

  .withdrawn,
  .lapsed,
  .unsettled {
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    color: var(--color-text-muted);
  }

  .banner-actions {
    margin-top: 0.5rem;
  }
</style>
