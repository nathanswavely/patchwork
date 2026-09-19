<script>
  import { api } from '../lib/api.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { formatDay } from '../lib/datetime.js';

  // An election (docs/adr/051): the one proposal that takes nominations before
  // it takes votes. The phase comes from the server rather than being worked
  // out from two dates here — the same reason `can_vote` does (docs/adr/044).
  let {
    proposal = null,
    canVote = false,
    canNominate = false,
    // Everybody this contest could have on its slate: the patch's active
    // members and admins (docs/adr/107). Empty where the page could not load
    // them, which costs the picker and nothing else.
    members = [],
    onChanged = () => {},
  } = $props();

  let phase = $derived(proposal?.election_phase || '');
  let candidates = $derived(proposal?.candidates || []);
  let seats = $derived(proposal?.seats_contested || 0);
  // A contest that seated nobody (docs/adr/051): quorum unmet, or nobody
  // approved. The panel used to read the seats off the tally alone, so a
  // closed election that missed quorum still tagged its most-approved
  // candidates "seated" — people who hold no seat, on a page whose banner
  // says the council held over.
  let settledNothing = $derived(proposal?.state === 'unsettled');
  // How many took part and how many had to. Turnout is the thing that
  // decides a contest and was the one number its page never showed: a
  // four-person collective failed six in a row, each reported as "Settled
  // nothing", while the ordinary proposal two rows up in the same record
  // said "Quorum met (3 of 4 voted, 50% needed)" and named the voters.
  // Server-side, from the same function that resolves the contest, so the
  // page cannot disagree with the outcome.
  let turnout = $derived(proposal?.election_turnout || null);
  // Whether this contest recorded its own outcome. Contests resolved before
  // the seated column existed have it on nobody, and fall back to the old
  // inference rather than reporting that a contest which seated a council
  // seated none.
  let outcomeStored = $derived(candidates.some((c) => c.seated));
  // Named rather than repeated inline, because the losing rows need its
  // negation and were rendering a blank: a former chair read one row saying
  // "seated" and the row under it saying nothing at all, and sat wondering
  // whether that meant still pending.
  function isSeated(c, i) {
    if (phase !== 'closed' || settledNothing) return false;
    return outcomeStored ? c.seated : i < seats && c.approvals > 0;
  }
  let me = $derived(getUser());
  let iAmStanding = $derived(candidates.some((c) => c.user_id === me?.id));

  // Who is left to put forward: everyone who may stand and is not already on
  // the slate, minus yourself — standing is its own button, and offering your
  // own name in a list called "put someone forward" is how somebody nominated
  // herself by accident in the first place.
  let nominatable = $derived(
    members.filter((m) => m.user_id !== me?.id && !candidates.some((c) => c.user_id === m.user_id)),
  );
  let nomineeId = $state('');

  let busy = $state(false);
  let error = $state('');
  // Whether this person's ballot is in. Seeded from the server rather than
  // only from a save, so it survives a reload — which is how three of four
  // simulated voters tried to find out, having been told nothing
  // (docs/adr/106).
  let saved = $state(false);
  let ballotIn = $derived(saved || candidates.some((c) => c.approved_by_me));

  // The ballot is the set you currently hold, seeded from what the server says
  // you already approved — approval voting replaces wholesale, so an empty
  // form would read as "approve nobody" rather than "unchanged".
  let approved = $state(new Set());
  let seeded = $state('');
  $effect(() => {
    if (proposal?.id && seeded !== proposal.id) {
      approved = new Set(candidates.filter((c) => c.approved_by_me).map((c) => c.id));
      seeded = proposal.id;
    }
  });

  function toggle(id) {
    const next = new Set(approved);
    if (next.has(id)) next.delete(id); else next.add(id);
    approved = next;
    // An edited ballot is not a saved one. The confirmation goes away the
    // moment the thing it is confirming stops being true.
    saved = false;
  }

  async function stand() {
    busy = true; error = '';
    try {
      await api(`proposals/${proposal.id}/candidates`, { method: 'POST', body: {} });
      seeded = '';
      onChanged();
    } catch (e) {
      error = e.message || 'Failed to stand';
    } finally {
      busy = false;
    }
  }

  // The act four surfaces have been promising (docs/adr/107). The server has
  // always taken a `user_id` here; only the button was missing.
  async function nominate() {
    if (!nomineeId) return;
    busy = true; error = '';
    try {
      await api(`proposals/${proposal.id}/candidates`, {
        method: 'POST',
        body: { user_id: nomineeId },
      });
      nomineeId = '';
      seeded = '';
      onChanged();
    } catch (e) {
      error = e.message || 'Failed to put them forward';
    } finally {
      busy = false;
    }
  }

  // And the way back off. Your own only, which is what makes putting somebody
  // else forward safe to offer at all.
  async function withdraw() {
    busy = true; error = '';
    try {
      await api(`proposals/${proposal.id}/candidates/me`, { method: 'DELETE' });
      seeded = '';
      onChanged();
    } catch (e) {
      error = e.message || 'Failed to withdraw';
    } finally {
      busy = false;
    }
  }

  async function submitBallot() {
    busy = true; error = '';
    try {
      await api(`proposals/${proposal.id}/ballot`, {
        method: 'PUT',
        body: { candidate_ids: [...approved] },
      });
      // Say so. This said nothing at all, and every simulated voter went
      // looking for proof somewhere else — one watched an approval counter
      // tick up, saw it tick again when somebody else voted, and spent ten
      // minutes working out whether the second one was hers.
      saved = true;
      onChanged();
    } catch (e) {
      error = e.message || 'Failed to save your ballot';
    } finally {
      busy = false;
    }
  }
