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
    onChanged = () => {},
  } = $props();

  let phase = $derived(proposal?.election_phase || '');
  let candidates = $derived(proposal?.candidates || []);
  let seats = $derived(proposal?.seats_contested || 0);
  let me = $derived(getUser());
  let iAmStanding = $derived(candidates.some((c) => c.user_id === me?.id));

  let busy = $state(false);
  let error = $state('');

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

  async function submitBallot() {
    busy = true; error = '';
    try {
      await api(`proposals/${proposal.id}/ballot`, {
        method: 'PUT',
        body: { candidate_ids: [...approved] },
      });
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
        Approve as many candidates as you like. The {seats} most approved take the seats.
      </p>
    {/if}

    {#if candidates.length === 0}
      <p class="muted small">
        {phase === 'nominating' ? 'Nobody has stood yet.' : 'Nobody stood.'}
      </p>
    {:else}
      <ul class="candidates">
        {#each candidates as c, i}
          <li class:seated={phase === 'closed' && i < seats && c.approvals > 0}>
            {#if phase === 'voting' && canVote}
              <label>
                <input type="checkbox" checked={approved.has(c.id)} onchange={() => toggle(c.id)} disabled={busy} />
                <span class="who">{c.display_name || c.username}</span>
              </label>
            {:else}
              <span class="who">{c.display_name || c.username}</span>
            {/if}
            {#if phase !== 'nominating'}
              <span class="count">{c.approvals} approval{c.approvals === 1 ? '' : 's'}</span>
            {/if}
            {#if phase === 'closed' && i < seats && c.approvals > 0}
              <span class="tag">seated</span>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}

    {#if phase === 'nominating' && canNominate && !iAmStanding}
      <button class="btn btn-sm" onclick={stand} disabled={busy}>
        {busy ? 'Standing…' : 'Stand for election'}
      </button>
    {:else if phase === 'nominating' && iAmStanding}
      <p class="muted small">You are standing in this election.</p>
    {/if}

    {#if phase === 'voting' && canVote && candidates.length > 0}
      <button class="btn btn-primary btn-sm" onclick={submitBallot} disabled={busy}>
        {busy ? 'Saving…' : 'Save my ballot'}
      </button>
      <p class="muted small">
        You can change this until voting closes. Approving nobody is the same as
        not voting.
      </p>
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

  .seated .who { font-weight: 600; }

  .err {
    font-size: 0.82rem;
    color: var(--color-danger, #c0392b);
    margin: 0.4rem 0 0;
  }
</style>
