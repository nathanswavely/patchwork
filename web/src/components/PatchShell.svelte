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
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import Skeleton from './Skeleton.svelte';
  import { Scales, UsersThree, CalendarBlank, GearSix, ArrowSquareOut } from 'phosphor-svelte';

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
      isMember = data.is_member || false;
      isAdmin = data.is_admin || false;
      membershipRole = data.membership_role || '';
      followerPermissions = (data.node || data).follower_permissions || null;
      isUnclaimed = data.is_unclaimed || false;
      isBanned = data.is_banned || false;
      setPatchName(node?.name || slug);
    } catch (e) {
      error = e.message || 'Failed to load patch';
      node = null;
    } finally {
      loading = false;
    }
  }

  async function handleJoin() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    const wasFollower = membershipRole === 'follower';
    joining = true;
    try {
      const result = await api(`nodes/${slug}/join`, { method: 'POST' });
      await loadNode();
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
  // The workspace is for everyone; role decides what shows. Admins get the
  // Settings tab, followers get permission-gated tabs.
  let basePath = $derived(`/patches/${slug}`);

  const tabs = $derived.by(() => {
    if (isUnclaimed) return [];

    const t = [
      { id: 'governance', label: 'Governance', href: `${basePath}/governance`, icon: Scales },
    ];

    const fp = followerPermissions;
    const isFollower = membershipRole === 'follower';

    if (!isFollower || fp?.members !== false)
      t.push({ id: 'members', label: 'Members', href: `${basePath}/members`, icon: UsersThree });
    if (!isFollower || fp?.events !== false)
      t.push({ id: 'events', label: 'Events', href: `${basePath}/events`, icon: CalendarBlank });
    if (isAdmin)
      t.push({ id: 'settings', label: 'Settings', href: `${basePath}/settings`, icon: GearSix });

    return t;
  });

  let finderProvider = $derived(workspaceFinderProvider(slug));

  function handleTabClick(e, href) {
    e.preventDefault();
    navigate(href);
  }
</script>

<div class="workspace">
  <GlobalBar>
    {#snippet leading()}
      <ContextCrumb label={node?.name || slug} href={`${basePath}/governance`} />
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

      <div class="workspace-cluster">
        {#if isAdmin}
          <span class="role-badge managing">Managing</span>
        {:else if membershipRole === 'member'}
          <span class="role-badge">Member</span>
        {:else if membershipRole === 'follower'}
          <span class="role-badge">Following</span>
        {/if}

        {#if !isAdmin}
          {#if isBanned}
            <span class="banned-notice">Removed from this community</span>
          {:else if isMember}
            {#if membershipRole === 'follower'}
              <button class="btn btn-primary btn-sm" onclick={handleJoin} disabled={joining}>Become Member</button>
              <button class="btn btn-secondary btn-sm" onclick={handleLeave} disabled={joining}>Unfollow</button>
            {:else}
              <button class="btn btn-secondary btn-sm" onclick={handleLeave} disabled={joining}>Leave</button>
            {/if}
          {:else}
            <button class="btn btn-primary btn-sm" onclick={handleJoin} disabled={joining}>Join</button>
            <button class="btn btn-secondary btn-sm" onclick={handleFollow} disabled={joining}>Follow</button>
          {/if}
        {/if}

        <a
          href="/patches/{slug}"
          class="view-profile"
          onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}`); }}
          title="View the public profile"
        >
          <ArrowSquareOut size={14} weight="duotone" />
          <span>View profile</span>
        </a>
      </div>
    </div>

    <!-- Tab content -->
    <div class="workspace-body work-content">
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

  .role-badge {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    background: color-mix(in srgb, var(--color-text-muted) 10%, transparent);
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    white-space: nowrap;
  }

  .role-badge.managing {
    color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  }

  .banned-notice {
    font-size: 0.78rem;
    color: var(--color-error);
  }

  .view-profile {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--color-text-muted);
    text-decoration: none;
    padding: 6px 8px;
    border-radius: var(--radius);
    white-space: nowrap;
  }

  .view-profile:hover {
    color: var(--color-text);
    background: var(--color-overlay);
    text-decoration: none;
  }

  .btn-sm {
    padding: 0.3rem 0.7rem;
    font-size: 0.8rem;
  }

  @media (max-width: 768px) {
    .view-profile span {
      display: none;
    }
  }
</style>
