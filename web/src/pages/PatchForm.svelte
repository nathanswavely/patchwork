<script>
  import * as d3 from 'd3';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import TemplatePreviewDrawer from '../components/TemplatePreviewDrawer.svelte';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import TagPicker from '../components/TagPicker.svelte';
  import { MOTIFS, MOTIF_KEYS } from '../lib/patchIcons.js';
  import { PALETTES, PALETTE_KEYS } from '../lib/quiltTheme.js';
  import { BLOCKS } from '../lib/quiltBlocks.js';

  let name = $state('');
  let description = $state('');
  let address = $state('');
  let website = $state('');
  let visibility = $state('public');
  let template = $state('casual');
  // Motif is optional at creation: '' means it is derived from the first
  // motif-bearing tag, falling back to the quilt mark.
  let motif = $state('');
  // Tags, in priority order — the first motif-bearing tag derives the
  // motif, and shared tags place new patches near their kind on the quilt.
  let tags = $state([]);

  // Tile appearance. The form always shows a concrete pick — seeded
  // randomly so every new patch starts somewhere real — and creation pins
  // it. Drafting your own block and swapping fabrics live in Patch
  // Settings → Appearance after creation.
  let palette = $state(PALETTE_KEYS[Math.floor(Math.random() * PALETTE_KEYS.length)]);
  let blockKey = $state(BLOCKS[Math.floor(Math.random() * BLOCKS.length)].key);
  let rotation = $state([0, 90, 180, 270][Math.floor(Math.random() * 4)]);

  const PREVIEW_SIZE = 84;
  const THUMB_SIZE = 40;
  let previewEl = $state(null);
  let thumbEls = $state({});

  function drawBlock(svgEl, size, key, pal, rot) {
    const svg = d3.select(svgEl);
    svg.selectAll('*').remove();
    const g = svg.append('g');
    if (rot) g.attr('transform', `rotate(${rot}, ${size / 2}, ${size / 2})`);
    const block = BLOCKS.find(b => b.key === key) || BLOCKS[0];
    block.render(g, size, { primary: pal.primary, secondary: pal.secondary, bg: pal.bg });
  }

  $effect(() => {
    if (previewEl) drawBlock(previewEl, PREVIEW_SIZE, blockKey, PALETTES[palette], rotation);
  });

  $effect(() => {
    const pal = PALETTES[palette];
    for (const b of BLOCKS) {
      const el = thumbEls[b.key];
      if (el) drawBlock(el, THUMB_SIZE, b.key, pal, 0);
    }
  });

  let submitting = $state(false);
  // The lining (docs/adr/036): shown at creation because adoption should
  // never be a surprise. Text fetched lazily when the drawer opens.
  let liningOpen = $state(false);
  let lining = $state(null);

  function toggleLining() {
    liningOpen = !liningOpen;
    if (liningOpen && !lining) {
      api('instance/lining').then((l) => { lining = l; }).catch(() => {});
    }
  }
  let error = $state('');
  let previewTemplate = $state('');

  const templates = [
    { id: 'minimal', name: 'Minimal', desc: 'Just a listing. You run it, no overhead.', leadership: 'Maintainer', bestFor: 'Bands, solo artists, pop-up projects' },
    { id: 'casual', name: 'Casual', desc: 'Small crew, lightweight process. Majority rules.', leadership: 'Maintainer', bestFor: 'Small collectives, meetups, studios (5\u201320 people)' },
    { id: 'collaborative', name: 'Collaborative', desc: 'Open community, structured process. Earn trust through contribution.', leadership: 'Meritocratic', bestFor: 'Venues, co-ops, makerspaces (20\u2013100 people)' },
    { id: 'formal', name: 'Formal', desc: 'Coalition-scale governance. Elected council, term limits, full accountability.', leadership: 'Elected Council', bestFor: 'Arts districts, coalitions (100+ people)' },
  ];

  function validate() {
    if (!name.trim()) return 'Name is required';
    return '';
  }

  async function handleSubmit() {
    const validationError = validate();
    if (validationError) {
      error = validationError;
      return;
    }

    error = '';
    submitting = true;
    try {
      const body = {
        name: name.trim(),
        description: description.trim() || undefined,
        address: address.trim() || undefined,
        website: website.trim() || undefined,
        visibility,
        template,
        appearance: {
          palette,
          block: blockKey,
          rotation,
          ...(motif ? { icon: motif } : {}),
        },
        tags: tags.length > 0 ? tags : undefined,
      };
      const result = await api('nodes', { method: 'POST', body });
      showToast('Patch created', 'success');
      navigate(`/patches/${result.slug}`);
    } catch (e) {
      error = e.message || 'Failed to create patch';
      showToast('Something went wrong. Please try again.', 'error');
    } finally {
      submitting = false;
    }
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <div>
      <h1>Create Patch</h1>
      <p class="muted" style="margin-bottom: {getSubmissionsEnabled() ? '0.35rem' : '1.5rem'};">Start a new community, collective, venue, or group.</p>
      {#if getSubmissionsEnabled()}
        <p class="muted" style="margin-bottom: 1.5rem;">Creating a patch makes you its admin. Know a group that isn't yours to run? <a href="/submit" class="suggest-link" onclick={(e) => { e.preventDefault(); navigate('/submit'); }}>Suggest a patch</a> instead.</p>
      {/if}

      <form class="card" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
        <div class="field">
          <label for="name">Name <span class="required">*</span></label>
          <input id="name" type="text" bind:value={name} disabled={submitting} required placeholder="e.g. Gallery Row, Lancaster Beats Lab" />
        </div>

        <div class="field">
          <label for="description">Description</label>
          <textarea id="description" bind:value={description} rows="4" disabled={submitting} placeholder="What is this patch about?"></textarea>
        </div>

        <div class="field">
          <label for="address">Location</label>
          <input id="address" type="text" bind:value={address} disabled={submitting} placeholder="Where is this based?" />
        </div>

        <div class="field">
          <label for="website">Website</label>
          <input id="website" type="url" bind:value={website} disabled={submitting} placeholder="https://..." />
        </div>

        <div class="field">
          <label>Tags</label>
          <p class="field-hint muted">
            What kind of patch is this? Tags help people find you, and new
            patches are placed near others with the same tags on the quilt.
            The first tag decides your default motif.
          </p>
          <TagPicker bind:selected={tags} disabled={submitting} />
        </div>

        <div class="field">
          <label>Tile</label>
          <p class="field-hint muted">
            How this patch looks on the quilt. You can change it anytime —
            and draft your own block — in Patch Settings → Appearance.
          </p>
          <div class="tile-picker">
            <div class="tile-preview-col">
              <svg
                bind:this={previewEl}
                class="tile-preview"
                viewBox="0 0 {PREVIEW_SIZE} {PREVIEW_SIZE}"
                width={PREVIEW_SIZE}
                height={PREVIEW_SIZE}
                role="img"
                aria-label="Tile preview"
              ></svg>
              <button
                type="button"
                class="btn btn-secondary btn-xs"
                onclick={() => { rotation = (rotation + 90) % 360; }}
                disabled={submitting}
                title="Rotate 90°"
              >
                Rotate
              </button>
            </div>
            <div class="tile-choices">
              <div class="palette-row" role="group" aria-label="Palette">
                {#each PALETTE_KEYS as key (key)}
                  {@const p = PALETTES[key]}
                  <button
                    type="button"
                    class="palette-chip"
                    class:selected={palette === key}
                    onclick={() => { palette = key; }}
                    disabled={submitting}
                    title="{p.name}: {p.subtitle}"
                    aria-label={p.name}
                    aria-pressed={palette === key}
                  >
                    <span style="background: {p.primary}"></span>
                    <span style="background: {p.secondary}"></span>
                    <span style="background: {p.bg}"></span>
                  </button>
                {/each}
              </div>
              <div class="block-row" role="group" aria-label="Block">
                {#each BLOCKS as block (block.key)}
                  <button
                    type="button"
                    class="block-chip"
                    class:selected={blockKey === block.key}
                    onclick={() => { blockKey = block.key; }}
                    disabled={submitting}
                    title={block.name}
                    aria-label={block.name}
                    aria-pressed={blockKey === block.key}
                  >
                    <svg
                      bind:this={thumbEls[block.key]}
                      viewBox="0 0 {THUMB_SIZE} {THUMB_SIZE}"
                      width={THUMB_SIZE}
                      height={THUMB_SIZE}
                      role="img"
                      aria-label={block.name}
                    ></svg>
                  </button>
                {/each}
              </div>
            </div>
          </div>
        </div>

        <div class="field">
          <label>Motif</label>
          <p class="field-hint muted">
            Optional. The mark that appears beside the name on the quilt.
            Unset, it follows your first tag. You can change it later in
            Patch Settings.
          </p>
          <div class="motif-grid">
            {#each MOTIF_KEYS as key (key)}
              {@const m = MOTIFS[key]}
              {@const MotifIcon = m.component}
              <button
                type="button"
                class="motif-swatch"
                class:selected={motif === key}
                onclick={() => { motif = motif === key ? '' : key; }}
                disabled={submitting}
                title={m.name}
                aria-pressed={motif === key}
                aria-label={m.name}
              >
                <MotifIcon size={18} weight="fill" />
              </button>
            {/each}
          </div>
        </div>

        <div class="field">
          <label>The lining</label>
          <p class="field-hint muted">
            Every patch starts with the lining — this quilt's shared community
            standards. It is always public, and if your patch amends it, the
            changes are public and the patch is marked as having amended the
            lining.
          </p>
          <button type="button" class="lining-toggle" onclick={toggleLining}>
            {liningOpen ? 'Hide the lining' : 'Read the lining'}
          </button>
          {#if liningOpen}
            <div class="lining-text">
              {#if lining}
                <MarkdownRenderer content={lining.body} />
              {:else}
                <span class="muted">Loading...</span>
              {/if}
            </div>
          {/if}
        </div>

        <div class="field">
          <label>Governance Template</label>
          <p class="field-hint muted">How should this patch be organized? You can change this later.</p>
          <div class="template-grid">
            {#each templates as t}
              <label class="template-card" class:selected={template === t.id}>
                <input type="radio" name="template" value={t.id} bind:group={template} disabled={submitting} />
                <div class="template-info">
                  <div class="template-top">
                    <strong>{t.name}</strong>
                    <button type="button" class="preview-link" onclick={(e) => { e.preventDefault(); e.stopPropagation(); previewTemplate = t.id; }}>Preview</button>
                  </div>
                  <span class="template-desc">{t.desc}</span>
                  <span class="template-meta">{t.leadership} · {t.bestFor}</span>
                </div>
              </label>
            {/each}
          </div>
        </div>

        {#if error}
          <p class="error-text">{error}</p>
        {/if}

        <div class="field-actions">
          <button type="submit" class="btn btn-primary" disabled={submitting}>
            {submitting ? 'Creating...' : 'Create Patch'}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            onclick={() => navigate('/dashboard')}
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
</div>

{#if previewTemplate}
  <TemplatePreviewDrawer templateId={previewTemplate} onClose={() => previewTemplate = ''} />
{/if}

<style>
  h1 {
    margin-bottom: 0.25rem;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    flex: 1;
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .required {
    color: var(--color-error);
  }

  textarea {
    resize: vertical;
    min-height: 80px;
  }

  .field-row {
    display: flex;
    gap: 1rem;
  }

  .field-hint {
    font-size: 0.8rem;
    margin-bottom: 0.5rem;
  }

  .tile-picker {
    display: flex;
    gap: 0.85rem;
    align-items: flex-start;
  }

  .tile-preview-col {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.35rem;
    flex-shrink: 0;
  }

  .tile-preview {
    border-radius: 4px;
    border: 2px solid var(--lt-thread, var(--color-border));
    display: block;
  }

  .btn-xs {
    padding: 0.2rem 0.55rem;
    font-size: 0.72rem;
  }

  .tile-choices {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .palette-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .palette-chip {
    display: flex;
    width: 44px;
    height: 26px;
    border: 2px solid var(--color-border);
    border-radius: 4px;
    overflow: hidden;
    padding: 0;
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .palette-chip span {
    flex: 1;
  }

  .palette-chip:hover {
    border-color: var(--color-text-muted);
  }

  .palette-chip.selected {
    border-color: var(--color-primary);
  }

  .block-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .block-chip {
    display: flex;
    padding: 2px;
    border: 2px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .block-chip svg {
    border-radius: 2px;
    display: block;
  }

  .block-chip:hover {
    border-color: var(--color-text-muted);
  }

  .block-chip.selected {
    border-color: var(--color-primary);
  }

  @media (max-width: 640px) {
    .tile-picker {
      flex-direction: column;
    }
  }

  .motif-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
    gap: 0.35rem;
  }

  .motif-swatch {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.45rem;
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
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
  }

  .template-grid {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .template-card {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .template-card:hover {
    border-color: var(--color-primary);
  }

  .template-card.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
  }

  .template-card input[type="radio"] {
    margin-top: 0.15rem;
    flex-shrink: 0;
  }

  .template-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    /* Fill the card so .template-top's space-between actually reaches the
       right edge — without this the column shrink-wraps and Preview hugs
       the title. */
    flex: 1;
    min-width: 0;
  }

  .template-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .template-info strong {
    font-size: 0.9rem;
  }

  .preview-link {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.78rem;
    cursor: pointer;
    padding: 0;
    text-decoration: none;
  }

  .preview-link:hover {
    text-decoration: underline;
  }

  .template-desc {
    font-size: 0.82rem;
    color: var(--color-text-muted);
  }

  .template-meta {
    font-size: 0.72rem;
    color: var(--color-text-muted);
    margin-top: 0.15rem;
  }

  .field-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }

  .suggest-link {
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .suggest-link:hover {
    text-decoration: underline;
  }

  @media (max-width: 640px) {
    .field-row {
      flex-direction: column;
    }
  }
  .lining-toggle {
    align-self: flex-start;
    border: none;
    background: none;
    padding: 0;
    font-size: 0.85rem;
    color: var(--color-primary);
    text-decoration: underline;
    cursor: pointer;
  }

  .lining-text {
    margin-top: 0.6rem;
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg);
    font-size: 0.88rem;
    line-height: 1.7;
    max-height: 320px;
    overflow-y: auto;
  }

</style>
