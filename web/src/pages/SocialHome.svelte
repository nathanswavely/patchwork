<script>
  import { Heart, Wrench, UsersThree } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { identityColorForPatch } from '../lib/quiltTheme.js';
  import { motifComponentForPatch } from '../lib/patchIcons.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { getMembershipRoles, loadMemberships } from '../stores/memberships.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import {
    getSelectedTags,
    getSearchQuery,
    getInstanceModules,
    getSubmissionsEnabled,
    clearTags,
  } from '../stores/quilt.svelte.js';
  import {
    getRemoteFollows, findRemoteFollow,
    followRemotePatch, unfollowRemotePatch,
  } from '../stores/multiQuilt.svelte.js';
  import QuiltCanvas from '../components/QuiltCanvas.svelte';
  import MapView from '../components/MapView.svelte';
  import PatchTile from '../components/PatchTile.svelte';

  let { quiltScope = 'local', routeName = 'home', onScopeChange = () => {} } = $props();

  // --- Map view (the module gates the toggle and route) ---
  let mapEnabled = $derived(getInstanceModules().map !== false);
  let showMap = $derived(routeName === 'map' && mapEnabled);

  // --- Map view data (full node records carry lat/lng; the tree doesn't) ---
  let mapNodes = $state([]);
  let mapCenter = $state(null);
  let mapRadius = $state(10);

  async function loadMapData() {
    try {
      // The map answers to the scope switcher the same way the quilt and the
      // cards list do — otherwise "My Quilt" silently means "everything" here.
      const scopeParam = quiltScope === 'my' ? '&scope=my' : '';
      const [nodesResp, instance] = await Promise.all([
        api(`nodes?limit=500${scopeParam}`),
        api('instance'),
      ]);
      mapNodes = nodesResp.items || [];
      if (quiltScope === 'my') {
        // Remote follows with a stored position appear on the My Quilt map
        // too — every surface renders the same quilt (CONTEXT.md).
        const remote = getRemoteFollows()
          .filter((f) => f.snapshot?.latitude && f.snapshot?.longitude)
          .map((f) => ({
            id: f.node_ap_id,
            slug: f.node_slug,
            name: f.node_name || f.node_slug,
            latitude: f.snapshot.latitude,
            longitude: f.snapshot.longitude,
            tags: f.snapshot.tags || [],
            appearance: f.snapshot.appearance || null,
            _source: f.quilt_url,
          }));
        mapNodes = [...mapNodes, ...remote];
      }
      if (instance.geography) {
        mapCenter = { lat: instance.geography.latitude, lng: instance.geography.longitude };
        mapRadius = instance.geography.radius || 10;
      }
    } catch {
      mapNodes = [];
    }
  }

  $effect(() => {
    // quiltScope is a dependency, not just a value read inside the call —
    // without it the map never refetches when the scope switcher flips.
    void quiltScope;
    if (showMap) loadMapData();
  });

  let mapNodesFiltered = $derived.by(() => {
    const query = getSearchQuery();
    if (!query.trim()) return mapNodes;
    const q = query.toLowerCase();
    return mapNodes.filter(n =>
      n.name?.toLowerCase().includes(q) || n.description?.toLowerCase().includes(q)
    );
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
  let loading = $state(true);

  async function loadPatches() {
    loading = true;
    try {
      const resp = await api(`nodes/tree${quiltScope === 'my' ? '?scope=my' : ''}`);
      const tree = resp.tree || resp;
      allPatches = tree.children || [];
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
          _source: f.quilt_url,
        }));
        allPatches = [...allPatches, ...remote];
      }
    } catch {
      allPatches = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void quiltScope;
    void getRemoteFollows().length;
    loadPatches();
  });

  // Filter patches by active search/tags
  let filtered = $derived.by(() => {
    let list = allPatches;
    const tags = getSelectedTags();
    const query = getSearchQuery();

    if (tags.length > 0) {
      list = list.filter(p => (p.tags || []).some(t => tags.includes(t)));
    }
    if (query.trim()) {
      const q = query.toLowerCase();
      list = list.filter(p =>
        p.name?.toLowerCase().includes(q) || p.description?.toLowerCase().includes(q)
      );
    }
    return list;
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

  function handleCanvasPatchClick(slug, source = null) {
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

<svelte:window bind:innerWidth={winW} />

<div class="social-home">
  <!-- Mobile header: view toggle floating below the global bar (the bar
       already carries the scope switcher on mobile) -->
  <div class="mobile-header">
    <div class="mobile-pill-toggle">
      <button class="pill-option" class:active={mobileView === 'main' && !showMap} onclick={() => { if (showMap) navigate('/'); mobileView = 'main'; }}>Quilt</button>
      {#if mapEnabled}
        <button class="pill-option" class:active={mobileView === 'main' && showMap} onclick={() => { if (!showMap) navigate('/map'); mobileView = 'main'; }}>Map</button>
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
        onMarkerClick={(node) => node._source
          ? navigate(`/quilts/${remoteHost(node._source)}/patches/${node.slug}`)
          : navigate(`/patches/${node.slug}`)}
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
        onClearFilter={clearTags}
      />
    {/if}
  </div>

  <!-- Patch cards panel -->
  <div class="cards-pane" class:mobile-hidden={mobileView !== 'list'}>
    <div class="cards-header">
      <h2>Patches</h2>
      <span class="cards-count">{resultCount} results</span>
      {#if mapEnabled}
        <div class="view-toggle">
          <button class="view-option" class:active={!showMap} onclick={() => navigate('/')}>Quilt</button>
          <button class="view-option" class:active={showMap} onclick={() => navigate('/map')}>Map</button>
        </div>
      {/if}
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
          {#if getSelectedTags().length > 0 || getSearchQuery().trim()}
            <!-- Name the active lenses (docs/adr/022): composed narrowing
                 must explain itself where it produces nothing. -->
            <p class="muted">
              No patches match your
              {getSelectedTags().length > 0 && getSearchQuery().trim() ? 'search and filter'
                : getSelectedTags().length > 0 ? 'filter' : 'search'}{quiltScope === 'my' ? ' in My Quilt' : ''}.
            </p>
            <div class="empty-actions">
              {#if getSelectedTags().length > 0}
                <button class="btn btn-secondary" onclick={clearTags}>Clear filter</button>
              {/if}
              {#if quiltScope === 'my'}
                <button class="btn btn-secondary" onclick={() => onScopeChange('local')}>Search the whole quilt</button>
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
            {@const Motif = motifComponentForPatch(patch)}
            <div class="patch-card" onclick={() => handlePatchCardClick(patch)} role="button" tabindex="0">
              <div class="card-image" style="background: {identityColorForPatch(patch)}">
                <PatchTile {patch} />
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
                <p class="card-stats">{patch.is_unclaimed ? `${patch.follower_count || 0} Following` : `${patch.member_count || 0} Members`} - {patch.event_count || 0} Upcoming Events</p>
                {#if patch.description}
                  <p class="card-desc">{patch.description}</p>
                {/if}
              </div>
            </div>
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
    height: 100dvh; /* track the visible viewport so absolutely-positioned
                       chrome (the mobile view switcher) aligns with the
                       fixed bottom nav on mobile browsers */
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

  /* Quilt / Map view switcher — lives in the cards header (right-aligned) so it
     never overlaps the full-bleed quilt tiles or the Leaflet controls. */
  .view-toggle {
    margin-left: auto;
    display: flex;
    background: var(--color-overlay);
    border-radius: 999px;
    padding: 3px;
  }

  .view-option {
    padding: 4px 12px;
    border: none;
    background: none;
    border-radius: 999px;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: background 150ms ease, color 150ms ease;
  }

  .view-option.active {
    background: var(--color-surface);
    color: var(--color-text);
    box-shadow: 0 1px 3px var(--color-shadow);
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


  .cards-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 12px 16px;
  }

  /* ================================================================
     PATCH CARDS
     ================================================================ */
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
      height: 100dvh; /* dynamic viewport so the floating view switcher clears
                         the fixed bottom nav on real mobile browsers */
    }

    /* View toggle floats just above the bottom nav bar, in thumb reach.
       pointer-events pass through around the pill so the canvas stays
       pannable. */
    .mobile-header {
      display: flex;
      justify-content: center;
      position: absolute;
      bottom: calc(64px + env(safe-area-inset-bottom, 0px));
      left: 0;
      right: 0;
      padding: 0 16px;
      z-index: 20;
      pointer-events: none;
    }

    .mobile-pill-toggle {
      display: flex;
      pointer-events: auto;
      background: var(--color-glass);
      backdrop-filter: blur(16px);
      -webkit-backdrop-filter: blur(16px);
      border-radius: 999px;
      padding: 4px;
      box-shadow: 0 2px 12px var(--color-shadow);
    }

    .pill-option {
      padding: 6px 16px;
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
