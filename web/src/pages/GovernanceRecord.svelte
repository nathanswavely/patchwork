<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { formatDay } from '../lib/datetime.js';
  import GovernanceShell from '../components/GovernanceShell.svelte';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  // What this patch has decided, in order (docs/adr/055). Every entry is a
  // view of something another feature owns — a resolved proposal, a recorded
  // council, an adopted text — so there is nothing to create here and no
  // permission of its own.
  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isUnclaimed = $derived(patch.value.isUnclaimed);

  let entries = $state([]);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    if (slug && isUnclaimed) {
      navigate(`/patches/${slug}/events`);
      return;
    }
    if (slug) load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      const data = await api(`nodes/${slug}/governance/record`);
      entries = data.items || [];
    } catch (e) {
      error = e.message || 'Could not load the record';
      entries = [];
    } finally {
      loading = false;
    }
  }

  const KIND_LABEL = {
    vote: 'Vote',
    direct: 'Direct change',
    election: 'Election',
    council: 'Council',
    adoption: 'Adopted elsewhere',
  };

  // One sentence per entry saying how it was settled.
  //
  // No tally here, on purpose. The outcome is stored when a vote resolves and
  // never moves; the counts are recomputed on every read and drop ballots from
  // people who have since left the patch (docs/adr/044). The two drift apart,
  // and the first seeded patch I looked at already read "Did not carry. 2 for,
  // 1 against." The arithmetic that actually decided it was never stored. So
  // the record states what was settled and links to the proposal, where the
  // whole voter list and the frozen terms live.
  function outcomeLine(e) {
    if (e.kind === 'vote') {
      return e.outcome === 'carried' ? 'Carried by a vote.' : 'Put to a vote and did not carry.';
    }
    if (e.kind === 'direct') {
      return e.actor ? `Applied by ${e.actor}.` : 'Applied without a vote.';
    }
    if (e.kind === 'election') {
      return e.outcome === 'seated'
        ? 'The electorate seated a council.'
        : 'Settled nothing. The council kept serving.';
    }
    if (e.kind === 'council') {
      const who = e.names?.length ? e.names.join(', ') : '';
      return who ? `Seated ${who}.` : 'A meeting chose the council.';
    }
    if (e.kind === 'adoption') return 'A meeting adopted this text.';
    return '';
  }
</script>

<GovernanceShell>
  {#snippet children()}
    <div class="record-page page-fade">
      <div class="record-head">
        <h1>Record</h1>
        <!-- No gloss on what the kinds are. Each entry wears its own label,
             and a sentence listing them ahead of the list teaches the reader
             what they are one second from seeing. -->
        <p class="muted">Everything this patch has settled, newest first.</p>
      </div>

      {#if loading}
        <Skeleton lines={6} height="1rem" />
      {:else if error}
        <ErrorState message={error} retry={load} />
      {:else if entries.length === 0}
        <p class="muted empty">
          Nothing settled yet. Proposals show up here once they close.
        </p>
      {:else}
        <ol class="entries">
          {#each entries as e}
            <li class="entry" class:unsettled={e.outcome === 'unsettled' || e.outcome === 'failed'}>
              <div class="entry-head">
                <span class="kind">{KIND_LABEL[e.kind] || e.kind}</span>
                <span class="when muted">{formatDay(e.at)}</span>
              </div>
              {#if e.link}
                <a class="entry-title" href={e.link} onclick={(ev) => { ev.preventDefault(); navigate(e.link); }}>
                  {e.title}
                </a>
              {:else}
                <span class="entry-title plain">{e.title}</span>
              {/if}
              <p class="outcome">{outcomeLine(e)}</p>
              {#if e.summary}<p class="summary muted">{e.summary}</p>{/if}
            </li>
          {/each}
        </ol>
      {/if}
    </div>
  {/snippet}
</GovernanceShell>

<style>
  .record-page {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding-top: 2rem;
  }

  .record-head {
    margin-bottom: 1.5rem;
  }

  .record-head h1 {
    margin-bottom: 0.35rem;
  }

  .record-head p {
    font-size: 0.88rem;
    line-height: 1.6;
    margin: 0;
  }

  .empty {
    font-size: 0.88rem;
    padding: 2rem 0;
  }

  .entries {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
  }

  .entry {
    padding: 0.9rem 0;
    border-top: 1px solid var(--color-border);
  }

  .entry:last-child {
    border-bottom: 1px solid var(--color-border);
  }

  .entry-head {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    margin-bottom: 0.2rem;
  }

  .kind {
    font-size: 0.7rem;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .when {
    font-size: 0.78rem;
  }

  .entry-title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--color-text);
    text-decoration: none;
  }

  a.entry-title:hover {
    text-decoration: underline;
  }

  .outcome {
    font-size: 0.85rem;
    margin: 0.2rem 0 0;
  }

  .summary {
    font-size: 0.83rem;
    margin: 0.15rem 0 0;
  }

  /* A vote that failed and an election that settled nothing are both part of
     the record. Muted, never hidden. */
  .unsettled .outcome {
    color: var(--color-text-muted);
  }
</style>
