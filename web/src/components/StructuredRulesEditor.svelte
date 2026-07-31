<script>
  let { currentRules = null, onSave = () => {} } = $props();

  const DECISION_OPTIONS = [
    { value: 'admin', label: 'Admin decides' },
    { value: 'majority', label: 'Majority vote' },
    { value: 'supermajority', label: 'Supermajority (2/3)' },
    { value: 'consensus', label: 'Full consensus' },
  ];

  // Amendment thresholds are always votes — "admin decides" is a decision
  // method, not a threshold (under it, amendments never reach a vote).
  const THRESHOLD_OPTIONS = [
    { value: 'majority', label: 'Majority vote' },
    { value: 'supermajority', label: 'Supermajority (2/3)' },
    { value: 'consensus', label: 'Full consensus' },
  ];

  const VOTING_PERIOD_OPTIONS = [
    { value: 24, label: '24 hours' },
    { value: 48, label: '48 hours' },
    { value: 72, label: '72 hours (3 days)' },
    { value: 168, label: '1 week' },
    { value: 336, label: '2 weeks' },
  ];

  const SUCCESSION_OPTIONS = [
    { value: 'longest_tenure', label: 'Longest-tenured member' },
    { value: 'instance_admin', label: 'Instance admin intervenes' },
    { value: 'freeze', label: 'Patch freezes' },
  ];

  const TENURE_OPTIONS = [
    { value: 0, label: 'Immediate' },
    { value: 7, label: '7 days' },
    { value: 30, label: '30 days' },
    { value: 90, label: '90 days' },
  ];

  const MEMBERSHIP_OPTIONS = [
    { value: 'open', label: 'Open' },
    { value: 'approval_required', label: 'Approval required' },
    { value: 'invite_only', label: 'Invite only' },
  ];

  // Editable state derived from currentRules
  let decisionMethod = $state('majority');
  let quorumPercent = $state(0);
  let votingPeriodHours = $state(72);
  let amendmentThreshold = $state('majority');
  let autoApply = $state(true);
  let successionPolicy = $state('longest_tenure');
  let minVotingTenureDays = $state(0);
  // Where admins are actually chosen (docs/adr/052). A community whose board
  // is elected at its annual meeting records that here rather than staging a
  // vote it does not hold.
  let leadershipVenue = $state('patchwork');
  // Where the things proposals are about get decided (docs/adr/053). Elsewhere
  // removes the ballot and keeps the discussion; what the meeting adopts comes
  // back as a record on the charter.
  let proposalVenue = $state('patchwork');
  // Whether a proposal's subject may vote on it (docs/adr/051).
  let subjectRecusal = $state(false);
  let membershipPolicy = $state('open');
  let followerEvents = $state(true);
  let followerProposals = $state(true);
  let followerCharters = $state(true);
  let followerMembers = $state(true);

  let adminDecides = $derived(decisionMethod === 'admin');

  // Initialize from currentRules
  $effect(() => {
    if (currentRules) {
      // 'admin_decides' is a value an older editor build could have written
      // into a rules file; the backend's word is 'admin'.
      const dm = currentRules.decision_method === 'admin_decides' ? 'admin' : currentRules.decision_method;
      decisionMethod = dm || 'majority';
      quorumPercent = currentRules.quorum_percent ?? 0;
      votingPeriodHours = currentRules.default_vote_duration_hours || 72;
      amendmentThreshold = currentRules.amendment_threshold || 'majority';
      autoApply = currentRules.amendment_auto_apply ?? true;
      successionPolicy = currentRules.succession_policy || 'longest_tenure';
      minVotingTenureDays = currentRules.min_voting_tenure_days ?? 0;
      subjectRecusal = currentRules.subject_recusal === true;
      leadershipVenue = currentRules.leadership_venue === 'elsewhere' ? 'elsewhere' : 'patchwork';
      proposalVenue = currentRules.proposal_venue === 'elsewhere' ? 'elsewhere' : 'patchwork';
      membershipPolicy = currentRules.membership_policy || 'open';
      const fp = currentRules.follower_permissions || {};
      followerEvents = fp.events !== false;
      followerProposals = fp.proposals !== false;
      followerCharters = fp.charters !== false;
      followerMembers = fp.members !== false;
    }
  });

  function buildRules() {
    // Spread first: fields this form doesn't edit (leadership_model,
    // succession_method, max_admins, …) must survive the round trip —
    // the rules file is whole-document, not a patch.
    const rules = {
      ...(currentRules || {}),
      decision_method: decisionMethod,
      leadership_venue: leadershipVenue,
      proposal_venue: proposalVenue,
      succession_policy: successionPolicy,
      membership_policy: membershipPolicy,
      follower_permissions: {
        events: followerEvents,
        proposals: followerProposals,
        charters: followerCharters,
        members: followerMembers,
      },
    };
    // The voting knobs are hidden and inert under admin-decides — their
    // stored values pass through untouched rather than being rewritten
    // from editor state.
    if (!adminDecides) {
      rules.quorum_percent = quorumPercent;
      rules.default_vote_duration_hours = votingPeriodHours;
      rules.amendment_threshold = amendmentThreshold;
      rules.amendment_auto_apply = autoApply;
      rules.min_voting_tenure_days = minVotingTenureDays;
      rules.subject_recusal = subjectRecusal;
    }
    return rules;
  }

  function handleSave() {
    onSave(buildRules());
  }

  // Auto-notify parent on every change
  $effect(() => {
    // Touch all reactive values to track them
    decisionMethod; quorumPercent; votingPeriodHours; amendmentThreshold;
    autoApply; successionPolicy; minVotingTenureDays; membershipPolicy;
    subjectRecusal; leadershipVenue; proposalVenue;
    followerEvents; followerProposals; followerCharters; followerMembers;
    onSave(buildRules());
  });
