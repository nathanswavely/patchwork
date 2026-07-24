<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { getParams, navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import DiffView from '../components/DiffView.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  let params = $derived(getParams());
  let docId = $derived(params.id);

  let doc = $state(null);
  let loading = $state(true);
  let error = $state('');

  let versions = $state([]);
  let expandedVersion = $state(-1);
  let diffData = $state(null);
  let diffLoading = $state(false);

  // Comparison tool state.
  let compareFrom = $state('');
  let compareTo = $state('');
  let compareDiff = $state(null);
  let compareLoading = $state(false);

  $effect(() => {
    if (docId) loadDoc();
  });

  $effect(() => {
    if (doc) {
      patch.value.setBreadcrumbExtra?.([
        { label: doc.title, href: `/patches/${slug}/governance/docs/${docId}` },
        { label: 'History' },
      ]);
    }
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  async function loadDoc() {
    loading = true;
    error = '';
    try {
      doc = await api(`governance/${docId}`);
      const data = await api(`governance/${docId}/versions`);
      versions = data.items || [];
    } catch (e) {
      error = e.message || 'Failed to load document';
      doc = null;
    } finally {
      loading = false;
    }
  }

  async function toggleDiff(i) {
    if (expandedVersion === i) {
      expandedVersion = -1;
      diffData = null;
      return;
    }

    if (i >= versions.length - 1) {
      // First version — nothing to diff against.
      expandedVersion = i;
      diffData = null;
      return;
    }

    expandedVersion = i;
    diffLoading = true;
    try {
      const fromSHA = versions[i + 1].sha;
      const toSHA = versions[i].sha;
      diffData = await api(`governance/${docId}/diff?from=${fromSHA}&to=${toSHA}`);
    } catch {
      diffData = null;
    } finally {
      diffLoading = false;
    }
  }

  async function handleCompare() {
    if (!compareFrom || !compareTo || compareFrom === compareTo) return;
    compareLoading = true;
    compareDiff = null;
    try {
      compareDiff = await api(`governance/${docId}/diff?from=${compareFrom}&to=${compareTo}`);
    } catch {
      compareDiff = null;
    } finally {
      compareLoading = false;
    }
  }

  function formatDate(dateStr) {
    if (!dateStr) return '';
    return new Date(dateStr).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    {#if loading}
      <div style="padding: 2rem 0;">
        <Skeleton lines={1} height="2rem" width="60%" />
        <Skeleton lines={4} height="0.9rem" />
      </div>
    {:else if error}
      <ErrorState message={error} retry={loadDoc} />
    {:else if doc}
      <div class="history-page">
        <div class="history-header">
          <h1>{doc.title}</h1>
          <p class="muted">Version history &middot; {versions.length} version{versions.length !== 1 ? 's' : ''}</p>
        </div>

        {#if versions.length > 0}
          <div class="version-timeline">
            {#each versions as ver, i}
              <div class="version-entry" class:expanded={expandedVersion === i}>
                <div class="version-dot" class:current={i === 0}></div>
                <div class="version-content">
                  <button class="version-toggle" onclick={() => toggleDiff(i)}>
                    <div class="version-main">
                      <span class="version-number">v{ver.version_number}</span>
                      <span class="version-message">{ver.message || 'No description'}</span>
                    </div>
                    <div class="version-meta">
                      <span>{ver.author_name || 'System'}</span>
                      <span>{formatDate(ver.date)}</span>
                      {#if i < versions.length - 1}
                        <span class="diff-link">{expandedVersion === i ? 'Hide diff' : 'View diff'}</span>
                      {/if}
                    </div>
                  </button>

                  {#if expandedVersion === i}
                    <div class="version-diff">
                      {#if i >= versions.length - 1}
                        <p class="muted">This is the initial version.</p>
                      {:else if diffLoading}
                        <Skeleton lines={3} height="0.8rem" />
                      {:else if diffData?.old_content != null && diffData?.new_content != null}
                        <DiffView
                          oldText={diffData.old_content}
                          newText={diffData.new_content}
                          oldLabel="v{versions[i + 1].version_number}"
                          newLabel="v{ver.version_number}"
                        />
                      {:else if diffData?.unified}
                        <pre class="unified-diff">{diffData.unified}</pre>
                      {:else}
                        <p class="muted">Diff not available.</p>
                      {/if}
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>

          <!-- Compare any two versions -->
          {#if versions.length > 1}
            <div class="compare-section">
              <h2>Compare versions</h2>
              <div class="compare-controls">
                <select bind:value={compareFrom}>
                  <option value="">From...</option>
                  {#each versions as ver}
                    <option value={ver.sha}>v{ver.version_number}</option>
                  {/each}
                </select>
                <span class="compare-arrow">to</span>
                <select bind:value={compareTo}>
                  <option value="">To...</option>
                  {#each versions as ver}
                    <option value={ver.sha}>v{ver.version_number}</option>
                  {/each}
                </select>
                <button class="btn btn-secondary btn-sm" onclick={handleCompare} disabled={!compareFrom || !compareTo || compareFrom === compareTo || compareLoading}>
                  {compareLoading ? 'Loading...' : 'Compare'}
                </button>
              </div>

              {#if compareDiff}
                <div class="compare-result">
                  {#if compareDiff.old_content != null && compareDiff.new_content != null}
                    <DiffView
                      oldText={compareDiff.old_content}
                      newText={compareDiff.new_content}
                      oldLabel="Older"
                      newLabel="Newer"
                    />
                  {:else if compareDiff.unified}
                    <pre class="unified-diff">{compareDiff.unified}</pre>
                  {:else}
                    <p class="muted">No differences found.</p>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}
        {:else}
          <p class="muted" style="padding: 2rem 0; text-align: center;">No version history available.</p>
        {/if}

        <div class="history-actions">
          <button class="btn btn-secondary" onclick={() => navigate(`/patches/${slug}/governance/docs/${docId}`)}>
            Back to document
          </button>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .history-page {
    padding-top: 1rem;
  }

  .history-header {
    margin-bottom: 1.5rem;
  }

  .history-header h1 {
    font-size: 1.2rem;
    margin-bottom: 0.15rem;
  }

  /* Timeline */
  .version-timeline {
    position: relative;
    padding-left: 1.5rem;
  }

  .version-timeline::before {
    content: '';
    position: absolute;
    left: 5px;
    top: 8px;
    bottom: 8px;
    width: 2px;
    background: var(--color-border);
  }

  .version-entry {
    position: relative;
    margin-bottom: 0.25rem;
  }

  .version-dot {
    position: absolute;
    left: -1.5rem;
    top: 0.65rem;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--color-surface);
    border: 2px solid var(--color-border);
    z-index: 1;
  }

  .version-dot.current {
    background: var(--color-primary);
    border-color: var(--color-primary);
  }

  .version-toggle {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: none;
    cursor: pointer;
    text-align: left;
    transition: background 100ms ease;
  }

  .version-toggle:hover {
    background: var(--color-overlay);
  }

  .version-entry.expanded .version-toggle {
    background: var(--color-overlay);
    border-color: var(--color-border);
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
  }

  .version-main {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .version-number {
    font-weight: 700;
    font-size: 0.85rem;
    color: var(--color-text);
    flex-shrink: 0;
  }

  .version-message {
    font-size: 0.88rem;
    color: var(--color-text);
  }

  .version-meta {
    display: flex;
    gap: 0.6rem;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .diff-link {
    color: var(--color-primary);
  }

  .version-diff {
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-top: none;
    border-radius: 0 0 var(--radius) var(--radius);
    background: var(--color-bg);
    font-size: 0.85rem;
  }

  /* Compare */
  .compare-section {
    margin-top: 2rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--color-border);
  }

  .compare-section h2 {
    font-size: 0.88rem;
    font-weight: 600;
    margin-bottom: 0.75rem;
  }

  .compare-controls {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .compare-controls select {
    flex: 1;
    max-width: 150px;
  }

  .compare-arrow {
    color: var(--color-text-muted);
    font-size: 0.82rem;
  }

  .compare-result {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .unified-diff {
    padding: 0.75rem;
    font-size: 0.8rem;
    overflow-x: auto;
    white-space: pre-wrap;
  }

  .history-actions {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  @media (max-width: 640px) {
    .compare-controls {
      flex-wrap: wrap;
    }

    .compare-controls select {
      max-width: none;
    }
  }
</style>
