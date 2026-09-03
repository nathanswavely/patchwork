<script>
  /**
   * Discovery mode (docs/adr/075).
   *
   * The quilt shows everything and asks nothing; this surface asks one
   * question and shows a short answer. That is the whole distinction between
   * the two, and it is why there is no canvas here — a canvas is the
   * show-everything gesture, and putting one on this page would collapse it
   * back into `/`.
   *
   * Standing and public: re-enterable from the rail, reachable by anonymous
   * visitors, never spent. It was `/welcome`'s steps 2 and 3, shown once to
   * signed-in newcomers and then destroyed; the orientation step it used to
   * share a file with stays gated where docs/adr/040 put it.
   *
   * It ends in follows rather than navigation — people leave with
   * relationships, which is what makes My Quilt worth having next visit.
   *
   * No per-person state anywhere: nothing is stored about who saw what, and
   * the page behaves identically for a stranger and a regular.
   */
  import { Check, Heart, CalendarBlank } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { getRankedTags, areTagsLoaded } from '../stores/quilt.svelte.js';
  import { getMembershipRoles, loadMemberships } from '../stores/memberships.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { formatDay } from '../lib/datetime.js';

  // --- The question ---
  // The shortlist is the instance's most-worn tags (docs/adr/075). Never a
  // baked-in vocabulary: a disc golf quilt surfaces its own.
  const SHORTLIST_SIZE = 8;
  let rankedTags = $derived(getRankedTags());
  let shortlist = $derived(rankedTags.slice(0, SHORTLIST_SIZE));
  let remainingTags = $derived(rankedTags.slice(SHORTLIST_SIZE));
  let showAllTags = $state(false);
  let visibleTags = $derived(showAllTags ? rankedTags : shortlist);

  let selectedInterests = $state(new Set());

  function toggleInterest(tag) {
    const next = new Set(selectedInterests);
    if (next.has(tag)) next.delete(tag);
    else next.add(tag);
    selectedInterests = next;
  }

  // 'ask' defers the wall of results until the person has said something —
  // the Zillow front door, which asks "where?" rather than rendering four
  // thousand listings with facets alongside. A quilt whose admin has curated
  // no vocabulary has no question to ask, so it opens on the answer.
  //
  // Gated on the vocabulary having actually loaded: an empty `rankedTags`
  // before the round-trip resolves is not a quilt without tags, and reading
  // it as one sent every visitor straight past the question.
  let phase = $state('ask');
  $effect(() => {
    if (areTagsLoaded() && rankedTags.length === 0) phase = 'answer';
  });

  // --- The answer ---
  let allPatches = $state([]);
  let patchesLoaded = $state(false);
  // patch id -> the soonest upcoming event on it.
  let nextEventByNode = $state(new Map());

  async function loadAnswer() {
    try {
      // Omitting `from` means upcoming (CLAUDE.md): the list sorts
      // starts_at ascending, so the first row seen for a patch is its next
      // event. This is the "what is happening on it" half of the engine —
      // a fact about the world, self-refreshing as the calendar moves, and
      // the same for every viewer.
      const [treeResp, eventsResp] = await Promise.all([
        api('nodes/tree'),
        api('events?limit=200').catch(() => ({ items: [] })),
      ]);
      const tree = treeResp.tree || treeResp;
      allPatches = (tree.children || []).filter((p) => p.slug);

      const next = new Map();
      for (const e of eventsResp.items || []) {
        if (e.node_id && !next.has(e.node_id)) next.set(e.node_id, e);
      }
      nextEventByNode = next;
    } catch {
      allPatches = [];
    }
    patchesLoaded = true;
  }

  $effect(() => {
    if (phase === 'answer' && !patchesLoaded) loadAnswer();
  });

  function matchesInterests(p) {
    return (p.tags || []).some((t) => selectedInterests.has(t));
  }

  // Ordered by what is actually happening: a patch with something coming up
  // leads, soonest first, then the rest by name. Deliberately not by member
  // or follower count — docs/adr/015 established that ranking a field of
  // zeros invents a winner, and on a directory-seeded quilt almost every
  // patch is a zero. A date is a fact; a count of nothing is not a ranking.
  function byWhatIsHappening(a, b) {
    const ea = nextEventByNode.get(a.id);
    const eb = nextEventByNode.get(b.id);
    if (ea && eb) return ea.starts_at.localeCompare(eb.starts_at) || a.name.localeCompare(b.name);
    if (ea) return -1;
    if (eb) return 1;
    return (a.name || '').localeCompare(b.name || '');
  }

  let matching = $derived.by(() => {
    const list = selectedInterests.size === 0
      ? [...allPatches]
      : allPatches.filter(matchesInterests);
    return list.sort(byWhatIsHappening);
  });

  let others = $derived.by(() => {
    if (selectedInterests.size === 0) return [];
    return allPatches.filter((p) => !matchesInterests(p)).sort(byWhatIsHappening);
  });

  let showOthers = $state(false);

  // --- Follows: the thing people leave with ---
  let roles = $derived(getMembershipRoles());
  let busy = $state(new Set());

  function isFollowing(slug) {
    return roles.get(slug) === 'follower';
  }

  async function toggleFollow(patch) {
    // Anonymous visitors reach this page by design, so Follow is a prompt to
    // sign in rather than a button that fails — the pattern SocialHome uses.
    if (!isLoggedIn()) {
      navigate('/login');
      return;
    }
    const slug = patch.slug;
    if (busy.has(slug)) return;
    busy = new Set(busy).add(slug);
    const following = isFollowing(slug);
    try {
      if (following) {
        await api(`nodes/${slug}/leave`, { method: 'POST' });
      } else {
        await api(`nodes/${slug}/join`, { method: 'POST', body: { role: 'follower' } });
      }
      await loadMemberships();
    } catch (err) {
      showToast(err.message || 'Something went wrong', 'error');
    } finally {
      const next = new Set(busy);
      next.delete(slug);
      busy = next;
    }
  }

  let followedHere = $derived(
    [...allPatches].filter((p) => isFollowing(p.slug)).length
  );

  // --- The bulletin's offer (docs/adr/076) ---
  // Two named choices, neither pre-selected. Not a checkbox: docs/adr/040
  // deleted one from Welcome, and while what it removed was a *signature*
  // and this is a *setting*, the register binds — a pre-checked box would be
  // default-on wearing opt-in's clothes.
  //
  // It lives at the end of the discovery flow rather than in onboarding
  // because onboarding is spent: everyone already on a live instance would
  // otherwise never be offered it.
  //
  // Shown only until it is answered. Declining writes an explicit "no",
  // which is what makes it distinguishable from never having been asked —
  // and what stops the offer coming back every visit.
  const BULLETIN = 'quilt.bulletin';
  let bulletinDecided = $state(true); // assume answered until told otherwise
  let bulletinChannels = $state([]);
  let bulletinSaving = $state(false);

  async function loadBulletinOffer() {
    if (!isLoggedIn()) return;
    try {
      const data = await api('notifications/preferences');
      bulletinChannels = data.channels || [];
      for (const cat of data.categories || []) {
        for (const t of cat.types || []) {
          if (t.type === BULLETIN) bulletinDecided = !!t.decided;
        }
      }
    } catch {
      // A preferences endpoint that will not answer is no reason to press
      // an offer on someone.
      bulletinDecided = true;
    }
  }

  $effect(() => {
    if (phase === 'answer') loadBulletinOffer();
  });

  async function answerBulletin(enabled) {
    if (bulletinSaving) return;
    bulletinSaving = true;
    try {
      await api('notifications/preferences', {
        method: 'PUT',
        body: {
          preferences: bulletinChannels.map((channel) => ({
            type: BULLETIN, channel, enabled,
          })),
        },
      });
      bulletinDecided = true;
      showToast(
        enabled ? "You'll get a note once a month." : 'No bulletin then.',
        'success',
      );
    } catch (err) {
      showToast(err.message || 'Something went wrong', 'error');
    } finally {
      bulletinSaving = false;
    }
  }
