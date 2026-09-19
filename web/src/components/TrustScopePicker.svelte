<script>
  /**
   * Trust scope picker — the shape of a trusted-contributor grant
   * (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar,
   * decisions 2 and 7). A grant is either the whole quilt or a named set of
   * unclaimed patches, so this is one checkbox over a list of checkboxes
   * rather than two controls that can disagree.
   *
   * Checking "Every unclaimed patch" keeps the per-patch picks instead of
   * clearing them: an admin widening a request to the quilt and changing
   * their mind should get back what was asked for, not an empty scope.
   *
   * Selection order is the order. There are no reorder controls, here or
   * anywhere: nothing downstream reads the order, so a control for it would
   * be a promise the grant does not keep.
   *
   * Active patches are never in the list. A grant only reaches unclaimed
   * ones, and it ends when the patch is claimed.
   */
  import { api } from '../lib/api.js';

  let {
    all = $bindable(false),
    selected = $bindable([]),
    disabled = false,
    label = 'Where',
    allowAll = true,
  } = $props();

  const bodyId = $props.id();

  // Expanded when there is nothing to summarize: an empty scope is a
  // question, a filled one is an answer somebody can leave alone.
  let expanded = $state(selected.length === 0 && !all);

  let choices = $state([]);
  let loadState = $state('idle'); // idle | loading | loaded | error
  let query = $state('');
  let filter = $state('');
  let filterTimer = null;

  let selectedIds = $derived(new Set(selected.map((n) => n.id)));

  let matches = $derived(
    choices
      .filter((n) => !selectedIds.has(n.id))
      .filter((n) => (n.name || '').toLowerCase().includes(filter.trim().toLowerCase()))
      .slice(0, 8)
  );

  let summary = $derived(summarize(all, selected));

  function summarize(everywhere, picks) {
    if (everywhere) return 'Every unclaimed patch';
    if (picks.length === 0) return 'No patch picked yet';
    const names = picks.slice(0, 2).map((n) => n.name);
    const shown = picks.length > names.length ? [...names, '…'] : names;
    return `${picks.length} ${picks.length === 1 ? 'patch' : 'patches'}: ${shown.join(', ')}`;
  }

  /**
   * Loaded once, on first focus of the search box, and filtered in the
   * browser after that. The list is short enough to hold, and a keystroke
   * that reaches the server is a keystroke that can arrive out of order.
   */
  async function ensureLoaded() {
    if (loadState === 'loading' || loadState === 'loaded') return;
    loadState = 'loading';
    const found = [];
    let after = '';
    try {
      for (let page = 0; page < 10; page++) {
        const data = await api(
          `nodes?status=unclaimed&limit=100${after ? `&after=${encodeURIComponent(after)}` : ''}`
        );
        const items = data.items || [];
        found.push(...items);
        if (!data.next_cursor || items.length === 0) break;
        after = data.next_cursor;
      }
      choices = found.map((n) => ({ id: n.id, slug: n.slug, name: n.name }));
      loadState = 'loaded';
    } catch {
      choices = [];
      loadState = 'error';
    }
  }

  function onQueryInput(e) {
    query = e.target.value;
    if (filterTimer) clearTimeout(filterTimer);
    filterTimer = setTimeout(() => { filter = query; }, 200);
  }

  function add(node) {
    if (disabled || all || selectedIds.has(node.id)) return;
    selected = [...selected, { id: node.id, slug: node.slug, name: node.name }];
  }

  function remove(id) {
    if (disabled || all) return;
    selected = selected.filter((n) => n.id !== id);
  }
</script>

<div class="trust-scope">
  <div class="scope-head">
    <span class="scope-label">{label}</span>
    <span class="scope-summary">{summary}</span>
    <button
      type="button"
      class="btn btn-secondary btn-sm"
      aria-expanded={expanded}
      aria-controls={bodyId}
      onclick={() => { expanded = !expanded; }}
    >{expanded ? 'Done' : 'Change'}</button>
  </div>

  {#if expanded}
    <div class="scope-body" id={bodyId}>
      {#if allowAll}
        <label class="scope-all">
          <input type="checkbox" bind:checked={all} {disabled} />
          Every unclaimed patch
        </label>
      {/if}

      <div class="scope-list" class:dimmed={all}>
        {#if selected.length > 0}
          <ul class="picked">
            {#each selected as node (node.id)}
              <li>
                <label>
                  <input
                    type="checkbox"
                    checked
                    disabled={disabled || all}
                    onchange={() => remove(node.id)}
                  />
                  {node.name}
                </label>
              </li>
            {/each}
          </ul>
        {/if}

        <label class="scope-search">
          <span>Add a patch</span>
          <input
            type="search"
            placeholder="Search unclaimed patches"
            value={query}
            disabled={disabled || all}
            onfocus={ensureLoaded}
            oninput={onQueryInput}
          />
        </label>

        {#if loadState === 'loading'}
          <p class="muted scope-note">Loading unclaimed patches...</p>
        {:else if loadState === 'error'}
          <p class="muted scope-note">
            Could not load the unclaimed patches.
            <button type="button" class="link-btn" onclick={() => { loadState = 'idle'; ensureLoaded(); }}>Try again</button>
          </p>
        {:else if loadState === 'loaded'}
          {#if matches.length === 0}
            <p class="muted scope-note">No unclaimed patch by that name.</p>
          {:else}
            <ul class="matches">
              {#each matches as node (node.id)}
                <li>
                  <label>
                    <input
                      type="checkbox"
                      checked={false}
                      disabled={disabled || all}
                      onchange={() => add(node)}
                    />
                    {node.name}
                  </label>
                </li>
              {/each}
            </ul>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .trust-scope {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .scope-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .scope-label {
    font-size: 0.78rem;
    font-weight: 500;
  }

  .scope-summary {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    flex: 1;
    min-width: 0;
  }

  .scope-body {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .scope-all {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .scope-list {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  /* Kept, not cleared: the picks are still there when "every unclaimed
     patch" is unchecked again. */
  .scope-list.dimmed {
    opacity: 0.45;
  }

  .picked,
  .matches {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .matches {
    max-height: 12rem;
    overflow-y: auto;
  }

  .picked label,
  .matches label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .scope-search {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    font-size: 0.78rem;
    color: var(--color-text-muted);
  }

  .scope-search input {
    width: 100%;
    max-width: 22rem;
    font-size: 0.85rem;
    padding: 0.3rem 0.45rem;
  }

  .scope-note {
    font-size: 0.75rem;
    margin: 0;
  }

  .link-btn {
    border: none;
    background: none;
    padding: 0;
    color: var(--color-primary);
    cursor: pointer;
    font: inherit;
    text-decoration: underline;
  }

  input[type="checkbox"] {
    width: 1rem;
    height: 1rem;
  }
</style>
