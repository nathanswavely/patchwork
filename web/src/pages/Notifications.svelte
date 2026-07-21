<script>
  import NotifIcon from '../components/NotifIcon.svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';

  let notifications = $state([]);
  let loading = $state(true);
  let nextCursor = $state('');
  let categoryFilter = $state('');
  let unreadOnly = $state(false);

  const categories = [
    { value: '', label: 'All' },
    { value: 'proposals', label: 'Proposals' },
    { value: 'governance', label: 'Governance' },
    { value: 'membership', label: 'Membership' },
    { value: 'events', label: 'Events' },
  ];

  $effect(() => {
    loadNotifications();
  });

  async function loadNotifications(append = false) {
    if (!append) loading = true;
    try {
      let params = '?limit=20';
      if (categoryFilter) params += `&category=${categoryFilter}`;
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

  function setFilter(cat) {
    categoryFilter = cat;
    loadNotifications();
  }

  function toggleUnread() {
    unreadOnly = !unreadOnly;
    loadNotifications();
  }

  async function markAllRead() {
    await api('notifications/read-all', { method: 'POST' });
    notifications = notifications.map(n => ({ ...n, read_at: n.read_at || new Date().toISOString() }));
  }

  async function clickNotif(notif) {
    if (!notif.read_at) {
      try {
        await api(`notifications/${notif.id}/read`, { method: 'PATCH' });
        notif.read_at = new Date().toISOString();
        notifications = [...notifications];
      } catch {}
    }
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
    const days = Math.floor(hrs / 24);
    if (days < 30) return `${days}d ago`;
    return new Date(iso).toLocaleDateString();
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <div class="notif-page-header">
      <h1>Notifications</h1>
      {#if notifications.some(n => !n.read_at)}
        <button class="btn btn-secondary btn-sm" onclick={markAllRead}>Mark all read</button>
      {/if}
    </div>

    <div class="filters">
      {#each categories as cat}
        <button
          class="chip"
          class:selected={categoryFilter === cat.value}
          onclick={() => setFilter(cat.value)}
        >{cat.label}</button>
      {/each}
      <button
        class="chip"
        class:selected={unreadOnly}
        onclick={toggleUnread}
      >Unread only</button>
    </div>

    {#if loading}
      <p class="muted center-text">Loading...</p>
    {:else if notifications.length === 0}
      <div class="empty-state">
        <p>You're all caught up</p>
        <p class="muted">No {categoryFilter ? categoryFilter : ''} notifications{unreadOnly ? ' (unread)' : ''}.</p>
      </div>
    {:else}
      <div class="notif-list">
        {#each notifications as notif (notif.id)}
          <button
            class="notif-item"
            class:unread={!notif.read_at}
            onclick={() => clickNotif(notif)}
          >
            <span class="notif-icon"><NotifIcon type={notif.type} /></span>
            <div class="notif-content">
              <div class="notif-title">{notif.title}</div>
              {#if notif.body}
                <div class="notif-body">{notif.body}</div>
              {/if}
            </div>
            <span class="notif-time">{timeAgo(notif.created_at)}</span>
          </button>
        {/each}
      </div>

      {#if nextCursor}
        <div class="load-more">
          <button class="btn btn-secondary btn-sm" onclick={() => loadNotifications(true)}>Load more</button>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .notif-page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0 0 1rem;
  }

  .notif-page-header h1 {
    font-size: 1.3rem;
  }

  .filters {
    display: flex;
    gap: 0.4rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }

  .chip {
    padding: 0.25rem 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    font-size: 0.8rem;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .chip:hover { border-color: var(--color-primary); }
  .chip.selected {
    background: var(--color-primary);
    border-color: var(--color-primary);
    color: var(--color-btn-on-primary);
  }

  .notif-list {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .notif-item {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    padding: 0.75rem;
    border: none;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-surface);
    cursor: pointer;
    text-align: left;
    width: 100%;
    font-size: 0.85rem;
  }

  .notif-item:last-child { border-bottom: none; }
  .notif-item:hover { background: var(--color-bg); }

  .notif-item.unread {
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
  }
  .notif-item.unread:hover {
    background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface));
  }

  .notif-icon { font-size: 1rem; flex-shrink: 0; margin-top: 0.1rem; }

  .notif-content { flex: 1; min-width: 0; }
  .notif-title { font-weight: 500; color: var(--color-text); margin-bottom: 0.1rem; }
  .notif-body {
    color: var(--color-text-muted);
    font-size: 0.8rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notif-time {
    color: var(--color-text-muted);
    font-size: 0.72rem;
    flex-shrink: 0;
    white-space: nowrap;
  }

  .empty-state {
    text-align: center;
    padding: 3rem 1rem;
  }
  .empty-state p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .center-text { text-align: center; padding: 2rem 0; }

  .load-more {
    text-align: center;
    padding: 1rem 0;
  }
</style>