</script>

<div class="rules-editor">
  <div class="field">
    <label for="re-decision">Decision Method</label>
    <select id="re-decision" bind:value={decisionMethod}>
      {#each DECISION_OPTIONS as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  {#if !adminDecides}
    <div class="field">
      <label for="re-quorum">Quorum (%)</label>
      <input id="re-quorum" type="number" min="0" max="100" bind:value={quorumPercent} />
    </div>

    <div class="field">
      <label for="re-voting-period">Default Voting Period</label>
      <select id="re-voting-period" bind:value={votingPeriodHours}>
        {#each VOTING_PERIOD_OPTIONS as opt}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
    </div>

    <div class="field">
      <label for="re-amendment">Amendment Threshold</label>
      <select id="re-amendment" bind:value={amendmentThreshold}>
        {#each THRESHOLD_OPTIONS as opt}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
    </div>

    <div class="field checkbox-field">
      <label>
        <input type="checkbox" bind:checked={autoApply} />
        Auto-Apply Amendments
      </label>
    </div>

    <!-- Recusal is a term of the contest — it decides who may vote — so it
         sits with the voting knobs and is hidden under admin-decides like
         the rest of them (docs/adr/041, docs/adr/051). -->
    <div class="field checkbox-field">
      <label>
        <input type="checkbox" bind:checked={subjectRecusal} />
        People don't vote on proposals about themselves
      </label>
      <p class="recusal-hint muted">
        Someone nominated for admin sits their own vote out. They stay in the
        electorate for everything else.
      </p>
    </div>
  {/if}

  <div class="field">
    <label for="re-venue">Where admins are chosen</label>
    <select id="re-venue" bind:value={leadershipVenue}>
      <option value="patchwork">In Patchwork</option>
      <option value="elsewhere">Somewhere else, recorded here</option>
    </select>
    <p class="venue-hint muted">
      Pick the second if your board is elected at a meeting, on paper, or in
      another tool. Patchwork will stop conducting leadership changes and let
      an admin record what was decided.
    </p>
  </div>

  <div class="field">
    <label for="re-proposal-venue">Where proposals are decided</label>
    <select id="re-proposal-venue" bind:value={proposalVenue}>
      <option value="patchwork">In Patchwork</option>
      <option value="elsewhere">Somewhere else, recorded here</option>
    </select>
    <p class="venue-hint muted">
      Pick the second if your members decide at meetings. Proposals stay open
      for discussion and lose the vote buttons, and an admin records the
      adopted text on the charter afterwards. Rules changes stay a direct
      change an admin applies.
    </p>
  </div>

  <div class="field">
    <label for="re-succession">Succession Policy</label>
    <select id="re-succession" bind:value={successionPolicy}>
      {#each SUCCESSION_OPTIONS as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  {#if !adminDecides}
    <div class="field">
      <label for="re-tenure">Minimum Voting Tenure</label>
      <select id="re-tenure" bind:value={minVotingTenureDays}>
        {#each TENURE_OPTIONS as opt}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
    </div>
  {/if}

  <div class="field">
    <label for="re-membership">Membership Policy</label>
    <select id="re-membership" bind:value={membershipPolicy}>
      {#each MEMBERSHIP_OPTIONS as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  <div class="field">
    <label class="section-label">Follower Permissions</label>
    <div class="checkbox-group">
      <label><input type="checkbox" bind:checked={followerEvents} /> Events</label>
      <label><input type="checkbox" bind:checked={followerProposals} /> Proposals</label>
      <label><input type="checkbox" bind:checked={followerCharters} /> Charters</label>
      <label><input type="checkbox" bind:checked={followerMembers} /> Members</label>
    </div>
  </div>
</div>

<style>
  .rules-editor {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .field label {
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .field select,
  .field input[type="number"] {
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 0.88rem;
    font-family: inherit;
  }

  .field select:focus,
  .field input:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  .venue-hint {
    font-size: 0.78rem;
    margin: 0.35rem 0 0;
  }

  .recusal-hint {
    font-size: 0.78rem;
    margin: 0.3rem 0 0 1.5rem;
  }

  .checkbox-field label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
    color: var(--color-text);
    cursor: pointer;
  }

  .checkbox-field input[type="checkbox"] {
    width: 1rem;
    height: 1rem;
  }

  .section-label {
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--color-text-muted);
    margin-bottom: 0.25rem;
  }

  .checkbox-group {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
    color: var(--color-text);
    cursor: pointer;
  }

  .checkbox-group input[type="checkbox"] {
    width: 1rem;
    height: 1rem;
  }
</style>
