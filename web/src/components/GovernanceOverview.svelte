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
  // An admin *of this patch*. The node payload sets is_admin for instance
  // admins too, and arranging a patch's council is not theirs to do — the
  // seat routes answer them 404.
  let isPatchAdmin = $derived(membershipRole === 'admin');

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

  // The council's chairs (docs/adr/100). The admin list above says who holds
  // power; this says how many positions exist, which is the number the next
  // contest contests and the number a member is asking about when they
  // wonder how to get onto the council.
  let isElectedModel = $derived(overview?.rules?.leadership_model === 'elected');
  let showsCouncil = $derived(isElectedModel && !leadershipElsewhere);
  let seats = $derived(overview?.seats || []);
  let vacantSeats = $derived(seats.filter((s) => s.vacant));
  let nextContestOpens = $derived(overview?.next_contest_opens || '');
  let contestDue = $derived.by(() => {
    if (!nextContestOpens) return false;
    const parts = /^(\d{4})-(\d{2})-(\d{2})$/.exec(nextContestOpens);
    const opens = parts
      ? new Date(Number(parts[1]), Number(parts[2]) - 1, Number(parts[3]))
      : new Date(nextContestOpens);
    return opens <= new Date();
  });

  // One sentence per chair, about that chair. This page used to carry three
  // general facts side by side — what an elected patch is, how a vacancy is
  // filled, when the next seat comes up — all true, all about different
  // chairs at different times, and a member looking at two empty seats could
  // not tell which one applied to them. The server picks the route per seat
  // (`fill`); this puts it into words.
  function seatLine(seat) {
    if (seat.fill === 'contest_open') {
      return seat.vacant
        ? 'In the contest running now. Whoever the members approve takes it.'
        : `Held until ${formatDay(seat.term_ends_at)}, and in the contest running now.`;
    }
    if (seat.fill === 'nomination') {
      const how = 'Filled by nomination: an admin puts a member forward and the members ratify it. That can happen today.';
      // And the chair's own term, because whoever is nominated inherits it
      // rather than starting a fresh one (docs/adr/051), and because it is
      // the date an admin just set on this row.
      return seat.term_ends_at
        ? `${how} Whoever takes it serves out the term, to ${formatDay(seat.term_ends_at)}.`
        : how;
    }
    if (seat.fill === 'contest_scheduled') {
      return seat.contest_due
        ? `Term ended ${formatDay(seat.term_ends_at)}. The election for it is due now, and the holder serves until a successor is elected.`
        : `Term ends ${formatDay(seat.term_ends_at)}. Contested at the election that opens ${formatDay(seat.contest_opens)}.`;
    }
    if (seat.term_ends_at) {
      return `Term ends ${formatDay(seat.term_ends_at)}. No election is scheduled.`;
    }
    return 'No term end, so nothing brings this seat up for election.';
  }

  // What the reader can do about this council today — including, when it is
  // the honest answer, nothing until a date.
  let councilAction = $derived.by(() => {
    if (!showsCouncil) return '';
    if (election?.phase === 'nominating') {
      return canPropose
        ? 'Nominations are open. You can stand, or put another member forward.'
        : 'Nominations are open. Members of this patch can stand for a seat.';
    }
    if (election?.phase === 'voting') {
      return 'The ballot is open. The members are choosing between the candidates.';
    }
    if (vacantSeats.length > 0) {
      if (isPatchAdmin) {
        return 'You can nominate a member for a vacant seat today. The members ratify it, and the appointee serves out that seat\u2019s term.';
      }
      // A follower is not eligible to be nominated \u2014 a nominee must be an
      // active member \u2014 so "ask one to nominate you" would send them at a
      // door the server closes.
      return canPropose
        ? 'Only an admin can put a name forward for a vacant seat. Ask one to nominate you.'
        : 'A vacant seat is filled by nomination, and only this patch\u2019s members can be nominated for one.';
    }
    if (!nextContestOpens) {
      return 'Every seat is held and no election is scheduled. Nothing changes on this council until a seat is added or a term end is set.';
    }
    return contestDue
      ? 'Every seat is held and the next contest is due now. It opens shortly, and any member may stand.'
      : `Every seat is held. There is nothing to do until ${formatDay(nextContestOpens)}, when the next contest opens and any member may stand.`;
  });

  // A patch with nobody in the admin role, which inactivity can reach without
  // anybody choosing it (docs/adr/051: the vacancy rule may empty a patch).
  // Left unsaid, the page shows a governance section with no admins listed
  // and no explanation, and a member's first sign is that nothing works.
  //
  // What refills it differs by model, so the second sentence does too — and
  // on an elected patch it stops at "by election", because the council block
  // below says which chairs and when, per chair.
  //
  // Never on an unclaimed patch: it has no admins because nobody has claimed
  // it, which is a different sentence and one the claim surfaces already say.
  // PatchShell redirects the governance tab away from an unclaimed patch, so
  // this only covers the frame before that effect runs — but that frame would
  // otherwise carry the wrong answer.
  let noAdmins = $derived(
    (overview?.admins?.length ?? 0) === 0 && !patch.value.isUnclaimed,
  );
  let noAdminsLine = $derived.by(() => {
    if (leadershipElsewhere)
      return 'Nobody holds the admin role here. This patch records its leadership decisions elsewhere, so a decision made there is what puts somebody back.';
    if (isElectedModel) return 'Nobody holds the admin role here. This council is filled by election.';
    if (overview?.rules?.leadership_model === 'meritocratic')
      return 'Nobody holds the admin role here. An admin is nominated by an admin and ratified by the members, so this patch cannot start one on its own until an instance admin puts somebody back.';
    return 'Nobody holds the admin role here, so nobody can manage this patch until an instance admin puts somebody back.';
  });

  let seatBusy = $state(false);
  let seatError = $state('');

  // The election calendar, next to the chairs (docs/adr/051: the clock
  // belongs to the seat). A vacant chair takes any future date. A held one
  // can only be brought forward — pushing it back would hand out a term
  // nobody voted for, which is what 051 put the clock on the seat to
  // prevent — so the form offers the control and the server holds the rule.
  let editingSeat = $state('');
  let termDraft = $state('');

  function editTerm(seat) {
    editingSeat = seat.id;
    termDraft = seat.term_ends_at || '';
    seatError = '';
  }

  async function saveTerm(seatId) {
    seatBusy = true;
    seatError = '';
    try {
      await api(`nodes/${slug}/seats/${seatId}`, { method: 'PATCH', body: { term_ends_at: termDraft } });
      editingSeat = '';
      await loadOverview();
    } catch (e) {
      seatError = e.message || 'Could not set the term end';
    } finally {
      seatBusy = false;
    }
  }

  async function addSeat() {
    seatBusy = true;
    seatError = '';
    try {
      await api(`nodes/${slug}/seats`, { method: 'POST' });
      await loadOverview();
    } catch (e) {
      seatError = e.message || 'Could not add a seat';
    } finally {
      seatBusy = false;
    }
  }

  async function removeSeat(seatId) {
    seatBusy = true;
    seatError = '';
    try {
      await api(`nodes/${slug}/seats/${seatId}`, { method: 'DELETE' });
      await loadOverview();
    } catch (e) {
      seatError = e.message || 'Could not remove the seat';
    } finally {
      seatBusy = false;
    }
  }

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
      admin: 'The maintainer makes all decisions for this patch, and may ask the members before deciding.',
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
          Proposals are decided outside Patchwork. They stay open here for discussion, and adoption is recorded on the charter.
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
      <!-- The model's general description, withheld where the council block
           below is about to say the same thing about the actual chairs, with
           dates. Kept, it was the first of three true-but-unhelpful sentences
           one screen was carrying: "the community elects admins for fixed
           terms" answers a question nobody with two empty chairs in front of
           them is asking. -->
      {#if !showsCouncil && describeLeadership(rules)}
        <p class="overview-narrative">{describeLeadership(rules)}</p>
      {/if}

      {#if !noAdmins}
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
                     admin. Labelled "Admin since" it read as a governed fact
                     and was routinely false: a member of eight months elected
                     to the council this morning was shown as an admin since
                     eight months ago. Migration 072 does now record when a
                     role was taken, but only for roles taken since it landed,
                     so every admin seated before that would still be shown a
                     date derived from their joining. The honest label is the
                     one about joining, and the seats below carry the governed
                     dates. -->
                <span class="admin-since muted">Member since {formatDate(admin.joined_at)}</span>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <p class="no-admins">{noAdminsLine}</p>
      {/if}

      <!-- The council's chairs (docs/adr/100), each saying what is true of
           *it*. The list above says who holds power; this says what happens
           to every position and when. One screen used to carry the three
           general sentences instead — what an elected patch is, how a
           vacancy is filled, when the next seat comes up — and a member
           looking at two empty chairs could not tell which applied to them.
           Adding a chair is an admin's act and filling it is the
           community's, so the controls here never seat anybody. -->
      {#if showsCouncil}
        <div class="council">
          <p class="council-line">
            {#if seats.length === 0}
              This council has no seats yet. Until a seat exists there is nothing to elect and nothing to nominate anyone into.
            {:else}
              {seats.length} seat{seats.length === 1 ? '' : 's'} on the council,
              {vacantSeats.length === 0
                ? 'all held'
                : `${seats.length - vacantSeats.length} held and ${vacantSeats.length} vacant`}.
              {#if rules?.admin_term_months > 0}{` Each term runs ${rules.admin_term_months} months.`}{/if}
            {/if}
          </p>

          {#if seats.length > 0}
            <ul class="seat-list">
              {#each seats as seat}
                <li class="seat-row">
                  <div class="seat-main">
                    <span class="seat-who" class:vacant={seat.vacant}>
                      {seat.vacant ? 'Vacant' : (seat.display_name || seat.username)}
                    </span>
                    <span class="seat-fate muted">{seatLine(seat)}</span>
                    {#if seat.fill === 'contest_open' && seat.contest_id}
                      <a
                        class="seat-contest"
                        href="/patches/{slug}/governance/{seat.contest_id}"
                        onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/${seat.contest_id}`); }}
                      >
                        See the contest
                      </a>
                    {/if}
                  </div>
                  {#if isPatchAdmin}
                    <div class="seat-controls">
                      {#if editingSeat === seat.id}
                        <input type="date" bind:value={termDraft} disabled={seatBusy} aria-label="Term end" />
                        <button class="btn btn-sm btn-primary" disabled={seatBusy || !termDraft} onclick={() => saveTerm(seat.id)}>
                          Save
                        </button>
                        <button class="btn btn-sm" disabled={seatBusy} onclick={() => { editingSeat = ''; }}>Cancel</button>
                      {:else}
                        <button class="btn btn-sm" disabled={seatBusy} onclick={() => editTerm(seat)}>
                          {seat.term_ends_at ? 'Change term end' : 'Set term end'}
                        </button>
                        {#if seat.vacant}
                          <button class="btn btn-sm" disabled={seatBusy} onclick={() => removeSeat(seat.id)}>
                            Remove seat
                          </button>
                        {/if}
                      {/if}
                    </div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}

          <!-- What the reader can do about this council today. Second person,
               one sentence, and where the answer is "nothing until a date"
               it says that rather than leaving three general rules to be
               sorted out by the person who wanted a seat. -->
          {#if councilAction}
            <p class="council-do">{councilAction}</p>
          {/if}

          {#if isPatchAdmin}
            <div class="council-controls">
              <button class="btn btn-sm" disabled={seatBusy} onclick={addSeat}>Add a seat</button>
              <span class="council-hint muted">
                Adding a seat does not make anybody an admin. The community fills it.
              </span>
            </div>
            <p class="council-hint muted">
              A seat's term end is this patch's election calendar. A vacant seat takes any future date; a held one can only be brought
              forward, because pushing it back would extend a term nobody voted for. Nobody loses their seat when a term ends — the
              holder serves until a successor is elected.
            </p>
            {#if seatError}
              <p class="successor-error">{seatError}</p>
            {/if}
          {/if}
        </div>
      {/if}

      <!-- When this council next faces the electorate, for a patch carrying
           seats that the council block above does not render — an elected
           patch that moved its venue elsewhere and kept its chairs. Where
           the block does render, every seat states its own date and this
           line would be the fourth general sentence. -->
      {#if nextTermEnd && !showsCouncil}
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
              Successor: <strong>{overview.successor.display_name || overview.successor.username}</strong>. If the current admin steps away, the patch passes to them.
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

  .council {
    margin-top: 0.85rem;
    padding-top: 0.85rem;
    border-top: 1px solid var(--color-border);
  }

  .council-line {
    font-size: 0.85rem;
    margin: 0;
  }

  .seat-list {
    list-style: none;
    margin: 0.5rem 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .seat-row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.5rem;
    font-size: 0.82rem;
    padding: 0.35rem 0;
  }

  .seat-row + .seat-row {
    border-top: 1px solid var(--color-border);
  }

  .seat-main {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
    flex: 1 1 14rem;
  }

  .seat-who {
    font-weight: 500;
  }

  .seat-who.vacant {
    font-weight: 400;
    color: var(--color-text-muted);
    font-style: italic;
  }

  .seat-fate {
    font-size: 0.78rem;
    line-height: 1.5;
  }

  .seat-contest {
    font-size: 0.78rem;
    color: var(--color-primary);
    text-decoration: none;
  }

  .seat-contest:hover {
    text-decoration: underline;
  }

  .seat-controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem;
  }

  .seat-controls input[type='date'] {
    font-size: 0.78rem;
    min-width: 0;
  }

  .no-admins {
    font-size: 0.85rem;
    line-height: 1.55;
    margin: 0;
    padding-left: 0.6rem;
    border-left: 2px solid var(--color-primary);
  }

  .council-do {
    font-size: 0.82rem;
    line-height: 1.5;
    margin: 0.7rem 0 0;
    padding-left: 0.6rem;
    border-left: 2px solid var(--color-primary);
  }

  .council-controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.6rem;
  }

  .council-hint {
    font-size: 0.78rem;
    margin: 0.4rem 0 0;
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
