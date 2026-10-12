<script>
  /**
   * The workspace: a patch's full-screen management and participation
   * surface (docs/adr/005). Renders the global bar with the patch's
   * context crumb and scoped finder, then its own tab row — no discovery
   * chrome. Role decides what shows: admins get Settings, followers get
   * permission-gated tabs, non-members get Join/Follow in the right
   * cluster.
   *
   * On a narrow screen the tabs are a bottom bar (Liquid Textile's
   * .lt-tabbar), where the quilt's own rail sits at that width, so a
   * phone's navigation is at the foot of the screen everywhere.
   * workspaceTabs never returns more than five, which is what makes a bar
   * of fixed slots possible. A single tab is no choice at all and gets no
   * bar. The relationship cluster stays at the top.
   */
  import { setContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { patchLoadError, archivedPatchName } from '../lib/patchError.js';
  import { navigate } from '../stores/router.svelte.js';
  import { setPatchName } from '../stores/patchName.svelte.js';
  import { workspaceFinderProvider } from '../lib/finderProviders.js';
  import { workspaceTabs } from '../lib/patchWorkspace.js';
  import { ltTabs } from '../lib/lt.js';
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import Skeleton from './Skeleton.svelte';
  import PatchRelationship from './PatchRelationship.svelte';
  import PatchBarMenu from './PatchBarMenu.svelte';
  import { getPendingMembershipSlugs, getInvitedMembershipSlugs, loadMemberships } from '../stores/memberships.svelte.js';
  import { Scales, UsersThree, CalendarBlank, GearSix, Eye, Chalkboard } from 'phosphor-svelte';

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

  // The link up a level: on a phone, the ‹ that opens the patch bar
  // (PatchBarMenu). With none, that ‹ goes back to the quilt.
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

  // An archived patch this viewer administers, named. Empty for everybody
  // else, who is told exactly what they were told before (docs/adr/034).
  let archivedName = $state('');

  // Whether the load failed, held apart from the message.
  //
  // The error branch used to test the message string, which worked only
  // because every failure had one. A plain not-found now deliberately has
  // none — its heading says everything there is to say — and an empty
  // string is falsy, so the page fell past the error branch and sat on its
  // loading skeleton for ever. Whether something failed and what to print
  // about it are two questions.
  let loadFailed = $state(false);

  async function loadNode() {
    loading = true;
    error = '';
    loadFailed = false;
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
      error = patchLoadError(e);
      archivedName = archivedPatchName(e) || (e?.data?.archived ? slug : '');
      loadFailed = true;
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
</script>

<div class="workspace">
  <GlobalBar patchContext>
    {#snippet leading()}
      <!-- A phone: the patch's name is the menu (PatchBarMenu), with the way
           up before it and a visitor's next rung after it. Wide: the crumb
           and the public-profile eye, with the cluster on the row below. -->
      <div class="patch-bar-slot">
        <PatchBarMenu
          {slug}
          {node}
          {isAdmin}
          {isUnclaimed}
          {isBanned}
          {viewerTrusted}
          {membershipRole}
          {requestPending}
          {invited}
          {liningStatus}
          {upLink}
          onChanged={reloadStanding}
        />
      </div>
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
  {:else if loadFailed && !node}
    <div class="workspace-body container">
      <div class="shell-error">
        <h2>{archivedName ? `${archivedName} is archived` : 'Patch not found'}</h2>
        {#if error}<p class="lt-muted">{error}</p>{/if}
        <div class="shell-error-actions">
          <button class="lt-btn lt-btn-secondary" onclick={loadNode}>Retry</button>
          <a href="/" class="lt-btn lt-btn-secondary" onclick={(e) => { e.preventDefault(); navigate('/'); }}>Back to Quilt</a>
        </div>
      </div>
    </div>
  {:else if node}
    <!-- Workspace nav: tabs + relationship cluster, directly under the bar -->
    <div class="workspace-nav">
      <!-- Liquid Textile's tab row (ltTabs), which fades an edge with more
           tabs past it and brings the current one into view: a member of a
           year never found the Noticeboard tab off the edge of a phone
           (f2660b9). At or below 768px, two or more tabs are its tab bar. -->
      <nav
        class="lt-tabs workspace-tabs"
        class:lt-tabbar={tabs.length >= 2}
        class:single={tabs.length < 2}
        use:ltTabs={{ current: activeTab }}
      >
        {#each tabs as tab (tab.id)}
          {@const Icon = tab.icon}
          <a
            href={tab.href}
            class="lt-tab workspace-tab"
            aria-current={activeTab === tab.id ? 'page' : undefined}
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

  /* Liquid Textile's tab row (ltTabs); the nav under it draws the rule. */
  .workspace-tabs {
    border-bottom: 0;
  }

  .tab-icon {
    display: flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .workspace-tab[aria-current="page"] .tab-icon {
    color: var(--color-accent);
  }

  /* --- Narrow screen: the tabs become a bottom bar ---
     Liquid Textile's .lt-tabbar: the same footprint and glass as
     SocialShell's mobile rail, and while it is on the page it sets
     --lt-chrome-bottom, which everything else ending at the foot of the
     screen (toasts, the vote bar) clears. The row the tabs leave keeps
     only the relationship cluster, and stops being sticky: a Join button
     does not need to follow you down the page. */
  /* The phone's patch bar (PatchBarMenu); a wide screen has the crumb. */
  .patch-bar-slot {
    display: none;
    flex: 1;
    min-width: 0;
  }

  @media (max-width: 768px) {
    .workspace-nav {
      position: static;
      justify-content: flex-end;
      background: none;
      border-bottom: none;
    }

    .workspace-body.work-content {
      padding-top: 0.25rem;
    }

    .workspace-tabs.single {
      display: none;
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

    .workspace-tab[aria-current="page"] .tab-icon {
      color: var(--color-primary);
    }

    .workspace-body.over-tab-bar {
      padding-bottom: calc(var(--lt-chrome-bottom) + 2rem);
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

  /* Last in the file on purpose: the crumb and the cluster's own rules
     above would otherwise win at equal specificity. The bar carries the
     patch on a phone (PatchBarMenu): the way up, the name as the menu,
     and a visitor's next rung. The row under it keeps only the tabs,
     which are fixed to the foot. */
  @media (max-width: 768px) {
    .crumb-group,
    .workspace-cluster {
      display: none;
    }

    .patch-bar-slot {
      display: flex;
    }
  }
</style>
