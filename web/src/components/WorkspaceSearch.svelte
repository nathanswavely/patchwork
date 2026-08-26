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
   *    state rather than a navigation. Hidden on a zero-result query:
   *    narrowing to a provably empty set is a dead action.
   *  - suggestLabel/onSuggest: a navigation row shown only when a query
   *    matches nothing — the moment someone learns their group isn't here.
   *    It leaves for the submission form like any result and never sets the
   *    search chip, so the action row above still owns narrowing alone.
   *  - intercept: sees each input value first (pasted patch URLs); returning
   *    true consumes it and closes the dropdown.
   *  - variant="takeover": renders inside the mobile search takeover, where
   *    the default desktop-only media hide must not apply.
   *
   * Picker extras, for callers choosing a thing rather than going to it:
   *  - onSelect: replaces navigation — the chosen item is handed back
   *    instead of being followed. An item may also carry disabled:true,
   *    which shows it and refuses it (a patch that isn't accepting event
   *    suggestions is worth seeing; silence would read as "not here").
   *  - alwaysSuggest: keeps the bottom row on screen even when results
   *    exist. Discovery deliberately hides it until a query matches
   *    nothing — there, it is how someone learns their group isn't here —
   *    but a picker whose whole job is choosing a patch should always
   *    offer making one.
   *  - variant="picker": a form field in a page rather than the bar's
   *    search — rectangular, no magnifying glass, no '/' badge, and a
   *    dropdown free to be wider than the field it hangs off. The bar's
   *    posture assumes a 420px field: pinned to a narrow one, the flex
   *    row collapses every result's name to an ellipsis and leaves only
   *    its description showing.
   *  - matchField: pins the dropdown to the field's own width instead of
   *    letting it grow past one edge. The widening above assumes free room
   *    beside the field; inside a scrolling box — a modal, a panel — there
   *    is none, and the overflow is clipped rather than overlaid, because
   *    `overflow-y: auto` computes `overflow-x` to `auto` too. Pass it
   *    wherever an ancestor scrolls, and give the field the room instead.
   *  - inputId: puts an id on the field so a form's <label for> can name it.
   *    The bar's search is its own landmark and needs none; a picker sitting
   *    in a labelled form row is a form control like any other.
   *  - shortcut={false}: gives up the '/' focus key. Several pickers on a
   *    page would otherwise all bind it and fight over the global bar's.
   *    Implied by variant="picker" — a slash shortcut into one of nine
   *    fields on a page means nothing.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { fold } from '../lib/textMatch.js';
  import { MagnifyingGlass } from 'phosphor-svelte';

  let {
    placeholder = 'Search…',
    provider,
    actionLabel = null,
    onAction = null,
    suggestLabel = null,
    onSuggest = null,
    intercept = null,
    variant = 'bar',
    autofocus = false,
    onSelect = null,
    alwaysSuggest = false,
    shortcut = true,
    matchField = false,
    inputId = null,
  } = $props();

  let isPicker = $derived(variant === 'picker');

  let query = $state('');
  let open = $state(false);
  let loading = $state(false);
  let items = $state(null); // null = not fetched yet
  let activeIndex = $state(-1);
  let inputEl = $state(null);

  // When a shell mounts this in response to a tap (the mobile search
  // takeover), that tap is still propagating toward window while Svelte
  // flushes us in — so the outside-click listener below sees the very
  // gesture that opened us and closes the panel autofocus just opened. The
  // field keeps focus, so it reads as "typing does nothing until I tap the
  // field again". Anything that started before we existed isn't ours.
  const mountedAt = performance.now();

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

  // The bottom row navigates with the keyboard like any result, one slot
  // past the last. Exactly one can ever show: with results, the action row
  // narrows; with none, the suggest row offers the only useful move left.
  // Both gate on their own callback so workspace/admin callers are untouched.
  let hasSuggest = $derived(
    !!onSuggest && !!query.trim() && !loading && (alwaysSuggest || results.length === 0),
  );
  let hasAction = $derived(!!onAction && !!query.trim() && results.length > 0);
  let navLength = $derived(results.length + (hasAction || hasSuggest ? 1 : 0));

  function runAction() {
    const q = query.trim();
    open = false;
    query = '';
    onAction?.(q);
  }

  function runSuggest() {
    const q = query.trim();
    open = false;
    query = '';
    onSuggest?.(q);
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
    if (item.disabled) return;
    open = false;
    query = '';
    if (onSelect) {
      onSelect(item);
      return;
    }
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
      else if (activeIndex === results.length) {
        if (hasAction) runAction();
        else if (hasSuggest) runSuggest();
      }
    }
  }

  // Preselect the first result so Enter takes the obvious match — but never
  // preselect the bottom row. On a zero-result query it is the only row, so
  // preselecting it turned Enter (the key people press to submit a search)
  // into a silent departure for the submission form. Arrow to it or tap it.
  $effect(() => {
    void query;
    activeIndex = results.length > 0 ? 0 : -1;
  });

  $effect(() => {
    if (autofocus && inputEl) inputEl.focus();
  });

  function onWindowKeydown(e) {
    if (!shortcut || isPicker) return;
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const tag = document.activeElement?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || document.activeElement?.isContentEditable) return;
      e.preventDefault();
      inputEl?.focus();
    }
  }

  function onWindowClick(e) {
    if (e.timeStamp < mountedAt) return;
    if (open && !e.target.closest('.finder')) open = false;
  }
