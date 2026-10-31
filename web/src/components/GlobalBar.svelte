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
   * it paints nothing at all (`glass`) — no fill, no seam, and each control
   * carries its own chip — and in workspace/admin takeovers the
   * tab row directly below the bar is the seam — those shells pass neither
   * prop. `glass` and `bordered` are mutually exclusive by contract.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { isHandheld } from '../stores/viewport.svelte.js';
  import { isLoggedIn, isAdmin as isGlobalAdmin, getUser, logout } from '../stores/auth.svelte.js';
  import { Plus, Sliders } from 'phosphor-svelte';
  import DisplayMenu from './DisplayMenu.svelte';
  import { ltMenu } from '../lib/lt.js';
  import NotificationBell from './NotificationBell.svelte';
  import NotificationDropdown from './NotificationDropdown.svelte';

  // shelfBell: the discovery shell shows notifications on its mobile bottom
  // shelf, so the bar's bell hides under 768px there. Workspace/admin shells
  // have no shelf and keep the bar bell on every size.
  // patchContext: a patch workspace on a phone puts creating an event in the
  // patch's own sheet (PatchBarMenu), scoped to that patch, and gives the
  // bar's width to the patch name; the global "+" steps aside there.
  let { glass = false, bordered = false, shelfBell = false, patchContext = false, leading, search } = $props();

  let userMenuOpen = $state(false);
  let newMenuEl = $state(null);
  let displayMenuOpen = $state(false);

  // The bell's dropdown. A desktop opens it under the bell; a phone goes
  // to the notifications page instead, as it always has.
  let notifMenuOpen = $state(false);

  function handleBellOpen() {
    if (isHandheld()) {
      navigate('/notifications');
      return;
    }
    notifMenuOpen = !notifMenuOpen;
  }

  function closeNotifMenu({ refocus = false } = {}) {
    notifMenuOpen = false;
    if (refocus) document.querySelector('.notif-menu-container .bell-btn')?.focus();
  }

  // Close menus on navigation
  let path = $derived(getPath());
  $effect(() => {
    void path;
    userMenuOpen = false;
    displayMenuOpen = false;
    notifMenuOpen = false;
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
    if (notifMenuOpen && !e.target.closest('.notif-menu-container')) {
      notifMenuOpen = false;
    }
  }

  function handleWindowKeydown(e) {
    if (e.key === 'Escape' && notifMenuOpen) closeNotifMenu({ refocus: true });
  }
</script>

<svelte:window onclick={handleWindowClick} onkeydown={handleWindowKeydown} />

<header class="top-bar" class:quilt-mode={glass} class:bordered class:patch-context={patchContext}>
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
          aria-label="Create new"
          use:ltMenu={{ list: newMenuEl, align: 'end' }}
        >
          <Plus size={16} weight="bold" />
          <span class="bar-new-label">New</span>
        </button>
        <div class="lt-popover lt-menu new-dropdown" bind:this={newMenuEl}>
          <a href="/patches/new" class="lt-menu-item" onclick={(e) => handleNav(e, '/patches/new')}>New patch</a>
          <a href="/events/new" class="lt-menu-item" onclick={(e) => handleNav(e, '/events/new')}>New event</a>
        </div>
      </div>
      <div class="notif-menu-container">
        <div class="bar-bell" class:shelf-bell={shelfBell}>
          <NotificationBell onOpen={handleBellOpen} expanded={isHandheld() ? undefined : notifMenuOpen} controls="notif-dropdown" />
        </div>
        {#if notifMenuOpen}
          <NotificationDropdown id="notif-dropdown" onClose={closeNotifMenu} />
        {/if}
      </div>
      <div class="user-menu-container">
        <button
          class="bar-avatar-btn"
          onclick={() => { userMenuOpen = !userMenuOpen; }}
          title={getUser()?.display_name || getUser()?.username}
        >
          <div class="lt-avatar bar-avatar">
            {#if getUser()?.avatar_url}
              <img src={getUser().avatar_url} alt="" />
            {:else}
              {(getUser()?.display_name || getUser()?.username || '?')[0].toUpperCase()}
            {/if}
          </div>
        </button>
        {#if userMenuOpen}
          <div class="lt-popover user-dropdown">
            <div class="user-dropdown-name">
              {getUser()?.display_name || getUser()?.username}
              {#if isGlobalAdmin()}
                <span class="user-dropdown-role">Instance admin</span>
              {/if}
            </div>
            <a class="lt-menu-item" href="/users/{getUser()?.username}" onclick={(e) => { handleNav(e, `/users/${getUser()?.username}`); userMenuOpen = false; }}>Your profile</a>
            <a class="lt-menu-item" href="/settings" onclick={(e) => { handleNav(e, '/settings'); userMenuOpen = false; }}>Settings</a>
            <a class="lt-menu-item" href="/label" onclick={(e) => { handleNav(e, '/label'); userMenuOpen = false; }}>The Label</a>
            {#if isGlobalAdmin()}
              <a class="lt-menu-item" href="/admin" onclick={(e) => { handleNav(e, '/admin'); userMenuOpen = false; }}>Admin</a>
            {/if}
            <div class="lt-menu-divider"></div>
            <DisplayMenu />
            <div class="lt-menu-divider"></div>
            <button class="lt-menu-item" onclick={handleLogout}>Log Out</button>
          </div>
        {/if}
      </div>
    {:else}
      <!-- Anonymous: auth entries only — "What is Patchwork?" lives in the
           sidebar, where there's room (docs/adr/040). /login leads with
           "Join {instanceName}" (signup-primary), so a single "Log In" link
           disagreed with its own destination. Sign Up is primary and
           matches /login's default panel; Log In is secondary and opens
           /login straight to the sign-in panel via ?mode=signin. Under
           640px only Sign Up remains: /login's sign-in toggle is one tap
           in, the unified-auth-page pattern. -->
      <div class="display-menu-container">
        <button
          class="bar-display-btn"
          class:open={displayMenuOpen}
          onclick={() => { displayMenuOpen = !displayMenuOpen; }}
          aria-label="Display"
          aria-haspopup="menu"
          aria-expanded={displayMenuOpen}
        >
          <Sliders size={18} />
        </button>
        {#if displayMenuOpen}
          <div class="lt-popover user-dropdown display-dropdown">
            <DisplayMenu />
          </div>
        {/if}
      </div>
      <a href="/login?mode=signin" class="bar-login" onclick={(e) => handleNav(e, '/login?mode=signin')}>Log In</a>
      <a href="/login" class="lt-btn lt-btn-primary bar-signup" onclick={(e) => handleNav(e, '/login')}>Sign Up</a>
    {/if}
  </div>
</header>

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

  /* Over the quilt/map: no bar at all — the canvas runs to the top of the
     window and the controls float on it. Glass was still a band: at 0.92
     alpha it read as an opaque strip laid over the tiles, and the seam it
     drew across the canvas was the jank. The controls carry their own
     surfaces (the search pill, the avatar, the New button), so the bar
     itself has nothing left to paint. Specificity keeps this winning over
     the mobile `.top-bar` background below. */
  .top-bar.quilt-mode {
    background: transparent;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }

  /* With nothing behind them, the bare controls (bell, + New, Log In) would
     sit straight on the tiles, and a quilt is the worst possible backdrop
     for muted text — saturated, light, and different under every control.
     So each carries its own chip: the same backdrop the collapsed rail's
     chips use, which makes the two floating clusters read as one family
     rather than as a bar that lost its paint. The avatar, the Sign Up
     button and the search pill already own surfaces and are left alone. */
  .top-bar.quilt-mode .bar-new-btn,
  .top-bar.quilt-mode .bar-login,
  .top-bar.quilt-mode .bar-bell {
    background: color-mix(in srgb, var(--color-bg) 65%, transparent);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border-radius: var(--radius);
  }

  .top-bar.quilt-mode .bar-new-btn:hover,
  .top-bar.quilt-mode .bar-new-btn:global([aria-expanded="true"]),
  .top-bar.quilt-mode .bar-login:hover {
    background: color-mix(in srgb, var(--color-bg) 80%, transparent);
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
    --lt-avatar-size: 30px;
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

  .bar-signup {
    flex-shrink: 0;
    font-size: 0.85rem;
  }

  /* Narrow: one auth entry. Sign Up stays (it's the primary ask of an
     anonymous visitor); /login's own sign-in toggle covers returning
     users, so the bar never overflows. */
  @media (max-width: 640px) {
    .bar-login {
      display: none;
    }
  }

  /* --- + New button --- */
  /* Liquid Textile's menu (ltMenu); the list is the popover below. */
  .new-menu-container {
    position: relative;
    --lt-z-menu: 200;
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
    min-width: var(--lt-target);
    min-height: var(--lt-target);
  }

  .bar-new-btn:hover,
  .bar-new-btn:global([aria-expanded="true"]) {
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

  .user-menu-container {
    position: relative;
  }

  .display-menu-container {
    position: relative;
  }

  /* Signed out, this occupies the slot the avatar holds once you join, so
     the Display control does not move when somebody signs up (docs/adr/112).
     It also closes a gap that predates muted colors: an anonymous reader had
     no theme control at all, and got whatever their OS asked for. */
  .bar-display-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: var(--radius);
    border: 1px solid transparent;
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .bar-display-btn:hover,
  .bar-display-btn.open {
    border-color: var(--color-border);
    color: var(--color-text);
  }

  .display-dropdown {
    min-width: 232px;
    padding: 0;
  }

  .user-dropdown {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
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

  .notif-menu-container {
    position: relative;
  }

  /* The bar's buttons and every menu row are at least --lt-target tall,
     a thumb's height on a finger; there the new button's icon sits
     centred in the larger box. */
  @media (pointer: coarse) {
    .bar-new-btn {
      justify-content: center;
    }
  }

  @media (max-width: 768px) {
    .top-bar.patch-context .new-menu-container {
      display: none;
    }
  }
</style>
