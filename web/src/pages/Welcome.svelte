<script>
  import { Check, Heart } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { getInstanceName, getAllTags, getTagCounts } from '../stores/quilt.svelte.js';
  import { loadMemberships } from '../stores/memberships.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { PALETTES, PALETTE_KEYS } from '../lib/quiltTheme.js';
  import { dismissOnboarding } from '../lib/onboarding.js';
  import { getLabel, loadLabel, formatMoney } from '../stores/label.svelte.js';

  let user = $derived(getUser());
  let instanceName = $derived(getInstanceName());
  let allTags = $derived(getAllTags());

  // The Label's onboarding panel (docs/adr/023): a compact summary beside
  // the agreement — the "what are you joining" beat. A panel, not a step:
  // a step people click past teaches them the Label is a formality. It
  // simply doesn't render when no Label is published.
  let label = $derived(getLabel());
  $effect(() => { loadLabel(); });
  let labelProseLine = $derived.by(() => {
    const prose = label?.prose || '';
    const line = prose.split('\n').map((l) => l.trim()).find((l) => l && !l.startsWith('#')) || '';
    return line.length > 140 ? line.slice(0, 140).trimEnd() + '…' : line;
  });
  let labelStewardLine = $derived.by(() => {
    const s = label?.stewards || [];
    if (s.length === 0) return '';
    const names = s.slice(0, 3).map((x) => x.display_name || `@${x.username}`);
    return s.length > 3 ? `${names.join(', ')} +${s.length - 3}` : names.join(names.length > 2 ? ', ' : ' and ');
  });

  // --- Step state ---
  let step = $state(1);
  let agreed = $state(false);

  // --- Step 2: Interests ---
  let selectedInterests = $state(new Set());
  let showAllTags = $state(false);

  // Shortlist = this instance's most-worn tags (public patch counts from
  // the tags endpoint), never a baked-in vocabulary — white-label quilts
  // surface their own categories. Unworn tags sort alphabetically after.
  const SHORTLIST_SIZE = 8;
  let tagCounts = $derived(getTagCounts());
  let rankedTags = $derived(
    [...allTags].sort((a, b) =>
      (tagCounts[b] || 0) - (tagCounts[a] || 0) || a.localeCompare(b))
  );
  let popularTags = $derived(rankedTags.slice(0, SHORTLIST_SIZE));
  let remainingTags = $derived(rankedTags.slice(SHORTLIST_SIZE));
  let visibleTags = $derived(showAllTags ? rankedTags : popularTags);

  function toggleInterest(tag) {
    const next = new Set(selectedInterests);
    if (next.has(tag)) next.delete(tag);
    else next.add(tag);
    selectedInterests = next;
  }

  // --- Step 3: Patches ---
  let allPatches = $state([]);
  let patchesLoaded = $state(false);
  let followedSlugs = $state(new Set());
  let followingInProgress = $state(new Set());
  let showOtherPatches = $state(false);

  let matchingPatches = $derived.by(() => {
    if (selectedInterests.size === 0) return allPatches;
    return allPatches.filter(p =>
      (p.tags || []).some(t => selectedInterests.has(t))
    );
  });

  let otherPatches = $derived.by(() => {
    if (selectedInterests.size === 0) return [];
    return allPatches.filter(p =>
      !(p.tags || []).some(t => selectedInterests.has(t))
    );
  });

  async function loadPatches() {
    try {
      const resp = await api('nodes/tree');
      const tree = resp.tree || resp;
      allPatches = (tree.children || [])
        .filter(p => p.slug)
        .sort((a, b) =>
          ((b.member_count || 0) + (b.follower_count || 0)) -
          ((a.member_count || 0) + (a.follower_count || 0)));
    } catch {
      allPatches = [];
    }
    patchesLoaded = true;
  }

  async function handleFollow(slug) {
    const next = new Set(followingInProgress);
    next.add(slug);
    followingInProgress = next;

    // Optimistic
    const nextFollowed = new Set(followedSlugs);
    nextFollowed.add(slug);
    followedSlugs = nextFollowed;

    try {
      await api(`nodes/${slug}/join`, { method: 'POST', body: { role: 'follower' } });
    } catch (e) {
      // Revert
      const reverted = new Set(followedSlugs);
      reverted.delete(slug);
      followedSlugs = reverted;
      showToast(e.message || 'Failed to follow', 'error');
    } finally {
      const done = new Set(followingInProgress);
      done.delete(slug);
      followingInProgress = done;
    }
  }

  function handleUnfollow(slug) {
    const next = new Set(followedSlugs);
    next.delete(slug);
    followedSlugs = next;
    // Fire and forget — the leave API isn't critical during onboarding
    api(`nodes/${slug}/leave`, { method: 'POST' }).catch(() => {});
  }

  // --- Navigation ---
  function goToStep(n) {
    if (n === 3 && allPatches.length === 0) loadPatches();
    step = n;
  }

  let completing = $state(false);

  async function handleComplete() {
    completing = true;
    dismissOnboarding(user?.id);
    await loadMemberships();
    // Brief pause for the celebration moment
    setTimeout(() => navigate('/'), 1200);
  }

  // Skip must genuinely exit: without the persisted dismissal, the
  // zero-membership redirect in App.svelte sends the user straight back here
  // — a softlock on an empty instance where there is nothing to follow.
  function handleSkip() {
    dismissOnboarding(user?.id);
    navigate('/');
  }

  function handleCreateFirst() {
    dismissOnboarding(user?.id);
    navigate('/patches/new');
  }

  // --- Mini-quilt colors for Step 1 ---
  const miniQuiltColors = PALETTE_KEYS.slice(0, 9).map(k => PALETTES[k].primary);
