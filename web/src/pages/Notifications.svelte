<script>
  import NotificationRow from '../components/NotificationRow.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { clearUnread, decrementUnread, getUnread } from '../stores/notifications.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  let notifications = $state([]);
  let loading = $state(true);
  let nextCursor = $state('');
  // All or unread, and nothing finer. The type chips this replaced covered
  // five prefixes, so a reply on a notice, an admin queue, a claim or a
  // trust answer appeared under All and nowhere else, and most people's list
  // is short enough to read. The API keeps its category filter.
  let unreadOnly = $state(false);

  $effect(() => {
    loadNotifications();
  });

  async function loadNotifications(append = false) {
    if (!append) loading = true;
    try {
      let params = '?limit=20';
      if (unreadOnly) params += '&unread=true';
      if (append && nextCursor) params += `&after=${nextCursor}`;
      const data = await api(`notifications${params}`);
      const items = data.items || [];
      notifications = append ? [...notifications, ...items] : items;
      nextCursor = data.next_cursor || '';
    } catch {
      if (!append) notifications = [];
    } finally {
      loading = false;
    }
  }

  // The effect above reads unreadOnly before its first await, so setting
  // it is the reload. Calling loadNotifications here as well fetched twice.
  function setUnreadOnly(value) {
    unreadOnly = value;
  }

  async function markAllRead() {
    await api('notifications/read-all', { method: 'POST' });
    notifications = notifications.map(n => ({ ...n, read_at: n.read_at || new Date().toISOString() }));
    clearUnread();
  }

  // Dismissing one: the row goes, and an unread row takes the badge down with
  // it. A read row never counted, so the badge does not move.
  async function dismissNotif(notif) {
    try {
      await api(`notifications/${notif.id}`, { method: 'DELETE' });
      notifications = notifications.filter(n => n.id !== notif.id);
      if (!notif.read_at) decrementUnread();
    } catch (e) {
      showToast(e.message || 'Could not dismiss that notification', 'error');
    }
  }

  // Clear-all empties the whole table server-side, not the page in view, so
  // the badge goes to zero rather than down by the rows rendered.
  async function clearAll() {
    try {
      await api('notifications', { method: 'DELETE' });
      notifications = [];
      nextCursor = '';
      clearUnread();
    } catch (e) {
      showToast(e.message || 'Could not clear notifications', 'error');
    }
  }

  async function clickNotif(notif) {
    if (!notif.read_at) {
      try {
        await api(`notifications/${notif.id}/read`, { method: 'PATCH' });
        notif.read_at = new Date().toISOString();
        notifications = [...notifications];
        decrementUnread();
      } catch {}
    }
    if (notif.link) navigate(notif.link);
  }
</script>

<div class="lt-fade-in">
  <div class="container-narrow">
    <div class="notif-page-header">
      <h1>Notifications</h1>
      <div class="header-actions">
        {#if getUnread() > 0 || notifications.some(n => !n.read_at)}
          <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={markAllRead}>Mark all read</button>
        {/if}
        {#if notifications.length > 0}
          <ConfirmAction
            label="Clear all"
            confirmLabel="Delete them all"
            variant="danger"
            onConfirm={clearAll}
          />
        {/if}
        <a
          href="/settings/notifications"
          class="lt-btn lt-btn-ghost lt-btn-sm"
          onclick={(e) => { e.preventDefault(); navigate('/settings/notifications'); }}
        >Settings</a>
      </div>
    </div>

    <div class="filters">
      <button class="lt-chip" aria-pressed={!unreadOnly} onclick={() => setUnreadOnly(false)}>All</button>
      <button class="lt-chip" aria-pressed={unreadOnly} onclick={() => setUnreadOnly(true)}>Unread</button>
    </div>

    {#if loading}
      <p class="lt-muted center-text">Loading...</p>
    {:else if notifications.length === 0}
      <div class="lt-empty">
        <p class="lt-empty-title">{unreadOnly ? "You're all caught up" : 'No notifications'}</p>
      </div>
    {:else}
      <div class="notif-list">
        {#each notifications as notif (notif.id)}
          <NotificationRow {notif} onOpen={clickNotif} onDismiss={dismissNotif} />
        {/each}
      </div>

      {#if nextCursor}
        <div class="load-more">
          <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => loadNotifications(true)}>Load more</button>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .notif-page-header {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0 1rem;
  }

  .notif-page-header h1 {
    font-size: 1.3rem;
  }

  /* A narrow screen puts the actions under the heading rather than
     wrapping each button's label onto two lines beside it. */
  .header-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .header-actions :global(.lt-btn) {
    white-space: nowrap;
  }

  .filters {
    display: flex;
    gap: 0.4rem;
    margin-bottom: 1rem;
  }

  .notif-list {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    overflow: hidden;
  }

  .center-text { text-align: center; padding: 2rem 0; }

  .load-more {
    text-align: center;
    padding: 1rem 0;
  }
</style>
