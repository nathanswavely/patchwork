<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
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
                </div>
              </a>
              <div class="doc-actions">
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
  }

  .doc-link {
    text-decoration: none;
    color: inherit;
    flex: 1;
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
    font-size: 0.8rem;
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
    flex-shrink: 0;
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
