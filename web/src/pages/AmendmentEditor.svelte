<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { getParams, navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import DiffView from '../components/DiffView.svelte';
  import Skeleton from '../components/Skeleton.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let docId = $derived(getParams().id || '');

  let doc = $state(null);
  let loading = $state(true);
  let error = $state('');

  let proposedContent = $state('');
  let hasChanges = $derived(doc && proposedContent !== doc.body);

  // Two-step flow: 'editing' or 'reviewing'.
  let step = $state('editing');
  let title = $state('');
  let description = $state('');
  let submitting = $state(false);

  // Auto-save.
  let draftKey = $derived(docId ? `amendment-draft-${docId}` : null);
  let draftRestored = $state(false);
  let draftTimestamp = $state(null);

  // Diff preview toggle.
  let showDiff = $state(false);

  $effect(() => {
    if (docId) loadDoc();
  });

  $effect(() => {
    if (doc) {
      patch.value.setBreadcrumbExtra?.([
        { label: doc.title, href: `/patches/${slug}/governance/docs/${docId}` },
        { label: 'Propose change' },
      ]);
    }
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  // Restore draft from localStorage after doc loads.
  $effect(() => {
    if (draftKey && doc) {
      try {
        const saved = localStorage.getItem(draftKey);
        if (saved) {
          const parsed = JSON.parse(saved);
          if (parsed.content && parsed.content !== doc.body) {
            proposedContent = parsed.content;
            draftTimestamp = parsed.timestamp;
            draftRestored = true;
          }
        }
      } catch {}
    }
  });

  // Debounced auto-save when content changes.
  $effect(() => {
    if (!draftKey || !hasChanges) return;
    const timeout = setTimeout(() => {
      localStorage.setItem(draftKey, JSON.stringify({
        content: proposedContent,
        timestamp: Date.now(),
      }));
    }, 1000);
    return () => clearTimeout(timeout);
  });

  // Warn before navigating away with unsaved changes.
  $effect(() => {
    if (!hasChanges) return;
    const handler = (e) => { e.preventDefault(); };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  async function loadDoc() {
    loading = true;
    error = '';
    try {
      doc = await api(`governance/${docId}`);
      proposedContent = doc.body || '';
    } catch (e) {
      error = e.message || 'Failed to load document';
      doc = null;
    } finally {
      loading = false;
    }
  }

  function dismissDraft() {
    draftRestored = false;
  }

  function discardDraft() {
    if (doc) proposedContent = doc.body;
    draftRestored = false;
    if (draftKey) localStorage.removeItem(draftKey);
  }

  function goToReview() {
    if (!title) title = `Update ${doc.title}`;
    step = 'reviewing';
  }

  function backToEditing() {
    step = 'editing';
  }

  async function handleSubmit() {
    if (!title.trim()) {
      showToast('Title is required', 'error');
      return;
    }
    submitting = true;
    try {
      const payload = {
        title: title.trim(),
        body: description.trim() || '',
        proposal_type: 'amendment',
        target_doc: doc.filename || doc.title,
        proposed_body: proposedContent,
        proposed_title: doc.title,
        change_summary: title.trim(),
      };
      const result = await api(`nodes/${slug}/proposals`, { method: 'POST', body: payload });
      if (draftKey) localStorage.removeItem(draftKey);
      showToast('Proposal created', 'success');
      navigate(`/patches/${slug}/governance/${result.id}`);
    } catch (e) {
      showToast(e.message || 'Failed to create proposal', 'error');
    } finally {
      submitting = false;
    }
  }

  function formatTime(ts) {
    if (!ts) return '';
    const d = new Date(ts);
    const now = new Date();
    const diffMs = now - d;
    const mins = Math.floor(diffMs / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return d.toLocaleDateString();
  }
</script>

<div class="amendment-editor page-fade">
  {#if loading}
    <Skeleton lines={1} height="2rem" width="60%" />
    <Skeleton lines={6} height="0.9rem" />
  {:else if error}
    <div class="error-state">
      <p class="error-text">{error}</p>
      <button class="btn btn-secondary" onclick={loadDoc}>Retry</button>
    </div>
  {:else if doc}

    <div class="editor-header">
      <span class="editor-context muted">Proposing changes to</span>
      <h1>{doc.title}</h1>
      <span class="muted">Current version: v{doc.version}</span>
    </div>

    {#if step === 'editing'}

      {#if draftRestored}
        <div class="draft-banner">
          <span>Restored unsaved changes from {formatTime(draftTimestamp)}</span>
          <div class="draft-actions">
            <button class="btn-link" onclick={dismissDraft}>Dismiss</button>
            <button class="btn-link danger" onclick={discardDraft}>Discard changes</button>
          </div>
        </div>
      {/if}

      <!-- Editor card: textarea + actions together -->
      <div class="editor-card">
        <textarea
          class="full-editor"
          bind:value={proposedContent}
          rows={Math.max(16, proposedContent.split('\n').length + 4)}
          disabled={submitting}
          spellcheck="true"
        ></textarea>

        <div class="editor-footer">
          <div class="editor-actions">
            <button class="btn btn-primary" onclick={goToReview} disabled={!hasChanges}>
              Review & submit
            </button>
            <button class="btn btn-secondary" onclick={() => navigate(`/patches/${slug}/governance/docs/${docId}`)}>
              Cancel
            </button>
          </div>
          {#if hasChanges}
            <button class="diff-toggle" onclick={() => showDiff = !showDiff}>
              {showDiff ? 'Hide' : 'Show'} diff preview
            </button>
          {/if}
        </div>
      </div>

      <!-- Diff preview: below the editor card, collapsible -->
      {#if hasChanges && showDiff}
        <div class="diff-preview">
          <DiffView
            oldText={doc.body}
            newText={proposedContent}
            oldLabel="Current"
            newLabel="Your changes"
          />
        </div>
      {/if}

    {:else}
      <!-- Review step: diff card + metadata card -->
      <div class="review-step">
        <div class="review-card">
          <div class="review-card-header muted">Your changes</div>
          <DiffView
            oldText={doc.body}
            newText={proposedContent}
            oldLabel="Current"
            newLabel="Proposed"
          />
        </div>

        <div class="review-card">
          <div class="review-card-header">Describe your proposal</div>
          <div class="review-card-body">
            <div class="field">
              <label for="amendment-title">Summary of changes</label>
              <input id="amendment-title" type="text" bind:value={title} disabled={submitting} placeholder="e.g. Update conflict resolution process" />
            </div>

            <div class="field">
              <label for="amendment-desc">Why are you proposing this? <span class="muted">(optional)</span></label>
              <textarea id="amendment-desc" bind:value={description} rows="4" disabled={submitting} placeholder="Help others understand why this change matters."></textarea>
            </div>

            <div class="review-actions">
              <button class="btn btn-primary" onclick={handleSubmit} disabled={submitting || !title.trim()}>
                {submitting ? 'Submitting...' : 'Submit proposal'}
              </button>
              <button class="btn btn-secondary" onclick={backToEditing} disabled={submitting}>
                Back to editing
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

  {/if}
</div>

<style>
  .amendment-editor {
    max-width: 1000px;
  }

  .editor-header {
    margin-bottom: 1.5rem;
  }

  .editor-context {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .editor-header h1 {
    font-size: 1.3rem;
    margin: 0.15rem 0 0.25rem;
  }

  .draft-banner {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.6rem 0.85rem;
    margin-bottom: 1rem;
    border: 1px solid color-mix(in srgb, var(--color-accent) 40%, var(--color-border));
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--color-accent) 8%, var(--color-surface));
    font-size: 0.82rem;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .draft-actions {
    display: flex;
    gap: 0.75rem;
  }

  /* Editor card: wraps textarea + action buttons together */
  .editor-card {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--color-surface);
  }

  .full-editor {
    width: 100%;
    min-height: 400px;
    padding: 0.75rem;
    border: none;
    border-bottom: 1px solid var(--color-border);
    font-family: 'SF Mono', 'Fira Code', 'Fira Mono', Menlo, Consolas, monospace;
    font-size: 0.85rem;
    line-height: 1.6;
    resize: vertical;
    background: var(--color-surface);
    color: var(--color-text);
    tab-size: 2;
  }

  .full-editor:focus {
    outline: none;
    box-shadow: inset 0 0 0 2px var(--color-primary);
  }

  .editor-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem;
    background: var(--color-bg);
  }

  .editor-actions {
    display: flex;
    gap: 0.75rem;
  }

  .diff-toggle {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.8rem;
    cursor: pointer;
    padding: 0;
  }

  .diff-toggle:hover {
    text-decoration: underline;
  }

  /* Diff preview below the card */
  .diff-preview {
    margin-top: 1rem;
  }

  /* Review step */
  .review-step {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .review-card {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .review-card-header {
    padding: 0.6rem 0.85rem;
    font-size: 0.82rem;
    font-weight: 600;
    background: var(--color-bg);
    border-bottom: 1px solid var(--color-border);
  }

  .review-card-body {
    padding: 1.25rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 1rem;
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
  }

  .review-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }

  .error-state {
    padding: 2rem 0;
    text-align: center;
  }

  @media (max-width: 640px) {
    .full-editor {
      min-height: 300px;
      font-size: 0.8rem;
    }

    .draft-banner {
      flex-direction: column;
      align-items: flex-start;
    }

    .editor-footer {
      flex-direction: column;
      gap: 0.75rem;
      align-items: flex-start;
    }
  }
</style>
