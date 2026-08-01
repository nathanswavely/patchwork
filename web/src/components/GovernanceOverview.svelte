<script>
  import { CheckSquare, UsersThree } from 'phosphor-svelte';
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { formatDay } from '../lib/datetime.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, getUser } from '../stores/auth.svelte.js';
  import { markGovernanceHubVisited } from '../lib/onboarding.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from './PasskeyNotice.svelte';
  import AttestationRecords from './AttestationRecords.svelte';
  import Skeleton from './Skeleton.svelte';
  import UnlockPanel from './UnlockPanel.svelte';
  import SetupChecklist from './SetupChecklist.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);
  let nodeId = $derived(patch.value.node?.id);

  // Not `isMember`: the node payload sets is_member for followers too, and
  // proposing a rules change is a member act — following carries no
  // governance rights.
  let canPropose = $derived(membershipRole === 'member' || membershipRole === 'admin');

  // Setup checklist fallback (docs/adr/040, CONTEXT.md "Setup checklist"):
  // "decide how you govern" has no single derivable signal for a patch
  // that never amends anything, so an admin actually reaching this page
  // counts as having decided.
  $effect(() => {
    if (isAdmin && nodeId) markGovernanceHubVisited(getUser()?.id, nodeId);
  });

  let overview = $state(null);
  let loading = $state(true);

  $effect(() => {
    if (slug) loadOverview();
  });

  async function loadOverview() {
    loading = true;
    try {
      overview = await api(`nodes/${slug}/governance/overview`);
      successorChoice = overview?.successor?.user_id || '';
    } catch {
      overview = null;
    } finally {
      loading = false;
    }
  }

  // Succession, maintainer model only (docs/adr/051). A maintainer names who
  // inherits the patch; meritocratic ratifies a nomination and elected runs a
  // cycle, so neither sees any of this.
  let isMaintainerModel = $derived(overview?.rules?.leadership_model === 'maintainer');
  // Where this patch actually chooses its admins (docs/adr/052). Elsewhere
  // means Patchwork conducts nothing and records what the community decided.
  let leadershipElsewhere = $derived(overview?.rules?.leadership_venue === 'elsewhere');
  // Where this patch decides the things proposals are about (docs/adr/053).
  // Elsewhere, a rules change is a direct change an admin applies — the rules
  // file is machine configuration, not a text a meeting adopts — so a member
  // gets told that rather than a link the server refuses.
  let proposalsElsewhere = $derived(overview?.rules?.proposal_venue === 'elsewhere');

  // The contest this patch is running, and when its council next faces the
  // electorate (docs/adr/051). Both come from the server rather than being
  // worked out here from dates and a status — the same reason `election_phase`
  // does on the proposal page.
  let election = $derived(overview?.election || null);
  let nextTermEnd = $derived(overview?.next_term_end || '');

  // A term that has run out removes nobody: the council serves until a
  // successor is elected (docs/adr/051). What it changes is that the patch is
  // visibly overdue, which is the accountability "power rotates" is promising.
  let termLapsed = $derived.by(() => {
    if (!nextTermEnd) return false;
    const parts = /^(\d{4})-(\d{2})-(\d{2})$/.exec(nextTermEnd);
    const end = parts
      ? new Date(Number(parts[1]), Number(parts[2]) - 1, Number(parts[3]))
      : new Date(nextTermEnd);
    return end < new Date();
  });
  let successorChoice = $state('');
  let savingSuccessor = $state(false);
  let successorError = $state('');
  let patchMembers = $state([]);

  // A successor has to already be in the patch, and cannot be the person
  // naming them — the same two conditions the server enforces.
  let eligibleSuccessors = $derived(
    patchMembers.filter((m) => (m.role === 'member' || m.role === 'admin') && m.user_id !== getUser()?.id)
  );

  // Read the passkey state on load so a missing one shows up *before*
  // someone picks a name and hits a wall they could not see.
  let hasPasskey = $state(true);

  $effect(() => {
    if (isAdmin && isMaintainerModel && slug) {
      loadPatchMembers();
      stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
    }
  });

  async function loadPatchMembers() {
    try {
      const data = await api(`nodes/${slug}/members`);
      patchMembers = data.items || data || [];
    } catch {
      patchMembers = [];
    }
  }

  async function saveSuccessor() {
    savingSuccessor = true;
    successorError = '';
    try {
      if (successorChoice) {
        // Naming a successor decides who inherits the patch, so the server
        // asks for a fresh passkey the way promotion does (docs/adr/017).
        // Clearing is the safe direction and goes through untouched.
        await withStepUp(() => api(`nodes/${slug}/successor`, { method: 'PUT', body: { user_id: successorChoice } }));
      } else {
        await api(`nodes/${slug}/successor`, { method: 'DELETE' });
      }
      await loadOverview();
    } catch (e) {
      successorError = e instanceof PasskeyRequiredError
        ? 'Naming a successor needs a passkey. Enroll one in Security settings first.'
        : e.message || 'Failed to save';
    } finally {
      savingSuccessor = false;
    }
  }

  // Human-readable descriptions.
  function describeDecisionMethod(rules) {
    if (!rules) return '';
    // A patch that decides its proposals elsewhere runs none of this. Reciting
    // its quorum and voting period would be docs/adr/049's failure — stating
    // what isn't enforced — reintroduced by the feature written to end it, and
    // the venue line below says what actually happens. The rules still exist
    // and still govern the votes this patch is not holding, which is why they
    // stay editable and are simply not narrated.
    if (rules.proposal_venue === 'elsewhere') return '';
    const methods = {
      admin: 'The maintainer makes all decisions for this patch.',
      majority: 'Your patch decides things by majority vote. More than half must agree.',
      supermajority: 'Decisions require a supermajority: at least 2 out of 3 voters must agree.',
      consensus: 'Decisions require consensus. Everyone, or nearly everyone, must agree.',
    };
    let desc = methods[rules.decision_method] || `Decisions use ${rules.decision_method} voting.`;

    if (rules.quorum_percent > 0) {
      desc += ` At least ${rules.quorum_percent}% of members must participate for a vote to count.`;
    } else if (rules.decision_method !== 'admin') {
      desc += ' Any number of votes counts. No minimum participation required.';
    }

    if (rules.decision_method !== 'admin' && rules.default_vote_duration_hours > 0) {
      const days = Math.round(rules.default_vote_duration_hours / 24);
      desc += ` Proposals stay open for ${days <= 1 ? rules.default_vote_duration_hours + ' hours' : days + ' days'}.`;
    }

    return desc;
  }

  function describeLeadership(rules) {
    if (!rules) return '';
    // A patch that chooses its admins elsewhere does not run any of these
    // mechanics, so describing one would be the same false narration
    // docs/adr/049 was written about. The venue line and the records say what
    // actually happens.
    if (rules.leadership_venue === 'elsewhere') return '';
    const models = {
      maintainer: 'One person maintains this patch. They handle day-to-day decisions and can designate a successor.',
      meritocratic: 'Admins earn their role through sustained contribution. When a seat opens, existing admins nominate from active members and the community ratifies.',
      elected: 'The community elects admins for fixed terms. Regular elections ensure power rotates.',
    };
    let desc = models[rules.leadership_model] || '';

    if (rules.admin_term_months > 0) {
      desc += ` Terms last ${rules.admin_term_months} months.`;
    }
    if (rules.inactivity_days > 0) {
      desc += ` Admins inactive for ${rules.inactivity_days} days may be asked to step down.`;
    }

    return desc;
  }

  function leadershipLabel(model) {
    const labels = { maintainer: 'Maintainer', meritocratic: 'Meritocratic', elected: 'Elected Council' };
    return labels[model] || model || 'Not set';
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', year: 'numeric' });
  }

  let rules = $derived(overview?.rules ? (typeof overview.rules === 'string' ? JSON.parse(overview.rules) : overview.rules) : null);