</script>

<svelte:window onkeydown={onWindowKeydown} onclick={onWindowClick} />

<div
  class="finder"
  class:finder-takeover={variant === 'takeover'}
  class:finder-picker={isPicker}
  class:finder-match-field={matchField}
>
  {#if !isPicker}
    <span class="finder-icon"><MagnifyingGlass size={15} weight="duotone" /></span>
  {/if}
  <input
    bind:this={inputEl}
    id={inputId}
    class="finder-input"
    type="search"
    {placeholder}
    bind:value={query}
    oninput={onInput}
    onfocus={onFocus}
    onclick={onFocus}
    onkeydown={onKeydown}
  />
  {#if variant !== 'takeover' && !isPicker}
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
                class:finder-item-disabled={item.disabled}
                disabled={item.disabled}
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
        {:else if hasSuggest}
          <button
            class="finder-item finder-action"
            class:active={activeIndex === results.length}
            onclick={runSuggest}
          >
            <span class="finder-item-label">{suggestLabel ? suggestLabel(query.trim()) : `Suggest “${query.trim()}” as a patch`}</span>
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

  /* Shown but refused: a patch that isn't accepting suggestions is worth
     seeing, and hiding it would read as "not on this quilt". */
  .finder-item-disabled {
    cursor: default;
    opacity: 0.55;
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

  /* Picker variant: an ordinary form field, and a dropdown that sizes to
     its content rather than to the field. */
  .finder.finder-picker {
    max-width: none;
    height: auto;
    padding: 0;
    border: none;
    background: none;
    border-radius: 0;
  }

  .finder-picker .finder-input {
    width: 100%;
    height: 34px;
    padding: 0 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg);
    color: var(--color-text);
  }

  /* Right-anchored, so a dropdown wider than its field grows inward
     rather than off the page — a picker sits at the end of its row, and
     the room is always to its left. */
  .finder-picker .finder-results {
    left: auto;
    right: 0;
    min-width: min(26rem, 90vw);
    max-width: calc(100vw - 2rem);
  }

  /* Except where an ancestor scrolls. A scrolling box clips on both axes —
     `overflow-y: auto` computes `overflow-x` to `auto` — so a dropdown
     reaching past the field is cut off rather than drawn over, and the box
     grows a horizontal scrollbar. Pinned to the field, it cannot reach. */
  .finder-picker.finder-match-field .finder-results {
    left: 0;
    right: 0;
    min-width: 0;
    max-width: none;
  }

  /* Stacked, so a name is never the half that ellipsises away. */
  .finder-picker .finder-item {
    flex-direction: column;
    align-items: stretch;
    gap: 1px;
  }

  .finder-picker .finder-item-sub {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Mobile: the bar's search hides — the shelf's search button opens the
     takeover, which hosts the takeover variant instead. A picker is a
     field inside a page, with no takeover to fall back to, so it stays. */
  @media (max-width: 768px) {
    .finder:not(.finder-takeover):not(.finder-picker) {
      display: none;
    }

    /* Narrow: the field is already as wide as the row, so the dropdown
       simply matches it. Nothing to grow into, and a wider panel would
       only be a panel hanging off the screen. */
    .finder-picker .finder-results {
      left: 0;
      right: 0;
      min-width: 0;
      max-width: none;
    }
  }
</style>
