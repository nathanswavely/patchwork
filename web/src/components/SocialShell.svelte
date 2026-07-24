<script>
  /**
   * Discovery shell: the global bar (with the scope switcher and quilt
   * search in its slots) plus the collapsible left rail and mobile pill
   * nav. Workspaces and the admin panel do NOT render inside this — they
   * are full-screen takeovers with their own shells (docs/adr/005).
   */
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import {
    getInstanceName,
    getInstanceIconUrl,
    getNeighborQuilts,
    setSearchQuery,
    getActiveFilterCount,
  } from '../stores/quilt.svelte.js';
  import { switcherQuilts, fetchQuiltInfo } from '../stores/multiQuilt.svelte.js';
  import GlobalBar from './GlobalBar.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import FilterChips from './FilterChips.svelte';
  import { discoveryFinderProvider } from '../lib/finderProviders.js';
  import { getUnread } from '../stores/notifications.svelte.js';
  import { ArrowSquareOut, Bell, CaretDown, FunnelSimple, Info, MagnifyingGlass, SquaresFour, CalendarBlank, Gauge, SidebarSimple, House } from 'phosphor-svelte';
  import LabelFooter from './LabelFooter.svelte';
  import { getLabel, loadLabel, formatMoney } from '../stores/label.svelte.js';

  let { children, routeName = 'home', quiltScope = 'local', onScopeChange = () => {} } = $props();

  let scopeMenuOpen = $state(false);

  // The search (docs/adr/033): one autocomplete posture on every social
  // page. The discovery corpus is fetched once per shell mount, on first
  // focus. The action row is the only path from typed text to standing
  // narrowing state.
  const discoveryProvider = discoveryFinderProvider();

  function showMatchesOnQuilt(q) {
    setSearchQuery(q);
    navigate('/');
  }

  let activeScopeLabel = $derived(quiltScope === 'my' ? 'My Quilt' : getInstanceName());

  // The quilt switcher's other-quilt entries: neighbors (instance-curated,
  // visible to everyone) + personal connected quilts + registry overlay.
  // Every one is a doorway — objects blend, places don't (docs/adr/024):
  // another quilt is visited at its own address, never rendered here.
  let otherQuilts = $derived(switcherQuilts(getNeighborQuilts(), window.location.origin));

  // Display names fetched lazily when the menu opens.
  let quiltDetails = $state({});
  $effect(() => {
    if (!scopeMenuOpen) return;
    for (const q of otherQuilts) {
      if (quiltDetails[q.url] !== undefined) continue;
      quiltDetails[q.url] = null; // pending
      fetchQuiltInfo(q.url).then((info) => {
        quiltDetails = {
          ...quiltDetails,
          [q.url]: { name: info?.name || q.name || q.url.replace(/^https?:\/\//, '') },
        };
      });
    }
  });

  function selectQuilt(q) {
    scopeMenuOpen = false;
    window.open(q.url, '_blank', 'noopener');
  }

  // Paste-a-link follow path (docs/adr/024): a patch URL pasted into the
  // search opens that patch's remote card, where Follow lives. Wired as the
  // search's intercept — returning true consumes the input.
  function recognizePatchLink(value) {
    const m = value.trim().match(/^https?:\/\/([^/]+)\/patches\/([a-z0-9-]+)\/?$/);
    if (!m) return false;
    const [, host, slug] = m;
    if (host === window.location.host) {
      navigate(`/patches/${slug}`);
    } else {
      navigate(`/quilts/${host}/patches/${slug}`);
    }
    mobileSearchOpen = false;
    return true;
  }

  // Sidebar collapse — persisted, defaults open on wide screens.
  const SIDEBAR_KEY = 'patchwork-sidebar-collapsed';
  let sidebarCollapsed = $state(
    localStorage.getItem(SIDEBAR_KEY) != null
      ? localStorage.getItem(SIDEBAR_KEY) === '1'
      : window.innerWidth < 1024
  );

  function toggleSidebar() {
    sidebarCollapsed = !sidebarCollapsed;
    localStorage.setItem(SIDEBAR_KEY, sidebarCollapsed ? '1' : '0');
  }

  // Quilt routes get the immersive treatment: glass bar, floating icon rail.
  const quiltRoutes = new Set(['home', 'patchList', 'map']);
  let isQuiltRoute = $derived(quiltRoutes.has(routeName));

  // The Label's mobile affordance (docs/adr/023): the quilt view has no
  // scroll end for a footer and the rail owns the bottom edge, so a small
  // info button opens a summary sheet instead.
  let labelSheetOpen = $state(false);
  let label = $derived(getLabel());
  $effect(() => { loadLabel(); });
  $effect(() => {
    // Leaving the quilt view closes the sheet.
    if (!isQuiltRoute) labelSheetOpen = false;
  });

  // Mobile search: the shelf's search button swaps the top bar for a
  // back-button + search takeover. The search works from any page now —
  // results navigate — so the takeover opens in place, and any navigation
  // (tapping a result, the action row) dismisses it.
  let mobileSearchOpen = $state(false);

  $effect(() => {
    void routeName;
    mobileSearchOpen = false;
  });

  // Mobile canvas filter sheet (docs/adr/033): open-while-using, dismissed
  // when done — never a preference, unlike the chips' collapse state.
  let filterSheetOpen = $state(false);
  $effect(() => {
    // Leaving the canvas closes the sheet, like the label sheet.
    if (!isQuiltRoute) filterSheetOpen = false;
  });

  // Nav items for the sidebar
  const navItems = [
    { id: 'patches', label: 'Patches', href: '/', icon: SquaresFour,
      routes: ['home', 'quilt', 'patchList', 'map'] },
    { id: 'events', label: 'Events', href: '/events', icon: CalendarBlank,
      routes: ['eventList', 'eventDetail'] },
    { id: 'dashboard', label: 'Dashboard', href: '/dashboard', icon: Gauge,
      routes: ['dashboard'] },
  ];

  let activeNavId = $derived.by(() => {
    for (const item of navItems) {
      if (item.routes.includes(routeName)) return item.id;
    }
    if (routeName.startsWith && routeName.includes('patch')) return 'patches';
    return null;
  });

  function handleNav(e, href) {
    e.preventDefault();
    navigate(href);
  }

  function handleWindowClick(e) {
    if (scopeMenuOpen && !e.target.closest('.scope-switcher')) {
      scopeMenuOpen = false;
    }
    if (filterSheetOpen && !e.target.closest('.filter-sheet') && !e.target.closest('.filter-fab')) {
      filterSheetOpen = false;
    }
    if (mobileSearchOpen && !e.target.closest('.mobile-search-bar') && !e.target.closest('.rail-search')) {
      mobileSearchOpen = false;
    }
  }

  // Picking a quilt in the switcher goes to that quilt's view — including
  // the one already selected. The switcher names places, so it should land
  // you in one, not just re-lens whatever page you happen to be on.
  function selectScope(scope) {
    onScopeChange(scope);
    scopeMenuOpen = false;
    navigate('/');
  }
</script>

<svelte:window onclick={handleWindowClick} />

<div class="social-layout" class:quilt-mode={isQuiltRoute}>
  <GlobalBar glass={isQuiltRoute} bordered={!isQuiltRoute} shelfBell>
    {#snippet leading()}
      <button
        class="bar-sidebar-toggle"
        onclick={toggleSidebar}
        title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        aria-label={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      >
        <SidebarSimple size={20} weight={sidebarCollapsed ? 'duotone' : 'fill'} />
      </button>
      <div class="scope-switcher">
        <button class="scope-btn" onclick={() => { scopeMenuOpen = !scopeMenuOpen; }}>
          {#if quiltScope === 'my'}
            <House class="logo-icon" size={20} weight="duotone" />
          {:else if getInstanceIconUrl()}
            <img class="logo-icon quilt-icon-img" src={getInstanceIconUrl()} alt="" width="20" height="20" />
          {:else}
            <svg class="logo-icon" width="20" height="20" viewBox="0 0 24 24" fill="none">
              <rect x="2" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
              <rect x="13" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
              <rect x="2" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
              <rect x="13" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="2"/>
            </svg>
          {/if}
          <span class="logo-label">{activeScopeLabel}</span>
          <span class="logo-chevron" class:open={scopeMenuOpen}>
            <CaretDown size={12} weight="bold" />
          </span>
        </button>
        {#if scopeMenuOpen}
          <div class="scope-dropdown">
            <button class="scope-option" class:active={quiltScope === 'local'} onclick={() => selectScope('local')}>
              {#if getInstanceIconUrl()}
                <img class="quilt-icon-img" src={getInstanceIconUrl()} alt="" width="16" height="16" />
              {:else}
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
                  <rect x="2" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
                  <rect x="13" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
                  <rect x="2" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
                  <rect x="13" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
                </svg>
              {/if}
              <span>{getInstanceName()}</span>
            </button>
            {#if isLoggedIn()}
              <button class="scope-option" class:active={quiltScope === 'my'} onclick={() => selectScope('my')}>
                <House size={16} weight="duotone" />
                <span>My Quilt</span>
              </button>
            {/if}
            {#if otherQuilts.length > 0}
              <div class="scope-section-label">Connected quilts</div>
              {#each otherQuilts as q (q.url)}
                {@const detail = quiltDetails[q.url]}
                <button
                  class="scope-option"
                  onclick={() => selectQuilt(q)}
                  title="Opens {detail?.name || q.name || q.url} on its own site"
                >
                  <img class="quilt-icon-img" src="{q.url}/api/v1/instance/icon" alt="" width="16" height="16" />
                  <span>{detail?.name || q.name || q.url.replace(/^https?:\/\//, '')}</span>
                  <span class="scope-doorway"><ArrowSquareOut size={13} weight="bold" /></span>
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
    {/snippet}

    {#snippet search()}
      <!-- The search (docs/adr/033): the same autocomplete posture the
           workspace and admin contexts use, over the discovery corpus. -->
      <WorkspaceSearch
        placeholder="Search patches and events…"
        provider={discoveryProvider}
        actionLabel={(q) => `Show matches on the quilt for “${q}”`}
        onAction={showMatchesOnQuilt}
        intercept={recognizePatchLink}
      />
    {/snippet}
  </GlobalBar>

  <!-- Mobile search takeover: covers the global bar while open. Full
       width, no back button — the shelf's search button toggles it, and
       tapping away or navigating dismisses it. -->
  {#if mobileSearchOpen}
    <div class="mobile-search-bar">
      <WorkspaceSearch
        variant="takeover"
        autofocus
        placeholder="Search patches and events…"
        provider={discoveryProvider}
        actionLabel={(q) => `Show matches on the quilt for “${q}”`}
        onAction={showMatchesOnQuilt}
        intercept={recognizePatchLink}
      />
    </div>
  {/if}

  <!-- Filter chips over the canvas (docs/adr/033): desktop-only here — the
       row overlays the quilt/map top edge, bounded away from the rail and
       the cards pane. Mobile canvases get the FAB + sheet below instead. -->
  {#if isQuiltRoute}
    <div class="quilt-chips" class:rail-collapsed={sidebarCollapsed}>
      <FilterChips variant="overlay" />
    </div>
  {/if}

  <!-- ===== SIDEBAR: nav, collapsible ===== -->
  <nav class="sidebar-rail" class:quilt-mode={isQuiltRoute} class:collapsed={sidebarCollapsed}>
    <div class="rail-center">
      {#each navItems as item (item.id)}
        {@const Icon = item.icon}
        <a
          href={item.href}
          class="rail-item"
          class:active={activeNavId === item.id}
          onclick={(e) => handleNav(e, item.href)}
          title={item.label}
        >
          <span class="rail-icon"><Icon size={22} weight="duotone" /></span>
          <span class="rail-label">{item.label}</span>
        </a>
      {/each}
      <!-- Mobile only: search swaps the top bar for a search takeover.
           The filter announces itself on the surfaces it narrows (the
           canvas FAB, the events page chips), not here (docs/adr/033). -->
      <button
        class="rail-item rail-search"
        class:active={mobileSearchOpen}
        onclick={() => { mobileSearchOpen = !mobileSearchOpen; }}
        title="Search"
      >
        <span class="rail-icon">
          <MagnifyingGlass size={22} weight="duotone" />
        </span>
        <span class="rail-label">Search</span>
      </button>
      {#if isLoggedIn()}
        <!-- Mobile only: the bar's bell moves onto the shelf, and
             notifications are a page rather than an overlay. -->
        <a
          href="/notifications"
          class="rail-item rail-notif"
          class:active={routeName === 'notifications'}
          onclick={(e) => handleNav(e, '/notifications')}
          title="Notifications"
        >
          <span class="rail-icon">
            <Bell size={22} weight="duotone" />
            {#if getUnread() > 0}
              <span class="rail-badge">{getUnread() > 99 ? '99+' : getUnread()}</span>
            {/if}
          </span>
          <span class="rail-label">Notifications</span>
        </a>
      {/if}
    </div>
  </nav>

  <!-- Mobile only, quilt/map only: the Label's info button. It floats over
       the quilt/map viewport at the bottom-left corner, on the same row as
       the Quilt/Map/List pill — chrome belonging to the canvas, not a
       peer of the nav items in the bottom bar (docs/adr/024).
       Hidden above 768px, where the desktop attribution strip exists. -->
  {#if isQuiltRoute && label?.published}
    <button
      class="quilt-info-fab"
      class:active={labelSheetOpen}
      onclick={() => { labelSheetOpen = !labelSheetOpen; }}
      title="The Label"
      aria-label="About this quilt"
    >
      <Info size={20} weight="duotone" />
    </button>
  {/if}

  <!-- Mobile only, quilt/map only: the filter FAB, opposite the info button
       on the right — same floating row as the view pill. Its badge is the
       filter's mobile announcement (docs/adr/033). -->
  {#if isQuiltRoute}
    <button
      class="filter-fab"
      class:active={filterSheetOpen}
      onclick={() => { filterSheetOpen = !filterSheetOpen; }}
      title="Filter"
      aria-label="Filter by tag{getActiveFilterCount() > 0 ? `, ${getActiveFilterCount()} active` : ''}"
      aria-expanded={filterSheetOpen}
    >
      <FunnelSimple size={16} weight="bold" />
      {#if getActiveFilterCount() > 0}
        <span class="filter-fab-badge">{getActiveFilterCount()}</span>
      {/if}
    </button>
  {/if}

  <!-- The filter sheet: chips over the canvas, just above the nav row.
       Open-while-using — the collapse preference never governs a sheet. -->
  {#if filterSheetOpen && isQuiltRoute}
    <div class="filter-sheet">
      <FilterChips variant="sheet" />
    </div>
  {/if}

  <!-- The Label's mobile summary sheet (docs/adr/023) -->
  {#if labelSheetOpen && label?.published}
    <div class="label-sheet">
      <div class="label-sheet-head">
        <strong>{getInstanceName()}</strong>
        <button class="label-sheet-close" onclick={() => { labelSheetOpen = false; }} aria-label="Close">✕</button>
      </div>
      <p class="label-sheet-line">
        Stewarded by {(label.stewards || []).map((s) => `@${s.username}`).join(', ')}
      </p>
      {#if label.total_monthly_minor > 0}
        <p class="label-sheet-line">
          About {formatMoney(label.total_monthly_minor, label.currency)}/month to keep running
        </p>
      {/if}
      <p class="label-sheet-line muted-line">Run by real people. Yours to seamrip if it ever comes to that.</p>
      <a
        href="/label"
        class="label-sheet-link"
        onclick={(e) => { e.preventDefault(); labelSheetOpen = false; navigate('/label'); }}
      >
        Read the Label &rarr;
      </a>
    </div>
  {/if}

  <!-- Main content area -->
  <main class="social-main" class:quilt-mode={isQuiltRoute} class:rail-collapsed={sidebarCollapsed}>
    {@render children()}
    {#if !isQuiltRoute}
      <LabelFooter variant="page" />
    {/if}
  </main>

  <!-- Desktop attribution strip over the quilt/map (docs/adr/024) -->
  {#if isQuiltRoute}
    <LabelFooter variant="overlay" />
  {/if}
</div>

<style>
  .social-layout {
    min-height: 100vh;
  }

  .bar-sidebar-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    background: none;
    border-radius: var(--radius);
    color: var(--color-text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: background 150ms ease, color 150ms ease;
  }

  .bar-sidebar-toggle:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }

  .scope-switcher {
    position: relative;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .scope-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px;
    height: 44px;
    border: none;
    background: none;
    color: var(--color-text);
    border-radius: var(--radius);
    white-space: nowrap;
    cursor: pointer;
    transition: background 150ms ease;
  }

  .scope-btn:hover {
    background: var(--color-overlay);
  }

  .logo-icon {
    flex-shrink: 0;
    color: var(--color-primary);
  }

  /* Quilt icon (docs/adr/014): the instance's chosen mark in the switcher.
     Hard edges — textile surfaces have zero border-radius. */
  .quilt-icon-img {
    flex-shrink: 0;
    object-fit: cover;
    display: block;
  }

  .logo-label {
    font-family: var(--font-display);
    font-variation-settings: 'BNCE' 20;
    font-weight: 700;
    font-size: 1.05rem;
    color: var(--color-text);
    line-height: 1;
  }

  .logo-chevron {
    display: flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
    margin-left: -2px;
    transition: transform 150ms ease;
  }

  .logo-chevron.open {
    transform: rotate(180deg);
  }

  .scope-dropdown {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 4px 16px var(--color-shadow);
    min-width: 200px;
    z-index: 200;
    overflow: hidden;
    padding: 4px;
  }

  .scope-section-label {
    padding: 8px 10px 3px;
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-muted);
    border-top: 1px solid var(--color-border);
    margin-top: 4px;
  }

  .scope-doorway {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    color: var(--color-text-muted);
  }

  .scope-option {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 8px 10px;
    border: none;
    background: none;
    border-radius: 4px;
    color: var(--color-text);
    font-size: 0.88rem;
    font-weight: 500;
    cursor: pointer;
    text-align: left;
    transition: background 100ms ease;
  }

  .scope-option:hover {
    background: var(--color-overlay);
  }

  .scope-option.active {
    color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  }

  .scope-option svg {
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .scope-option.active svg {
    color: var(--color-primary);
  }

  /* --- Filter chips over the canvas (docs/adr/033) --- */
  /* Desktop only: overlays the quilt/map top edge under the glass bar,
     bounded left of the rail's floating card and right of the cards pane —
     wrapping within that span, never extending across it. */
  .quilt-chips {
    position: fixed;
    top: calc(56px + 12px);
    left: 224px; /* clear the rail's floating glass card (12px + 200px + gap) */
    right: calc(45% + 16px); /* clear the cards pane */
    z-index: 20; /* the canvas chrome layer — same as the view pill */
  }

  .quilt-chips.rail-collapsed {
    left: 68px; /* clear the collapsed rail's floating chips */
  }

  /* The FAB and sheet are the mobile canvas home — hidden on desktop. */
  .filter-fab {
    display: none;
  }
  .filter-sheet {
    display: none;
  }

  /* ================================================================
     SIDEBAR — nav items, collapsible
     ================================================================ */
  .sidebar-rail {
    position: fixed;
    top: 56px;
    left: 0;
    bottom: 0;
    width: 200px;
    z-index: 55;
    display: flex;
    flex-direction: column;
    padding: 12px 8px;
    background: var(--color-surface);
    border-right: 1px solid var(--color-border);
    transition: width 150ms ease;
  }

  .sidebar-rail.collapsed {
    width: 56px;
  }

  /* Over the quilt, collapsed: no panel, items float as glass chips.
     Expanded over the quilt: a glass *card* with a border on all four
     sides. The quilt/map routes have no shell
     borders — the bar is borderless glass — so a full-height drawer has
     nothing to butt against and reads as a stray panel. Inset from the left
     and sized to hug its items (bottom: auto), it reads as chrome floating
     over the canvas, like the view pill. Top stays at 56px so the items sit
     at the same height they do collapsed — toggling must not shift them. */
  .sidebar-rail.quilt-mode {
    top: 56px;
    left: 12px;
    bottom: auto;
    background: var(--color-glass);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 2px 12px var(--color-shadow);
  }

  /* Collapsed over the quilt the panel dissolves, but the geometry stays
     the hugged card's (left inset, bottom: auto) — NOT full height. The
     hover peek below must only change width; if hovering also moved or
     shrank the box, the cursor could land outside the new bounds, ending
     :hover and snapping the box back under the cursor — an open/close
     flicker loop. Height-hugging also keeps the quilt clickable below
     the chips. */
  .sidebar-rail.quilt-mode.collapsed {
    top: 56px;
    left: 12px;
    bottom: auto;
    background: transparent;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
    border: none;
    border-radius: 0;
    box-shadow: none;
  }

  .rail-center {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex: 1;
  }

  /* The Label's info button and sheet are mobile-only (docs/adr/023):
     desktop already has the attribution strip over the quilt. Deliberate —
     do not "fix" this into showing on desktop. */
  .quilt-info-fab {
    display: none;
  }
  .label-sheet {
    display: none;
  }

  .rail-label {
    white-space: nowrap;
    font-size: 0.85rem;
    font-weight: 500;
    padding-right: 4px;
  }

  .sidebar-rail.collapsed .rail-label {
    display: none;
  }

  /* Peek: hovering the collapsed rail overlays the expanded panel without
     shifting the layout — main keeps its collapsed margin, so this floats. */
  .sidebar-rail.collapsed:hover {
    width: 200px;
    box-shadow: 4px 0 24px var(--color-shadow);
  }

  .sidebar-rail.collapsed:hover .rail-label {
    display: inline;
  }

  /* Over the quilt the peek brings the glass panel with it, and the
     individual chip backdrops give way to the panel's. Width is the ONLY
     geometry hover may change (see the collapsed rule above). */
  .sidebar-rail.quilt-mode.collapsed:hover {
    background: var(--color-glass);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 2px 12px var(--color-shadow);
  }

  .sidebar-rail.quilt-mode.collapsed:hover .rail-item {
    background: none;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
    align-self: stretch;
  }

  .sidebar-rail.quilt-mode.collapsed:hover .rail-item:hover {
    background: var(--color-overlay);
  }

  .sidebar-rail.quilt-mode.collapsed:hover .rail-item.active {
    background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  }

  .rail-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px;
    border: none;
    background: none;
    border-radius: var(--radius);
    color: var(--color-text-muted);
    cursor: pointer;
    text-decoration: none;
    transition: background 150ms ease, color 150ms ease;
    min-height: 44px;
  }

  /* Collapsed over the quilt, the chips need their own backdrop */
  .sidebar-rail.quilt-mode.collapsed .rail-item {
    background: color-mix(in srgb, var(--color-bg) 65%, transparent);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    align-self: flex-start;
  }

  .rail-item:hover {
    background: var(--color-overlay);
    color: var(--color-text);
    text-decoration: none;
  }

  .sidebar-rail.quilt-mode.collapsed .rail-item:hover {
    background: color-mix(in srgb, var(--color-bg) 80%, transparent);
  }

  .rail-item.active {
    color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  }

  .sidebar-rail.quilt-mode.collapsed .rail-item.active {
    background: color-mix(in srgb, var(--color-primary) 15%, color-mix(in srgb, var(--color-bg) 65%, transparent));
  }

  .rail-icon {
    display: flex;
    flex-shrink: 0;
    position: relative;
  }

  /* Notifications live in the global bar on desktop and search lives in
     the bar's search slot; both shelf items are mobile-only (see the
     media query). */
  .rail-notif,
  .rail-search {
    display: none;
  }

  .rail-item.rail-search {
    font-family: inherit;
  }

  /* Mobile search takeover — sits over the global bar while open. */
  .mobile-search-bar {
    display: none;
  }

  .rail-badge {
    position: absolute;
    top: -4px;
    right: -8px;
    background: var(--color-error);
    color: var(--color-on-error);
    font-size: 0.6rem;
    font-weight: 700;
    min-width: 16px;
    height: 16px;
    line-height: 16px;
    text-align: center;
    border-radius: 8px;
    padding: 0 3px;
  }

  /* ================================================================
     MAIN CONTENT
     ================================================================ */
  /* The container owns content padding — pages should not re-pad
     themselves (see docs — issue #17). 56px clears the fixed global bar;
     2rem/1.5rem is the app's standard breathing room. */
  .social-main {
    margin-left: 200px;
    padding: calc(56px + 2rem) 1.5rem 2rem;
    min-height: 100vh;
    transition: margin-left 150ms ease;
  }

  .social-main.rail-collapsed {
    margin-left: 56px;
  }

  /* Quilt mode: full bleed under the glass bar, rail floats over the canvas */
  .social-main.quilt-mode {
    margin-left: 0;
    padding: 0;
  }

  /* ================================================================
     RESPONSIVE — mobile keeps the bottom pill nav; the bar stays up top
     ================================================================ */
  @media (max-width: 768px) {
    .bar-sidebar-toggle {
      display: none;
    }

    /* The desktop chips overlay yields to the FAB + sheet on mobile. */
    .quilt-chips {
      display: none;
    }

    /* Filter FAB: opposite the info button, same floating row as the view
       pill — canvas chrome, right where the thumb lives. */
    /* Styled like the desktop chips-toggle — an outlined circle, not the
       glass-highlight treatment the info button wears. */
    .filter-fab {
      display: flex;
      align-items: center;
      justify-content: center;
      position: fixed;
      right: 12px;
      bottom: calc(64px + env(safe-area-inset-bottom, 0px));
      z-index: 20; /* same floating layer as the view pill and info button */
      width: 36px;
      height: 36px;
      padding: 0;
      /* Mixed from text-muted, not --color-border: the dark theme's border
         color disappears against the dark canvas, and a floating circle
         with no visible edge reads as a smudge. */
      border: 1px solid color-mix(in srgb, var(--color-text-muted) 45%, transparent);
      border-radius: 999px;
      background: var(--color-surface);
      color: var(--color-text-muted);
      cursor: pointer;
      transition: border-color 150ms ease, color 150ms ease;
    }

    .filter-fab.active,
    .filter-fab:hover {
      border-color: var(--color-primary);
      color: var(--color-text);
    }

    .filter-fab-badge {
      position: absolute;
      top: -4px;
      right: -4px;
      background: var(--color-primary);
      color: var(--color-btn-on-primary);
      font-size: 0.6rem;
      font-weight: 700;
      min-width: 16px;
      height: 16px;
      line-height: 16px;
      text-align: center;
      border-radius: 8px;
      padding: 0 3px;
    }

    /* The filter sheet: rises from the FAB, clearing the floating row so
       its toggle stays visible — the label sheet's geometry. */
    .filter-sheet {
      display: block;
      position: fixed;
      left: 8px;
      right: 8px;
      bottom: calc(108px + env(safe-area-inset-bottom, 0px));
      z-index: 56; /* just above the rail (55), under the bar (60) */
      background: var(--color-surface);
      border: 1px solid var(--color-border);
      border-radius: 12px;
      box-shadow: 0 8px 24px var(--color-shadow);
      padding: 14px 16px;
      max-height: 45vh;
      overflow-y: auto;
    }

    /* Full-width bottom bar. The extra selector outranks the desktop
       "collapsed over the quilt is fully transparent" rule — on mobile the
       bar always keeps its glass. */
    .sidebar-rail,
    .sidebar-rail.quilt-mode,
    .sidebar-rail.quilt-mode.collapsed,
    .sidebar-rail.quilt-mode.collapsed:hover {
      top: auto;
      bottom: 0;
      left: 0;
      right: 0;
      width: auto !important;
      height: auto;
      flex-direction: row;
      padding: 6px 8px calc(6px + env(safe-area-inset-bottom, 0px));
      background: var(--color-glass);
      backdrop-filter: blur(16px);
      -webkit-backdrop-filter: blur(16px);
      border: none;
      border-top: 1px solid var(--color-border);
      border-radius: 0;
      box-shadow: none;
    }

    .rail-center {
      flex-direction: row;
      flex: 1;
      justify-content: space-around;
      gap: 0;
    }

    /* Mobile: the info button floats inside the quilt/map viewport at the
       bottom-left, level with the Quilt/Map/List pill — canvas chrome, not
       a nav item (docs/adr/024). Glass like the pill so it reads as the
       same floating layer. */
    .quilt-info-fab {
      display: flex;
      align-items: center;
      justify-content: center;
      position: fixed;
      left: 12px;
      bottom: calc(64px + env(safe-area-inset-bottom, 0px));
      z-index: 20; /* same floating layer as the view pill, under the rail (55) */
      width: 36px;
      height: 36px;
      padding: 0;
      border: none;
      border-radius: 999px;
      background: var(--color-glass);
      backdrop-filter: blur(16px);
      -webkit-backdrop-filter: blur(16px);
      box-shadow: 0 2px 12px var(--color-shadow);
      color: var(--color-text);
      opacity: 0.85;
      cursor: pointer;
    }
    .quilt-info-fab.active,
    .quilt-info-fab:hover {
      opacity: 1;
    }

    /* The Label's summary sheet: rises from the info button, clearing the
       floating row (button + view pill) so its toggle stays visible. */
    .label-sheet {
      display: block;
      position: fixed;
      left: 8px;
      right: 8px;
      bottom: calc(108px + env(safe-area-inset-bottom, 0px));
      z-index: 56; /* just above the rail (55), under the bar (60) */
      background: var(--color-surface);
      border: 1px solid var(--color-border);
      border-radius: 12px;
      box-shadow: 0 8px 24px var(--color-shadow);
      padding: 14px 16px;
    }
    .label-sheet-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 6px;
    }
    .label-sheet-close {
      background: none;
      border: none;
      color: var(--color-text);
      opacity: 0.6;
      cursor: pointer;
      padding: 2px 6px;
    }
    .label-sheet-line {
      margin: 0 0 4px;
      font-size: 0.85rem;
    }
    .muted-line {
      opacity: 0.7;
    }
    .label-sheet-link {
      display: inline-block;
      margin-top: 6px;
      font-size: 0.85rem;
      font-weight: 600;
      color: var(--color-primary);
      text-decoration: none;
    }

    .rail-notif,
    .rail-search {
      display: flex;
    }

    .mobile-search-bar {
      position: fixed;
      top: 0;
      left: 0;
      right: 0;
      height: 56px;
      z-index: 70; /* above the global bar (60) */
      display: flex;
      align-items: center;
      padding: 0 12px;
      background: var(--color-surface);
    }

    .rail-label {
      display: none !important;
    }

    .rail-item {
      padding: 8px 12px;
      min-height: unset;
      background: none !important;
      backdrop-filter: none !important;
      -webkit-backdrop-filter: none !important;
      border-radius: 999px;
    }

    .rail-item:hover {
      background: var(--color-overlay) !important;
    }

    .rail-item.active {
      background: color-mix(in srgb, var(--color-primary) 15%, transparent) !important;
    }

    .social-main {
      margin-left: 0 !important;
      padding-bottom: 84px;
    }

    /* Quilt routes are full bleed — the canvas shows through the
       translucent pill nav instead of stopping above it. */
    .social-main.quilt-mode {
      padding-bottom: 0;
    }

    .social-main:not(.quilt-mode) {
      padding-top: calc(56px + 2rem);
    }
  }
</style>
