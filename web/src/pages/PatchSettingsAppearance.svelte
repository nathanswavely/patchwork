<script>
  import { getContext } from 'svelte';
  import * as d3 from 'd3';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { PALETTES, PALETTE_KEYS, paletteForPatch, darken } from '../lib/quiltTheme.js';
  import { BLOCKS, getBlockIndex, getRotation, renderDraftBlock } from '../lib/quiltBlocks.js';
  import { MOTIFS, MOTIF_KEYS, motifKeyForPatch } from '../lib/patchIcons.js';
  import BundlePicker from '../components/BundlePicker.svelte';
  import BlockDrafter from '../components/BlockDrafter.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

  // Draft starts from the patch's *effective* appearance: stored values
  // where chosen, hash-assigned/tag-derived values where not. Saving pins
  // everything. Two modes share the bundle, rotation, and motif:
  // 'traditional' picks a curated block, 'draft' opens the block drafter
  // (docs/adr/029).
  let draft = $state({
    mode: 'traditional',
    palette: '',
    block: '',
    draftBlock: { grid: 3, seams: [], colors: {} },
    bundle: [],
    rotation: 0,
    icon: '',
  });
  let seeded = $state('');
  let saving = $state(false);

  let stored = $derived(node?.appearance || null);
  let isPinned = $derived(stored != null);

  /** The appearance object a given draft state would save. */
  function buildAppearanceFrom(d) {
    const a = { rotation: d.rotation, icon: d.icon, bundle: [...d.bundle] };
    if (d.mode === 'draft') {
      a.block = JSON.parse(JSON.stringify(d.draftBlock));
    } else {
      a.block = d.block;
      if (d.palette) a.palette = d.palette;
    }
    return a;
  }

  // Re-seed the draft whenever the node (re)loads.
  $effect(() => {
    if (!node?.id) return;
    const ap = node.appearance || null;
    const pal = paletteForPatch(node.id, ap);
    const isDraft = !!ap?.block && typeof ap.block === 'object';
    const d = {
      mode: isDraft ? 'draft' : 'traditional',
      palette: pal.paletteKey || '',
      block: isDraft ? BLOCKS[0].key : BLOCKS[getBlockIndex(node.id, ap)].key,
      draftBlock: isDraft
        ? JSON.parse(JSON.stringify({ colors: {}, seams: [], ...ap.block }))
        : { grid: 3, seams: [], colors: {} },
      bundle: Array.isArray(ap?.bundle) && ap.bundle.length ? [...ap.bundle] : [...pal.slots],
      rotation: getRotation(node.id, ap),
      icon: motifKeyForPatch(node),
    };
    draft = d;
    seeded = JSON.stringify(buildAppearanceFrom(d));
  });

  let dirty = $derived(!!node?.id && JSON.stringify(buildAppearanceFrom(draft)) !== seeded);

  // The colors everything previews with. The bundle always wins — a
  // palette pick is just a pre-cut bundle (one color system).
  let draftPalette = $derived.by(() => {
    const b = draft.bundle;
    if (b.length) {
      return {
        primary: b[0],
        secondary: b[1] || b[0],
        bg: b[2] || darken(b[0], 0.55),
        slots: [...b],
      };
    }
    const p = PALETTES[draft.palette] || PALETTES[PALETTE_KEYS[0]];
    return { primary: p.primary, secondary: p.secondary, bg: p.bg, slots: [p.primary, p.secondary, p.bg] };
  });

  function pickPalette(key) {
    const p = PALETTES[key];
    draft.palette = key;
    draft.bundle = [p.primary, p.secondary, p.bg];
  }

  // A palette chip only stays "selected" while the bundle still is that
  // pre-cut; hand-editing fabrics makes the bundle custom.
  $effect(() => {
    if (!draft.palette) return;
    const p = PALETTES[draft.palette];
    if (!p) return;
    const cut = [p.primary, p.secondary, p.bg];
    const match =
      draft.bundle.length === 3 &&
      cut.every((c, i) => c.toLowerCase() === (draft.bundle[i] || '').toLowerCase());
    if (!match) draft.palette = '';
  });

  // --- Preview rendering (the real quilt block renderers) ---
  let previewEl = $state(null);
  const PREVIEW_SIZE = 168;
  const THUMB_SIZE = 56;

  $effect(() => {
    if (!previewEl) return;
    const block = draft.mode === 'draft' ? draft.draftBlock : draft.block;
    // Deep-touch everything the preview depends on so mutations redraw.
    void JSON.stringify(block);
    void draftPalette;
    void draft.rotation;
    drawBlock(previewEl, PREVIEW_SIZE, block, draftPalette, draft.rotation);
  });

  let thumbEls = $state({});

  $effect(() => {
    const p = draftPalette;
    for (const b of BLOCKS) {
      const el = thumbEls[b.key];
      if (el) drawBlock(el, THUMB_SIZE, b.key, p, 0);
    }
  });

  function drawBlock(svgEl, size, blockKeyOrDraft, palette, rotation) {
    const svg = d3.select(svgEl);
    svg.selectAll('*').remove();
    const g = svg.append('g');
    if (rotation) {
      g.attr('transform', `rotate(${rotation}, ${size / 2}, ${size / 2})`);
    }
    if (blockKeyOrDraft && typeof blockKeyOrDraft === 'object') {
      renderDraftBlock(g, size, blockKeyOrDraft, palette);
      return;
    }
    const block = BLOCKS.find(b => b.key === blockKeyOrDraft) || BLOCKS[0];
    block.render(g, size, { primary: palette.primary, secondary: palette.secondary, bg: palette.bg });
  }

  function rotate() {
    draft.rotation = (draft.rotation + 90) % 360;
  }

  async function save() {
    saving = true;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { appearance: buildAppearanceFrom(draft) } });
      showToast('Appearance saved', 'success');
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to save appearance', 'error');
    } finally {
      saving = false;
    }
  }

  async function reset() {
    saving = true;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { appearance: null } });
      showToast('Appearance reset', 'success');
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to reset appearance', 'error');
    } finally {
      saving = false;
    }
  }
