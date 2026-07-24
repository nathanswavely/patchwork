<script>
  import { CalendarBlank, CaretDown } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getSelectedTags, getSearchQuery, resetFilters } from '../stores/quilt.svelte.js';
  import FilterChips from '../components/FilterChips.svelte';
  import { getRemoteFollows } from '../stores/multiQuilt.svelte.js';
  import { sortByDate } from '../lib/multiQuilt.js';
  import { textMatches } from '../lib/textMatch.js';

  let { quiltScope = 'local' } = $props();

  let allEvents = $state([]);
  let loading = $state(true);
  let listCursor = $state('');
  let hasMore = $state(false);
  let patchMap = $state(new Map());

  // Date filter state
  let dateFilterOpen = $state(false);
  let datePreset = $state('any'); // 'any','today','tomorrow','weekend','week','nextweek','month','custom'
  let customFrom = $state('');
  let customTo = $state('');
  let showCustomPicker = $state(false);

  // Compute date range from preset
  function getDateRange(preset) {
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const fmt = (d) => d.toISOString().slice(0, 10);

    switch (preset) {
      case 'today':
        return { from: fmt(today), to: fmt(today) };
      case 'tomorrow': {
        const tom = new Date(today); tom.setDate(tom.getDate() + 1);
        return { from: fmt(tom), to: fmt(tom) };
      }
      case 'weekend': {
        const day = today.getDay();
        const sat = new Date(today); sat.setDate(sat.getDate() + (6 - day));
        const sun = new Date(sat); sun.setDate(sun.getDate() + 1);
        return { from: fmt(sat), to: fmt(sun) };
      }
      case 'week': {
        const end = new Date(today); end.setDate(end.getDate() + (6 - today.getDay()));
        return { from: fmt(today), to: fmt(end) };
      }
      case 'nextweek': {
        const day = today.getDay();
        const nextMon = new Date(today); nextMon.setDate(nextMon.getDate() + (8 - day));
        const nextSun = new Date(nextMon); nextSun.setDate(nextSun.getDate() + 6);
        return { from: fmt(nextMon), to: fmt(nextSun) };
      }
      case 'month': {
        const end = new Date(today.getFullYear(), today.getMonth() + 1, 0);
        return { from: fmt(today), to: fmt(end) };
      }
      case 'custom':
        return { from: customFrom || fmt(today), to: customTo || '' };
      default: // 'any'
        return { from: fmt(today), to: '' };
    }
  }

  let dateRange = $derived(getDateRange(datePreset));

  let dateLabel = $derived.by(() => {
    if (datePreset === 'any') return 'Any date';
    if (datePreset === 'today') return 'Today';
    if (datePreset === 'tomorrow') return 'Tomorrow';
    if (datePreset === 'weekend') return 'This weekend';
    if (datePreset === 'week') return 'This week';
    if (datePreset === 'nextweek') return 'Next week';
    if (datePreset === 'month') return 'This month';
    if (datePreset === 'custom') {
      if (customFrom && customTo) return `${customFrom} – ${customTo}`;
      if (customFrom) return `From ${customFrom}`;
      return 'Custom range';
    }
    return 'Any date';
  });

  function selectPreset(preset) {
    datePreset = preset;
    showCustomPicker = false;
    dateFilterOpen = false;
    loadData();
  }

  function applyCustomRange() {
    datePreset = 'custom';
    showCustomPicker = false;
    dateFilterOpen = false;
    loadData();
  }

  function handleWindowClick(e) {
    if (dateFilterOpen && !e.target.closest('.date-filter')) {
      dateFilterOpen = false;
      showCustomPicker = false;
    }
  }

  $effect(() => {
    void quiltScope;
    void getRemoteFollows().length;
    loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const { from, to } = dateRange;
      let params = `?from=${from}&limit=50`;
      if (to) params += `&to=${to}`;

      if (patchMap.size === 0) {
        const treeResp = await api('nodes/tree');
        const tree = treeResp.tree || treeResp;
        const map = new Map();
        for (const child of (tree.children || [])) {
          map.set(child.id, child);
        }
        patchMap = map;
      }

      const data = await api(`events${params}`);
      let events = data.items || [];
      listCursor = data.next_cursor || '';
      hasMore = !!data.next_cursor;

      // My Quilt merges events from remote followed patches, date-sorted,
      // each carrying its source quilt (docs/adr/024). Unreachable quilts
      // simply contribute nothing.
      if (quiltScope === 'my') {
        const follows = getRemoteFollows();
        const byQuilt = new Map();
        for (const f of follows) {
          if (!byQuilt.has(f.quilt_url)) byQuilt.set(f.quilt_url, new Map());
          byQuilt.get(f.quilt_url).set(f.node_slug, f);
        }
        const remoteBatches = await Promise.all(
          [...byQuilt.entries()].map(async ([url, slugMap]) => {
            try {
              const res = await fetch(`${url}/api/v1/events${params}`);
              if (!res.ok) return [];
              const rd = await res.json();
              return (rd.items || [])
                .filter((e) => slugMap.has(e.node_slug))
                .map((e) => ({
                  ...e,
                  _source: url,
                  _tags: slugMap.get(e.node_slug)?.snapshot?.tags || [],
                }));
            } catch {
              return [];
            }
          })
        );
        events = sortByDate([...events, ...remoteBatches.flat()], 'starts_at');
        // The merged feed reads soonest-first like the local list.
        events.reverse();
      }
      allEvents = events;
    } catch {
      allEvents = [];
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!listCursor) return;
    try {
      const { from, to } = dateRange;
      let params = `?from=${from}&limit=50&after=${encodeURIComponent(listCursor)}`;
      if (to) params += `&to=${to}`;
      const data = await api(`events${params}`);
      allEvents = [...allEvents, ...(data.items || [])];
      listCursor = data.next_cursor || '';
      hasMore = !!data.next_cursor;
    } catch {}
  }

  let filtered = $derived.by(() => {
    const tags = getSelectedTags();
    const query = getSearchQuery();
    if (tags.length === 0 && !query.trim()) return allEvents;

    const visiblePatchIds = new Set();
    for (const [id, patch] of patchMap) {
      let matches = true;
      if (tags.length > 0) {
        matches = (patch.tags || []).some(t => tags.includes(t));
      }
      if (matches && query.trim()) {
        matches = textMatches(patch.name, query) || textMatches(patch.description, query);
      }
      if (matches) visiblePatchIds.add(id);
    }
    return allEvents.filter(e => {
      // Remote events carry their own tag/name context — their patches
      // aren't in this quilt's tree.
      if (e._source) {
        let matches = true;
        if (tags.length > 0) matches = (e._tags || []).some(t => tags.includes(t));
        if (matches && query.trim()) {
          matches = textMatches(e.title, query) || textMatches(e.node_name, query);
        }
        return matches;
      }
      return visiblePatchIds.has(e.node_id);
    });
  });

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }

  function formatTime(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  }

  const presets = [
    { id: 'any', label: 'Any date' },
    { id: 'today', label: 'Today' },
    { id: 'tomorrow', label: 'Tomorrow' },
    { id: 'weekend', label: 'This weekend' },
    { id: 'week', label: 'This week' },
    { id: 'nextweek', label: 'Next week' },
    { id: 'month', label: 'This month' },
  ];
