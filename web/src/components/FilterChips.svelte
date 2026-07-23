<script>
  /**
   * The filter chips (docs/adr/033): the filter's control and indicator as
   * one thing. The full tag vocabulary as toggleable chips, the search chip
   * among them, living on the surfaces the filter narrows.
   *
   * Variants:
   *  - "overlay": floats over a canvas (quilt/map) — glass chips, no panel.
   *  - "flow":    sits in a page's content flow (events list).
   *  - "sheet":   the mobile canvas sheet body — always expanded; the
   *               open/closed state belongs to the caller (a sheet is
   *               open-while-using, never a preference — docs/adr/033).
   *
   * Overlay and flow share the one collapse preference from the quilt
   * store; collapsed they render a single badged button.
   */
  import { FunnelSimple, X } from 'phosphor-svelte';
  import {
    getAllTags,
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

  let { variant = 'flow' } = $props();

  let collapsed = $derived(variant !== 'sheet' && getChipsCollapsed());
  let count = $derived(getActiveFilterCount());
  let query = $derived(getSearchQuery());
</script>

{#if getAllTags().length > 0 || query.trim()}
  <div class="chips chips-{variant}" class:collapsed>
    {#if collapsed}
      <button
        class="chips-toggle"
        onclick={() => setChipsCollapsed(false)}
        title="Show filters"
        aria-label="Show filters{count > 0 ? ` — ${count} active` : ''}"
        aria-expanded="false"
      >
        <FunnelSimple size={16} weight="bold" />
        {#if count > 0}<span class="chips-badge">{count}</span>{/if}
      </button>
    {:else}
      <div class="chips-row" role="group" aria-label="Filter by tag">
        {#if query.trim()}
          <!-- The search chip: the query's standing form, set only by the
               search dropdown's action row. Clears like any tag chip. -->
          <button
            class="chip search-chip lt-resin lt-resin-frosted"
            onclick={() => setSearchQuery('')}
            title={`Showing matches for “${query}” — click to clear`}
          >
            <span class="search-chip-q">“{query}”</span>
            <X size={11} weight="bold" />
          </button>
        {/if}
        {#each getAllTags() as tag (tag)}
          {@const active = getSelectedTags().includes(tag)}
          <button
            class="chip lt-resin lt-resin-frosted"
            style="--lt-resin-color: {colorForTag(tag)}; {active ? `background: ${colorForTag(tag)}; border-color: ${colorForTag(tag)}; color: ${textOnColor(colorForTag(tag))};` : `border-color: ${colorForTag(tag)}40;`}"
            aria-pressed={active}
            onclick={() => toggleTag(tag)}
          >{tag}</button>
        {/each}
        {#if count > 0}
          <button class="chips-clear" onclick={resetFilters}>Clear</button>
        {/if}
        {#if variant !== 'sheet'}
          <button
            class="chips-collapse"
            onclick={() => setChipsCollapsed(true)}
            title="Hide filters"
            aria-label="Hide filters"
            aria-expanded="true"
          >
            <X size={14} weight="bold" />
          </button>
        {/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  .chips-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }

  .chip {
    padding: 5px 12px;
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
    border: none;
    background: none;
    padding: 2px 6px;
    font-family: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-primary);
    cursor: pointer;
  }

  .chips-clear:hover {
    text-decoration: underline;
  }

  .chips-collapse {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border: none;
    background: none;
    border-radius: 999px;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .chips-collapse:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }

  .chips-toggle {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
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

  .chips-toggle:hover {
    border-color: var(--color-primary);
    color: var(--color-text);
  }

  .chips-badge {
    position: absolute;
    top: -5px;
    right: -7px;
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
    font-size: 0.6rem;
    font-weight: 700;
    min-width: 16px;
    height: 16px;
    line-height: 16px;
    text-align: center;
    border-radius: 8px;
    padding: 0 3px;
  }

  /* Overlay: chips float on the canvas — each carries its own glass
     backdrop, like the collapsed rail's items. */
  .chips-overlay .chip,
  .chips-overlay .chips-collapse,
  .chips-overlay .chips-toggle {
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
  }

  .chips-overlay .chips-toggle {
    background: var(--color-glass);
    box-shadow: 0 2px 12px var(--color-shadow);
  }

  .chips-overlay .chips-clear {
    background: color-mix(in srgb, var(--color-bg) 65%, transparent);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border-radius: 999px;
    padding: 4px 10px;
  }

  /* Flow: the page's own component — plain, wraps with the content. */
  .chips-flow {
    margin-bottom: 1rem;
  }
</style>
