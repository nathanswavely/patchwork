<script>
  import { X, Heart, Wrench, UsersThree, LinkBreak, FrameCorners } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { scopedPath, surfaceForRoute } from '../lib/scope.js';
  import { identityColorForPatch } from '../lib/quiltTheme.js';
  import { textMatches } from '../lib/textMatch.js';
  import { motifComponentForPatch } from '../lib/patchIcons.js';
  import { quiltOrder } from '../lib/quiltLayout.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { getMembershipRoles, loadMemberships } from '../stores/memberships.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import {
    getSelectedTags,
    getSearchQuery,
    getInstanceModules,
    getSubmissionsEnabled,
    resetFilters,
    getListOrder,
    setListOrder,
    getInViewOnly,
    setInViewOnly,
    toggleInViewOnly,
  } from '../stores/quilt.svelte.js';
  import {
    getRemoteFollows, findRemoteFollow,
    followRemotePatch, unfollowRemotePatch,
  } from '../stores/multiQuilt.svelte.js';
  import QuiltCanvas from '../components/QuiltCanvas.svelte';
  import MapView from '../components/MapView.svelte';
  import PatchTile from '../components/PatchTile.svelte';

  let { quiltScope = 'local', routeName = 'home' } = $props();

  // Scope-aware surface targets (docs/adr/035): the quilt/map toggles stay
  // in the scope you're already in — `/` vs `/my`, `/map` vs `/map/my`.
  let quiltPath = $derived(scopedPath('quilt', quiltScope));
  let mapPath = $derived(scopedPath('map', quiltScope));

  // --- Map view (the module gates the toggle and route) ---
  let mapEnabled = $derived(getInstanceModules().map !== false);
  let showMap = $derived(surfaceForRoute(routeName) === 'map' && mapEnabled);

  // --- Map view data (full node records carry lat/lng; the tree doesn't) ---
  let mapCenter = $state(null);
  let mapRadius = $state(10);

  async function loadMapData() {
    try {
      // The map reads the same source as the cards and the quilt. It used to
      // load `nodes?limit=500`, whose payload carries neither tags nor
      // counts — so every marker fell back to the quilt mark for want of a
      // motif, and patchActivity was zero for every patch, quietly making
      // label priority, marker stacking and cluster anchoring arbitrary
      // (docs/adr/078). One source also means the map and the cards can
      // never disagree about what a patch is.
      const instance = await api('instance');
      if (instance.geography) {
        mapCenter = { lat: instance.geography.latitude, lng: instance.geography.longitude };
        mapRadius = instance.geography.radius || 10;
      }
    } catch {
      mapCenter = null;
    }
  }

  let mapNodesFiltered = $derived.by(() => {
    const tags = getSelectedTags();
    const query = getSearchQuery();
    let list = mapNodes;

    if (tags.length > 0) {
      list = list.filter(n => (n.tags || []).some(t => tags.includes(t)));
    }
    if (query.trim()) {
      list = list.filter(n =>
        textMatches(n.name, query) || textMatches(n.description, query)
      );
    }
    return list;
  });

  // On desktop the cards panel floats over the right 45% of the canvas, so
  // the quilt centers itself in the remaining left portion. On mobile the
  // panes toggle full-screen instead — no inset.
  let winW = $state(window.innerWidth);
  let quiltInset = $derived(winW <= 768 ? 0 : 0.45);

  // Mobile view toggle. 'main' shows the full-bleed background pane (quilt
  // OR map, per the route); 'list' shows the patch cards. Quilt-vs-map stays
  // driven by the route so deep links and the desktop toggle agree.
  let mobileView = $state('main'); // 'main' or 'list'

  // --- Patch list data ---
  let allPatches = $state([]);
  // Affinity links from the same tree response the canvas reads. Quilt order
  // is computed from them, so the list and the canvas are ordering on
  // identical inputs rather than on two ideas of the same thing.
  let affinityData = $state([]);
  let loading = $state(true);

  // Placed patches, from the list the cards already loaded.
  let mapNodes = $derived(allPatches.filter(
    (p) => p.latitude != null && p.longitude != null,
  ));

  // Patch ids the canvas last reported as inside its viewport (docs/adr/074).
  // Null until a canvas has reported: "no layout yet" is not "nothing in
  // view", and treating them alike blinks the list empty on load.
  let inViewIds = $state(null);

  function reportInView(ids) {
    inViewIds = new Set(ids);
  }

  // A surface change invalidates the report: the quilt's viewport says nothing
  // about the map's, and the two canvases never both exist. Clearing to null
  // rather than to an empty set means the list shows everything until the new
  // canvas reports, instead of blinking empty on the way across.
  $effect(() => {
    void showMap;
    inViewIds = null;
  });

  async function loadPatches() {
    loading = true;
    try {
      const resp = await api(`nodes/tree${quiltScope === 'my' ? '?scope=my' : ''}`);
      const tree = resp.tree || resp;
      allPatches = tree.children || [];
      affinityData = resp.affinity || [];
      if (quiltScope === 'my') {
        // Remote follows join the cards list from their stored snapshots,
        // marked by source; the canvas refreshes those snapshots on
        // successful live fetches.
        const remote = getRemoteFollows().map((f) => ({
          id: f.node_ap_id,
          slug: f.node_slug,
          name: f.node_name || f.node_slug,
          description: f.snapshot?.description || '',
          tags: f.snapshot?.tags || [],
          icon: f.snapshot?.icon || '',
          appearance: f.snapshot?.appearance || null,
          member_count: f.snapshot?.member_count || 0,
          event_count: f.snapshot?.event_count || 0,
          is_unclaimed: !!f.snapshot?.is_unclaimed,
          // Carried so a followed remote patch still reaches the map, which
          // now reads this same list rather than loading its own.
          latitude: f.snapshot?.latitude ?? null,
          longitude: f.snapshot?.longitude ?? null,
          _source: f.quilt_url,
        }));
        allPatches = [...allPatches, ...remote];
      }
    } catch {
      allPatches = [];
      affinityData = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void quiltScope;
    void getRemoteFollows().length;
    loadPatches();
  });

  // Narrowed by the filter (docs/adr/022): tags OR together, the search chip
  // matches name or description. This is the set the canvas lays out, so it is
  // also the set quilt order is computed from.
  let narrowed = $derived.by(() => {
    let list = allPatches;
    const tags = getSelectedTags();
    const query = getSearchQuery();

    if (tags.length > 0) {
      list = list.filter(p => (p.tags || []).some(t => tags.includes(t)));
    }
    if (query.trim()) {
      list = list.filter(p =>
        textMatches(p.name, query) || textMatches(p.description, query)
      );
    }
    return list;
  });

  // Quilt order (docs/adr/074): the list reads the quilt. The order is the
  // layout engine's own placement pass over the same patches the canvas is
  // showing — largest tile at the centre, then outward by affinity — so the
  // two panes agree, including after a filter re-sews the quilt. It makes no
  // ranking claim of its own; it surfaces the one already drawn on screen.
  //
  // Remote follows (My Quilt, docs/adr/024) carry no affinity links and hold
  // no tile in the home layout, so they keep the tail rather than being
  // handed a place in a quilt they are not part of.
  let ordered = $derived.by(() => {
    const list = narrowed;
    if (getListOrder() === 'alpha') {
      return [...list].sort((a, b) => (a.name || '').localeCompare(b.name || ''));
    }
    const home = list.filter(p => !p._source);
    const remote = list.filter(p => p._source);
    const rank = new Map(quiltOrder(home, affinityData).map((id, i) => [id, i]));
    const place = (p) => rank.has(p.id) ? rank.get(p.id) : Number.MAX_SAFE_INTEGER;
    return [...home].sort((a, b) => place(a) - place(b)).concat(remote);
  });

  // The in-view lens is available only where both panes are on screen at
  // once. Below the breakpoint the panes *toggle* (mobileView) and the cards
  // header is display:none, so there would be neither a control to set the
  // lens nor a canvas to see it working — narrowing a list from a viewport
  // nobody can see is the silent-lens failure docs/adr/022 exists to prevent.
  // This is an absence, not a second behaviour: the lens needs two visible
  // panes, and mobile has one.
  let lensAvailable = $derived(winW > 768);
  let inViewActive = $derived(lensAvailable && getInViewOnly());

  let filtered = $derived.by(() => {
    if (!inViewActive || !inViewIds) return ordered;
    const ids = inViewIds;
    return ordered.filter(p => ids.has(p.id));
  });

  function remoteHost(source) {
    return source.replace(/^https?:\/\//, '');
  }

  function handlePatchCardClick(patch) {
    if (patch._source) {
      navigate(`/quilts/${remoteHost(patch._source)}/patches/${patch.slug}`);
    } else {
      navigate(`/patches/${patch.slug}`);
    }
  }

  // --- Card corner: the user's relationship to each patch ---
  // admin → "Manage" chip (link to workspace); member → "Member" chip;
  // follower/none → follow heart that actually follows.
  let roles = $derived(getMembershipRoles());
  let busySlugs = $state(new Set());

  async function toggleFollow(e, patch) {
    e.stopPropagation();
    if (!isLoggedIn()) { navigate('/login'); return; }
    const slug = patch.slug;
    if (busySlugs.has(slug)) return;
    busySlugs = new Set(busySlugs).add(slug);
    const isFollowing = roles.get(slug) === 'follower';
    try {
      if (isFollowing) {
        await api(`nodes/${slug}/leave`, { method: 'POST' });
        showToast(`Unfollowed ${patch.name}`, 'success');
      } else {
        await api(`nodes/${slug}/join`, { method: 'POST', body: { role: 'follower' } });
        showToast(`Following ${patch.name}`, 'success');
      }
      await loadMemberships();
    } catch (err) {
      showToast(err.message || 'Something went wrong', 'error');
    } finally {
      const next = new Set(busySlugs);
      next.delete(slug);
      busySlugs = next;
    }
  }

  function goManage(e, patch) {
    e.stopPropagation();
    navigate(`/patches/${patch.slug}/governance`);
  }

  // A preview costs a gesture the device can spare (docs/adr/078). With a
  // pointer, hover previews and a click opens. Without one there is a single
  // gesture, so the first tap docks the patch's card and the card is how the
  // patch is opened. Same rule on the quilt and the map.
  let docked = $state(null);
  let hasPointer = $state(window.matchMedia('(hover: hover) and (pointer: fine)').matches);

  function touchSelect(patch) {
    if (hasPointer) return false;
    docked = patch;
    return true;
  }

  function handleCanvasPatchClick(slug, source = null) {
    const patch = allPatches.find(p => p.slug === slug && (p._source || null) === source);
    if (patch && touchSelect(patch)) return;
    if (source) {
      navigate(`/quilts/${remoteHost(source)}/patches/${slug}`);
    } else {
      navigate(`/patches/${slug}`);
    }
  }

  // Follow/unfollow a patch on another quilt — the row lives at home
  // (docs/adr/024), so this works from any browsed quilt.
  let busyRemote = $state(new Set());
  async function toggleRemoteFollow(e, patch) {
    e.stopPropagation();
    if (!isLoggedIn()) { navigate('/login'); return; }
    const key = `${patch._source}:${patch.slug}`;
    if (busyRemote.has(key)) return;
    busyRemote = new Set(busyRemote).add(key);
    try {
      const existing = findRemoteFollow(patch._source, patch.slug);
      if (existing) {
        await unfollowRemotePatch(existing.id);
        showToast(`Unfollowed ${patch.name}`, 'success');
      } else {
        await followRemotePatch({ quiltUrl: patch._source, node: patch });
        showToast(`Following ${patch.name}`, 'success');
      }
    } catch (err) {
      showToast(err.message || 'Something went wrong', 'error');
    } finally {
      const next = new Set(busyRemote);
      next.delete(key);
      busyRemote = next;
    }
  }

  let resultCount = $derived(filtered.length);
</script>

<!-- One card definition, two homes (docs/adr/078, CONTEXT.md "Patch card"):
     the cards pane where there is room for a pane, and docked at the foot
     of the surface where there is not. A snippet rather than a component so
     the card keeps reaching this page's state directly instead of having
     eight callbacks threaded through it. -->
{#snippet patchCard(patch)}
          {@const Motif = motifComponentForPatch(patch)}
          <div class="patch-card" onclick={() => handlePatchCardClick(patch)} role="button" tabindex="0">
            <div class="card-image" style="background: {identityColorForPatch(patch)}">
              <PatchTile {patch} />
              <!-- Same mark the quilt tile wears, same corner (docs/adr/030).
                   The right corner is spoken for by the role/follow chip. -->
              {#if patch.is_unclaimed}
                <span class="card-unclaimed" title="Unclaimed" aria-label="Unclaimed">
                  <LinkBreak size={13} weight="bold" />
                </span>
              {/if}
              {#if patch._source}
                {@const remoteFollowing = !!findRemoteFollow(patch._source, patch.slug)}
                <button
                  class="card-corner card-follow-btn"
                  class:following={remoteFollowing}
                  onclick={(e) => toggleRemoteFollow(e, patch)}
                  disabled={busyRemote.has(`${patch._source}:${patch.slug}`)}
                  title={remoteFollowing ? 'Unfollow' : 'Follow'}
                  aria-pressed={remoteFollowing}
                >
                  <Heart size={14} weight={remoteFollowing ? 'fill' : 'duotone'} />
                  <span>{remoteFollowing ? 'Following' : 'Follow'}</span>
                </button>
                <span class="card-source-chip" title="On {remoteHost(patch._source)}">
                  {remoteHost(patch._source)}
                </span>
              {:else if roles.get(patch.slug) === 'admin'}
                <button class="card-corner card-manage-chip" onclick={(e) => goManage(e, patch)} title="You manage this patch">
                  <Wrench size={14} weight="duotone" />
                  <span>Manage</span>
                </button>
              {:else if roles.get(patch.slug) === 'member'}
                <span class="card-corner card-member-chip" title="You're a member of this patch">
                  <UsersThree size={14} weight="duotone" />
                  <span>Member</span>
                </span>
              {:else}
                {@const following = roles.get(patch.slug) === 'follower'}
                <button
                  class="card-corner card-follow-btn"
                  class:following
                  onclick={(e) => toggleFollow(e, patch)}
                  disabled={busySlugs.has(patch.slug)}
                  title={following ? 'Unfollow' : 'Follow'}
                  aria-pressed={following}
                >
                  <Heart size={14} weight={following ? 'fill' : 'duotone'} />
                  <span>{following ? 'Following' : 'Follow'}</span>
                </button>
              {/if}
            </div>
            <div class="card-body">
              <h3 class="card-title">
                <span class="card-motif" style="background: {identityColorForPatch(patch)}" aria-hidden="true">
                  <Motif size={12} weight="fill" color="#fff" />
                </span>
                {patch.name}
              </h3>
              <!-- "events", not "upcoming events": event_count is every
                   active event a patch owns, past and future, and for a
                   remote patch it comes from a cross-quilt snapshot that
                   carries no upcoming figure at all (CONTEXT.md
                   "Upcoming events"). -->
              <p class="card-stats">{patch.is_unclaimed ? `${patch.follower_count || 0} Following` : `${patch.member_count || 0} Members`} - {patch.event_count || 0} Events</p>
              {#if patch.description}
                <p class="card-desc">{patch.description}</p>
              {/if}
            </div>
          </div>
{/snippet}

<svelte:window bind:innerWidth={winW} />

<div class="social-home">
  <!-- Mobile header: view toggle floating below the global bar (the bar
       already carries the scope switcher on mobile) -->
  <!-- One temporary overlay at a time: the view pill steps aside while a
       card is docked over the same corner of the screen. -->
  <div class="mobile-header" class:hidden={!!docked}>
    <div class="mobile-pill-toggle">
      <button class="pill-option" class:active={mobileView === 'main' && !showMap} onclick={() => { if (showMap) navigate(quiltPath); mobileView = 'main'; }}>Quilt</button>
      {#if mapEnabled}
        <button class="pill-option" class:active={mobileView === 'main' && showMap} onclick={() => { if (!showMap) navigate(mapPath); mobileView = 'main'; }}>Map</button>
      {/if}
      <button class="pill-option" class:active={mobileView === 'list'} onclick={() => mobileView = 'list'}>List</button>
    </div>
  </div>

  <!-- Main pane: quilt canvas or community map -->
  <div class="quilt-pane" class:mobile-hidden={mobileView === 'list'}>
    {#if showMap}
      <MapView
        nodes={mapNodesFiltered}
        center={mapCenter}
        radius={mapRadius}
        insetRight={quiltInset}
        onMarkerClick={(node) => {
          if (touchSelect(node)) return;
          navigate(node._source
            ? `/quilts/${remoteHost(node._source)}/patches/${node.slug}`
            : `/patches/${node.slug}`);
        }}
        onBackgroundClick={() => { docked = null; }}
        onInViewChange={reportInView}
      />
    {:else}
      <QuiltCanvas
        filterTags={getSelectedTags()}
        searchQuery={getSearchQuery()}
        selectedPatchSlug={null}
        onPatchClick={handleCanvasPatchClick}
        myPatchRoles={roles}
        {quiltScope}
        insetRight={quiltInset}
        onClearFilter={resetFilters}
        onInViewChange={reportInView}
      />
    {/if}

    <!-- The card, docked: where there is no pointer there is no hover, so the
         first tap previews here and this card is how the patch is opened
         (docs/adr/078). Serves the quilt and the map alike. -->
    {#if docked}
      <div class="docked-card">
        <button class="docked-dismiss" onclick={() => { docked = null; }} aria-label="Dismiss">
          <X size={16} weight="bold" />
        </button>
        {@render patchCard(docked)}
      </div>
    {/if}
  </div>

  <!-- Patch cards panel -->
  <div class="cards-pane" class:mobile-hidden={mobileView !== 'list'}>
    <!-- The list's header carries only the list's own controls
         (docs/adr/074). The Quilt/Map switch used to sit here and now lives
         on the canvas, which is the thing it changes. -->
    <div class="cards-header">
      <h2>Patches</h2>
      <span class="cards-count">
        {#if inViewActive}{resultCount} of {ordered.length} in view{:else}{resultCount} results{/if}
      </span>
      <div class="list-controls">
        {#if lensAvailable}
          <button
            class="list-control"
            class:active={getInViewOnly()}
            aria-pressed={getInViewOnly()}
            onclick={toggleInViewOnly}
            title={getInViewOnly()
              ? 'Showing only the patches in view. Click to show the whole quilt.'
              : 'Show only the patches in view'}
          >
            <FrameCorners size={14} weight="bold" />
            In view
          </button>
        {/if}
        <select
          class="list-order"
          aria-label="Order patches"
          value={getListOrder()}
          onchange={(e) => setListOrder(e.currentTarget.value)}
        >
          <option value="quilt">Quilt order</option>
          <option value="alpha">A→Z</option>
        </select>
      </div>
    </div>

    <div class="cards-scroll">
      {#if loading}
        <div class="cards-loading">
          {#each Array(6) as _}
            <div class="card-skeleton">
              <div class="skel-image"></div>
              <div class="skel-text"></div>
              <div class="skel-text short"></div>
            </div>
          {/each}
        </div>
      {:else if filtered.length === 0}
        <div class="cards-empty">
          {#if inViewActive && ordered.length > 0}
            <!-- The lens narrowed to nothing while the quilt still holds
                 patches: say how many, and offer the one step back. -->
            <p class="muted">
              No patches in view — {ordered.length} elsewhere on the quilt.
            </p>
            <div class="empty-actions">
              <button class="btn btn-secondary" onclick={() => setInViewOnly(false)}>Show them all</button>
            </div>
          {:else if getSelectedTags().length > 0 || getSearchQuery().trim()}
            <!-- Name the active lenses (docs/adr/033): composed narrowing
                 must explain itself where it produces nothing. -->
            <p class="muted">
              No patches match your filter{quiltScope === 'my' ? ' in My Quilt' : ''}.
            </p>
            <div class="empty-actions">
              <button class="btn btn-secondary" onclick={resetFilters}>Clear filter</button>
              {#if quiltScope === 'my'}
                <button class="btn btn-secondary" onclick={() => navigate(scopedPath(surfaceForRoute(routeName) || 'quilt', 'local'))}>Search the whole quilt</button>
              {/if}
              {#if getSearchQuery().trim() && getSubmissionsEnabled()}
                <button class="btn btn-secondary" onclick={() => navigate(`/submit?name=${encodeURIComponent(getSearchQuery().trim())}`)}>Suggest a patch</button>
              {/if}
            </div>
          {:else}
            <p class="muted">No patches here yet.</p>
            {#if getSubmissionsEnabled()}
              <a href="/submit" class="suggest-link" onclick={(e) => { e.preventDefault(); navigate('/submit'); }}>Know a group that belongs here? Suggest a patch</a>
            {/if}
          {/if}
        </div>
      {:else}
        <div class="cards-grid">
          {#each filtered as patch (patch.id)}
            {@render patchCard(patch)}
          {/each}
        </div>
        {#if getSubmissionsEnabled()}
          <div class="cards-footer">
            <a href="/submit" class="suggest-link" onclick={(e) => { e.preventDefault(); navigate('/submit'); }}>Know a group that's missing? Suggest a patch</a>
          </div>
        {/if}
      {/if}
    </div>
  </div>
</div>

<style>
  .social-home {
    position: relative;
    display: flex;
    height: 100vh;
    height: 100dvh; /* track the visible viewport so the panes fill the
                       screen without the page scrolling behind them.
                       Nothing positions against this box's bottom edge —
                       the floating chrome is fixed to the viewport (see
                       .mobile-header below). */
    overflow: hidden;
  }

  /* ================================================================
     MOBILE HEADER — hidden on desktop/tablet, shown on mobile
     ================================================================ */
  .mobile-header {
    display: none;
  }

  /* ================================================================
     QUILT CANVAS — full-bleed behind everything; the quilt itself
     centers in the left portion via insetRight
     ================================================================ */
  .quilt-pane {
    position: absolute;
    inset: 0;
    min-width: 0;
    overflow: hidden;
    /* Own stacking context (z-index integer + positioned) so Leaflet's
       internal panes/controls — which carry z-index up to ~1000 — stay
       trapped below the chrome (bar 60, nav 55, pill 20) and the floating
       cards (10) instead of escaping to the root context. */
    z-index: 0;
  }

  /* ================================================================
     CARDS PANE — floats over the right side of the canvas; the pane
     itself is transparent so the quilt pans behind the cards
     ================================================================ */
  .cards-pane {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 45%;
    display: flex;
    flex-direction: column;
    padding-top: 56px; /* clear the glass top bar */
    min-height: 0;
    z-index: 10;
  }

  .cards-header {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin: 12px 16px 0;
    padding: 10px 14px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-glass);
    backdrop-filter: blur(14px) saturate(1.2);
    -webkit-backdrop-filter: blur(14px) saturate(1.2);
    flex-shrink: 0;
  }

  .cards-header h2 {
    font-size: 1.1rem;
    font-weight: 700;
  }

  .cards-count {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  /* The list's own controls (docs/adr/074) — both change the list, so both
     live on it. */
  .list-controls {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .list-control,
  .list-order {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 10px;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: none;
    font-family: inherit;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: background 150ms ease, color 150ms ease, border-color 150ms ease;
  }

  .list-control:hover,
  .list-order:hover {
    color: var(--color-text);
    border-color: var(--color-primary);
  }

  .list-control.active {
    background: var(--color-primary);
    border-color: var(--color-primary);
    color: var(--color-btn-on-primary);
  }


  .cards-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 12px 16px;
  }

  /* ================================================================
     PATCH CARDS
     ================================================================ */
  /* The card, docked (docs/adr/078). Sits over the foot of the surface it
     was summoned from, leaving most of the quilt or map visible behind it —
     a preview you glance at, not a page you land on. Touch-only: with a
     pointer, hover previews into the pane instead. */
  .docked-card {
    position: absolute;
    left: 0;
    right: 0;
    /* Clear of the shell's bottom rail, which is a tab bar at this width. */
    bottom: var(--shell-rail-h, 0px);
    /* Above Leaflet's own controls (attribution sits at 800): the card is
       app chrome laid over the map, not a thing inside it. */
    z-index: 900;
    padding: 0.6rem;
    padding-bottom: calc(0.6rem + env(safe-area-inset-bottom, 0px));
    background: var(--color-bg);
    border-top: 1px solid var(--color-border);
    box-shadow: 0 -6px 20px var(--color-shadow);
    animation: docked-rise 140ms ease-out;
  }

  @keyframes docked-rise {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .docked-card { animation: none; }
  }

  .docked-dismiss {
    position: absolute;
    top: 0.35rem;
    right: 0.45rem;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: none;
    border-radius: 50%;
    background: var(--color-surface);
    color: var(--color-text-muted);
    box-shadow: 0 1px 4px var(--color-shadow);
    cursor: pointer;
  }

  .mobile-header.hidden {
    display: none;
  }

  .cards-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
  }

  .patch-card {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
    cursor: pointer;
    background: var(--color-surface);
    text-align: left;
    box-shadow: 0 2px 10px var(--color-shadow);
    transition: box-shadow 150ms ease, border-color 150ms ease;
    padding: 0;
  }

  .patch-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 4px 16px var(--color-shadow);
  }

  .card-image {
    height: 100px;
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .card-corner {
    position: absolute;
    top: 8px;
    right: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: var(--color-glass);
    color: var(--color-text);
  }

  /* Unclaimed mark: matches the quilt tile's — dark disc, white broken link. */
  .card-unclaimed {
    position: absolute;
    top: 8px;
    left: 8px;
    width: 22px;
    height: 22px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--color-fabric-scrim);
    color: var(--color-on-fabric);
  }

  /* Source chip: which quilt a remote patch lives on (docs/adr/024). */
  .card-source-chip {
    position: absolute;
    bottom: 8px;
    left: 8px;
    background: var(--color-glass);
    color: var(--color-text);
    font-size: 0.68rem;
    font-weight: 700;
    padding: 2px 8px;
    border-radius: 999px;
    max-width: 70%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-manage-chip,
  .card-member-chip,
  .card-follow-btn {
    gap: 4px;
    padding: 5px 10px;
    border-radius: 999px;
    font-size: 0.7rem;
    font-weight: 700;
    font-family: inherit;
  }

  .card-manage-chip,
  .card-follow-btn {
    cursor: pointer;
    transition: color 150ms ease;
  }

  .card-manage-chip:hover {
    color: var(--color-primary);
  }

  .card-follow-btn:hover,
  .card-follow-btn.following {
    color: var(--color-error);
  }

  .card-follow-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .card-body {
    padding: 10px 12px;
  }

  .card-motif {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    flex-shrink: 0;
    vertical-align: -4px;
    margin-right: 4px;
  }

  .card-title {
    font-size: 0.9rem;
    font-weight: 700;
    color: var(--color-text);
    margin-bottom: 2px;
  }

  .card-stats {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--color-text);
    margin-bottom: 4px;
  }

  .card-desc {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    line-height: 1.4;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  /* Skeletons */
  .cards-loading {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
  }

  .card-skeleton {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--color-surface);
  }

  .skel-image {
    height: 100px;
    background: var(--color-overlay);
  }

  .skel-text {
    height: 14px;
    margin: 10px 12px 0;
    background: var(--color-overlay);
    border-radius: 4px;
  }

  .skel-text.short {
    width: 60%;
    margin-bottom: 10px;
  }

  .cards-empty {
    text-align: center;
    padding: 1.25rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-glass);
    backdrop-filter: blur(14px);
    -webkit-backdrop-filter: blur(14px);
  }

  .cards-empty .suggest-link {
    display: inline-block;
    margin-top: 0.5rem;
  }

  .empty-actions {
    display: flex;
    justify-content: center;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 0.75rem;
  }

  .cards-footer {
    text-align: center;
    padding: 1rem 0 0.5rem;
  }

  .suggest-link {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--color-text-muted);
    text-decoration: none;
  }

  .suggest-link:hover {
    color: var(--color-primary);
  }

  /* ================================================================
     TABLET — side-by-side, 1-col cards
     ================================================================ */
  @media (max-width: 1024px) {
    .cards-grid {
      grid-template-columns: 1fr;
    }

    .cards-loading {
      grid-template-columns: 1fr;
    }
  }

  /* ================================================================
     MOBILE — full-screen toggle, brand + tab switcher at top
     ================================================================ */
  @media (max-width: 768px) {
    .social-home {
      flex-direction: column;
      height: 100vh; /* full bleed — the quilt shows behind the chrome */
      height: 100dvh; /* dynamic viewport so the panes stop at the visible
                         bottom rather than under the browser's own bar */
    }

    /* View toggle floats just above the bottom nav bar, in thumb reach.
       pointer-events pass through around the pill so the canvas stays
       pannable.

       FIXED, not absolute, and sharing --pw-canvas-chrome-bottom with the
       info and filter buttons in SocialShell: the three are one floating
       row and must measure from the same box. Absolute inside the 100dvh
       .social-home agreed with those fixed buttons in a narrowed desktop
       window and sat about a safe-area's height above them on an iPhone,
       where the box a fixed element resolves against and the one 100dvh
       sizes are not reliably the same. Same offset was never enough. */
    .mobile-header {
      display: flex;
      justify-content: center;
      position: fixed;
      bottom: var(--pw-canvas-chrome-bottom);
      left: 0;
      right: 0;
      padding: 0 16px;
      z-index: 20;
      pointer-events: none;
    }

    /* 4px padding + a 28px option = 36px, the height of the info and
       filter buttons it sits between. Pinned rather than left to the
       button's default line box, so the row stays level. */
    .mobile-pill-toggle {
      display: flex;
      align-items: center;
      height: 36px;
      box-sizing: border-box;
      pointer-events: auto;
      background: var(--color-glass);
      backdrop-filter: blur(16px);
      -webkit-backdrop-filter: blur(16px);
      border-radius: 999px;
      padding: 4px;
      box-shadow: 0 2px 12px var(--color-shadow);
    }

    .pill-option {
      display: flex;
      align-items: center;
      height: 28px;
      padding: 0 16px;
      border: none;
      background: none;
      border-radius: 999px;
      font-size: 0.82rem;
      font-weight: 600;
      color: var(--color-text-muted);
      cursor: pointer;
      transition: background 150ms ease, color 150ms ease;
    }

    .pill-option.active {
      background: var(--color-surface);
      color: var(--color-text);
      box-shadow: 0 1px 3px var(--color-shadow);
    }

    /* Quilt pane: back in flow, full screen when active */
    .quilt-pane {
      position: relative;
      inset: auto;
      flex: 1;
      min-height: 0;
    }

    /* Cards pane: back in flow, full screen when active, opaque again.
       Top padding clears the fixed bar; the scroll area's bottom padding
       clears the bottom nav bar + the floating view toggle above it. */
    .cards-pane {
      position: relative;
      inset: auto;
      width: 100%;
      flex: 1;
      padding-top: 68px;
      background: var(--color-bg);
    }

    .cards-scroll {
      padding-bottom: calc(124px + env(safe-area-inset-bottom, 0px));
    }

    .cards-header {
      display: none;
    }

    /* Toggle visibility */
    .mobile-hidden {
      display: none !important;
    }
  }
</style>
