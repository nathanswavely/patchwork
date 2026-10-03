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
    seat: 'Seat',
    adoption: 'Adopted elsewhere',
  };

  // Names as a person would read them aloud. A record is prose, and
  // "Sam Pryor, Ana Lindqvist" in the middle of a sentence reads as a
  // database field rather than as two people.
  function listOf(names) {
    if (names.length === 1) return names[0];
    return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
  }

  // One sentence per entry saying how it was settled.
  //
  // No tally here, on purpose. The outcome is stored when a vote resolves and
  // never moves; the counts are recomputed on every read and drop ballots from
  // people who have since left the patch (docs/adr/044). The two drift apart,
  // and the first seeded patch I looked at already read "Did not carry. 2 for,
  // 1 against." The arithmetic that actually decided it was never stored. So
  // the record states what was settled and links to the proposal, where the
  // whole voter list and the frozen terms live.
  //
  // Who an election seated is the exception, and it is not a tally: it is
  // written onto the candidate when the contest resolves and never moves
  // either, which is the whole reason it is stored rather than derived.
  function outcomeLine(e) {
    if (e.kind === 'vote') {
      if (e.outcome === 'carried') return 'Carried by a vote.';
      // A lapse is not a vote that failed (docs/adr/097). The window closed
      // under quorum, so "did not carry" — which says the members answered
      // no — is the one thing the Record must not say about it.
      if (e.outcome === 'lapsed') {
        return 'Put to a vote. Nobody decided it either way; the proposal lapsed.';
      }
      return 'Put to a vote and did not carry.';
    }
    if (e.kind === 'direct') {
      // A maintainer's decision either way (docs/adr/092). A decline is
      // never "did not carry": the members' tally, if there was one, was
      // advice, and one person said no.
      if (e.outcome === 'declined') {
        return e.actor ? `Declined by ${e.actor}.` : 'Declined by the maintainer.';
      }
      return e.actor ? `Applied by ${e.actor}.` : 'Applied without a vote.';
    }
    if (e.kind === 'election') {
      // Not "the council kept serving" (docs/adr/106). This line is read
      // months later and cannot know what the council was on the day; five
      // members of a co-op with no admins at all read it five times down one
      // page, beside a council block saying nobody held the role. What the
      // record can always say truthfully is what the *contest* did, and the
      // council block two inches away says what the council is.
      // Leads with the fact, not with "Settled nothing", which one reader
      // took for a dispute: "the vote was tied or disputed, something
      // contentious. It means nobody turned up." The seats sentence is the
      // banner's, so the two pages say one thing (F-107).
      if (e.outcome !== 'seated') return 'Nobody was elected. The seats it was for are unchanged.';
      // Who, when the contest recorded it. "The electorate seated a council"
      // named nobody and overstated one chair of three as a whole council,
      // and "electorate" is not a word the co-op that read it has ever said
      // out loud. Contests that resolved before the outcome was stored keep
      // the unnamed sentence, in the patch's own vocabulary.
      return e.names?.length
        ? `The members seated ${listOf(e.names)}.`
        : 'The members seated a council.';
    }
    if (e.kind === 'council') {
      return e.names?.length
        ? `Seated ${listOf(e.names)}.`
        : 'A meeting chose the council.';
    }
    // Who came off the council and who went on (F-100). These are the
    // events a founder came back for twice and could not find: a seat
    // vacated for inactivity and an interim promotion are written to the
    // audit log, which is instance-admin only, so the largest thing that
    // can happen to a patch's governance reached no member-facing page.
    if (e.kind === 'seat') {
      if (e.outcome === 'vacated_inactivity') {
        return 'Their seat was vacated: they had taken no part in governance here for the patch’s inactivity period.';
      }
      if (e.outcome === 'vacated') return 'Their seat was vacated.';
      if (e.outcome === 'stepped_in') {
        return 'Stepped in as an interim admin, under this patch’s succession policy.';
      }
      if (e.outcome === 'made_admin') {
        return e.actor ? `Made an admin by ${e.actor}.` : 'Made an admin.';
      }
      return e.actor ? `Stopped being an admin. Recorded by ${e.actor}.` : 'Stopped being an admin.';
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
        <p class="muted">Every decision this patch has reached, and every one it failed to, newest first.</p>
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
            <li
              class="entry"
              class:unsettled={e.outcome === 'unsettled' || e.outcome === 'failed' || e.outcome === 'lapsed' || e.outcome === 'vacated' || e.outcome === 'vacated_inactivity' || e.outcome === 'stood_down'}
            >
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
