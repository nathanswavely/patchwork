<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  $effect(() => {
    patch.value.setBreadcrumbExtra?.([{ label: 'New Proposal' }]);
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  let title = $state('');
  let body = $state('');
  let proposalType = $state('action');
  let durationHours = $state(72);
  let submitting = $state(false);
  let error = $state('');
  let previewMode = $state(false);

  const durationOptions = [
    { value: 24, label: '24 hours' },
    { value: 48, label: '48 hours' },
    { value: 72, label: '72 hours (3 days)' },
    { value: 168, label: '1 week' },
    { value: 336, label: '2 weeks' },
  ];

  const typeOptions = [
    { value: 'action', label: 'Action', description: 'Propose a concrete action for the community to take' },
    { value: 'membership', label: 'Membership', description: 'Request or propose changes to membership' },
    { value: 'other', label: 'Other', description: 'Any other proposal that needs community input' },
  ];

  async function handleSubmit() {
    if (!title.trim()) {
      error = 'Title is required';
      return;
    }

    error = '';
    submitting = true;
    try {
      const payload = {
        title: title.trim(),
        body: body.trim() || '',
        proposal_type: proposalType,
        duration_hours: durationHours,
      };
      const result = await api(`nodes/${slug}/proposals`, {
        method: 'POST',
        body: payload,
      });
      navigate(`/patches/${slug}/governance/${result.id}`);
    } catch (e) {
      error = e.message || 'Failed to create proposal';
    } finally {
      submitting = false;
    }
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <div style="padding-top: 2rem;">
      <p class="amendment-hint muted">Want to change a governance document or rules? Go to the <a href="/patches/{slug}/governance/docs" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs`); }}>Documents</a> page and click "Propose change" on the document you want to edit.</p>

      <form class="card" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
        <div class="field">
          <label for="title">Title <span class="required">*</span></label>
          <input id="title" type="text" bind:value={title} disabled={submitting} required />
        </div>

        <div class="field">
          <label>
            Description
            <div class="toggle-group">
              <button type="button" class="toggle-btn" class:active={!previewMode} onclick={() => previewMode = false}>Write</button>
              <button type="button" class="toggle-btn" class:active={previewMode} onclick={() => previewMode = true}>Preview</button>
            </div>
          </label>
          {#if previewMode}
            <div class="preview-pane">
              {#if body.trim()}
                <MarkdownRenderer content={body} />
              {:else}
                <span class="muted">Nothing to preview</span>
              {/if}
            </div>
          {:else}
            <textarea
              id="body"
              bind:value={body}
              rows="8"
              placeholder="Explain your proposal and why it matters... Use **bold**, *italic*, and - lists."
              disabled={submitting}
            ></textarea>
          {/if}
        </div>

        <div class="field">
          <label>Type</label>
          <div class="type-radio-group">
            {#each typeOptions as opt}
              <label class="type-radio-option" class:selected={proposalType === opt.value}>
                <input
                  type="radio"
                  name="proposal-type"
                  value={opt.value}
                  bind:group={proposalType}
                  disabled={submitting}
                />
                <div class="type-radio-content">
                  <span class="type-radio-label">{opt.label}</span>
                  <span class="type-radio-desc">{opt.description}</span>
                </div>
              </label>
            {/each}
          </div>
        </div>

        <div class="field">
          <label for="duration">Voting Duration</label>
          <select id="duration" bind:value={durationHours} disabled={submitting}>
            {#each durationOptions as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
          <span class="duration-tip">Tip: 72 hours gives everyone a chance to vote before the question goes stale.</span>
        </div>

        {#if error}
          <p class="error-text">{error}</p>
        {/if}

        <div class="field-actions">
          <button type="submit" class="btn btn-primary" disabled={submitting}>
            {submitting ? 'Creating...' : 'Submit Proposal'}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            onclick={() => navigate(`/patches/${slug}/governance`)}
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
</div>

<style>
  form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  /* Direct child only: the field's header label (e.g. "Description" + its
     Write/Preview toggle). Without `>` this also matched the nested
     .type-radio-option labels and shoved their radio and text to opposite
     edges via space-between. */
  .field > label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .required {
    color: var(--color-error);
  }

  textarea {
    resize: vertical;
    min-height: 160px;
  }

  .toggle-group {
    display: flex;
    gap: 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .toggle-btn {
    padding: 0.2rem 0.6rem;
    border: none;
    background: var(--color-surface);
    font-size: 0.75rem;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .toggle-btn.active {
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
  }

  .preview-pane {
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    min-height: 160px;
    background: var(--color-bg);
    line-height: 1.6;
    font-size: 0.9rem;
  }

  .preview-pane :global(strong) { font-weight: 700; }
  .preview-pane :global(em) { font-style: italic; }
  .preview-pane :global(ul) { padding-left: 1.5rem; margin: 0.5rem 0; }
  .preview-pane :global(p) { margin: 0.5rem 0; }

  .type-radio-group {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .type-radio-option {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .type-radio-option:hover {
    border-color: var(--color-primary);
  }

  .type-radio-option.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, transparent);
  }

  .type-radio-option input[type="radio"] {
    margin-top: 0.15rem;
    accent-color: var(--color-primary);
  }

  .type-radio-content {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .type-radio-label {
    font-size: 0.9rem;
    font-weight: 500;
    color: var(--color-text);
  }

  .type-radio-desc {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .duration-tip {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    margin-top: 0.25rem;
  }

  .field-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }
</style>
