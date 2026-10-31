<script>
  /**
   * The bell's dropdown on a desktop: the latest few notifications under the
   * bell, and the way through to the full page and to notification settings.
   * A phone never opens it; there the bell is a link to /notifications
   * (GlobalBar's handleBellOpen, and the shelf item in SocialShell).
   *
   * It replaced a side panel that slid in from the left edge while the bell
   * sits on the right. Fetched on every open, so it is as fresh as the page,
   * and it reconciles the badge while it is at it.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { clearUnread, decrementUnread, getUnread, refreshUnread } from '../stores/notifications.svelte.js';
  import NotificationRow from './NotificationRow.svelte';

  let { id = 'notif-dropdown', onClose } = $props();

  let items = $state([]);
  let loading = $state(true);

  let hasUnread = $derived(getUnread() > 0 || items.some((n) => !n.read_at));

  $effect(() => {
    load();
  });

  async function load() {
    loading = true;
    try {
      const data = await api('notifications?limit=10');
      items = data.items || [];
    } catch {
      items = [];
    }
    loading = false;
    refreshUnread();
  }

  async function markAllRead() {
    try {
      await api('notifications/read-all', { method: 'POST' });
      items = items.map((n) => ({ ...n, read_at: n.read_at || new Date().toISOString() }));
      clearUnread();
    } catch {}
  }

  async function openNotification(notif) {
    if (!notif.read_at) {
      try {
        await api(`notifications/${notif.id}/read`, { method: 'PATCH' });
        notif.read_at = new Date().toISOString();
        decrementUnread();
      } catch {}
    }
    onClose();
    if (notif.link) navigate(notif.link);
  }

  function go(e, href) {
    e.preventDefault();
    onClose();
    navigate(href);
  }
</script>

<div class="lt-popover notif-dropdown" {id} role="dialog" aria-label="Notifications">
  <div class="notif-dropdown-head">
    <h2>Notifications</h2>
    {#if hasUnread}
      <button class="notif-mark-all" onclick={markAllRead}>Mark all read</button>
    {/if}
  </div>

  <div class="notif-dropdown-list lt-scroll-thin">
    {#if loading && items.length === 0}
      <p class="notif-empty">Loading...</p>
    {:else if items.length === 0}
      <p class="notif-empty">You're all caught up</p>
    {:else}
      {#each items as notif (notif.id)}
        <NotificationRow {notif} onOpen={openNotification} clamp />
      {/each}
    {/if}
  </div>

  <div class="notif-dropdown-foot">
    <a href="/notifications" class="notif-view-all" onclick={(e) => go(e, '/notifications')}>See all notifications</a>
    <a href="/settings/notifications" class="notif-settings" onclick={(e) => go(e, '/settings/notifications')}>Settings</a>
  </div>
</div>

<style>
  .notif-dropdown {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 200;
    width: 380px;
    max-width: calc(100vw - 32px);
    max-height: min(560px, calc(100vh - 80px));
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .notif-dropdown-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 16px 10px;
    border-bottom: 1px solid var(--color-border);
  }

  .notif-dropdown-head h2 {
    margin: 0;
    font-size: 0.95rem;
  }

  .notif-mark-all {
    border: none;
    background: none;
    padding: 0;
    color: var(--color-primary);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .notif-mark-all:hover {
    text-decoration: underline;
  }

  .notif-dropdown-list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .notif-empty {
    margin: 0;
    padding: 2rem 1rem;
    text-align: center;
    font-size: 0.88rem;
    color: var(--color-text-muted);
  }

  .notif-dropdown-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-top: 1px solid var(--color-border);
  }

  .notif-dropdown-foot a {
    padding: 10px 16px;
    font-size: 0.82rem;
    text-decoration: none;
  }

  .notif-view-all {
    color: var(--color-primary);
  }

  .notif-settings {
    color: var(--color-text-muted);
  }

  .notif-dropdown-foot a:hover {
    text-decoration: underline;
  }
</style>
