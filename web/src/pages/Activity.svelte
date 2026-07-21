<script>
  import NotifIcon from '../components/NotifIcon.svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';

  let items = $state([]);
  let loading = $state(true);
  let nextCursor = $state('');

  $effect(() => {
    loadActivity();
  });

  async function loadActivity(append = false) {
    if (!append) loading = true;
    try {
      let params = '?limit=30';
      if (append && nextCursor) params += `&after=${nextCursor}`;
      const data = await api(`activity${params}`);
      const newItems = data.items || [];
      items = append ? [...items, ...newItems] : newItems;
      nextCursor = data.next_cursor || '';
    } catch {
      if (!append) items = [];
    } finally {
      loading = false;
    }
  }

  function typeLabel(type) {
    const labels = { proposal: 'Proposal', event: 'Event', governance: 'Document', membership: 'Membership' };
    return labels[type] || type;
  }

  // Group items by day.
  let grouped = $derived.by(() => {
    const groups = [];
    let currentDay = '';
    for (const item of items) {
      const day = item.created_at?.substring(0, 10) || '';
      if (day !== currentDay) {
        currentDay = day;
        groups.push({ day, label: formatDay(day), items: [] });
      }
      groups[groups.length - 1].items.push(item);
    }
    return groups;
  });

  function formatDay(dateStr) {
    if (!dateStr) return '';
    const d = new Date(dateStr + 'T00:00:00');
    const now = new Date();
    const today = now.toISOString().substring(0, 10);
    const yesterday = new Date(now - 86400000).toISOString().substring(0, 10);
    if (dateStr === today) return 'Today';
    if (dateStr === yesterday) return 'Yesterday';
    return d.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
  }

  function timeOnly(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <div class="activity-header">
      <h1>What's New</h1>
    </div>

    {#if loading}
      <p class="muted center-text">Loading...</p>
    {:else if items.length === 0}
      <div class="empty-state">
        <p>Nothing new yet</p>
        <p class="muted">Activity from your patches will show up here.</p>
      </div>
    {:else}
      {#each grouped as group}
        <div class="day-group">
          <div class="day-label">{group.label}</div>
          <div class="day-items">
            {#each group.items as item}
              <a
                href={item.link}
                class="activity-item"
                onclick={(e) => { e.preventDefault(); navigate(item.link); }}
              >
                <span class="item-icon"><NotifIcon type={`${item.type}.`} size={16} /></span>
                <div class="item-content">
                  <div class="item-title">{item.title}</div>
                  <div class="item-meta">
                    <span class="badge badge-type">{typeLabel(item.type)}</span>
                    <span class="muted">{item.patch_name}</span>
                    {#if item.actor_name}
                      <span class="muted">by {item.actor_name}</span>
                    {/if}
                  </div>
                </div>
                <span class="item-time muted">{timeOnly(item.created_at)}</span>
              </a>
            {/each}
          </div>
        </div>
      {/each}

      {#if nextCursor}
        <div class="load-more">
          <button class="btn btn-secondary btn-sm" onclick={() => loadActivity(true)}>Load more</button>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .activity-header {
    padding: 0 0 1rem;
  }

  .activity-header h1 {
    font-size: 1.3rem;
  }

  .day-group {
    margin-bottom: 1.5rem;
  }

  .day-label {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: 0.03em;
    margin-bottom: 0.5rem;
    padding-bottom: 0.25rem;
    border-bottom: 1px solid var(--color-border);
  }

  .day-items {
    display: flex;
    flex-direction: column;
  }

  .activity-item {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    padding: 0.6rem 0.5rem;
    text-decoration: none;
    color: inherit;
    border-radius: var(--radius);
    transition: background 100ms ease;
  }

  .activity-item:hover {
    background: var(--color-bg);
  }

  .item-icon {
    font-size: 1rem;
    flex-shrink: 0;
    margin-top: 0.15rem;
  }

  .item-content {
    flex: 1;
    min-width: 0;
  }

  .item-title {
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--color-text);
    margin-bottom: 0.15rem;
  }

  .item-meta {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    font-size: 0.78rem;
    flex-wrap: wrap;
  }

  .badge-type {
    font-size: 0.68rem;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
    color: var(--color-primary);
    font-weight: 500;
  }

  .item-time {
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
  .load-more { text-align: center; padding: 1rem 0; }
</style>
