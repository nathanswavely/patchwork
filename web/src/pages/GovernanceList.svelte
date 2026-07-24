<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import GovernanceShell from '../components/GovernanceShell.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);
  let followerPermissions = $derived(patch.value.followerPermissions);
  let permissionDenied = $derived(membershipRole === 'follower' && followerPermissions?.charters === false);

  let docs = $state([]);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    if (slug) {
      loadDocs();
    }
  });

  async function loadDocs() {
    loading = true;
    error = '';
    try {
      const data = await api(`nodes/${slug}/governance`);
      docs = data.items || data || [];
    } catch (e) {
      error = e.message || 'Failed to load governance documents';
      docs = [];
    } finally {
      loading = false;
    }
  }

  // Per-document visibility (docs/adr/036). Publishing is one click, but it is
  // the click that puts a document in front of the whole internet, so the
  // label says which way it goes rather than naming a state.
  let savingVisibility = $state('');

  async function setVisibility(doc, visibility) {
    savingVisibility = doc.id;
    try {
      await api(`governance/${doc.id}`, { method: 'PUT', body: { visibility } });
      doc.visibility = visibility;
      docs = docs;
      showToast(visibility === 'public' ? 'Published' : 'Hidden from visitors', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to change visibility', 'error');
    } finally {
      savingVisibility = '';
    }
  }
</script>

<GovernanceShell activeSection="documents">
  {#snippet children()}
{#if permissionDenied}
  <div class="permission-notice">
    <p>This content is only visible to members.</p>
    <p class="muted">Become a member to access documents.</p>
  </div>
{:else}
<div class="page-fade">
  <div>
      <div class="page-header">
        <div>
          <h2>Documents</h2>
          {#if isAdmin}
            <p class="muted section-hint">
              New documents start members only. Publish one to let visitors and
              other quilts read it.
            </p>
          {/if}
        </div>
        <div class="header-actions">
          {#if isLoggedIn() && isAdmin}
            <a
              href="/patches/{slug}/governance/docs/new"
              class="btn btn-primary"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs/new`); }}
            >
              New Document
            </a>
          {:else if !isMember || membershipRole === 'follower'}
            <p class="role-prompt muted">These documents define how this community makes decisions.</p>
          {/if}
        </div>
      </div>

      {#if loading}
        <p class="muted" style="padding: 2rem 0; text-align: center;">Loading...</p>
      {:else if error}
        <p class="error-text" style="padding: 2rem 0; text-align: center;">{error}</p>
      {:else if docs.length === 0}
        <p class="muted" style="padding: 2rem 0; text-align: center;">No governance documents yet.</p>
      {:else}
        <div class="doc-list">
          {#each docs as doc (doc.id)}
            <div class="doc-card card">
              <a
                href="/patches/{slug}/governance/docs/{doc.id}"
                class="doc-link"
                onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs/${doc.id}`); }}
              >
                <h3>{doc.title}</h3>
                <div class="doc-meta">
                  <span class="muted">v{doc.version}</span>
                  <span class="muted">Updated {new Date(doc.updated_at).toLocaleDateString()}</span>
                  {#if doc.kind === 'lining'}
                    <span
                      class="vis-chip lining"
                      title="The shared baseline every patch starts with. Always public; changed only by amendment."
                    >The lining</span>
                  {:else if isAdmin}
                    <span class="vis-chip" class:public={doc.visibility === 'public'}>
                      {doc.visibility === 'public' ? 'Public' : 'Members only'}
                    </span>
                  {/if}
                </div>
              </a>
              <div class="doc-actions">
                {#if isLoggedIn() && isAdmin && doc.kind !== 'lining'}
                  <button
                    class="btn btn-secondary btn-sm"
                    disabled={savingVisibility === doc.id}
                    onclick={() => setVisibility(doc, doc.visibility === 'public' ? 'members' : 'public')}
                  >
                    {doc.visibility === 'public' ? 'Make members only' : 'Publish'}
                  </button>
                {/if}
                <a
                  href="/patches/{slug}/governance/docs/{doc.id}/history"
                  class="history-link"
                  onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs/${doc.id}/history`); }}
                >
                  History
                </a>
                {#if isLoggedIn() && isMember && membershipRole !== 'follower'}
                  <a
                    href="/patches/{slug}/governance/docs/{doc.id}/propose"
                    class="btn btn-secondary btn-sm"
                    onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs/${doc.id}/propose`); }}
                  >
                    Propose change
                  </a>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}
  {/snippet}
</GovernanceShell>

<style>
  .permission-notice {
    text-align: center;
    padding: 3rem 1rem;
  }

  .permission-notice p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 1.5rem;
  }

  .role-prompt {
    font-size: 0.85rem;
  }

  .doc-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .doc-card {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.75rem;
  }

  /* The row now carries a third action, so it wraps as a block rather than
     squeezing the title into one word per line. */
  .doc-link {
    text-decoration: none;
    color: inherit;
    flex: 1 1 18rem;
  }

  .doc-link:hover {
    text-decoration: none;
  }

  .doc-link:hover h3 {
    color: var(--color-primary);
  }

  .doc-link h3 {
    font-size: 1rem;
    margin-bottom: 0.2rem;
    transition: color 150ms ease;
  }

  .doc-meta {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    font-size: 0.8rem;
    flex-wrap: wrap;
  }

  .section-hint {
    font-size: 0.8rem;
    margin-top: 0.2rem;
    max-width: 46ch;
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

  .vis-chip.public {
    border-color: color-mix(in srgb, var(--color-primary) 45%, transparent);
    color: var(--color-primary);
  }

  .vis-chip.lining {
    border-color: color-mix(in srgb, var(--color-primary) 45%, transparent);
    color: var(--color-primary);
    cursor: help;
  }

  .header-actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .doc-actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;
  }

  .history-link {
    font-size: 0.8rem;
    color: var(--color-primary);
    text-decoration: underline;
    white-space: nowrap;
  }

  .history-link:hover {
    opacity: 0.8;
  }

  @media (max-width: 640px) {
    .page-header {
      flex-direction: column;
      gap: 1rem;
    }
  }
</style>
