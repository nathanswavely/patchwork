<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { getParams, navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let membershipRole = $derived(patch.value.membershipRole);

  let docId = $derived(getParams().id || '');
  let doc = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    if (docId) {
      loadDoc();
    }
  });

  $effect(() => {
    if (doc) {
      patch.value.setBreadcrumbExtra?.([{ label: doc.title }]);
    }
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  async function loadDoc() {
    loading = true;
    error = '';
    try {
      doc = await api(`governance/${docId}`);
    } catch (e) {
      error = e.message || 'Failed to load document';
      doc = null;
    } finally {
      loading = false;
    }
  }

</script>

<div class="page-fade">
  <div class="container-narrow">
    {#if loading}
      <div style="padding: 2rem 0;">
        <Skeleton lines={1} height="2rem" width="60%" />
        <div style="margin-top: 1rem;">
          <Skeleton lines={6} height="0.9rem" />
        </div>
      </div>
    {:else if error}
      <ErrorState message={error} retry={loadDoc} />
    {:else if doc}
      <div style="padding-top: 2rem;">
        <div class="doc-header">
          <div>
            <h1>{doc.title}</h1>
            <div class="doc-meta">
              <span class="badge">v{doc.version}</span>
              <span class="muted">Last updated {new Date(doc.updated_at).toLocaleDateString()}</span>
              {#if isMember && doc.visibility !== 'public'}
                <!-- Only members see this doc at all; the chip tells them the
                     public can't (docs/adr/035). -->
                <span class="vis-chip">Members only</span>
              {/if}
            </div>
          </div>
          <div class="doc-actions">
            <a
              href="/patches/{slug}/governance/docs/{doc.id}/history"
              class="btn btn-link"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs/${doc.id}/history`); }}
            >
              History ({doc.version} versions)
            </a>
            {#if isLoggedIn() && isMember && membershipRole !== 'follower'}
              <a
                href="/patches/{slug}/governance/docs/{doc.id}/propose"
                class="btn btn-primary"
                onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs/${doc.id}/propose`); }}
              >
                Propose change
              </a>
            {/if}
          </div>
        </div>

        <div class="doc-body">
          <MarkdownRenderer content={doc.body} />
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .doc-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 1.5rem;
  }

  .doc-header h1 {
    margin-bottom: 0.5rem;
  }

  .doc-meta {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    font-size: 0.85rem;
    flex-wrap: wrap;
  }

  .vis-chip {
    font-size: 0.7rem;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    padding: 0.1rem 0.4rem;
    border-radius: 999px;
    border: 1px solid var(--color-border);
    color: var(--color-text-muted);
  }

  .doc-body {
    line-height: 1.8;
    padding: 1.25rem;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
  }

  .doc-body :global(strong) {
    font-weight: 700;
  }

  .doc-body :global(em) {
    font-style: italic;
  }

  .doc-body :global(ul) {
    padding-left: 1.5rem;
    margin: 0.5rem 0;
  }

  .doc-body :global(p) {
    margin: 0.5rem 0;
  }

  .doc-actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-shrink: 0;
  }

  .btn-link {
    background: none;
    border: none;
    color: var(--color-primary);
    font-size: 0.85rem;
    text-decoration: underline;
    cursor: pointer;
    padding: 0;
  }

  .btn-link:hover {
    color: var(--color-primary);
    opacity: 0.8;
  }

  @media (max-width: 640px) {
    .doc-header {
      flex-direction: column;
      gap: 1rem;
    }

    .doc-actions {
      flex-wrap: wrap;
    }
  }
</style>