</script>

<div class="governance-overview">
  <!-- Onboarding panels (docs/adr/040) live inside the overview pane, the
       workspace's landing view — self-gating, each renders nothing when it
       doesn't apply. -->
  <UnlockPanel />
  <SetupChecklist />
  {#if loading}
    <Skeleton lines={6} height="1rem" />
  {:else if !overview}
    <p class="muted">Failed to load governance overview.</p>
  {:else}
    <!-- Action banner — needs your vote -->
    {#if overview.needs_vote > 0}
      <div class="attention-banner">
        <span class="attention-icon">
          <CheckSquare size={16} weight="duotone" />
        </span>
        <div>
          <strong>{overview.needs_vote} proposal{overview.needs_vote > 1 ? 's' : ''} need{overview.needs_vote === 1 ? 's' : ''} your vote</strong>
          <a href="/patches/{slug}/governance/proposals" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/proposals`); }}>
            Review & vote &rarr;
          </a>
        </div>
      </div>
    {/if}

    <!-- An election taking nominations (docs/adr/051). The needs-a-vote banner
         above deliberately stays quiet through this phase — nominations are
         not a ballot — so without this the hub said nothing for the whole
         fortnight, which is the only stretch when standing is possible. -->
    {#if election?.phase === 'nominating'}
      <div class="attention-banner">
        <span class="attention-icon">
          <UsersThree size={16} weight="duotone" />
        </span>
        <div>
          <strong>
            Nominations are open for {election.seats} seat{election.seats === 1 ? '' : 's'}
          </strong>
          <span class="banner-detail muted">
            {election.candidates === 0
              ? 'Nobody has stood yet.'
              : `${election.candidates} standing.`}
            {#if election.nominations_close_at}
              Closing {formatDay(election.nominations_close_at)}.
            {/if}
          </span>
          <a href="/patches/{slug}/governance/{election.id}" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/${election.id}`); }}>
            {canPropose ? 'Stand, or put someone forward' : 'See who is standing'} &rarr;
          </a>
        </div>
      </div>
    {/if}

    <!-- Decision making -->
    <section class="overview-section">
      <h3>How decisions are made</h3>
      {#if describeDecisionMethod(rules)}
        <p class="overview-narrative">{describeDecisionMethod(rules)}</p>
      {/if}
      {#if proposalsElsewhere}
        <p class="overview-narrative">
          Proposals are decided outside Patchwork. They stay open here for
          discussion, and what the meeting adopts is recorded on the charter.
        </p>
        {#if membershipRole === 'admin'}
          <a class="section-action" href="/patches/{slug}/governance/rules/propose" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/rules/propose`); }}>
            Change these rules
          </a>
        {:else if canPropose}
          <p class="venue-line muted">An admin changes these rules directly.</p>
        {/if}
      {:else if membershipRole === 'admin' && rules?.decision_method === 'admin'}
        <a class="section-action" href="/patches/{slug}/governance/rules/propose" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/rules/propose`); }}>
          Change these rules
        </a>
      {:else if canPropose}
        <a class="section-action" href="/patches/{slug}/governance/rules/propose" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/rules/propose`); }}>
          Propose a change to these rules
        </a>
      {/if}
    </section>

    <!-- Leadership -->
    <section class="overview-section">
      <h3>Leadership: {leadershipLabel(rules?.leadership_model)}</h3>
      {#if describeLeadership(rules)}
        <p class="overview-narrative">{describeLeadership(rules)}</p>
      {/if}

      {#if overview.admins.length > 0}
        <div class="admin-list">
          {#each overview.admins as admin}
            <div class="admin-item">
              <div class="admin-avatar">
                {#if admin.avatar_url}
                  <img src={admin.avatar_url} alt="" />
                {:else}
                  {(admin.display_name || admin.username || '?')[0].toUpperCase()}
                {/if}
              </div>
              <div class="admin-info">
                <span class="admin-name">{admin.display_name || admin.username}</span>
                <!-- `joined_at` is when this person joined the patch, in
                     whatever role they joined as — not when they became an
                     admin, which nothing records. Labelled "Admin since" it
                     read as a governed fact and was routinely false: a member
                     of eight months elected to the council this morning was
                     shown as an admin since eight months ago. -->
                <span class="admin-since muted">Member since {formatDate(admin.joined_at)}</span>
              </div>
            </div>
          {/each}
        </div>
      {/if}

      <!-- When this council next faces the electorate. The rules narration
           above says terms last N months, which is the policy; this is the
           date, which is the accountability. Absent on a patch that sets no
           term length — elected once, then stable, a real position. -->
      {#if nextTermEnd}
        <p class="term-line" class:lapsed={termLapsed}>
          {#if termLapsed}
            This council's term ended {formatDay(nextTermEnd)}. It serves until a successor is elected.
          {:else}
            Next seat comes up {formatDay(nextTermEnd)}.
          {/if}
        </p>
      {/if}

      <!-- The ballot, once nominations have closed. The needs-a-vote banner
           already carries this for anyone who may vote; this line is for
           everyone else, so a member outside the electorate still knows the
           council is being decided this week. -->
      {#if election?.phase === 'voting'}
        <p class="term-line">
          A ballot is open for {election.seats} seat{election.seats === 1 ? '' : 's'}.
          <a href="/patches/{slug}/governance/{election.id}" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/${election.id}`); }}>
            See the candidates
          </a>
        </p>
      {/if}

      <!-- Chosen elsewhere (docs/adr/052): the records are how leadership
           changes here, so they sit with the admin list rather than on a
           page of their own. -->
      {#if leadershipElsewhere}
        <p class="venue-line muted">
          Admins are chosen outside Patchwork. What the community decided is
          recorded below.
        </p>
        <AttestationRecords {slug} isAdmin={isAdmin} hasTerms={overview?.rules?.leadership_model === 'elected'} />
      {/if}

      <!-- Succession, maintainer model only (docs/adr/051). The other two
           models fill admin seats their own way and this section stays away
           from them. A patch that decides elsewhere has no successor to name:
           the record is what moves the role. -->
      {#if isMaintainerModel && !leadershipElsewhere}
        <div class="successor">
          {#if overview.successor?.user_id}
            <p class="successor-line">
              Successor: <strong>{overview.successor.display_name || overview.successor.username}</strong>
              — if the current admin steps away, the patch passes to them.
            </p>
          {:else}
            <p class="successor-line muted">No successor named.</p>
          {/if}

          {#if isAdmin}
            <div class="successor-controls">
              <select bind:value={successorChoice} disabled={savingSuccessor}>
                <option value="">Nobody</option>
                {#each eligibleSuccessors as m}
                  <option value={m.user_id}>{m.display_name || m.username}</option>
                {/each}
              </select>
              <button class="btn btn-sm" onclick={saveSuccessor} disabled={savingSuccessor || successorChoice === (overview.successor?.user_id || '')}>
                {savingSuccessor ? 'Saving...' : 'Save'}
              </button>
            </div>
            <PasskeyNotice show={!hasPasskey} action="name a successor" />
            {#if successorError}
              <p class="successor-error">{successorError}</p>
            {/if}
            <p class="successor-hint muted">
              Naming a successor is also what lets you leave a patch you run alone.
            </p>
          {/if}
        </div>
      {/if}
    </section>

    <!-- Quick stats -->
    <section class="overview-section stats-row">
      <a class="stat-link" href="/patches/{slug}/governance/docs" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs`); }}>
        {overview.document_count} document{overview.document_count !== 1 ? 's' : ''}
      </a>
      <span class="stat-sep">&middot;</span>
      <a class="stat-link" href="/patches/{slug}/governance/proposals" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/proposals`); }}>
        {overview.open_proposals} open proposal{overview.open_proposals !== 1 ? 's' : ''}
      </a>
      <span class="stat-sep">&middot;</span>
      <span class="muted">{overview.member_count} member{overview.member_count !== 1 ? 's' : ''}</span>
    </section>
  {/if}
</div>

<style>
  .attention-banner {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
    padding: 0.75rem 1rem;
    border: 1px solid var(--color-accent);
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--color-accent) 5%, var(--color-surface));
    margin-bottom: 1.5rem;
  }

  .attention-icon {
    color: var(--color-accent);
    flex-shrink: 0;
    padding-top: 0.1rem;
  }

  .attention-banner strong {
    display: block;
    font-size: 0.88rem;
    margin-bottom: 0.15rem;
  }

  .attention-banner a {
    font-size: 0.82rem;
    color: var(--color-primary);
    text-decoration: none;
  }

  .attention-banner a:hover {
    text-decoration: underline;
  }

  .overview-section {
    padding: 1rem 0;
  }

  .overview-section + .overview-section {
    border-top: 1px solid var(--color-border);
  }

  .overview-section h3 {
    font-size: 0.88rem;
    font-weight: 600;
    margin-bottom: 0.4rem;
  }

  .overview-narrative {
    font-size: 0.88rem;
    line-height: 1.6;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  .section-action {
    font-size: 0.82rem;
    color: var(--color-primary);
    text-decoration: none;
  }

  .section-action:hover {
    text-decoration: underline;
  }

  .admin-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 0.75rem;
  }

  .admin-item {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .admin-avatar {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
    font-size: 0.78rem;
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    flex-shrink: 0;
  }

  .admin-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .admin-info {
    display: flex;
    flex-direction: column;
    gap: 0.05rem;
  }

  .admin-name {
    font-size: 0.88rem;
    font-weight: 500;
  }

  .admin-since {
    font-size: 0.75rem;
  }

  .term-line {
    font-size: 0.82rem;
    margin: 0.6rem 0 0;
    color: var(--color-text-muted);
  }

  .term-line.lapsed {
    color: var(--color-accent, #b8860b);
    font-weight: 600;
  }

  .banner-detail {
    display: block;
    font-size: 0.82rem;
    margin-bottom: 0.15rem;
  }

  .venue-line {
    font-size: 0.82rem;
    margin: 0.6rem 0 0;
  }

  .successor {
    margin-top: 0.85rem;
    padding-top: 0.85rem;
    border-top: 1px solid var(--color-border);
  }

  .successor-line {
    font-size: 0.85rem;
    margin: 0;
  }

  .successor-controls {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .successor-controls select {
    flex: 1 1 12rem;
    min-width: 0;
  }

  .successor-error {
    font-size: 0.8rem;
    color: var(--color-danger, #c0392b);
    margin: 0.4rem 0 0;
  }

  .successor-hint {
    font-size: 0.78rem;
    margin: 0.4rem 0 0;
  }

  .stats-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.82rem;
  }

  .stat-link {
    color: var(--color-primary);
    text-decoration: none;
  }

  .stat-link:hover {
    text-decoration: underline;
  }

  .stat-sep {
    color: var(--color-text-muted);
  }
</style>
