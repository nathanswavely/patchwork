<script>
  /**
   * The filter chips (docs/adr/033): the filter's control and indicator as
   * one thing. The tag vocabulary as toggleable chips, the search chip
   * among them, living on the surfaces the filter narrows.
   *
   * Variants:
   *  - "overlay": floats over a canvas (quilt/map) — glass chips, no panel.
   *  - "flow":    sits in a page's content flow (events list).
   *  - "sheet":   the mobile canvas sheet body — every tag, with a field to
   *               find one (TagFinder); the open/closed state belongs to the
   *               caller (a sheet is open-while-using, never a preference —
   *               docs/adr/033).
   *
   * Overlay and flow share the one collapse preference from the quilt
   * store; collapsed they render a single badged funnel button. That is
   * the one show/hide control in both states: shown, it leads the row from
   * the same spot as a hide button, so the hand that hid the chips finds
   * them again.
   *
   * Overlay and flow show one row: the most-worn tags that fit, in usage
   * order, then "N more". A selected tag is always in the row, wherever it
   * ranks, so an active filter is never hidden behind the button.
   *
   * "N more" grows the row into a card: the card hangs from the same line
   * as the canvas's other cards, a find field takes the row's place, and
   * the row slides down whole with every other tag filling in after it.
   * Nothing in the first row moves relative to anything else, which is
   * what keeps the open from reading as a re-layout. The card is not
   * modal: the canvas narrowing beside it is how a person sees what a tag
   * does.
   */
  import { tick } from 'svelte';
  import { CaretDown, EyeSlash, FunnelSimple, MagnifyingGlass, X } from 'phosphor-svelte';
  import {
    getAllTags,
    getRankedTags,
    getSelectedTags,
    toggleTag,
    getSearchQuery,
    setSearchQuery,
    getChipsCollapsed,
    setChipsCollapsed,
    resetFilters,
    getActiveFilterCount,
  } from '../stores/quilt.svelte.js';
  import { colorForTag, textOnColor } from '../lib/quiltTheme.js';
  import { findTags } from '../lib/tagFind.js';
  import TagFinder from './TagFinder.svelte';

  let { variant = 'flow' } = $props();

  let collapsed = $derived(variant !== 'sheet' && getChipsCollapsed());
  let count = $derived(getActiveFilterCount());
  let query = $derived(getSearchQuery());

  // --- One row, then the rest (overlay and flow) ---
  const GAP = 6; // .chips-row gap

  // Widths come from a hidden copy of every chip, so the row can be planned
  // before anything in it is drawn. The row's own width is the budget; the
  // controls that always ride in it (search chip, Clear, the dismiss X) are
  // measured where they stand.
  let measuredRowWidth = $state(0);
  let rowWidth = $state(0);
  let chipWidths = $state({});
  let moreWidth = $state(0);
  let findButtonWidth = $state(0);
  let searchWidth = $state(0);
  let clearWidth = $state(0);
  let collapseWidth = $state(0);

  let ranked = $derived(getRankedTags());

  let rowTags = $derived.by(() => {
    if (!rowWidth) return ranked;
    const selected = new Set(getSelectedTags());
    const w = (t) => (chipWidths[t] || 0) + GAP;
    // A binding keeps its last width after its element leaves, so each
    // control counts only while it is actually in the row.
    const fixed = [
      query.trim() ? searchWidth : 0,
      count > 0 ? clearWidth : 0,
      collapseWidth,
    ]
      .filter(Boolean)
      .reduce((sum, x) => sum + x + GAP, 0);
    let budget = rowWidth + GAP - fixed;
    // Everything fits: the find button stands where "N more" would.
    if (ranked.reduce((sum, t) => sum + w(t), 0) <= budget - findButtonWidth - GAP) return ranked;

    budget -= moreWidth + GAP;
    for (const t of ranked) if (selected.has(t)) budget -= w(t);
    const keep = new Set(selected);
    // Strictly in rank order: a narrow tag further down never jumps a wide
    // one that missed, or the row would stop meaning "the most used".
    for (const t of ranked) {
      if (selected.has(t)) continue;
      if (w(t) > budget) break;
      keep.add(t);
      budget -= w(t);
    }
    return ranked.filter((t) => keep.has(t));
  });

  let hiddenCount = $derived(ranked.length - rowTags.length);

  // --- The open card ---
  let open = $state(false);
  let closing = false;
  // The order the card lists, fixed at the moment it opens: the row as it
  // stood, then everything else. Re-planning while open would let a chip in
  // the first row move under the pointer.
  let order = $state([]);
  let find = $state('');
  let shown = $derived(open ? findTags(order, find) : rowTags);
  let countLabel = $derived(
    find.trim()
      ? `${shown.length} of ${ranked.length}`
      : `${ranked.length} ${ranked.length === 1 ? 'tag' : 'tags'}`
  );

  // The row's width plans the closed row only. Frozen while open, so the
  // card's padding never re-plans the row the close animation slides back.
  $effect(() => {
    if (!open) rowWidth = measuredRowWidth;
  });

  // While the card is open the body leaves the flow; the root keeps the
  // closed row's height so nothing below it moves.
  let measuredBodyHeight = $state(0);
  let closedHeight = $state(0);
  $effect(() => {
    if (!open) closedHeight = measuredBodyHeight;
  });

  let rootEl = $state(null);
  let bodyEl = $state(null);
  let rowEl = $state(null);
  let cardEl = $state(null);
  let headEl = $state(null);
  let findEl = $state(null);
  let openerEl = $state(null);

  const EASE_OUT = 'cubic-bezier(0.2, 0.7, 0.2, 1)';
  const EASE_IN = 'cubic-bezier(0.4, 0, 0.8, 0.4)';
  // The card is revealed from its top edge down, never grown in height: a
  // clip costs no layout per frame. The 16px of outset keeps the shadow.
  const CLIP_OPEN = 'inset(-16px -16px -16px -16px)';
  const CLIP_SHUT = 'inset(-16px -16px 100% -16px)';

  function firstChip() {
    return rowEl?.querySelector('.lt-chip');
  }

  function reduceMotion() {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  }

  async function openCard() {
    if (open || closing) return;
    // The first chip, not the row's first child: closed, the funnel leads
    // the row, and the card has no funnel in it.
    const before = firstChip()?.getBoundingClientRect();
    order = [...rowTags, ...ranked.filter((t) => !rowTags.includes(t))];
    find = '';
    open = true;
    await tick();
    findEl?.focus({ preventScroll: true });
    if (reduceMotion()) return;
    cardEl?.animate([{ clipPath: CLIP_SHUT }, { clipPath: CLIP_OPEN }], { duration: 200, easing: EASE_OUT });
    headEl?.animate([{ opacity: 0, transform: 'translateY(-4px)' }, { opacity: 1, transform: 'none' }], {
      duration: 160, delay: 60, easing: EASE_OUT, fill: 'backwards',
    });
    // FLIP: the row is already where it lands; start it where it stood.
    const after = firstChip()?.getBoundingClientRect();
    if (before && after) {
      const dx = before.left - after.left;
      const dy = before.top - after.top;
      rowEl.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }], {
        duration: 200, easing: EASE_OUT,
      });
    }
  }

  async function closeCard({ refocus = false } = {}) {
    if (!open || closing) return;
    closing = true;
    if (!reduceMotion()) {
      // A close can land while the open is still playing. Each part closes
      // from where it is on screen, not from where it was headed, or an
      // early Escape would snap the card open before shutting it.
      const clipNow = cardEl ? getComputedStyle(cardEl).clipPath : 'none';
      const fadeNow = headEl ? getComputedStyle(headEl).opacity : '1';
      const shift = rowEl ? new DOMMatrix(getComputedStyle(rowEl).transform) : new DOMMatrix();
      const chip = firstChip()?.getBoundingClientRect();
      const home = rootEl?.getBoundingClientRect();
      for (const el of [cardEl, headEl, rowEl]) for (const a of el?.getAnimations() || []) a.cancel();

      const runs = [
        cardEl?.animate([{ clipPath: clipNow === 'none' ? CLIP_OPEN : clipNow }, { clipPath: CLIP_SHUT }], { duration: 160, easing: EASE_IN, fill: 'forwards' }),
        headEl?.animate([{ opacity: fadeNow }, { opacity: 0 }], { duration: 100, fill: 'forwards' }),
      ];
      // The row slides home with the card, to where the closed row's first
      // chip stands: the root's corner, past the funnel that leads it. The
      // chip was measured mid-slide, so its laid-out place is that less the
      // shift it was wearing. With a find typed, the closed row holds
      // different chips, so it stays put and the card closes around it.
      if (!find.trim() && chip && home) {
        const dx = home.left + collapseWidth + GAP - (chip.left - shift.e);
        const dy = home.top - (chip.top - shift.f);
        runs.push(rowEl.animate(
          [{ transform: `translate(${shift.e}px, ${shift.f}px)` }, { transform: `translate(${dx}px, ${dy}px)` }],
          { duration: 160, easing: EASE_IN, fill: 'forwards' },
        ));
      }
      await Promise.all(runs.filter(Boolean).map((a) => a.finished.catch(() => {})));
    }
    open = false;
    find = '';
    closing = false;
    await tick();
    // The row is in its closed place now; drop the held end frame.
    for (const a of rowEl?.getAnimations() || []) a.cancel();
    if (refocus) openerEl?.focus();
  }

  // Enter takes the first match, so typing "cer" and pressing Enter is the
  // whole act. The field empties so the next word starts fresh.
  function onFindKeydown(e) {
    if (e.key !== 'Enter' || !find.trim() || !shown.length) return;
    e.preventDefault();
    toggleTag(shown[0]);
    find = '';
  }

  // Captured, so it is decided before anything under the pointer reacts. A
  // click outside closes the card; on the canvas it does only that, so the
  // click that dismisses the card cannot also open a patch underneath.
  // Elsewhere (the rail, the list) it goes on to do what it was for. A drag
  // fires no click, so panning leaves the card open.
  function onWindowClickCapture(e) {
    if (!open) return;
    const path = e.composedPath();
    if (path.includes(bodyEl)) return;
    closeCard();
    if (path.some((el) => el instanceof Element && el.hasAttribute('data-canvas'))) {
      e.stopPropagation();
      e.preventDefault();
    }
  }

  function onWindowKeydown(e) {
    if (open && e.key === 'Escape') closeCard({ refocus: true });
  }
