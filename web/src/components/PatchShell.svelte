<script>
  /**
   * The workspace: a patch's full-screen management and participation
   * surface (docs/adr/005). Renders the global bar with the patch's
   * context crumb and scoped finder, then its own tab row — no discovery
   * chrome. Role decides what shows: admins get Settings, followers get
   * permission-gated tabs, non-members get Join/Follow in the right
   * cluster.
   */
  import { setContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { setPatchName } from '../stores/patchName.svelte.js';
  import { workspaceFinderProvider } from '../lib/finderProviders.js';
  import { workspaceTabs } from '../lib/patchWorkspace.js';
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import Skeleton from './Skeleton.svelte';
  import UnlockPanel from './UnlockPanel.svelte';
  import SetupChecklist from './SetupChecklist.svelte';
  import JoinSheet from './JoinSheet.svelte';
  import { Scales, UsersThree, CalendarBlank, GearSix, Eye } from 'phosphor-svelte';

  let { slug = '', activeTab = 'governance', children } = $props();

  // --- Patch data (fetched once, shared via context) ---
  let node = $state(null);
  let isMember = $state(false);
  let isAdmin = $state(false);
  let membershipRole = $state('');
  let followerPermissions = $state(null);
  let loading = $state(true);
  let error = $state('');
  let joining = $state(false);

  let isUnclaimed = $state(false);
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

  // The join sheet stands between the click and membership (docs/adr/040)
  // — this workspace path must not bypass what the profile path shows.
  let joinSheetOpen = $state(false);
  let liningStatus = $state('');

  function openJoinSheet() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    joinSheetOpen = true;
  }

  async function handleJoin(message) {
    const wasFollower = membershipRole === 'follower';
    joining = true;
    try {
      const result = await api(`nodes/${slug}/join`, { method: 'POST', body: message ? { message } : undefined });
      await loadNode();
      joinSheetOpen = false;
      if (result.status === 'pending') {
        showToast('Membership request sent', 'success');
      } else {
        showToast(wasFollower ? 'You are now a member' : 'Joined patch', 'success');
      }
    } catch (e) {
      showToast(e.message || 'Failed to join', 'error');
    } finally {
      joining = false;
    }
  }

  async function handleFollow() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    joining = true;
    try {
      await api(`nodes/${slug}/join`, { method: 'POST', body: { role: 'follower' } });
      await loadNode();
      showToast('Following patch', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to follow', 'error');
    } finally {
      joining = false;
    }
  }

  async function handleLeave() {
    const wasFollower = membershipRole === 'follower';
    joining = true;
    try {
      await api(`nodes/${slug}/leave`, { method: 'POST' });
      await loadNode();
      showToast(wasFollower ? 'Unfollowed patch' : 'Left patch', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to leave', 'error');
    } finally {
      joining = false;
    }
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
    settings: GearSix,
  };

  const tabs = $derived.by(() =>
    workspaceTabs({ isUnclaimed, isAdmin, membershipRole, followerPermissions }).map((t) => ({
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
      <nav class="workspace-tabs">
        {#each tabs as tab (tab.id)}
          {@const Icon = tab.icon}
          <a
            href={tab.href}
            class="workspace-tab"
            class:active={activeTab === tab.id}
            onclick={(e) => handleTabClick(e, tab.href)}
          >
            <span class="tab-icon"><Icon size={16} weight="duotone" /></span>
            {tab.label}
          </a>
        {/each}
      </nav>

      {#if !isAdmin}
        <div class="workspace-cluster">
          {#if isBanned}
            <span class="banned-notice">Removed from this community</span>
          {:else if isMember}
            {#if membershipRole === 'follower'}
              <button class="btn btn-primary btn-sm" onclick={openJoinSheet} disabled={joining}>Become Member</button>
              <button class="btn btn-secondary btn-sm" onclick={handleLeave} disabled={joining}>Unfollow</button>
            {:else}
              <button class="btn btn-secondary btn-sm" onclick={handleLeave} disabled={joining}>Leave</button>
            {/if}
          {:else}
            <button class="btn btn-primary btn-sm" onclick={openJoinSheet} disabled={joining}>Join</button>
            <button class="btn btn-secondary btn-sm" onclick={handleFollow} disabled={joining}>Follow</button>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Onboarding surfaces (docs/adr/040): panels, never wizards — self-gating,
         each renders nothing when it doesn't apply. -->
    <UnlockPanel />
    <SetupChecklist />

    <!-- Tab content -->
    <div class="workspace-body work-content">
      {@render children()}
    </div>
  {/if}
</div>

<JoinSheet
  open={joinSheetOpen}
  onClose={() => { joinSheetOpen = false; }}
  onConfirm={handleJoin}
  slug={slug}
  patchName={node?.name || ''}
  membershipPolicy={node?.membership_policy || 'open'}
  liningStatus={liningStatus}
  submitting={joining}
/>

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

  /* --- Relationship cluster, right end --- */
  .workspace-cluster {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
    flex-shrink: 0;
    padding: 8px 0;
  }

  .banned-notice {
    font-size: 0.78rem;
    color: var(--color-error);
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
