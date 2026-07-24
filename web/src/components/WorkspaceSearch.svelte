<script>
  /**
   * The search (docs/adr/033): the global bar's contextual autocomplete,
   * one posture in every context — workspace, admin panel, discovery.
   * Lazy: nothing is fetched until the field is first focused; one fetch per
   * visit, filtered in memory, results grouped by type. `/` focuses the
   * field from anywhere (unless typing elsewhere).
   *
   * Discovery extras, all optional:
   *  - actionLabel/onAction: one final row after the results ("Show matches
   *    on the quilt") — the only place a query becomes standing narrowing
   *    state rather than a navigation.
   *  - intercept: sees each input value first (pasted patch URLs); returning
   *    true consumes it and closes the dropdown.
   *  - variant="takeover": renders inside the mobile search takeover, where
   *    the default desktop-only media hide must not apply.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { fold } from '../lib/textMatch.js';
  import { MagnifyingGlass } from 'phosphor-svelte';

  let {
    placeholder = 'Search…',
    provider,
    actionLabel = null,
    onAction = null,
    intercept = null,
    variant = 'bar',
    autofocus = false,
  } = $props();

  let query = $state('');
  let open = $state(false);
  let loading = $state(false);
  let items = $state(null); // null = not fetched yet
  let activeIndex = $state(-1);
  let inputEl = $state(null);

  async function ensureLoaded() {
    if (items !== null || loading) return;
    loading = true;
    try {
      items = await provider();
    } catch {
      items = [];
    }
    loading = false;
  }

  let results = $derived.by(() => {
    if (!items || !query.trim()) return [];
    // Diacritic-folded match: "tornado" finds Tornādo Tornädo.
    const q = fold(query);
    return items.filter(i =>
      fold(i.label).includes(q) || fold(i.sublabel).includes(q)
    ).slice(0, 12);
  });

  // The action row navigates with the keyboard like any result, one slot
  // past the last. hasAction gates every mention so workspace/admin callers
  // are untouched.
  let hasAction = $derived(!!onAction && !!query.trim());
  let navLength = $derived(results.length + (hasAction ? 1 : 0));

  function runAction() {
    const q = query.trim();
    open = false;
    query = '';
    onAction?.(q);
  }

  function onInput() {
    if (intercept?.(query)) {
      query = '';
      open = false;
    }
  }

  // Grouped for display, preserving result order within groups.
  let grouped = $derived.by(() => {
    const groups = new Map();
    for (const r of results) {
      if (!groups.has(r.type)) groups.set(r.type, []);
      groups.get(r.type).push(r);
    }
    return [...groups.entries()];
  });

  function select(item) {
    open = false;
    query = '';
    navigate(item.href);
  }

  function onFocus() {
    open = true;
    ensureLoaded();
  }

  function onKeydown(e) {
    if (e.key === 'Escape') {
      open = false;
      inputEl?.blur();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = Math.min(activeIndex + 1, navLength - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
    } else if (e.key === 'Enter' && activeIndex >= 0) {
      e.preventDefault();
      if (results[activeIndex]) select(results[activeIndex]);
      else if (hasAction && activeIndex === results.length) runAction();
    }
  }

  $effect(() => {
    void query;
    activeIndex = navLength > 0 ? 0 : -1;
  });

  $effect(() => {
    if (autofocus && inputEl) inputEl.focus();
  });

  function onWindowKeydown(e) {
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const tag = document.activeElement?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || document.activeElement?.isContentEditable) return;
      e.preventDefault();
      inputEl?.focus();
    }
  }

  function onWindowClick(e) {
    if (open && !e.target.closest('.finder')) open = false;
  }
</script>

<svelte:window onkeydown={onWindowKeydown} onclick={onWindowClick} />

<div class="finder" class:finder-takeover={variant === 'takeover'}>
  <span class="finder-icon"><MagnifyingGlass size={15} weight="duotone" /></span>
  <input
    bind:this={inputEl}
    class="finder-input"
    type="search"
    {placeholder}
    bind:value={query}
    oninput={onInput}
    onfocus={onFocus}
    onclick={onFocus}
    onkeydown={onKeydown}
  />
  {#if variant !== 'takeover'}
    <kbd class="finder-kbd">/</kbd>
  {/if}

  {#if open && query.trim()}
    <div class="finder-results">
      {#if loading}
        <div class="finder-empty">Searching…</div>
      {:else}
        {#if results.length === 0}
          <div class="finder-empty">No matches in this context</div>
        {:else}
          {#each grouped as [type, group] (type)}
            <div class="finder-group-label">{type}</div>
            {#each group as item (item.href + item.label)}
              <button
                class="finder-item"
                class:active={results.indexOf(item) === activeIndex}
                onclick={() => select(item)}
              >
                <span class="finder-item-label">{item.label}</span>
                {#if item.sublabel}
                  <span class="finder-item-sub">{item.sublabel}</span>
                {/if}
              </button>
            {/each}
          {/each}
        {/if}
        {#if hasAction}
          <button
            class="finder-item finder-action"
            class:active={activeIndex === results.length}
            onclick={runAction}
          >
            <span class="finder-item-label">{actionLabel ? actionLabel(query.trim()) : `Show matches for “${query.trim()}”`}</span>
          </button>
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .finder {
    position: relative;
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    max-width: 420px;
    height: 36px;
    padding: 0 12px;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-bg);
    color: var(--color-text-muted);
    transition: border-color 150ms ease;
  }

  .finder:focus-within {
    border-color: var(--color-primary);
  }

  .finder-icon {
    display: flex;
    flex-shrink: 0;
  }

  .finder-input {
    flex: 1;
    min-width: 0;
    border: none;
    background: none;
    padding: 0;
    font-size: 0.88rem;
    color: var(--color-text);
    outline: none;
  }

  .finder-input::placeholder {
    color: var(--color-text-muted);
  }

  .finder-kbd {
    flex-shrink: 0;
    font-size: 0.68rem;
    font-family: var(--font-mono, monospace);
    color: var(--color-text-muted);
    border: 1px solid var(--color-border);
    border-radius: 4px;
    padding: 1px 5px;
    line-height: 1.3;
  }

  .finder:focus-within .finder-kbd {
    display: none;
  }

  .finder-results {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    right: 0;
    max-height: 380px;
    overflow-y: auto;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 6px 24px var(--color-shadow);
    z-index: 220;
    padding: 4px;
  }

  .finder-empty {
    padding: 0.9rem;
    text-align: center;
    font-size: 0.82rem;
    color: var(--color-text-muted);
  }

  .finder-group-label {
    padding: 6px 10px 2px;
    font-size: 0.68rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
  }

  .finder-item {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 7px 10px;
    border: none;
    background: none;
    border-radius: 4px;
    cursor: pointer;
    text-align: left;
    font-size: 0.86rem;
    color: var(--color-text);
  }

  .finder-item:hover,
  .finder-item.active {
    background: var(--color-overlay);
  }

  .finder-item-label {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .finder-item-sub {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    flex-shrink: 0;
  }

  /* The action row (docs/adr/033): set apart from the results above it. */
  .finder-action {
    border-top: 1px solid var(--color-border);
    border-radius: 0 0 4px 4px;
    margin-top: 4px;
    padding-top: 9px;
  }

  .finder-action .finder-item-label {
    color: var(--color-primary);
    font-weight: 600;
  }

  /* Takeover variant: fills the mobile search bar it renders inside. */
  .finder.finder-takeover {
    max-width: none;
  }

  /* Mobile: the bar's search hides — the shelf's search button opens the
     takeover, which hosts the takeover variant instead. */
  @media (max-width: 768px) {
    .finder:not(.finder-takeover) {
      display: none;
    }
  }
</style>
