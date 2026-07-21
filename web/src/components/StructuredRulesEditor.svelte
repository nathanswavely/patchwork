<script>
  let { currentRules = null, onSave = () => {} } = $props();

  const DECISION_OPTIONS = [
    { value: 'admin_decides', label: 'Admin decides' },
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
  let quorumPercent = $state(25);
  let votingPeriodHours = $state(72);
  let amendmentThreshold = $state('supermajority');
  let autoApply = $state(true);
  let successionPolicy = $state('longest_tenure');
  let minVotingTenureDays = $state(0);
  let membershipPolicy = $state('open');
  let followerEvents = $state(true);
  let followerProposals = $state(true);
  let followerCharters = $state(true);
  let followerMembers = $state(true);

  // Initialize from currentRules
  $effect(() => {
    if (currentRules) {
      decisionMethod = currentRules.decision_method || 'majority';
      quorumPercent = currentRules.quorum_percent ?? currentRules.quorum ?? 25;
      votingPeriodHours = currentRules.voting_period_hours ?? currentRules.default_vote_duration_hours ?? 72;
      amendmentThreshold = currentRules.amendment_threshold ?? currentRules.amendment_method ?? 'supermajority';
      autoApply = currentRules.auto_apply ?? currentRules.amendment_auto_apply ?? true;
      successionPolicy = currentRules.succession_policy ?? currentRules.succession_rule ?? 'longest_tenure';
      minVotingTenureDays = currentRules.min_voting_tenure_days ?? 0;
      membershipPolicy = currentRules.membership_policy ?? 'open';
      const fp = currentRules.follower_permissions || {};
      followerEvents = fp.events !== false;
      followerProposals = fp.proposals !== false;
      followerCharters = fp.charters !== false;
      followerMembers = fp.members !== false;
    }
  });

  function buildRules() {
    return {
      decision_method: decisionMethod,
      quorum_percent: quorumPercent,
      voting_period_hours: votingPeriodHours,
      amendment_threshold: amendmentThreshold,
      auto_apply: autoApply,
      succession_policy: successionPolicy,
      min_voting_tenure_days: minVotingTenureDays,
      membership_policy: membershipPolicy,
      follower_permissions: {
        events: followerEvents,
        proposals: followerProposals,
        charters: followerCharters,
        members: followerMembers,
      },
    };
  }

  function handleSave() {
    onSave(buildRules());
  }

  // Auto-notify parent on every change
  $effect(() => {
    // Touch all reactive values to track them
    decisionMethod; quorumPercent; votingPeriodHours; amendmentThreshold;
    autoApply; successionPolicy; minVotingTenureDays; membershipPolicy;
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
      {#each DECISION_OPTIONS as opt}
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

  <div class="field">
    <label for="re-succession">Succession Policy</label>
    <select id="re-succession" bind:value={successionPolicy}>
      {#each SUCCESSION_OPTIONS as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  <div class="field">
    <label for="re-tenure">Minimum Voting Tenure</label>
    <select id="re-tenure" bind:value={minVotingTenureDays}>
      {#each TENURE_OPTIONS as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

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