</script>

{#if phase}
  <section class="election">
    <h3>
      {#if phase === 'nominating'}Nominations{:else if phase === 'voting'}The ballot{:else}Result{/if}
      <span class="seats">{seats} seat{seats === 1 ? '' : 's'}</span>
    </h3>

    {#if phase === 'nominating'}
      <p class="lede">
        Anyone who is a member can stand.
        {#if proposal.nominations_close_at}
          Nominations close {formatDay(proposal.nominations_close_at)}, and voting opens then.
        {/if}
      </p>
    {:else if phase === 'voting'}
      <p class="lede">
        Approve as many candidates as you like.
        {#if seats === 1}
          The most approved candidate takes the seat.
        {:else}
          The {seats} most approved take the seats.
        {/if}
      </p>
    {/if}

    <!--
      What happened to the window somebody has just missed (docs/adr/106).

      Standing used to end by the Stand button disappearing, with nothing in
      its place and nothing in the history. Three members turned up to stand
      after nominations had closed, and the page's answer to "am I too late?"
      was silence: Teo called it "the button gone without a trace". A closed
      door says it is closed, and says when it closed.
    -->
    {#if phase !== 'nominating' && proposal.nominations_close_at}
      <p class="muted small closed-window">
        Standing closed {formatDay(proposal.nominations_close_at)}.
      </p>
    {/if}

    <!-- Turnout, in the same words and the same place the ordinary proposal
         puts it. Not during nominations: no ballot may be cast yet, so there
         is nothing to be short of. -->
    {#if turnout && phase !== 'nominating'}
      <p class="small turnout" class:quorum-unmet={!turnout.met} class:quorum-met={turnout.met}>
        {#if turnout.needed === 0}
          No quorum required. {turnout.voted} of {turnout.eligible} voted.
        {:else if turnout.met}
          Quorum met: {turnout.voted} of {turnout.eligible} voted, {turnout.needed} needed.
        {:else}
          {phase === 'closed' ? 'Quorum not met' : 'Quorum not yet met'}:
          {turnout.voted} of {turnout.eligible} voted, {turnout.needed} needed.
        {/if}
      </p>
    {/if}

    {#if candidates.length === 0}
      <p class="muted small">
        {phase === 'nominating' ? 'Nobody has stood yet.' : 'Nobody stood.'}
      </p>
    {:else}
      <ul class="candidates">
        {#each candidates as c, i}
          <li class:seated={isSeated(c, i)}>
            <!-- The name is not the control (docs/adr/109). It used to sit
                 inside the checkbox's own <label>, so tapping it cast a vote:
                 a member on a phone tapped a candidate's name to find out who
                 he was and found she had approved him. "Same names, two
                 pages, opposite behaviour." The box is the box; the name is a
                 link to the person, which is what it is everywhere else. -->
            {#if phase === 'voting' && canVote}
              <label class="tick" aria-label={`Approve ${c.display_name || c.username}`}>
                <input type="checkbox" checked={approved.has(c.id)} onchange={() => toggle(c.id)} disabled={busy} />
              </label>
            {/if}
            <a class="who" href={`/users/${c.username}`}>{c.display_name || c.username}</a>
            {#if phase !== 'nominating'}
              <span class="count">{c.approvals} approval{c.approvals === 1 ? '' : 's'}</span>
            {/if}
            <!-- Both outcomes say themselves. A blank beside a name is not
                 an answer, and on a closed contest the reader has no other
                 way to tell "lost" from "still being counted". -->
            {#if isSeated(c, i)}
              <span class="tag">seated</span>
            {:else if phase === 'closed'}
              <span class="tag tag-quiet">not seated</span>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}

    {#if phase === 'nominating' && canNominate}
      <div class="nominating-actions">
        {#if iAmStanding}
          <p class="standing small">
            You are standing in this election.
            <button class="btn-link" onclick={withdraw} disabled={busy}>
              {busy ? 'Withdrawing…' : 'Withdraw'}
            </button>
          </p>
        {:else}
          <button class="btn btn-sm" onclick={stand} disabled={busy}>
            {busy ? 'Standing…' : 'Stand for election'}
          </button>
        {/if}

        <!-- Putting somebody else forward, which every other surface has been
             telling members they can do (docs/adr/107). Their own name, not
             yours: the one button here used to stand you, whatever you came
             to do. -->
        {#if nominatable.length > 0}
          <div class="nominate-other">
            <label for="nominate-who">Put another member forward</label>
            <div class="nominate-row">
              <select id="nominate-who" bind:value={nomineeId} disabled={busy}>
                <option value="">Choose a member</option>
                {#each nominatable as m (m.user_id)}
                  <option value={m.user_id}>{m.display_name || m.username}</option>
                {/each}
              </select>
              <button class="btn btn-sm" onclick={nominate} disabled={busy || !nomineeId}>
                {busy ? 'Adding…' : 'Put forward'}
              </button>
            </div>
            <p class="muted small">
              They go on the ballot straight away, and can withdraw themselves
              until nominations close.
            </p>
          </div>
        {/if}
      </div>
    {/if}

    {#if phase === 'voting' && canVote && candidates.length > 0}
      <button class="btn btn-primary btn-sm" onclick={submitBallot} disabled={busy}>
        {busy ? 'Saving…' : ballotIn ? 'Update my ballot' : 'Save my ballot'}
      </button>
      <!-- One sentence, not two saying the same thing. Read on the page: the
           confirmation and the standing advice sat one above the other both
           offering to let you change your mind. -->
      {#if ballotIn}
        <p class="ballot-in small">Your ballot is in. You can change it until voting closes.</p>
      {:else}
        <p class="muted small">
          You can change this until voting closes. Approving nobody is the same as
          not voting.
        </p>
      {/if}
    {/if}

    {#if error}<p class="err">{error}</p>{/if}
  </section>
{/if}

<style>
  .election {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    padding: 0.9rem 1rem;
    margin-bottom: 1.25rem;
  }

  h3 {
    font-size: 0.95rem;
    margin: 0 0 0.35rem;
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .closed-window {
    margin: -0.15rem 0 0.5rem;
  }

  .tick {
    display: inline-flex;
    align-items: center;
    /* A thumb-sized target of its own, now that the name is not part of it. */
    padding: 0.25rem;
    margin: -0.25rem 0 -0.25rem -0.25rem;
  }

  .nominating-actions {
    display: flex;
    flex-direction: column;
    /* Or a column stretches every button to the panel's full width, which is
       how the theme-less `.btn` got noticed: a full-bleed pale pill. */
    align-items: flex-start;
    gap: 0.6rem;
  }

  .standing {
    margin: 0;
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .nominate-other label {
    display: block;
    font-size: 0.8rem;
    font-weight: 500;
    margin-bottom: 0.3rem;
  }

  .nominate-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
  }

  .nominate-row select {
    min-width: 0;
    flex: 1 1 12rem;
  }

  .ballot-in {
    margin: 0.5rem 0 0;
    color: var(--color-success, var(--color-text));
    font-weight: 500;
  }

  .seats {
    font-size: 0.78rem;
    font-weight: 400;
    color: var(--color-text-muted);
  }

  .lede {
    font-size: 0.85rem;
    margin: 0 0 0.6rem;
  }

  .small { font-size: 0.8rem; }

  .candidates {
    list-style: none;
    padding: 0;
    margin: 0 0 0.7rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .candidates li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    font-size: 0.88rem;
  }

  .candidates label {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    cursor: pointer;
  }

  .count {
    font-size: 0.78rem;
    color: var(--color-text-muted);
  }

  .tag {
    font-size: 0.72rem;
    padding: 0.05rem 0.35rem;
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--color-success) 18%, transparent);
    color: var(--color-text);
  }
  /* Losing is an outcome, not a warning: the row states it and does not
     shout it. */
  .tag-quiet {
    background: color-mix(in srgb, var(--color-text) 8%, transparent);
    color: var(--color-text-muted);
  }
  /* The same two colours the ordinary proposal's quorum line uses, because
     it is the same sentence about the same thing. */
  .turnout { margin: 0.35rem 0 0; }
  .quorum-met { color: var(--color-success); }
  .quorum-unmet { color: var(--color-accent); }

  /* A link, but a quiet one: a ballot of six blue underlined names reads as
     navigation rather than as a list of people to choose between. */
  .who {
    color: inherit;
    text-decoration: none;
  }

  .who:hover,
  .who:focus-visible {
    text-decoration: underline;
  }

  .seated .who { font-weight: 600; }

  .err {
    font-size: 0.82rem;
    color: var(--color-danger, #c0392b);
    margin: 0.4rem 0 0;
  }
</style>
