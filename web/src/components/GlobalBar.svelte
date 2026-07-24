<script>
  /**
   * The one global bar (see docs/adr/005). Persists on every screen:
   * a leading slot (scope switcher in discovery, context crumb in
   * workspaces/admin), a contextual search slot, and the bell + user menu —
   * which never move. Consumers provide `leading` and `search` snippets;
   * everything to the right is owned here.
   *
   * Seam ownership: the bar carries its own bottom border (`bordered`) only
   * in discovery's ordinary page views, where it would otherwise bleed
   * surface-on-surface into the content. Over the quilt/map canvas
   * it's glass (`glass`) with no seam, and in workspace/admin takeovers the
   * tab row directly below the bar is the seam — those shells pass neither
   * prop. `glass` and `bordered` are mutually exclusive by contract.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { isLoggedIn, isAdmin as isGlobalAdmin, getUser, logout } from '../stores/auth.svelte.js';
  import { toggleTheme, getResolvedTheme } from '../stores/theme.svelte.js';
  import { Plus } from 'phosphor-svelte';
  import NotificationBell from './NotificationBell.svelte';
  import SidePanel from './SidePanel.svelte';
  import NotifIcon from './NotifIcon.svelte';
  import { api } from '../lib/api.js';

  // shelfBell: the discovery shell shows notifications on its mobile bottom
  // shelf, so the bar's bell hides under 768px there. Workspace/admin shells
  // have no shelf and keep the bar bell on every size.
  let { glass = false, bordered = false, shelfBell = false, leading, search } = $props();

  let userMenuOpen = $state(false);
  let newMenuOpen = $state(false);
  let isDark = $state(getResolvedTheme() === 'dark');

  // Notification panel state
  let notifPanelOpen = $state(false);
  let notifLoading = $state(false);
  let notifications = $state([]);

  function handleBellOpen() {
    // On mobile, notifications are a page, not an overlay.
    if (window.matchMedia('(max-width: 768px)').matches) {
      navigate('/notifications');
      return;
    }
    openNotifPanel();
  }

  async function openNotifPanel() {
    notifPanelOpen = true;
    notifLoading = true;
    try {
      const data = await api('notifications?limit=20');
      notifications = data.items || [];
    } catch {
      notifications = [];
    }
    notifLoading = false;
  }

  async function markAllRead() {
    try {
      await api('notifications/read-all', { method: 'POST' });
      notifications = notifications.map(n => ({ ...n, read_at: new Date().toISOString() }));
    } catch {}
  }

  async function clickNotification(notif) {
    if (!notif.read_at) {
      try { await api(`notifications/${notif.id}/read`, { method: 'PATCH' }); } catch {}
    }
    notifPanelOpen = false;
    if (notif.link) navigate(notif.link);
  }

  function timeAgo(iso) {
    if (!iso) return '';
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
  }

  function handleThemeToggle() {
    toggleTheme();
    isDark = getResolvedTheme() === 'dark';
  }

  // Close menus on navigation
  let path = $derived(getPath());
  $effect(() => {
    void path;
    userMenuOpen = false;
    newMenuOpen = false;
  });

  function handleNav(e, href) {
    e.preventDefault();
    navigate(href);
  }

  async function handleLogout() {
    userMenuOpen = false;
    await logout();
    navigate('/');
  }

  function handleWindowClick(e) {
    if (userMenuOpen && !e.target.closest('.user-menu-container')) {
      userMenuOpen = false;
    }
    if (newMenuOpen && !e.target.closest('.new-menu-container')) {
      newMenuOpen = false;
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<header class="top-bar" class:quilt-mode={glass} class:bordered>
  {#if leading}
    {@render leading()}
  {/if}

  {#if search}
    {@render search()}
  {/if}

  <div class="bar-right">
    {#if isLoggedIn()}
      <div class="new-menu-container">
        <button
          class="bar-new-btn"
          class:open={newMenuOpen}
          onclick={() => { newMenuOpen = !newMenuOpen; }}
          aria-label="Create new"
          aria-haspopup="menu"
          aria-expanded={newMenuOpen}
        >
          <Plus size={16} weight="bold" />
          <span class="bar-new-label">New</span>
        </button>
        {#if newMenuOpen}
          <div class="new-dropdown" role="menu">
            <a href="/patches/new" role="menuitem" onclick={(e) => { handleNav(e, '/patches/new'); newMenuOpen = false; }}>New patch</a>
            <a href="/events/new" role="menuitem" onclick={(e) => { handleNav(e, '/events/new'); newMenuOpen = false; }}>New event</a>
          </div>
        {/if}
      </div>
      <div class="bar-bell" class:shelf-bell={shelfBell}>
        <NotificationBell onOpen={handleBellOpen} />
      </div>
      <div class="user-menu-container">
        <button
          class="bar-avatar-btn"
          onclick={() => { userMenuOpen = !userMenuOpen; }}
          title={getUser()?.display_name || getUser()?.username}
        >
          <div class="bar-avatar">
            {#if getUser()?.avatar_url}
              <img src={getUser().avatar_url} alt="" />
            {:else}
              {(getUser()?.display_name || getUser()?.username || '?')[0].toUpperCase()}
            {/if}
          </div>
        </button>
        {#if userMenuOpen}
          <div class="user-dropdown">
            <div class="user-dropdown-name">
              {getUser()?.display_name || getUser()?.username}
              {#if isGlobalAdmin()}
                <span class="user-dropdown-role">Instance admin</span>
              {/if}
            </div>
            <a href="/users/{getUser()?.username}" onclick={(e) => { handleNav(e, `/users/${getUser()?.username}`); userMenuOpen = false; }}>Your profile</a>
            <a href="/settings" onclick={(e) => { handleNav(e, '/settings'); userMenuOpen = false; }}>Settings</a>
            <a href="/label" onclick={(e) => { handleNav(e, '/label'); userMenuOpen = false; }}>The Label</a>
            {#if isGlobalAdmin()}
              <a href="/admin" onclick={(e) => { handleNav(e, '/admin'); userMenuOpen = false; }}>Admin</a>
            {/if}
            <div class="dropdown-divider"></div>
            <button onclick={() => { handleThemeToggle(); }}>
              {isDark ? 'Light mode' : 'Dark mode'}
            </button>
            <button onclick={handleLogout}>Log Out</button>
          </div>
        {/if}
      </div>
    {:else}
      <a href="/login" class="bar-login" onclick={(e) => handleNav(e, '/login')}>Log In</a>
    {/if}
  </div>
</header>

<!-- Notification panel -->
<SidePanel open={notifPanelOpen} onClose={() => { notifPanelOpen = false; }} title="Notifications" side="left">
  {#snippet children()}
    {#if notifications.some(n => !n.read_at)}
      <div class="notif-actions">
        <button class="notif-mark-all" onclick={markAllRead}>Mark all read</button>
      </div>
    {/if}

    {#if notifLoading}
      <div class="notif-empty">Loading...</div>
    {:else if notifications.length === 0}
      <div class="notif-empty">You're all caught up</div>
    {:else}
      <div class="notif-list">
        {#each notifications as notif (notif.id)}
          <button
            class="notif-item"
            class:unread={!notif.read_at}
            onclick={() => clickNotification(notif)}
          >
            <span class="notif-icon"><NotifIcon type={notif.type} /></span>
            <div class="notif-content">
              <div class="notif-title">{notif.title}</div>
              {#if notif.body}
                <div class="notif-body">{notif.body}</div>
              {/if}
              <div class="notif-time">{timeAgo(notif.created_at)}</div>
            </div>
          </button>
        {/each}
      </div>
      <a
        href="/notifications"
        class="notif-view-all"
        onclick={(e) => { e.preventDefault(); notifPanelOpen = false; navigate('/notifications'); }}
      >View all notifications</a>
    {/if}
  {/snippet}
</SidePanel>

<style>
  .top-bar {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    height: 56px;
    z-index: 60;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 16px 0 8px;
    background: var(--color-surface);
  }

  /* Discovery page views: the bar owns its bottom seam (box-sizing is
     border-box globally, so the 56px height is unchanged). Takeover shells
     never set this — their tab row carries the seam. */
  .top-bar.bordered {
    border-bottom: 1px solid var(--color-border);
  }

  /* Over the quilt: glass so the canvas reads through */
  .top-bar.quilt-mode {
    background: var(--color-glass);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
  }

  /* Mobile: breathe a little, and let the content read through the bar */
  @media (max-width: 768px) {
    .top-bar {
      gap: 12px;
      padding: 0 16px 0 12px;
      background: color-mix(in srgb, var(--color-surface) 72%, transparent);
      backdrop-filter: blur(14px);
      -webkit-backdrop-filter: blur(14px);
    }

    /* Bell lives on the bottom shelf when the shell provides one. Keep it
       mounted — it owns the unread-count polling the shelf badge reads. */
    .bar-bell.shelf-bell {
      display: none;
    }
  }

  /* --- Right cluster --- */
  .bar-right {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
    flex-shrink: 0;
  }

  .bar-bell :global(.bell-btn) {
    color: var(--color-text-muted);
  }

  .bar-avatar-btn {
    border: none;
    background: none;
    padding: 4px;
    border-radius: 50%;
    cursor: pointer;
  }

  .bar-avatar-btn:hover {
    background: var(--color-overlay);
  }

  .bar-avatar {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
    font-size: 0.75rem;
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
  }

  .bar-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .bar-login {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--color-primary);
    padding: 8px 14px;
    border-radius: var(--radius);
  }

  .bar-login:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  /* --- + New button --- */
  .new-menu-container {
    position: relative;
  }

  .bar-new-btn {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 0.4rem 0.7rem 0.4rem 0.55rem;
    border: none;
    background: none;
    color: var(--color-text-muted);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    border-radius: var(--radius);
  }

  .bar-new-btn:hover,
  .bar-new-btn.open {
    color: var(--color-text);
    background: var(--color-overlay);
  }

  /* Match the notification bell: icon-only under 768px */
  @media (max-width: 768px) {
    .bar-new-label {
      display: none;
    }

    .bar-new-btn {
      padding: 0.4rem;
    }
  }

  .new-dropdown {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 4px 16px var(--color-shadow);
    min-width: 160px;
    z-index: 200;
    overflow: hidden;
    padding: 4px;
  }

  .new-dropdown a {
    display: block;
    width: 100%;
    padding: 0.5rem 0.75rem;
    text-align: left;
    font-size: 0.85rem;
    color: var(--color-text);
    text-decoration: none;
    border-radius: 4px;
  }

  .new-dropdown a:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .user-menu-container {
    position: relative;
  }

  .user-dropdown {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 4px 16px var(--color-shadow);
    min-width: 180px;
    z-index: 200;
    overflow: hidden;
    padding: 4px;
  }

  .user-dropdown-name {
    padding: 0.5rem 0.75rem;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--color-text-muted);
    border-bottom: 1px solid var(--color-border);
    margin-bottom: 4px;
  }

  /* The instance-admin role, stated once here rather than as a chip in the
     bar — it describes the person, not the context they're in. */
  .user-dropdown-role {
    display: block;
    margin-top: 2px;
    font-size: 0.72rem;
    font-weight: 500;
    color: var(--color-accent);
  }

  .user-dropdown a,
  .user-dropdown button {
    display: block;
    width: 100%;
    padding: 0.5rem 0.75rem;
    text-align: left;
    font-size: 0.85rem;
    color: var(--color-text);
    text-decoration: none;
    border: none;
    background: none;
    cursor: pointer;
    border-radius: 4px;
  }

  .user-dropdown a:hover,
  .user-dropdown button:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .dropdown-divider {
    height: 1px;
    background: var(--color-border);
    margin: 0.25rem 0;
  }

  /* ================================================================
     NOTIFICATION PANEL CONTENT
     ================================================================ */
  .notif-actions {
    display: flex;
    justify-content: flex-end;
    padding: 8px 20px;
    border-bottom: 1px solid var(--color-border);
  }

  .notif-mark-all {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.8rem;
    cursor: pointer;
    padding: 0;
  }

  .notif-mark-all:hover {
    text-decoration: underline;
  }

  .notif-empty {
    padding: 2rem;
    text-align: center;
    font-size: 0.88rem;
    color: var(--color-text-muted);
  }

  .notif-list {
    display: flex;
    flex-direction: column;
  }

  .notif-item {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    width: 100%;
    padding: 12px 20px;
    text-align: left;
    border: none;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-surface);
    cursor: pointer;
    font-size: 0.88rem;
    transition: background 100ms ease;
  }

  .notif-item:hover {
    background: var(--color-overlay);
  }

  .notif-item.unread {
    background: color-mix(in srgb, var(--color-primary) 6%, var(--color-surface));
  }

  .notif-item.unread:hover {
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
  }

  .notif-icon {
    display: flex;
    flex-shrink: 0;
    margin-top: 0.1rem;
    color: var(--color-primary);
  }

  .notif-content {
    flex: 1;
    min-width: 0;
  }

  .notif-title {
    font-weight: 600;
    margin-bottom: 2px;
    color: var(--color-text);
  }

  .notif-body {
    color: var(--color-text-muted);
    font-size: 0.82rem;
    line-height: 1.4;
    margin-bottom: 4px;
  }

  .notif-time {
    color: var(--color-text-muted);
    font-size: 0.72rem;
  }

  .notif-view-all {
    display: block;
    text-align: center;
    padding: 12px;
    font-size: 0.85rem;
    color: var(--color-primary);
    text-decoration: none;
    border-top: 1px solid var(--color-border);
  }

  .notif-view-all:hover {
    background: var(--color-overlay);
  }
</style>