</script>

<div class="appearance-settings">
  <p class="section-intro">
    Choose how this patch appears on the quilt. The first fabric of the
    bundle also becomes the patch's color on cards and labels.
  </p>

  <div class="preview-row">
    <div class="preview-frame">
      <svg
        bind:this={previewEl}
        class="preview-tile"
        viewBox="0 0 {PREVIEW_SIZE} {PREVIEW_SIZE}"
        width={PREVIEW_SIZE}
        height={PREVIEW_SIZE}
        role="img"
        aria-label="Tile preview"
      ></svg>
      <button class="btn btn-secondary btn-sm rotate-btn" onclick={rotate} title="Rotate 90°">
        Rotate ({draft.rotation}°)
      </button>
    </div>
    <div class="preview-meta">
      <p class="preview-status">
        {#if isPinned}
          This appearance is chosen by patch admins.
        {:else}
          This appearance was assigned automatically. Pick and save to make it yours.
        {/if}
      </p>
      <div class="preview-actions">
        <button class="btn btn-primary" onclick={save} disabled={saving || !dirty}>
          {saving ? 'Saving...' : 'Save appearance'}
        </button>
        {#if isPinned}
          <button class="btn btn-secondary" onclick={reset} disabled={saving}>
            Reset appearance
          </button>
        {/if}
      </div>
      {#if isPinned}
        <p class="muted reset-hint">
          After a reset, the tile gets an automatically assigned look, which
          may change as the quilt grows.
        </p>
      {/if}
    </div>
  </div>

  <div class="mode-tabs" role="tablist" aria-label="Block source">
    <button
      role="tab"
      aria-selected={draft.mode === 'traditional'}
      class:active={draft.mode === 'traditional'}
      onclick={() => { draft.mode = 'traditional'; }}
    >
      Traditional blocks
    </button>
    <button
      role="tab"
      aria-selected={draft.mode === 'draft'}
      class:active={draft.mode === 'draft'}
      onclick={() => { draft.mode = 'draft'; }}
    >
      Draft your own
    </button>
  </div>

  {#if draft.mode === 'traditional'}
    <div class="picker-group">
      <span class="picker-label">Palette</span>
      <p class="picker-hint muted">
        Pre-cut bundles from the record crate. Picking one fills your fabrics below.
      </p>
      <div class="palette-grid">
        {#each PALETTE_KEYS as key (key)}
          {@const p = PALETTES[key]}
          <button
            class="palette-swatch"
            class:selected={draft.palette === key}
            onclick={() => pickPalette(key)}
            title="{p.name}: {p.subtitle}"
            aria-pressed={draft.palette === key}
          >
            <span class="swatch-colors">
              <span style="background: {p.primary}"></span>
              <span style="background: {p.secondary}"></span>
              <span style="background: {p.bg}"></span>
            </span>
            <span class="swatch-name">{p.name}</span>
          </button>
        {/each}
      </div>
    </div>

    <div class="picker-group">
      <BundlePicker
        bind:bundle={draft.bundle}
        hint="Swap any fabric off the wall — the first is the patch's identity color."
      />
    </div>

    <div class="picker-group">
      <span class="picker-label">Block</span>
      <div class="block-grid">
        {#each BLOCKS as block (block.key)}
          <button
            class="block-thumb"
            class:selected={draft.block === block.key}
            onclick={() => { draft.block = block.key; }}
            title={block.name}
            aria-pressed={draft.block === block.key}
          >
            <svg
              bind:this={thumbEls[block.key]}
              viewBox="0 0 {THUMB_SIZE} {THUMB_SIZE}"
              width={THUMB_SIZE}
              height={THUMB_SIZE}
              role="img"
              aria-label={block.name}
            ></svg>
            <span class="block-name">{block.name}</span>
          </button>
        {/each}
      </div>
    </div>
  {:else}
    <div class="picker-group">
      <BlockDrafter bind:draft={draft.draftBlock} bind:bundle={draft.bundle} />
    </div>
  {/if}

  <div class="picker-group">
    <span class="picker-label">Motif</span>
    <p class="picker-hint muted">
      The mark beside this patch's name on the quilt and on cards.
    </p>
    <div class="motif-grid">
      {#each MOTIF_KEYS as key (key)}
        {@const m = MOTIFS[key]}
        {@const MotifIcon = m.component}
        <button
          class="motif-swatch"
          class:selected={draft.icon === key}
          onclick={() => { draft.icon = key; }}
          title={m.name}
          aria-pressed={draft.icon === key}
          aria-label={m.name}
        >
          <span
            class="motif-badge"
            style="background: {draft.icon === key ? draftPalette.primary : 'transparent'}"
          >
            <MotifIcon size={18} weight="fill" color={draft.icon === key ? '#fff' : 'currentColor'} />
          </span>
        </button>
      {/each}
    </div>
  </div>
</div>

<style>
  .appearance-settings {
    max-width: 640px;
  }

  .section-intro {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    margin-bottom: 1rem;
  }

  .preview-row {
    display: flex;
    gap: 1.25rem;
    align-items: flex-start;
    margin-bottom: 1.25rem;
  }

  .preview-frame {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .preview-tile {
    border-radius: 4px;
    border: 2px solid var(--lt-thread, var(--color-border));
    display: block;
  }

  .preview-meta {
    min-width: 0;
  }

  .preview-status {
    font-size: 0.88rem;
    margin-bottom: 0.6rem;
  }

  .preview-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .reset-hint {
    font-size: 0.78rem;
    margin-top: 0.5rem;
  }

  .mode-tabs {
    display: flex;
    gap: 0.25rem;
    border-bottom: 1px solid var(--color-border);
    margin-bottom: 0.25rem;
  }

  .mode-tabs button {
    padding: 0.45rem 0.9rem;
    font-size: 0.82rem;
    font-weight: 600;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .mode-tabs button.active {
    color: var(--color-text);
    border-bottom-color: var(--color-primary);
  }

  .picker-group {
    padding: 0.75rem 0;
    border-top: 1px solid var(--color-border);
  }

  .picker-group:first-of-type {
    border-top: none;
  }

  .picker-label {
    display: block;
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 0.6rem;
  }

  .palette-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
    gap: 0.5rem;
  }

  .palette-swatch {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.5rem;
    border: 2px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    cursor: pointer;
    text-align: left;
    transition: border-color 120ms ease;
  }

  .palette-swatch:hover {
    border-color: var(--color-text-muted);
  }

  .palette-swatch.selected {
    border-color: var(--color-primary);
  }

  .swatch-colors {
    display: flex;
    flex-shrink: 0;
    width: 36px;
    height: 24px;
    border-radius: 3px;
    overflow: hidden;
  }

  .swatch-colors span {
    flex: 1;
  }

  .swatch-name {
    font-size: 0.78rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .picker-hint {
    font-size: 0.78rem;
    margin: -0.35rem 0 0.6rem;
  }

  .motif-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(44px, 1fr));
    gap: 0.4rem;
  }

  .motif-swatch {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.3rem;
    border: 2px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .motif-swatch:hover {
    border-color: var(--color-text-muted);
  }

  .motif-swatch.selected {
    border-color: var(--color-primary);
  }

  .motif-badge {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
  }

  .block-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(82px, 1fr));
    gap: 0.5rem;
  }

  .block-thumb {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.3rem;
    padding: 0.5rem 0.25rem;
    border: 2px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .block-thumb:hover {
    border-color: var(--color-text-muted);
  }

  .block-thumb.selected {
    border-color: var(--color-primary);
  }

  .block-thumb svg {
    border-radius: 2px;
  }

  .block-name {
    font-size: 0.68rem;
    color: var(--color-text-muted);
    text-align: center;
    line-height: 1.2;
  }

  .btn-sm {
    padding: 0.25rem 0.6rem;
    font-size: 0.78rem;
  }

  @media (max-width: 600px) {
    .preview-row {
      flex-direction: column;
    }
  }
</style>
