<script>
  import { Heart, Wrench, UsersThree, LinkBreak, FrameCorners } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate, replaceRoute } from '../stores/router.svelte.js';
  import { scopedPath, surfaceForRoute } from '../lib/scope.js';
  import { identityColorForPatch } from '../lib/quiltTheme.js';
  import { textMatches } from '../lib/textMatch.js';
  import { motifComponentForPatch } from '../lib/patchIcons.js';
  import { quiltOrder } from '../lib/quiltLayout.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { getMembershipRoles, getPendingMembershipSlugs, loadMemberships } from '../stores/memberships.svelte.js';
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
  import DockedProfile from '../components/DockedProfile.svelte';

  let {
    quiltScope = 'local',
    routeName = 'home',
    // The patch docked over this surface, if any: App holds the address,
    // this surface knows which of its edges it can spare (docs/adr/094).
    dockedSlug = null,
    dockedHost = null,
    onDockClose = () => {},
  } = $props();


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

  // Which form the dock takes is a fact about the room, and this surface
  // measures the room already — the same width the inset above reads.
  // Below the breakpoint there is no room beside the canvas, so a profile
  // docks at its foot; above it, the cards pane's slot is a profile wide
  // (docs/adr/094).
  let dockForm = $derived(winW <= 768 ? 'sheet' : 'panel');

  // Mobile view toggle. 'main' shows the full-bleed background pane (quilt
  // OR map, per the route); 'list' shows the patch cards. Quilt-vs-map stays
  // driven by the route so deep links and the desktop toggle agree.
  let mobileView = $state('main'); // 'main' or 'list'

  // --- Patch list data ---
  let allPatches = $state([]);

  // The row this surface already has for the docked patch, handed on so a
  // tap paints on the first frame rather than waiting for a request
  // (docs/adr/094). Absent — a slug not in the fetched set, or a set still
  // in flight — the dock's head fetches like any cold load.
  let dockedSeed = $derived.by(() => {
    if (!dockedSlug) return null;
    return allPatches.find((p) => (
      p.slug === dockedSlug
      && (dockedHost ? remoteHost(p._source || '') === dockedHost : !p._source)
    )) || null;
  });
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
    // Recently added (docs/adr/074) asks how new a patch is to the quilt, and
    // answers with when its community arrived if one has, or when its listing
    // appeared if not.
    //
    // Two different questions live near each other here, and conflating them
    // is what this fallback fixes. `activated_at` answers "when did a
    // community arrive" — the bulletin's question (docs/adr/076), and
    // deliberately unanswerable for a directory listing nobody has claimed.
    // An ordering asks the looser question, which every patch can answer.
    // They coincide for a patch someone created, differ for one that was
    // claimed (the listing predates the arrival, and the arrival is the newer
    // fact), and only `created_at` exists for a listing.
    //
    // Ordering by arrival alone sent 47 of the reference instance's 52
    // patches to the tail as undated, throwing away the real story that 24
    // arrived on launch day and 22 more over the following month.
    if (getListOrder() === 'recent') {
      const newness = (p) => p.activated_at || p.created_at || '';
      return [...list].sort(
        (a, b) => newness(b).localeCompare(newness(a))
          || (a.name || '').localeCompare(b.name || '')
      );
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

  // Where the docked profile grows from (docs/adr/094): the card the reader
  // clicked, measured before the list gives way to the profile. A click on
  // the canvas has no card, so the profile simply arrives.
  let dockOrigin = $state(null);

  function handlePatchCardClick(patch) {
    const rect = document.querySelector(`[data-patch-id="${CSS.escape(patch.id)}"]`)?.getBoundingClientRect();
    dockOrigin = rect ? { left: rect.left, top: rect.top, width: rect.width, height: rect.height } : null;
    openPatch(patch.slug, patch._source || null);
  }

  // --- Card corner: the user's relationship to each patch ---
  // admin → "Manage" chip (link to workspace); member → "Member" chip;
  // pending request → "Requested" chip; follower/none → follow heart that
  // actually follows.
  //
  // The role map holds active rows only, so a pending request has no role
  // and would otherwise fall through to the follow heart — a button the
  // server refuses with a 409. It gets its own chip: the wait is the state.
  let roles = $derived(getMembershipRoles());
  let requested = $derived(getPendingMembershipSlugs());
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
  // pointer, hover previews and a click opens; without one there is a
  // single gesture, and it opens. What opening lands on is no longer this
  // surface's business: it navigates to the patch's address, and the room
  // decides whether that address is a page or a profile docked over this
  // surface (docs/adr/094). The sheet, its two heights and its pull live
  // in the dock, which is the thing being pulled.
  let hasPointer = $state(window.matchMedia('(hover: hover) and (pointer: fine)').matches);

  // The previewed patch, shared by both surfaces: whichever one the pointer
  // is over sets it, and both render emphasis from it (docs/adr/078). One
  // id rather than one per surface, because "the thing being pointed at" is
  // a single fact about the page.
  let previewing = $state(null);

  function preview(patch, fromMap = false) {
    previewing = patch?.id ?? null;
    // A preview that comes from the map brings its card to the reader; one
    // that comes from the card must not move the list under their pointer.
    if (!fromMap || !patch) return;
    document
      .querySelector(`[data-patch-id="${CSS.escape(patch.id)}"]`)
      ?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }

  // Opening a patch from this surface pushes its address; choosing another
  // one while a profile is already docked replaces it. The dock is the same
  // dock holding a different patch, and pushing each glance would make one
  // dismiss walk back through every patch the reader looked at.
  function openPatch(slug, source = null) {
    const path = source
      ? `/quilts/${remoteHost(source)}/patches/${slug}`
      : `/patches/${slug}`;
    if (dockedSlug) replaceRoute(path);
    else navigate(path);
  }

  // Tapping the surface behind a docked sheet dismisses it (docs/adr/078).
  // Only the sheet, and only where there is surface left to tap: in the
  // pane's slot the canvas stays live, so a pan that begins with a click
  // must not throw away what the reader is reading (docs/adr/094).
  function backgroundClick() {
    if (dockedSlug && dockForm === 'sheet') onDockClose();
  }

  function handleCanvasPatchClick(slug, source = null) {
    dockOrigin = null;
    openPatch(slug, source);
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

<!-- The card, in the one home it has (CONTEXT.md "Patch card"): the cards
     pane, which on a phone is the list view filling the screen. Its docked
     home retired with docs/adr/094 — a surface hands back the patch's own
     profile now, so a card standing alone at the foot of a canvas, naming
     the tap that would open the patch, has nothing left to be.
     A snippet rather than a component so the card keeps reaching this
     page's state directly instead of having eight callbacks threaded
     through it. -->
<!-- What the viewer is to this patch, and the one thing they can do about it
     from a card: admin → "Manage", member → "Member", anyone else → the
     follow heart. A labelled chip in the tile's corner — labelled because
     the ladder these three name (follower → member → admin) is the pattern
     a reader is here to learn, and an unlabelled wrench teaches nobody
     what an admin is. -->
{#snippet relationship(patch)}
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
  {:else if requested.has(patch.slug)}
    <span class="card-corner card-requested-chip" title="Your membership request is waiting on this patch's admins">
      <UsersThree size={14} weight="duotone" />
      <span>Requested</span>
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
{/snippet}

{#snippet patchCard(patch)}
          {@const Motif = motifComponentForPatch(patch)}
          <div
            class="patch-card"
            class:previewing={previewing === patch.id}
            data-patch-id={patch.id}
            onclick={() => handlePatchCardClick(patch)}
            onmouseenter={() => hasPointer && preview(patch)}
            onmouseleave={() => hasPointer && preview(null)}
            role="button"
            tabindex="0"
          >
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
                <span class="card-source-chip" title="On {remoteHost(patch._source)}">
                  {remoteHost(patch._source)}
                </span>
              {/if}
              {@render relationship(patch)}
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
  <div class="mobile-header" class:hidden={!!dockedSlug && dockForm === 'sheet'}>
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
      <!-- One voice per state: with the in-view lens on, the pane already
           says nothing is in view and offers the way back, so the map does
           not repeat it. With the lens off the pane lists everything and
           says nothing about the blank map, which is the case the map's own
           notice exists for (docs/adr/078). -->
      <MapView
        nodes={mapNodesFiltered}
        center={mapCenter}
        radius={mapRadius}
        insetRight={quiltInset}
        onMarkerClick={(node) => openPatch(node.slug, node._source || null)}
        onBackgroundClick={backgroundClick}
        announceOffscreen={!inViewActive}
        onPatchHover={(node) => hasPointer && preview(node, true)}
        hoveredId={previewing}
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
        onPatchHover={(patch) => hasPointer && preview(patch, true)}
        onBackgroundClick={backgroundClick}
        onInViewChange={reportInView}
      />
    {/if}

  </div>

  <!-- Patch cards panel — or, with a pointer and a patch docked, the
       patch's profile in the list's place (docs/adr/094): the same slot,
       the same width, the list back the moment it is dismissed. The pill,
       the chips and the canvas around it are untouched, and the card the
       reader clicked grows into it. -->
  <div class="cards-pane" class:mobile-hidden={mobileView !== 'list'}>
    {#if dockedSlug && dockForm === 'panel'}
      <DockedProfile
        slug={dockedSlug}
        host={dockedHost}
        seed={dockedSeed}
        form="panel"
        origin={dockOrigin}
        onClose={onDockClose}
      />
    {:else}
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
          <option value="recent">Recently added</option>
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
    {/if}
  </div>

  <!-- The profile docked as a sheet (docs/adr/094): the patch a reader
       touched, shown over the surface they touched it from rather than a
       card about it. A sibling of the panes rather than a child of the
       quilt pane, which is its own stacking context at z-index 0 to keep
       Leaflet's ~1000s off the chrome — inside it, no z-index the sheet
       could carry would clear the floating buttons, and the filter button
       sat on the card's description (docs/adr/078's own correction). The
       panel form is not here: it lives in the cards pane above, whose slot
       it takes. -->
  {#if dockedSlug && dockForm === 'sheet'}
    <DockedProfile
      slug={dockedSlug}
      host={dockedHost}
      seed={dockedSeed}
      form={dockForm}
      onClose={onDockClose}
    />
  {/if}
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

  /* The previewed card, whether the pointer is on it or on its pin. Border
     and lift only — nothing that changes the card's size, or the list would
     reflow under a pointer that is merely passing over the map. */
  .patch-card.previewing {
    border-color: var(--color-primary);
    box-shadow: 0 4px 16px var(--color-shadow);
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
  .card-requested-chip,
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

  /* A request nobody has answered: stated, not offered. Muted and italic
     so it reads as a state beside the chips that report standing. */
  .card-requested-chip {
    color: var(--color-text-muted);
    font-style: italic;
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