</script>

<svelte:window onclick={handleWindowClick} />

<div class="events-page">
  <div class="events-header">
    <h1>Events</h1>
  </div>

  <!-- The filter chips: the page's own top-of-flow home (docs/adr/033). -->
  <FilterChips variant="flow" />

  <!-- Date filter -->
  <div class="date-filter">
    <button class="date-filter-btn" class:active={datePreset !== 'any'} onclick={() => { dateFilterOpen = !dateFilterOpen; showCustomPicker = false; }}>
      <CalendarBlank size={14} weight="duotone" />
      {dateLabel}
      <span class="chevron" class:open={dateFilterOpen}><CaretDown size={10} weight="bold" /></span>
    </button>

    {#if dateFilterOpen}
      <div class="date-dropdown" onclick={(e) => e.stopPropagation()}>
        {#if showCustomPicker}
          <!-- Custom date range picker -->
          <button class="dropdown-back" onclick={() => { showCustomPicker = false; }}>
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" class="back-caret">
              <path d="M7.5 2L3.5 6l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
            Custom date range
          </button>
          <div class="custom-range">
            <label class="range-field">
              <span>From</span>
              <input type="date" bind:value={customFrom} />
            </label>
            <label class="range-field">
              <span>To</span>
              <input type="date" bind:value={customTo} />
            </label>
            <button class="apply-btn" onclick={applyCustomRange} disabled={!customFrom}>Apply</button>
          </div>
        {:else}
          <!-- Preset list -->
          {#each presets as preset (preset.id)}
            <button class="dropdown-option" class:active={datePreset === preset.id} onclick={() => selectPreset(preset.id)}>
              <span>{preset.label}</span>
              <div class="radio-dot" class:checked={datePreset === preset.id}></div>
            </button>
          {/each}
          <div class="dropdown-divider"></div>
          <button class="dropdown-option custom-option" onclick={() => { showCustomPicker = true; }}>
            <span>Custom date range</span>
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
              <path d="M4.5 2l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </button>
        {/if}
      </div>
    {/if}
  </div>

  {#if loading}
    <div class="events-empty">
      <p class="muted">Loading events...</p>
    </div>
  {:else if filtered.length === 0}
    <div class="events-empty">
      {#if getSelectedTags().length > 0 || getSearchQuery().trim()}
        <!-- The filter is standing state (docs/adr/033) — when it empties
             the list, say so and offer the way out. -->
        <p class="muted">
          No events match your filter{datePreset !== 'any' ? ' for this date range' : ''}.
        </p>
        <button class="btn btn-secondary" onclick={resetFilters}>Clear filter</button>
      {:else}
        <p class="muted">No upcoming events{datePreset !== 'any' ? ' for this date range' : ''}.</p>
      {/if}
    </div>
  {:else}
    <div class="event-list">
      {#each filtered as event (`${event._source || 'local'}:${event.id}`)}
        {@const remoteTarget = event._source
          ? `/quilts/${event._source.replace(/^https?:\/\//, '')}/patches/${event.node_slug}` : null}
        <a
          href={remoteTarget || `/events/${event.id}`}
          class="event-row"
          onclick={(e) => {
            e.preventDefault();
            navigate(remoteTarget || `/events/${event.id}`);
          }}
        >
          <div class="event-date-col">
            <span class="event-date-label">{formatDate(event.starts_at)}</span>
            <span class="event-time-label">{formatTime(event.starts_at)}</span>
          </div>
          <div class="event-info">
            <div class="event-title">{event.title}</div>
            {#if event.location}
              <div class="event-location">{event.location}</div>
            {/if}
            {#if event.node_name}
              <div class="event-patch">
                {event.node_name}
                {#if event._source}
                  <span class="event-source-chip">{event._source.replace(/^https?:\/\//, '')}</span>
                {/if}
                {#if event.node_status === 'unclaimed'}
                  <!-- Events on unclaimed patches wear the community-submitted
                       label away from their patch (docs/adr/026). -->
                  <span class="event-source-chip">Community-submitted</span>
                {/if}
              </div>
            {/if}
          </div>
        </a>
      {/each}
    </div>

    {#if hasMore}
      <button class="load-more" onclick={loadMore}>Load more events</button>
    {/if}
  {/if}
</div>

<style>
  .events-page {
    max-width: 680px;
    margin: 0 auto;
    /* Padding comes from SocialShell's .social-main container (issue #17). */
  }

  .events-header {
    margin-bottom: 1rem;
  }

  .event-source-chip {
    display: inline-block;
    margin-left: 6px;
    font-size: 0.68rem;
    font-weight: 700;
    padding: 0 7px;
    border-radius: 999px;
    border: 1px solid var(--color-border);
    color: var(--color-text-muted);
  }

  .events-header h1 {
    font-size: 1.5rem;
    font-weight: 700;
  }

  /* ================================================================
     DATE FILTER
     ================================================================ */
  .date-filter {
    position: relative;
    margin-bottom: 1.5rem;
  }

  .date-filter-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    transition: border-color 150ms ease;
  }

  .date-filter-btn:hover {
    border-color: var(--color-primary);
  }

  .date-filter-btn.active {
    border-color: var(--color-primary);
    color: var(--color-primary);
  }

  .chevron {
    transition: transform 150ms ease;
  }

  .chevron.open {
    transform: rotate(180deg);
  }

  .date-dropdown {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 4px 20px var(--color-shadow);
    min-width: 260px;
    z-index: 100;
    overflow: hidden;
    padding: 4px;
  }

  .dropdown-option {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 10px 12px;
    border: none;
    background: none;
    color: var(--color-text);
    font-size: 0.88rem;
    cursor: pointer;
    text-align: left;
    border-radius: 4px;
    transition: background 100ms ease;
  }

  .dropdown-option:hover {
    background: var(--color-overlay);
  }

  .dropdown-option.active {
    font-weight: 600;
  }

  .radio-dot {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 2px solid var(--color-border);
    flex-shrink: 0;
    transition: border-color 150ms ease;
  }

  .radio-dot.checked {
    border-color: var(--color-primary);
    border-width: 5px;
  }

  .custom-option svg {
    color: var(--color-text-muted);
  }

  .dropdown-divider {
    height: 1px;
    background: var(--color-border);
    margin: 4px 8px;
  }

  .dropdown-back {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 10px 12px;
    border: none;
    border-bottom: 1px solid var(--color-border);
    background: none;
    color: var(--color-text);
    font-size: 0.88rem;
    font-weight: 600;
    cursor: pointer;
    text-align: left;
    margin-bottom: 4px;
  }

  .dropdown-back:hover {
    color: var(--color-primary);
  }

  .custom-range {
    padding: 8px 12px 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .range-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .range-field span {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .range-field input {
    padding: 8px 10px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg);
    color: var(--color-text);
    font-size: 0.85rem;
  }

  .apply-btn {
    padding: 8px 16px;
    border: none;
    border-radius: var(--radius);
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 150ms ease;
  }

  .apply-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .apply-btn:not(:disabled):hover {
    opacity: 0.9;
  }

  /* ================================================================
     EVENT LIST
     ================================================================ */
  .events-empty {
    text-align: center;
    padding: 3rem 0;
  }

  .event-list {
    display: flex;
    flex-direction: column;
  }

  .event-row {
    display: flex;
    gap: 1rem;
    padding: 0.75rem 0.5rem;
    text-decoration: none;
    color: var(--color-text);
    border-bottom: 1px solid var(--color-border);
    border-radius: var(--radius);
    transition: background 100ms ease;
  }

  .event-row:last-child {
    border-bottom: none;
  }

  .event-row:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .event-date-col {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    min-width: 6rem;
    flex-shrink: 0;
  }

  .event-date-label {
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--color-primary);
  }

  .event-time-label {
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .event-info {
    flex: 1;
    min-width: 0;
  }

  .event-title {
    font-size: 0.92rem;
    font-weight: 600;
    margin-bottom: 2px;
  }

  .event-location {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .event-patch {
    font-size: 0.75rem;
    color: var(--color-primary);
    margin-top: 2px;
  }

  .load-more {
    display: block;
    width: 100%;
    padding: 0.6rem;
    margin-top: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: none;
    color: var(--color-text-muted);
    font-size: 0.85rem;
    cursor: pointer;
    transition: border-color 150ms ease, color 150ms ease;
  }

  .load-more:hover {
    border-color: var(--color-primary);
    color: var(--color-primary);
  }
</style>