</script>

<div class="discover">
  {#if phase === 'ask'}
    <div class="panel">
      <h1>What are you drawn to?</h1>
      <p class="subtitle">
        Pick a few and we'll pull up the patches that match. These are the
        tags this quilt actually wears, most-worn first.
      </p>

      <div class="tag-grid">
        {#each visibleTags as tag (tag)}
          <button
            class="tag-chip lt-resin"
            class:selected={selectedInterests.has(tag)}
            style="--lt-resin-color: {colorForTag(tag)}; {selectedInterests.has(tag) ? `background: ${colorForTag(tag)}; border-color: ${colorForTag(tag)}; color: ${textOnColor(colorForTag(tag))};` : `border-color: ${colorForTag(tag)}40;`}"
            aria-pressed={selectedInterests.has(tag)}
            onclick={() => toggleInterest(tag)}
          >
            {#if selectedInterests.has(tag)}
              <Check size={14} weight="bold" />
            {/if}
            {tag}
          </button>
        {/each}
      </div>

      {#if !showAllTags && remainingTags.length > 0}
        <button class="show-all-link" onclick={() => (showAllTags = true)}>
          Show all tags ({remainingTags.length} more)
        </button>
      {/if}

      {#if selectedInterests.size > 0}
        <p class="counter">{selectedInterests.size} selected</p>
      {/if}

      <div class="bottom-bar">
        <button
          class="btn cta-btn"
          class:btn-primary={selectedInterests.size > 0}
          class:btn-secondary={selectedInterests.size === 0}
          onclick={() => (phase = 'answer')}
        >
          {selectedInterests.size > 0 ? 'Show me patches' : 'Pick at least one'}
        </button>
        <button
          class="skip-link"
          onclick={() => { selectedInterests = new Set(); phase = 'answer'; }}
        >
          Show me everything instead
        </button>
      </div>
    </div>

  {:else}
    <div class="panel">
      {#if rankedTags.length > 0}
        <button class="back-link" onclick={() => (phase = 'ask')}>&larr; Ask me again</button>
      {/if}

      {#if !patchesLoaded}
        <p class="subtitle">Looking…</p>
      {:else if allPatches.length === 0}
        <!-- Nothing to find yet. The natural first act is making something
             for the next person to find. -->
        <h1>Nothing here yet</h1>
        <p class="subtitle">
          No patches on this quilt so far. Make one for your group, and the
          next person who comes looking will have something to find.
        </p>
        <div class="bottom-bar">
          <button class="btn btn-primary cta-btn" onclick={() => navigate('/patches/new')}>
            Create a patch
          </button>
        </div>
      {:else}
        <h1>{selectedInterests.size > 0 ? 'Patches you might like' : 'Everything on this quilt'}</h1>
        <p class="subtitle">
          {matching.length}
          {matching.length === 1 ? 'patch' : 'patches'}{selectedInterests.size > 0 ? ' match what you picked' : ''}{' '}
          — the ones with something coming up are first.
        </p>

        <div class="patch-list">
          {#each matching as patch (patch.id)}
            {@const nextEvent = nextEventByNode.get(patch.id)}
            <div class="patch-row">
              <div class="patch-dot" style="background: {colorForTag((patch.tags || [])[0])}"></div>
              <div class="patch-info">
                <a
                  class="patch-name"
                  href={`/patches/${patch.slug}`}
                  onclick={(e) => { e.preventDefault(); navigate(`/patches/${patch.slug}`); }}
                >{patch.name}</a>
                {#if patch.description}
                  <span class="patch-desc">
                    {patch.description.length > 90 ? patch.description.slice(0, 90) + '…' : patch.description}
                  </span>
                {/if}
                <span class="patch-meta">
                  {#each (patch.tags || []).slice(0, 3) as tag}
                    <span class="meta-tag" style="color: {colorForTag(tag)}">{tag}</span>
                  {/each}
                  <span class="muted">
                    {patch.is_unclaimed
                      ? `${patch.follower_count || 0} following`
                      : `${patch.member_count || 0} members`}
                  </span>
                </span>
                {#if nextEvent}
                  <span class="patch-next">
                    <CalendarBlank size={12} weight="duotone" />
                    {formatDay(nextEvent.starts_at)}{' · '}{nextEvent.title}
                  </span>
                {/if}
              </div>
              <button
                class="btn follow-btn"
                class:following={isFollowing(patch.slug)}
                class:btn-secondary={!isFollowing(patch.slug)}
                onclick={() => toggleFollow(patch)}
                disabled={busy.has(patch.slug)}
                aria-pressed={isFollowing(patch.slug)}
              >
                {#if isFollowing(patch.slug)}
                  <Heart size={12} weight="fill" />
                  Following
                {:else}
                  Follow
                {/if}
              </button>
            </div>
          {/each}
        </div>

        {#if others.length > 0}
          <button class="show-all-link" onclick={() => (showOthers = !showOthers)}>
            {showOthers ? 'Hide' : 'Show'} the rest of the quilt ({others.length})
          </button>

          {#if showOthers}
            <div class="patch-list">
              {#each others as patch (patch.id)}
                <div class="patch-row">
                  <div class="patch-dot" style="background: {colorForTag((patch.tags || [])[0])}"></div>
                  <div class="patch-info">
                    <a
                      class="patch-name"
                      href={`/patches/${patch.slug}`}
                      onclick={(e) => { e.preventDefault(); navigate(`/patches/${patch.slug}`); }}
                    >{patch.name}</a>
                    <span class="patch-meta">
                      {#each (patch.tags || []).slice(0, 3) as tag}
                        <span class="meta-tag" style="color: {colorForTag(tag)}">{tag}</span>
                      {/each}
                      <span class="muted">
                        {patch.is_unclaimed
                          ? `${patch.follower_count || 0} following`
                          : `${patch.member_count || 0} members`}
                      </span>
                    </span>
                  </div>
                  <button
                    class="btn follow-btn"
                    class:following={isFollowing(patch.slug)}
                    class:btn-secondary={!isFollowing(patch.slug)}
                    onclick={() => toggleFollow(patch)}
                    disabled={busy.has(patch.slug)}
                    aria-pressed={isFollowing(patch.slug)}
                  >
                    {#if isFollowing(patch.slug)}
                      <Heart size={12} weight="fill" />
                      Following
                    {:else}
                      Follow
                    {/if}
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        {/if}

        <div class="bottom-bar">
          {#if followedHere > 0}
            <p class="counter">
              Following {followedHere} {followedHere === 1 ? 'patch' : 'patches'}
            </p>
            <button class="btn btn-primary cta-btn" onclick={() => navigate('/my')}>
              See your quilt
            </button>
          {:else if isLoggedIn()}
            <p class="counter">Follow a few and they become your quilt.</p>
          {:else}
            <p class="counter">Following needs an account — reading never does.</p>
          {/if}

          {#if isLoggedIn() && !bulletinDecided && bulletinChannels.length > 0}
            <div class="bulletin-offer">
              <p class="bulletin-ask">
                Once a month, we can tell you which patches joined. The whole
                list, in the order they arrived — nothing picked out for you.
              </p>
              <div class="bulletin-choices">
                <button
                  class="btn btn-secondary"
                  disabled={bulletinSaving}
                  onclick={() => answerBulletin(true)}
                >Tell me who's new</button>
                <button
                  class="btn-plain"
                  disabled={bulletinSaving}
                  onclick={() => answerBulletin(false)}
                >No thanks</button>
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .discover {
    max-width: 640px;
    margin: 0 auto;
    padding-bottom: 3rem;
  }

  .panel {
    display: flex;
    flex-direction: column;
  }

  h1 {
    font-size: 1.5rem;
    margin-bottom: 0.5rem;
  }

  .subtitle {
    font-size: 0.9rem;
    color: var(--color-text-muted);
    margin-bottom: 1.5rem;
    line-height: 1.5;
  }

  .back-link {
    border: none;
    background: none;
    font-size: 0.82rem;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: 0 0 0.75rem;
    text-align: left;
    align-self: flex-start;
  }

  .back-link:hover {
    color: var(--color-primary);
  }

  .tag-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .tag-chip {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    padding: 0.7rem 0.8rem;
    font-size: 0.88rem;
    font-weight: 500;
    border: 1.5px solid;
    border-radius: 6px;
    cursor: pointer;
    transition: all 150ms ease;
    color: var(--color-text);
    background: var(--color-surface);
    min-height: 44px;
  }

  .tag-chip:hover {
    border-color: var(--lt-resin-color, var(--color-border));
  }

  .show-all-link {
    border: none;
    background: none;
    font-size: 0.82rem;
    color: var(--color-primary);
    cursor: pointer;
    padding: 0.5rem 0;
    text-align: left;
    align-self: flex-start;
  }

  .show-all-link:hover {
    text-decoration: underline;
  }

  .counter {
    font-size: 0.82rem;
    color: var(--color-text-muted);
    font-weight: 500;
    margin-bottom: 0.75rem;
  }

  .cta-btn {
    width: 100%;
    padding: 0.75rem;
    font-size: 0.95rem;
  }

  .skip-link {
    border: none;
    background: none;
    font-size: 0.8rem;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: 0.75rem 0;
    text-align: center;
    width: 100%;
  }

  .skip-link:hover {
    color: var(--color-primary);
  }

  .patch-list {
    display: flex;
    flex-direction: column;
    margin-bottom: 1rem;
  }

  .patch-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .patch-row:last-child {
    border-bottom: none;
  }

  .patch-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .patch-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .patch-name {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--color-text);
    text-decoration: none;
  }

  .patch-name:hover {
    color: var(--color-primary);
  }

  .patch-desc {
    font-size: 0.78rem;
    color: var(--color-text-muted);
    line-height: 1.4;
  }

  .patch-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    align-items: center;
    font-size: 0.72rem;
  }

  .meta-tag {
    font-weight: 600;
  }

  /* What is happening, stated as a date rather than implied by a rank. */
  .patch-next {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.72rem;
    color: var(--color-primary);
    font-weight: 600;
  }

  .follow-btn {
    flex-shrink: 0;
    padding: 0.35rem 0.8rem;
    font-size: 0.8rem;
    min-width: 90px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.3rem;
  }

  /* Follow = heart, in the same tone as the quilt-view follow button
     (CONTEXT.md "Role mark" — the star is reserved for membership). */
  .follow-btn.following {
    background: color-mix(in srgb, var(--color-error) 8%, transparent);
    border-color: var(--color-error);
    color: var(--color-error);
  }

  .bottom-bar {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  /* Two named choices, given equal room to be read (docs/adr/076). The
     decline is worded and clickable, like the intro card's "I'll lurk for
     now" — not a greyed-out afterthought beside a primary button. */
  .bulletin-offer {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  .bulletin-ask {
    font-size: 0.82rem;
    color: var(--color-text-muted);
    line-height: 1.5;
    margin-bottom: 0.75rem;
  }

  .bulletin-choices {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .btn-plain {
    border: none;
    background: none;
    font-family: inherit;
    font-size: 0.82rem;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: 0.4rem 0;
  }

  .btn-plain:hover {
    color: var(--color-primary);
  }

  @media (min-width: 640px) {
    .tag-grid {
      grid-template-columns: repeat(3, 1fr);
    }
  }
</style>