</script>

<svelte:window onclickcapture={onWindowClickCapture} onkeydown={onWindowKeydown} />

{#snippet tagChip(tag)}
  {@const active = getSelectedTags().includes(tag)}
  <button
    class="lt-chip lt-resin"
    style="--lt-resin-color: {colorForTag(tag)}; {active ? `background: ${colorForTag(tag)}; border-color: ${colorForTag(tag)}; color: ${textOnColor(colorForTag(tag))};` : `border-color: ${colorForTag(tag)}40;`}"
    aria-pressed={active}
    onclick={() => toggleTag(tag)}
  >{tag}</button>
{/snippet}

{#if getAllTags().length > 0 || query.trim()}
  <div
    class="chips chips-{variant}"
    class:collapsed
    bind:this={rootEl}
    style:height={open ? `${closedHeight}px` : null}
  >
    {#if variant === 'sheet'}
      {#if query.trim()}
        <div class="chips-row">
          <button
            class="lt-chip search-chip lt-resin"
            onclick={() => setSearchQuery('')}
            title={`Showing matches for “${query}”. Click to clear.`}
          >
            <span class="search-chip-q">“{query}”</span>
            <X size={11} weight="bold" />
          </button>
        </div>
      {/if}
      <TagFinder />
    {:else if collapsed}
      <button
        class="chips-toggle"
        onclick={() => setChipsCollapsed(false)}
        title="Show filters"
        aria-label="Show filters{count > 0 ? `, ${count} active` : ''}"
        aria-expanded="false"
      >
        <FunnelSimple size={14} weight="bold" />
        {#if count > 0}<span class="lt-count lt-count-corner lt-tone-primary chips-badge">{count}</span>{/if}
      </button>
    {:else}
      <div
        class="chips-body"
        class:open
        role={open ? 'dialog' : undefined}
        aria-label={open ? 'All tags' : undefined}
        bind:this={bodyEl}
        bind:clientHeight={measuredBodyHeight}
      >
        {#if open}
          <div class="chips-card pw-canvas-card" bind:this={cardEl}></div>
          <div class="chips-head" bind:this={headEl}>
            <label class="chips-find">
              <MagnifyingGlass size={14} weight="bold" />
              <input
                class="lt-field"
                type="text"
                placeholder="Find a tag"
                aria-label="Find a tag"
                bind:this={findEl}
                bind:value={find}
                onkeydown={onFindKeydown}
              />
              {#if find}
                <button
                  class="chips-find-clear"
                  aria-label="Clear what you typed"
                  onclick={() => { find = ''; findEl?.focus(); }}
                ><X size={12} weight="bold" /></button>
              {/if}
            </label>
            <div class="chips-head-end">
              <span class="chips-count" aria-live="polite">{countLabel}</span>
              {#if count > 0}
                <button class="chips-clear" onclick={resetFilters}>Clear</button>
              {/if}
              <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => closeCard({ refocus: true })}>Done</button>
            </div>
          </div>
        {/if}
        <div
          class="chips-row"
          role="group"
          aria-label="Filter by tag"
          bind:this={rowEl}
          bind:clientWidth={measuredRowWidth}
        >
          {#if !open}
            <!-- The button that showed the chips, on the spot it stood
                 while they were hidden, now hiding them. It says so: the
                 funnel becomes an eye shut, never an X, which would read as
                 clearing the filter it leaves standing. -->
            <button
              class="chips-toggle on"
              onclick={() => setChipsCollapsed(true)}
              title="Hide filters"
              aria-label="Hide filters"
              aria-expanded="true"
              bind:offsetWidth={collapseWidth}
            >
              <EyeSlash size={14} weight="bold" />
            </button>
          {/if}
          {#if query.trim()}
            <!-- The search chip: the query's standing form, set only by the
                 search dropdown's action row. Clears like any tag chip. -->
            <button
              class="lt-chip search-chip lt-resin"
              onclick={() => setSearchQuery('')}
              title={`Showing matches for “${query}”. Click to clear.`}
              bind:offsetWidth={searchWidth}
            >
              <span class="search-chip-q">“{query}”</span>
              <X size={11} weight="bold" />
            </button>
          {/if}
          {#each shown as tag (tag)}
            {@render tagChip(tag)}
          {/each}
          {#if open}
            {#if shown.length === 0}
              <p class="lt-muted chips-none">No tag matches “{find.trim()}”.</p>
            {/if}
          {:else}
            {#if hiddenCount > 0}
              <button
                class="chips-more"
                aria-haspopup="dialog"
                aria-expanded="false"
                bind:this={openerEl}
                onclick={openCard}
              >
                {hiddenCount} more <CaretDown size={11} weight="bold" />
              </button>
            {:else}
              <!-- Every tag fits, but finding one by typing stays one click
                   away: the card opens from here instead. -->
              <button
                class="chips-round chips-find-open"
                title="Find a tag"
                aria-label="Find a tag"
                aria-haspopup="dialog"
                aria-expanded="false"
                bind:this={openerEl}
                onclick={openCard}
              >
                <MagnifyingGlass size={13} weight="bold" />
              </button>
            {/if}
            {#if count > 0}
              <button class="chips-clear" onclick={resetFilters} bind:offsetWidth={clearWidth}>Clear</button>
            {/if}
          {/if}
        </div>
      </div>
      <!-- The ruler: every chip at its drawn width, never seen or reached.
           The "more" button is measured at its widest count so the row it
           is planned into cannot change size under it. -->
      <div class="chips-measure" aria-hidden="true" inert>
        {#each ranked as tag (tag)}
          <button class="lt-chip lt-resin" tabindex="-1" bind:offsetWidth={chipWidths[tag]}>{tag}</button>
        {/each}
        <button class="chips-more" tabindex="-1" bind:offsetWidth={moreWidth}>
          {ranked.length} more <CaretDown size={11} weight="bold" />
        </button>
        <button class="chips-round" tabindex="-1" bind:offsetWidth={findButtonWidth}>
          <MagnifyingGlass size={13} weight="bold" />
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .chips {
    /* The ruler's and the open card's anchor. How far the open card rises
       above the row and pads in from it: on the canvas, to the line the
       rail and the patch list hang from (the row sits 12px under it). */
    position: relative;
    --chips-lift: 12px;
  }

  .chips-flow {
    --chips-lift: 8px;
  }

  .chips-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }

  /* --- The open card --- */
  /* Out of the flow and over what is below; the root keeps the closed row's
     height. Bounded, so only the tags scroll and the find field stays. */
  .chips-body.open {
    position: absolute;
    top: calc(-1 * var(--chips-lift));
    left: 0;
    right: 0;
    z-index: 2;
    isolation: isolate;
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-height: min(60vh, 480px);
    padding: var(--chips-lift);
  }

  .chips-card {
    position: absolute;
    inset: 0;
    z-index: -1;
  }

  /* Frosted glass is for a canvas, where colour reading through is the
     point. Over a page of text the words behind showed through as smudge,
     so in the flow the card is solid. */
  .chips-flow .chips-card {
    background: var(--color-surface);
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }

  .chips-body.open .chips-row {
    min-height: 0;
    overflow-y: auto;
    align-content: flex-start;
  }

  /* On the card the chips stand on glass already. */
  .chips-body.open .lt-chip {
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }

  /* The field keeps the width a tag needs, not the card's: a word or two
     does not want 900px of box. The count and the controls take the far
     end, so the line still reads as the card's own. */
  .chips-head {
    container-type: inline-size;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .chips-find {
    position: relative;
    display: flex;
    align-items: center;
    flex: 0 1 280px;
    /* Low enough that the narrowest card (the rail open on an 800px window)
       still fits the field, the count's slot and Done on one line. */
    min-width: 96px;
    color: var(--color-text-muted);
  }

  .chips-find > :global(svg) {
    position: absolute;
    left: 10px;
    pointer-events: none;
  }

  /* The row's own height, so the line the field opens on is the line the
     row stood on. */
  .chips-find input {
    width: 100%;
    height: var(--pw-canvas-control-h);
    min-height: 0;
    padding-top: 0;
    padding-bottom: 0;
    padding-left: 30px;
    padding-right: 28px;
  }

  .chips-find-clear {
    position: absolute;
    right: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: 999px;
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .chips-find-clear:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }

  .chips-head-end {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
  }

  .chips-count {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  /* A narrow card gives the count's room to the field first. */
  @container (max-width: 380px) {
    .chips-count {
      display: none;
    }
  }

  .chips-none {
    margin: 0;
    font-size: 0.85rem;
  }

  /* Zero-sized and clipped, so the ruler can never widen a page or catch a
     pointer; its chips keep their own widths because they cannot shrink. */
  .chips-measure {
    position: absolute;
    top: 0;
    left: 0;
    width: 0;
    height: 0;
    overflow: hidden;
    visibility: hidden;
    display: flex;
    pointer-events: none;
  }

  .chips-measure > * {
    flex-shrink: 0;
  }

  .chips-more {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    height: var(--pw-canvas-control-h);
    border: none;
    background: none;
    padding: 0 6px;
    font-family: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-text);
    white-space: nowrap;
    cursor: pointer;
  }

  .chips-more:hover {
    text-decoration: underline;
  }

  .lt-chip {
    height: var(--pw-canvas-control-h);
    padding: 0 12px;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--color-text);
    cursor: pointer;
    white-space: nowrap;
  }

  .search-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    background: var(--color-primary);
    border-color: var(--color-primary);
    color: var(--color-btn-on-primary);
    --lt-resin-color: var(--color-primary);
  }

  .search-chip-q {
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .chips-clear {
    height: var(--pw-canvas-control-h);
    border: none;
    background: none;
    padding: 0 6px;
    font-family: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-primary);
    cursor: pointer;
  }

  .chips-clear:hover {
    text-decoration: underline;
  }

  /* The round find control that ends the row when every tag fits. */
  .chips-round {
    display: flex;
    align-items: center;
    justify-content: center;
    width: var(--pw-canvas-control-h);
    height: var(--pw-canvas-control-h);
    padding: 0;
    border: none;
    background: none;
    border-radius: 999px;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .chips-round:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }

  /* The show/hide button: one size whether it stands alone or leads the
     row, so showing and hiding the chips never moves it. On desktop that
     size is the canvas controls' height, so the row keeps its line; a phone
     keeps the larger target. */
  .chips-toggle {
    position: relative;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: var(--pw-canvas-control-h);
    height: var(--pw-canvas-control-h);
    padding: 0;
    /* Matches the mobile filter FAB: text-muted-derived border so the
       circle keeps a visible edge in the dark theme too. */
    border: 1px solid color-mix(in srgb, var(--color-text-muted) 45%, transparent);
    border-radius: 999px;
    background: var(--color-surface);
    color: var(--color-text-muted);
    cursor: pointer;
    transition: border-color 150ms ease, color 150ms ease;
  }

  @media (max-width: 768px) {
    .chips-toggle {
      width: 36px;
      height: 36px;
    }
  }

  .chips-toggle:hover {
    border-color: var(--color-primary);
    color: var(--color-text);
  }

  /* Shown: the button reads as switched on, the state it will undo. */
  .chips-toggle.on {
    border-color: color-mix(in srgb, var(--color-text) 55%, transparent);
    color: var(--color-text);
  }

  .chips-badge {
    top: -5px;
    right: -7px;
  }

  /* Overlay: chips float on the canvas — each carries its own glass
     backdrop, like the collapsed rail's items. */
  .chips-overlay .lt-chip,
  .chips-overlay .chips-round,
  .chips-overlay .chips-toggle {
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
  }

  .chips-overlay .chips-toggle {
    background: var(--color-glass);
    box-shadow: 0 2px 12px var(--color-shadow);
  }

  /* The round controls and Clear stand on the same canvas as the chips, so
     they take the same ground — bare, they were legible only over dark
     tiles. */
  .chips-overlay .chips-round {
    background: var(--color-glass);
    box-shadow: 0 1px 3px var(--color-shadow);
  }

  .chips-overlay .chips-round:hover {
    background: var(--color-glass-heavy);
  }

  .chips-overlay .chips-row .chips-clear,
  .chips-overlay .chips-more {
    background: var(--color-glass);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border-radius: 999px;
    padding: 0 10px;
  }

  /* Flow: the page's own component — plain, wraps with the content. */
  .chips-flow {
    margin-bottom: 1rem;
  }

  @media (prefers-reduced-motion: reduce) {
    .chips-toggle {
      transition: none;
    }
  }
</style>
