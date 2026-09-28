<script>
  /**
   * The workspace: a patch's full-screen management and participation
   * surface (docs/adr/005). Renders the global bar with the patch's
   * context crumb and scoped finder, then its own tab row — no discovery
   * chrome. Role decides what shows: admins get Settings, followers get
   * permission-gated tabs, non-members get Join/Follow in the right
   * cluster.
   *
   * On a narrow screen the tabs are a bottom bar, where the quilt's own
   * rail sits at that width, so a phone's navigation is at the foot of the
   * screen everywhere. workspaceTabs never returns more than five, which is
   * what makes a bar of fixed slots possible. A single tab is no choice at
   * all and gets no bar. The relationship cluster stays at the top.
   */
  import { setContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { setPatchName } from '../stores/patchName.svelte.js';
  import { workspaceFinderProvider } from '../lib/finderProviders.js';
  import { workspaceTabs } from '../lib/patchWorkspace.js';
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import Skeleton from './Skeleton.svelte';
  import PatchRelationship from './PatchRelationship.svelte';
  import { getPendingMembershipSlugs, getInvitedMembershipSlugs, loadMemberships } from '../stores/memberships.svelte.js';
  import { Scales, UsersThree, CalendarBlank, GearSix, Eye, Chalkboard, CaretLeft } from 'phosphor-svelte';

  // up: where this route's "up" goes when it has no section list on screen
  // (lib/patchWorkspace.js workspaceUpLink), or null.
  let { slug = '', activeTab = 'governance', up = null, children } = $props();

  // --- Patch data (fetched once, shared via context) ---
  let node = $state(null);
  let isMember = $state(false);
  let isAdmin = $state(false);
  let membershipRole = $state('');
  let followerPermissions = $state(null);
  let loading = $state(true);
  let error = $state('');

  let isUnclaimed = $state(false);
  // Whether this viewer's trusted-contributor grant reaches this patch
  // (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar).
  // Sent beside is_unclaimed, not on the node, so it lives beside it here.
  let viewerTrusted = $state(false);
  let isBanned = $state(false);
  let breadcrumbExtra = $state([]);

  let verificationDomain = $state('');

  // Expose to child pages via context
  const patchContext = $derived({
    node,
    slug,
    isMember,
    isAdmin,
    isUnclaimed,
    viewerTrusted,
    isBanned,
    membershipRole,
    followerPermissions,
    verificationDomain,
    loading,
    error,
    reload: loadNode,
    setBreadcrumbExtra: (segments) => { breadcrumbExtra = segments; },
  });

  setContext('patch', {
    get value() { return patchContext; }
  });

  // The link up a level, drawn on a phone at the left of the row that holds
  // the relationship cluster, so the two share a line instead of stacking.
  // A shell inside the workspace that has its own (Patch Settings, drilling
  // down) hands it here rather than drawing it inside the page. Failing
  // that, a page's breadcrumb that names a linked parent (a charter's
  // history names the charter) is nearer than the route's section list.
  let registeredUp = $state(null);
  setContext('workspaceUp', {
    set: (link) => { registeredUp = link; },
  });

  let upLink = $derived.by(() => {
    if (registeredUp) return registeredUp;
    const linked = breadcrumbExtra.filter((seg) => seg.href);
    if (linked.length > 0) return linked[linked.length - 1];
    return up;
  });

  // Fetch node data when slug changes
  let lastSlug = '';
  $effect(() => {
    if (slug && slug !== lastSlug) {
      lastSlug = slug;
      loadNode();
    }
  });

  async function loadNode() {
    loading = true;
    error = '';
    try {
      const data = await api(`nodes/${slug}`);
      node = data.node || data;
      liningStatus = data.lining_status || '';
      isMember = data.is_member || false;
      isAdmin = data.is_admin || false;
      membershipRole = data.membership_role || '';
      followerPermissions = (data.node || data).follower_permissions || null;
      isUnclaimed = data.is_unclaimed || false;
      viewerTrusted = data.viewer_trusted === true;
      isBanned = data.is_banned || false;
      verificationDomain = data.verification_domain || '';
      setPatchName(node?.name || slug);
    } catch (e) {
      error = e.message || 'Failed to load patch';
      node = null;
    } finally {
      loading = false;
    }
  }

  // Join/follow/leave — including the join sheet (docs/adr/040) — belong to
  // PatchRelationship, mounted in the cluster below.
  let liningStatus = $state('');

  // A join request nobody has answered. The node payload deliberately does
  // not carry it — membership_role is set only for an active row — so it
  // comes from the viewer's own memberships, which me/nodes does serve.
  let requestPending = $derived(getPendingMembershipSlugs().has(slug));
  // An unanswered invitation (docs/adr/098), from the same store for the
  // same reason: the node payload states standing for an active row only.
  let invited = $derived(getInvitedMembershipSlugs().has(slug));

  // Joining or following changes two things: the node payload's
  // membership_role and the store a pending request lives in. Refresh both,
  // or asking to join leaves the row offering to join again.
  async function reloadStanding() {
    await Promise.all([loadNode(), loadMemberships()]);
  }

  // --- Tabs (one URL scheme per screen — ADR 003) ---
  // The workspace is for everyone; role and claim state decide what shows.
  // Admins get the Settings tab, followers get permission-gated tabs, and
  // unclaimed patches get a pared-down subset (workspaceTabs owns that logic
  // so it can be tested on its own).
  let basePath = $derived(`/patches/${slug}`);

  const TAB_ICONS = {
    governance: Scales,
    members: UsersThree,
    events: CalendarBlank,
    noticeboard: Chalkboard,
    settings: GearSix,
  };

  const tabs = $derived.by(() =>
    workspaceTabs({
      isUnclaimed,
      isAdmin,
      membershipRole,
      followerPermissions,
      publicMemberList: node?.public_member_list || 'everyone',
    }).map((t) => ({
      ...t,
      href: `${basePath}/${t.id}`,
      icon: TAB_ICONS[t.id],
    }))
  );

  let finderProvider = $derived(workspaceFinderProvider(slug));

  // Unclaimed patches carry no governance at all (docs/adr/039) — absence,
  // not an empty state. workspaceTabs() already drops Governance from the
  // tab row for one, but a direct URL to any governance sub-route (Hub,
  // Documents, Proposals, a doc/proposal detail...) would otherwise still
  // render past that; every one of them maps to activeTab 'governance'
  // (see derivePatchTab in App.svelte), so this single guard covers all of
  // them and lands on the workspace's actual live surface instead.
  $effect(() => {
    if (node && isUnclaimed && activeTab === 'governance') {
      navigate(`${basePath}/events`);
    }
  });

  function handleTabClick(e, href) {
    e.preventDefault();
    navigate(href);
  }

  // The tab strip scrolls sideways on a narrow screen with its scrollbar
  // deliberately switched off, and nothing else said the row continued. A
  // member on a 390px phone: "I can see 'Governance' and 'Members' and then
  // the row runs off the side of the screen. There are apparently Events and
  // Noticeboard tabs too — I only know because I found them by other routes.
  // I never once thought to swipe that row sideways." She had been on the
  // site a year without knowing the patch had a noticeboard.
  //
  // Two things, because she was missing two. A fade on whichever side has
  // more tabs behind it, which is the "half-cut-off word peeking out" she
  // asked for in as many words. And the active tab scrolled into view, so
  // arriving at a tab that lives off the edge shows you where you are
  // instead of leaving the strip parked at the start.
  let tabStrip = $state(null);
  let moreLeft = $state(false);
  let moreRight = $state(false);

  function measureTabs() {
    const el = tabStrip;
    if (!el) return;
    // A pixel of slack: scrollWidth and clientWidth differ by fractions at
    // some zoom levels on a strip that does not actually overflow, and a
    // fade over nothing is its own small lie.
    moreLeft = el.scrollLeft > 1;
    moreRight = el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
  }

  $effect(() => {
    const el = tabStrip;
    if (!el) return;
    measureTabs();
    const ro = new ResizeObserver(measureTabs);
    ro.observe(el);
    return () => ro.disconnect();
  });

  // Bring the current tab into view when it is off the edge. Reads activeTab
  // so it re-runs on navigation.
  $effect(() => {
    const el = tabStrip;
    const current = activeTab;
    if (!el || !current) return;
    const tab = el.querySelector('.workspace-tab.active');
    if (tab?.scrollIntoView) {
      tab.scrollIntoView({ inline: 'nearest', block: 'nearest' });
    }
    measureTabs();
  });
</script>

<div class="workspace">
  <GlobalBar>
    {#snippet leading()}
      <div class="crumb-group">
        <ContextCrumb label={node?.name || slug} href={`${basePath}/governance`} />
        <a
          href="/patches/{slug}"
          class="view-profile-action"
          onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}`); }}
          title="View the public profile"
          aria-label="View the public profile"
        >
          <Eye size={18} weight="duotone" />
        </a>
      </div>
    {/snippet}
    {#snippet search()}
      <WorkspaceSearch placeholder="Search this patch…" provider={finderProvider} />
    {/snippet}
  </GlobalBar>

  {#if loading && !node}
    <div class="workspace-body container">
      <div class="shell-loading">
        <Skeleton lines={1} height="0.8rem" width="30%" />
        <Skeleton lines={1} height="1.8rem" width="60%" />
        <Skeleton lines={1} height="0.9rem" width="80%" />
      </div>
    </div>
  {:else if error && !node}
    <div class="workspace-body container">
      <div class="shell-error">
        <h2>Patch not found</h2>
        <p class="muted">{error}</p>
        <div class="shell-error-actions">
          <button class="btn btn-secondary" onclick={loadNode}>Retry</button>
          <a href="/" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate('/'); }}>Back to Quilt</a>
        </div>
      </div>
    </div>
  {:else if node}
    <!-- Workspace nav: tabs + relationship cluster, directly under the bar -->
    <div class="workspace-nav">
      {#if upLink}
        <a href={upLink.href} class="workspace-up" onclick={(e) => handleTabClick(e, upLink.href)}>
          <CaretLeft size={14} weight="bold" />
          <span class="workspace-up-label">{upLink.label}</span>
        </a>
      {/if}
      <nav
        class="workspace-tabs"
        class:single={tabs.length < 2}
        class:more-left={moreLeft}
        class:more-right={moreRight}
        bind:this={tabStrip}
        onscroll={measureTabs}
      >
        {#each tabs as tab (tab.id)}
          {@const Icon = tab.icon}
          <a
            href={tab.href}
            class="workspace-tab"
            class:active={activeTab === tab.id}
            onclick={(e) => handleTabClick(e, tab.href)}
          >
            <span class="tab-icon"><Icon size={16} weight="duotone" /></span>
            <span class="tab-label">{tab.label}</span>
          </a>
        {/each}
      </nav>

      <!-- One implementation of the relationship row, shared with the patch
           profile (docs/adr/042). The two surfaces each carried their own
           copy and had drifted: this one rendered nothing at all for
           admins, and the two worded the same join differently. -->
      <div class="workspace-cluster">
        <PatchRelationship
          {slug}
          {node}
          {isUnclaimed}
          {isBanned}
          {membershipRole}
          {requestPending}
          {invited}
          {liningStatus}
          onChanged={reloadStanding}
          size="sm"
        />
      </div>
    </div>

    <!-- Tab content -->
    <div class="workspace-body work-content" class:over-tab-bar={tabs.length >= 2}>
      {@render children()}
    </div>
  {/if}
</div>

<style>
  .workspace {
    min-height: 100vh;
  }

  .shell-loading {
    padding: 5rem 0 2rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .shell-error {
    padding: 6rem 0 3rem;
    text-align: center;
  }

  .shell-error h2 {
    margin-bottom: 0.5rem;
  }

  .shell-error-actions {
    display: flex;
    gap: 0.5rem;
    justify-content: center;
    margin-top: 1rem;
  }

  /* ================================================================
     WORKSPACE NAV — one row under the global bar
     ================================================================ */
  .workspace-nav {
    position: sticky;
    top: 0;
    margin-top: 56px; /* clear the fixed global bar */
    z-index: 50;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 16px;
    background: var(--color-surface);
    border-bottom: 1px solid var(--color-border);
  }

  .workspace-tabs {
    display: flex;
    align-items: stretch;
    gap: 4px;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .workspace-tabs::-webkit-scrollbar {
    display: none;
  }

  /* The edge says the row continues. A mask rather than an overlaid
     gradient, so it works on any background and needs no element of its
     own; the sides are set independently because the row can be cut at
     either end once it has been scrolled. */
  .workspace-tabs.more-right {
    mask-image: linear-gradient(to right, #000 calc(100% - 28px), transparent 100%);
  }

  .workspace-tabs.more-left {
    mask-image: linear-gradient(to right, transparent 0, #000 28px);
  }

  .workspace-tabs.more-left.more-right {
    mask-image: linear-gradient(
      to right,
      transparent 0,
      #000 28px,
      #000 calc(100% - 28px),
      transparent 100%
    );
  }

  .workspace-tab {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 12px 12px;
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--color-text-muted);
    text-decoration: none;
    white-space: nowrap;
    border-bottom: 2px solid transparent;
    transition: color 120ms ease;
  }

  .workspace-tab:hover {
    color: var(--color-text);
    text-decoration: none;
  }

  .workspace-tab.active {
    color: var(--color-text);
    font-weight: 600;
    border-bottom-color: var(--color-accent);
  }

  .tab-icon {
    display: flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .workspace-tab.active .tab-icon {
    color: var(--color-accent);
  }

  /* --- Narrow screen: the tabs become a bottom bar ---
     The same footprint and glass as SocialShell's mobile rail. Its height
     is app.css's --pw-nav-h, which everything else ending at the foot of
     the screen (toasts, the vote bar) already clears. The row the tabs
     leave keeps only the relationship cluster, and stops being sticky:
     a Join button does not need to follow you down the page. */
  /* The up link exists only on a narrow screen, where the tabs have left
     this row to the relationship cluster. */
  .workspace-up {
    display: none;
  }

  @media (max-width: 768px) {
    .workspace-nav {
      position: static;
      justify-content: flex-end;
      background: none;
      border-bottom: none;
      min-height: 52px;
    }

    .workspace-up {
      display: inline-flex;
      align-items: center;
      gap: 0.35rem;
      min-height: 44px;
      min-width: 0;
      margin-right: auto;
      font-size: 0.9rem;
      color: var(--color-text-muted);
      text-decoration: none;
    }

    .workspace-up:hover {
      color: var(--color-text);
      text-decoration: none;
    }

    .workspace-up-label {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .workspace-body.work-content {
      padding-top: 0.25rem;
    }

    .workspace-tabs {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: 55;
      height: var(--pw-nav-h);
      box-sizing: border-box;
      padding: 4px 4px env(safe-area-inset-bottom, 0px);
      gap: 0;
      overflow: visible;
      background: var(--color-glass);
      backdrop-filter: blur(16px);
      -webkit-backdrop-filter: blur(16px);
      border-top: 1px solid var(--color-border);
    }

    .workspace-tabs.more-left,
    .workspace-tabs.more-right,
    .workspace-tabs.more-left.more-right {
      mask-image: none;
    }

    .workspace-tabs.single {
      display: none;
    }

    .workspace-tab {
      flex: 1 1 0;
      min-width: 0;
      flex-direction: column;
      justify-content: center;
      gap: 2px;
      padding: 2px 4px;
      font-size: 0.68rem;
      border-bottom: none;
      border-radius: var(--radius);
    }

    .workspace-tab .tab-icon :global(svg) {
      width: 22px;
      height: 22px;
    }

    .tab-label {
      max-width: 100%;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .workspace-tab.active {
      color: var(--color-primary);
    }

    .workspace-tab.active .tab-icon {
      color: var(--color-primary);
    }

    .workspace-body.over-tab-bar {
      padding-bottom: calc(var(--pw-nav-h) + 2rem);
    }
  }

  /* --- Relationship cluster, right end --- */
  .workspace-cluster {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
    flex-shrink: 0;
    padding: 8px 0;
  }

  /* --- Public-profile action, beside the context crumb in the global bar.
     The crumb name truncates around it; the action itself never shrinks. --- */
  .crumb-group {
    display: flex;
    align-items: center;
    gap: 2px;
    min-width: 0;
    flex-shrink: 1;
  }

  .view-profile-action {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    flex-shrink: 0;
    color: var(--color-text-muted);
    text-decoration: none;
    border-radius: var(--radius);
    transition: background 150ms ease, color 150ms ease;
  }

  .view-profile-action:hover {
    color: var(--color-text);
    background: var(--color-overlay);
    text-decoration: none;
  }
</style>
