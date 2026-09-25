<script>
  let { currentRules = null, electorate = null, onSave = () => {} } = $props();

  // The option wording is shared with the read-only rules page
  // (web/src/lib/governanceRules.js). Two surfaces naming one setting two
  // ways is the drift that module exists to prevent (F-117).
  import {
    DECISION_OPTIONS,
    THRESHOLD_OPTIONS,
    VOTING_PERIOD_OPTIONS,
    SUCCESSION_OPTIONS,
    TENURE_OPTIONS,
    DECISION_HINTS,
    MEMBERSHIP_OPTIONS,
  } from '../lib/governanceRules.js';

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

  // A stored value this editor has no option for still round-trips: it is
  // shown as itself and left alone, never swapped for the first option. The
  // rules file is whole-document (CLAUDE.md: a field this drops is a field
  // the next unrelated edit resets).
  let successionOptions = $derived(
    SUCCESSION_OPTIONS.some((o) => o.value === successionPolicy)
      ? SUCCESSION_OPTIONS
      : [...SUCCESSION_OPTIONS, { value: successionPolicy, label: successionPolicy, hint: '' }]
  );
  let successionHint = $derived(successionOptions.find((o) => o.value === successionPolicy)?.hint || '');

  // What these numbers come to for this patch today (docs/adr/104).
  //
  // The arithmetic is the server's, mirrored: `votesNeeded` is
  // `votesNeededForQuorum`, and the tenure rule is `effectiveTenureDays` —
  // a patch younger than its own bar has no bar (docs/adr/098). A sentence
  // here that the gate does not honour would be worse than no sentence.
  let memberCount = $derived(electorate?.members ?? 0);
  let tenures = $derived(electorate?.tenure_days || []);
  let patchAgeDays = $derived(electorate?.patch_age_days ?? 0);
  let tenureInForce = $derived(minVotingTenureDays > 0 && patchAgeDays >= minVotingTenureDays);
  let canVoteToday = $derived(
    tenureInForce ? tenures.filter((d) => d >= minVotingTenureDays).length : memberCount,
  );
  let votesNeeded = $derived(
    quorumPercent <= 0 || canVoteToday <= 0
      ? 0
      : Math.min(canVoteToday, Math.ceil((canVoteToday * quorumPercent) / 100)),
  );

  let decisionHint = $derived(DECISION_HINTS[decisionMethod] || '');

  let quorumHint = $derived.by(() => {
    if (!electorate) return '';
    if (quorumPercent <= 0) {
      return 'No quorum: however few people vote, the result stands.';
    }
    if (canVoteToday === 0) {
      return 'Nobody can vote here today, so no quorum can be met and a proposal would close with nothing decided.';
    }
    // "1 of the 1 person" is arithmetic, not a sentence. On a patch of one
    // every quorum comes to the same thing, so say that instead.
    if (canVoteToday === 1) {
      return 'One person can vote here today, so that single ballot is the whole quorum. An abstention counts toward it.';
    }
    return `${votesNeeded} of the ${canVoteToday} people who can vote today must cast a ballot, or a proposal closes with nothing decided. An abstention counts toward it.`;
  });

  let tenureHint = $derived.by(() => {
    if (minVotingTenureDays <= 0) return 'Everybody votes from the day they join.';
    if (!electorate) return '';
    if (!tenureInForce) {
      // "0 days old" is what the number says and not what a person would.
      const age =
        patchAgeDays === 0 ? 'was made today'
        : patchAgeDays === 1 ? 'is a day old'
        : `is ${patchAgeDays} days old`;
      return `Not in force yet: this patch ${age}, younger than the wait it asks for, so every member can vote until it is ${minVotingTenureDays} days old.`;
    }
    const held = memberCount - canVoteToday;
    if (held === 0) {
      return `All ${memberCount} of your members have been here ${minVotingTenureDays} days, so all of them can vote.`;
    }
    return `${canVoteToday} of your ${memberCount} members have been here ${minVotingTenureDays} days. The other ${held} cannot vote yet, and ${held === 1 ? 'is' : 'are'} not counted toward a quorum.`;
  });

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
      // Charters alone reads `=== true`, matching the server's default
      // (docs/adr/116): the other three are on unless a patch turned them
      // off, this one is off unless a patch turned it on. Reading it the
      // same way as its neighbours is what made a patch that had never
      // chosen show the box ticked.
      followerCharters = fp.charters === true;
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
    {#if decisionHint}
      <p class="venue-hint muted">{decisionHint}</p>
    {/if}
  </div>

  {#if !adminDecides}
    <div class="field">
      <label for="re-quorum">Quorum (%)</label>
      <input id="re-quorum" type="number" min="0" max="100" bind:value={quorumPercent} />
      {#if quorumHint}
        <p class="venue-hint muted">{quorumHint}</p>
      {/if}
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
      {#each successionOptions as opt (opt.value)}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
    {#if successionHint}
      <p class="venue-hint muted">{successionHint}</p>
    {/if}
  </div>

  {#if !adminDecides}
    <div class="field">
      <label for="re-tenure">Minimum Voting Tenure</label>
      <select id="re-tenure" bind:value={minVotingTenureDays}>
        {#each TENURE_OPTIONS as opt}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
      {#if tenureHint}
        <p class="venue-hint muted">{tenureHint}</p>
      {/if}
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
      <p class="follower-hint muted">Off: followers see this patch's public events only.</p>
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

  .follower-hint {
    font-size: 0.78rem;
    margin: 0.15rem 0 0.4rem 1.5rem;
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