</script>

<div class="welcome" class:step-1={step === 1} class:step-2={step === 2} class:step-3={step === 3}>

  {#key step}
  <div class="step-content" style="animation: fadeIn 150ms ease">

    {#if step === 1}
      <!-- ===== STEP 1: Welcome + Agreement ===== -->
      <div class="step step-welcome">
        <span class="instance-label">{instanceName}</span>

        <div class="mini-quilt" aria-hidden="true">
          {#each miniQuiltColors as color, i}
            <div
              class="mini-tile"
              style="background: {color}; clip-path: polygon({2 + (i % 3)}% {1 + (i % 2)}%, {97 + (i % 2)}% {2 - (i % 3)}%, {99 - (i % 2)}% {98 + (i % 3)}%, {1 + (i % 2)}% {99 - (i % 3)}%)"
            ></div>
          {/each}
        </div>

        <h1>Your community, pieced together</h1>

        <div class="explainer">
          <p>{instanceName} is a quilt of the communities around you.</p>
          <p>Each patch is a group. A band, a venue, a collective, a club. Patches that share people sit closer together.</p>
          <p>Follow the ones you care about and your corner of the quilt takes shape.</p>
        </div>

        <div class="not-social">
          <h2>Built for organizing</h2>
          <p>Nobody is selling ads here, and no algorithm decides what you see. A person in your community runs this server, and what happens on it stays under the community's control.</p>
        </div>

        {#if label?.published}
          <div class="label-panel">
            <p class="label-panel-who">
              This quilt is stewarded by <strong>{labelStewardLine}</strong>{#if label.total_monthly_minor > 0}&nbsp;and costs
              about <strong>{formatMoney(label.total_monthly_minor, label.currency)}/month</strong> to run{/if}.
            </p>
            {#if labelProseLine}
              <p class="label-panel-quote">&ldquo;{labelProseLine}&rdquo;</p>
            {/if}
            <a href="/label" target="_blank" rel="noopener" class="label-panel-link">
              Read the Label &rarr;
            </a>
          </div>
        {/if}

        <div class="agreement">
          <h2>Before you begin</h2>
          <p>By joining this patchwork, you agree to:</p>
          <ul>
            <li>Treat every person with dignity and respect</li>
            <li>Participate in good faith</li>
            <li>Support the communities you join</li>
            <li>Report harmful behavior instead of ignoring it</li>
          </ul>
          <label class="agree-check">
            <input type="checkbox" bind:checked={agreed} />
            <span>I agree to the community standards</span>
          </label>
        </div>

        <!-- No tags means nothing to pick on step 2 (empty instance) — go
             straight to the patches step. -->
        <button
          class="btn btn-primary cta-btn"
          disabled={!agreed}
          onclick={() => goToStep(allTags.length > 0 ? 2 : 3)}
        >
          Build your quilt &rarr;
        </button>
      </div>

    {:else if step === 2}
      <!-- ===== STEP 2: Interest Selection ===== -->
      <div class="step step-interests">
        <button class="back-link" onclick={() => goToStep(1)}>&larr; Back</button>

        <div class="step-dots">
          <span class="dot" class:active={step >= 1}></span>
          <span class="dot" class:active={step >= 2}></span>
          <span class="dot" class:active={step >= 3}></span>
        </div>

        <h1>What kinds of communities do you care about?</h1>
        <p class="subtitle">Pick a few and we'll pull up the patches that match.</p>

        <div class="tag-grid">
          {#each visibleTags as tag (tag)}
            <button
              class="tag-chip lt-resin"
              class:selected={selectedInterests.has(tag)}
              style="--lt-resin-color: {colorForTag(tag)}; {selectedInterests.has(tag) ? `background: ${colorForTag(tag)}; border-color: ${colorForTag(tag)}; color: ${textOnColor(colorForTag(tag))};` : `border-color: ${colorForTag(tag)}40;`}"
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
          <button class="show-all-link" onclick={() => showAllTags = true}>
            Show all tags ({remainingTags.length} more)
          </button>
        {/if}

        {#if selectedInterests.size > 0}
          <p class="counter">{selectedInterests.size} selected</p>
        {/if}

        <button
          class="btn cta-btn"
          class:btn-primary={selectedInterests.size >= 3}
          class:btn-secondary={selectedInterests.size < 3}
          onclick={() => goToStep(3)}
        >
          {selectedInterests.size >= 3 ? 'Show me patches →' : 'Pick at least 3'}
        </button>

        <button class="skip-link" onclick={() => { selectedInterests = new Set(); goToStep(3); }}>
          Show me everything instead
        </button>
      </div>

    {:else if step === 3}
      <!-- ===== STEP 3: Patch Discovery ===== -->
      <div class="step step-patches">
        <button class="back-link" onclick={() => goToStep(allTags.length > 0 ? 2 : 1)}>&larr; Back</button>

        <div class="step-dots">
          <span class="dot active"></span>
          <span class="dot active"></span>
          <span class="dot active"></span>
        </div>

        {#if completing}
          <div class="completion">
            <h1>Your quilt is ready.</h1>
            <p class="subtitle">Taking you there...</p>
          </div>
        {:else if patchesLoaded && allPatches.length === 0}
          <!-- Empty instance: nothing to follow. The natural first act is
               creating a patch — the first account is the instance admin. -->
          <h1>You're the first one here</h1>
          <p class="subtitle">There are no patches yet. Create one for your group, and the next person who shows up will have something to find.</p>

          <div class="bottom-bar">
            <button class="btn btn-primary cta-btn" onclick={handleCreateFirst}>
              Create the first patch &rarr;
            </button>
            <button class="skip-link" onclick={handleSkip}>
              I'll explore on my own
            </button>
          </div>
        {:else}
          <h1>{selectedInterests.size > 0 ? 'Patches for you' : 'All patches'}</h1>
          <p class="subtitle">{matchingPatches.length} {matchingPatches.length === 1 ? 'patch' : 'patches'}{selectedInterests.size > 0 ? ' match your interests' : ' available'}</p>

          <div class="patch-list">
            {#each matchingPatches as patch (patch.id)}
              <div class="patch-row">
                <div class="patch-dot" style="background: {colorForTag((patch.tags || [])[0])}"></div>
                <div class="patch-info">
                  <span class="patch-name">{patch.name}</span>
                  {#if patch.description}
                    <span class="patch-desc">{patch.description.length > 80 ? patch.description.slice(0, 80) + '...' : patch.description}</span>
                  {/if}
                  <span class="patch-meta">
                    {#each (patch.tags || []).slice(0, 3) as tag}
                      <span class="meta-tag" style="color: {colorForTag(tag)}">{tag}</span>
                    {/each}
                    <span class="muted">{patch.is_unclaimed ? `${patch.follower_count || 0} following` : `${patch.member_count || 0} members`}</span>
                  </span>
                </div>
                {#if followedSlugs.has(patch.slug)}
                  <button class="btn follow-btn following" onclick={() => handleUnfollow(patch.slug)}>
                    <Heart size={12} weight="fill" />
                    Following
                  </button>
                {:else}
                  <button
                    class="btn btn-secondary follow-btn"
                    onclick={() => handleFollow(patch.slug)}
                    disabled={followingInProgress.has(patch.slug)}
                  >
                    Follow
                  </button>
                {/if}
              </div>
            {/each}
          </div>

          {#if otherPatches.length > 0}
            <button class="show-all-link" onclick={() => showOtherPatches = !showOtherPatches}>
              {showOtherPatches ? 'Hide' : 'Show'} other patches ({otherPatches.length})
            </button>

            {#if showOtherPatches}
              <div class="patch-list">
                {#each otherPatches as patch (patch.id)}
                  <div class="patch-row">
                    <div class="patch-dot" style="background: {colorForTag((patch.tags || [])[0])}"></div>
                    <div class="patch-info">
                      <span class="patch-name">{patch.name}</span>
                      <span class="patch-meta">
                        {#each (patch.tags || []).slice(0, 3) as tag}
                          <span class="meta-tag" style="color: {colorForTag(tag)}">{tag}</span>
                        {/each}
                        <span class="muted">{patch.is_unclaimed ? `${patch.follower_count || 0} following` : `${patch.member_count || 0} members`}</span>
                      </span>
                    </div>
                    {#if followedSlugs.has(patch.slug)}
                      <button class="btn follow-btn following" onclick={() => handleUnfollow(patch.slug)}>
                        Following
                      </button>
                    {:else}
                      <button class="btn btn-secondary follow-btn" onclick={() => handleFollow(patch.slug)}>
                        Follow
                      </button>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          {/if}

          <div class="bottom-bar">
            {#if followedSlugs.size > 0}
              <p class="counter">Following {followedSlugs.size} {followedSlugs.size === 1 ? 'patch' : 'patches'}</p>
            {/if}
            <button
              class="btn cta-btn"
              class:btn-primary={followedSlugs.size > 0}
              class:btn-secondary={followedSlugs.size === 0}
              onclick={handleComplete}
              disabled={completing}
            >
              {followedSlugs.size > 0 ? 'See your quilt →' : 'Follow at least 1'}
            </button>
            <button class="skip-link" onclick={handleSkip}>
              I'll explore on my own
            </button>
          </div>
        {/if}
      </div>
    {/if}

  </div>
  {/key}
</div>

<style>
  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  .welcome {
    min-height: 100vh;
    min-height: 100dvh;
    background: var(--color-bg);
    display: flex;
    flex-direction: column;
  }

  .step-content {
    flex: 1;
    display: flex;
    flex-direction: column;
  }

  .step {
    flex: 1;
    padding: 3rem 2rem 2rem;
    max-width: 560px;
    width: 100%;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
  }

  /* Step 1: Welcome */
  .instance-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--color-text-muted);
    margin-bottom: 2rem;
  }

  .mini-quilt {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 4px;
    width: 120px;
    margin-bottom: 2rem;
  }

  .mini-tile {
    aspect-ratio: 1;
    border-radius: 2px;
  }

  .step-welcome h1 {
    font-size: clamp(1.8rem, 5vw, 2.4rem);
    font-weight: 700;
    line-height: 1.2;
    margin-bottom: 1.5rem;
    color: var(--color-text);
  }

  .explainer {
    margin-bottom: 2rem;
  }

  .explainer p {
    font-size: 0.95rem;
    line-height: 1.6;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  /* The Label panel (docs/adr/023): the gist next to the agreement;
     the page holds the detail. */
  .label-panel {
    margin: -1rem 0 2rem;
    padding: 1rem 1.25rem;
    border: 1px dashed var(--color-border);
    border-radius: 8px;
    font-size: 0.9rem;
  }
  .label-panel-who {
    margin: 0 0 0.4rem;
  }
  .label-panel-quote {
    margin: 0 0 0.4rem;
    font-style: italic;
    opacity: 0.85;
  }
  .label-panel-link {
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .not-social {
    margin-bottom: 2rem;
    padding: 1.25rem;
    border-left: 3px solid var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
    border-radius: 0 6px 6px 0;
  }

  .not-social h2 {
    font-size: 1rem;
    font-weight: 700;
    margin-bottom: 0.4rem;
    color: var(--color-text);
  }

  .not-social p {
    font-size: 0.88rem;
    line-height: 1.6;
    color: var(--color-text-muted);
  }

  .agreement {
    margin-bottom: 2rem;
  }

  .agreement h2 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
    color: var(--color-text);
  }

  .agreement p {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  .agreement ul {
    list-style: none;
    padding: 0;
    margin: 0 0 1rem;
  }

  .agreement li {
    font-size: 0.88rem;
    color: var(--color-text);
    padding: 0.35rem 0;
    padding-left: 1.2rem;
    position: relative;
  }

  .agreement li::before {
    content: '•';
    position: absolute;
    left: 0;
    color: var(--color-primary);
    font-weight: 700;
  }

  .agree-check {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
    cursor: pointer;
    color: var(--color-text);
  }

  .agree-check input {
    width: 18px;
    height: 18px;
    accent-color: var(--color-primary);
  }

  /* Step 2: Interests */
  .step-interests h1,
  .step-patches h1 {
    font-size: clamp(1.5rem, 4vw, 2rem);
    font-weight: 700;
    line-height: 1.2;
    margin-bottom: 0.5rem;
    color: var(--color-text);
  }

  .subtitle {
    font-size: 0.9rem;
    color: var(--color-text-muted);
    margin-bottom: 1.5rem;
  }

  .back-link {
    border: none;
    background: none;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: 0;
    margin-bottom: 1rem;
    text-align: left;
    width: fit-content;
  }

  .back-link:hover {
    color: var(--color-primary);
  }

  .step-dots {
    display: flex;
    gap: 6px;
    margin-bottom: 1.5rem;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    border: 1.5px solid var(--color-border);
    background: transparent;
    transition: all 150ms ease;
  }

  .dot.active {
    background: var(--color-primary);
    border-color: var(--color-primary);
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
    margin-top: auto;
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

  /* Step 3: Patches */
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
    margin-top: auto;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  .completion {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
  }

  .completion h1 {
    font-size: 2rem;
    margin-bottom: 0.5rem;
  }

  @media (min-width: 640px) {
    .tag-grid {
      grid-template-columns: repeat(3, 1fr);
    }

    .step {
      padding: 4rem 2rem 3rem;
    }
  }
</style>
