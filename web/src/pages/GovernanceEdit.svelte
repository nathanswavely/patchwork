<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import SegmentedControl from '../components/SegmentedControl.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  let title = $state('');
  let body = $state('');
  let submitting = $state(false);
  let error = $state('');
  let previewMode = $state(false);
  let selectedTemplate = $state('');

  const templates = [
    { value: '', label: 'Blank', title: '', body: '' },
    {
      value: 'community-agreement',
      label: 'Community Agreement',
      title: 'Community Agreement',
      body: `# Community Agreement

## Purpose
[What this community exists to do]

## Values
- [Core value 1]
- [Core value 2]
- [Core value 3]

## Membership
[How people join, what's expected of members]

## Decision Making
[How the community makes decisions together]

## Amendments
This agreement can be amended through the proposal process. Any member can propose changes.`,
    },
    {
      value: 'code-of-conduct',
      label: 'Code of Conduct',
      title: 'Code of Conduct',
      body: `# Code of Conduct

## Our Standards
This community is dedicated to providing a welcoming and inclusive experience for everyone. We do not tolerate discrimination or harassment based on race, gender, sexual orientation, disability, age, religion, or any other protected characteristic.

## Expected Behavior
- Be respectful and considerate
- Listen actively and make space for different perspectives
- Give and receive constructive feedback gracefully
- Take responsibility for your impact on others

## Reporting
If you experience or witness behavior that violates this code, contact a patch admin. All reports will be handled confidentially.

## Enforcement
Violations may result in a warning, temporary suspension, or removal from the community, at the discretion of the admin team.`,
    },
  ];

  function applyTemplate(value) {
    selectedTemplate = value;
    const tmpl = templates.find(t => t.value === value);
    if (tmpl) {
      title = tmpl.title;
      body = tmpl.body;
    }
  }

  $effect(() => {
    patch.value.setBreadcrumbExtra?.([{ label: 'New Document' }]);
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  async function handleSubmit() {
    if (!title.trim()) {
      error = 'Title is required';
      return;
    }

    error = '';
    submitting = true;
    try {
      const result = await api(`nodes/${slug}/governance`, {
        method: 'POST',
        body: { title: title.trim(), body: body.trim() },
      });
      showToast('Document created', 'success');
      navigate(`/patches/${slug}/governance/docs/${result.id}`);
    } catch (e) {
      error = e.message || 'Failed to save document';
    } finally {
      submitting = false;
    }
  }

  function handleCancel() {
    navigate(`/patches/${slug}/governance`);
  }
</script>

<div class="page-fade">
  <div class="container">
    <div style="padding-top: 2rem;">
      <h2>New Charter</h2>

        <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
          <div class="field">
            <label>Start from a template</label>
            <div class="template-selector">
              {#each templates as tmpl}
                <button
                  type="button"
                  class="template-btn"
                  class:selected={selectedTemplate === tmpl.value}
                  onclick={() => applyTemplate(tmpl.value)}
                >
                  {tmpl.label}
                </button>
              {/each}
            </div>
          </div>

          <div class="field">
            <label for="doc-title">Title <span class="required">*</span></label>
            <input id="doc-title" type="text" bind:value={title} disabled={submitting} required />
          </div>

          <div class="editor-section">
            <div class="editor-header">
              <label>Content</label>
              <SegmentedControl
                label="Editor view"
                options={[{ value: 'write', label: 'Write' }, { value: 'preview', label: 'Preview' }]}
                value={previewMode ? 'preview' : 'write'}
                onchange={(v) => { previewMode = v === 'preview'; }}
              />
            </div>

            <div class="editor-panels">
              {#if !previewMode}
                <textarea
                  bind:value={body}
                  rows="20"
                  placeholder="Write your governance document here... Use **bold**, *italic*, and - lists."
                  disabled={submitting}
                  class="editor-textarea"
                ></textarea>
              {:else}
                <div class="preview-pane">
                  {#if body.trim()}
                    <MarkdownRenderer content={body} />
                  {:else}
                    <span class="muted">Nothing to preview</span>
                  {/if}
                </div>
              {/if}
            </div>
          </div>

          {#if error}
            <p class="error-text">{error}</p>
          {/if}

          <div class="field-actions">
            <button type="submit" class="btn btn-primary" disabled={submitting}>
              {submitting ? 'Saving...' : 'Save'}
            </button>
            <button type="button" class="btn btn-secondary" onclick={handleCancel}>
              Cancel
            </button>
          </div>
        </form>
    </div>
  </div>
</div>

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
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .required {
    color: var(--color-error);
  }

  .template-selector {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .template-btn {
    padding: 0.5rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.85rem;
    color: var(--color-text);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .template-btn:hover {
    border-color: var(--color-primary);
  }

  .template-btn.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 8%, transparent);
    color: var(--color-primary);
    font-weight: 500;
  }

  .editor-section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .editor-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .editor-header label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .editor-panels {
    min-height: 400px;
  }

  .editor-textarea {
    width: 100%;
    min-height: 400px;
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    font-family: var(--font);
    font-size: 0.9rem;
    line-height: 1.6;
    resize: vertical;
  }

  .preview-pane {
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    min-height: 400px;
    background: var(--color-bg);
    line-height: 1.7;
    font-size: 0.9rem;
  }

  .preview-pane :global(strong) {
    font-weight: 700;
  }

  .preview-pane :global(em) {
    font-style: italic;
  }

  .preview-pane :global(ul) {
    padding-left: 1.5rem;
    margin: 0.5rem 0;
  }

  .preview-pane :global(p) {
    margin: 0.5rem 0;
  }

  .field-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }

  @media (max-width: 640px) {
    .editor-panels {
      min-height: 300px;
    }

    .editor-textarea {
      min-height: 300px;
    }

    .preview-pane {
      min-height: 300px;
    }
  }
</style>
