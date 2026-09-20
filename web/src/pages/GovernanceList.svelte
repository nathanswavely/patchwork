<script>
  import { getContext } from 'svelte';
  import { formatDay } from '../lib/datetime.js';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import GovernanceShell from '../components/GovernanceShell.svelte';
  import AdoptedElsewhere from '../components/AdoptedElsewhere.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let isAdmin = $derived(patch.value.isAdmin);
  let isUnclaimed = $derived(patch.value.isUnclaimed);
  let membershipRole = $derived(patch.value.membershipRole);
  // `follower_permissions.charters` is deliberately not read on this page.
  // It grants a follower this patch's *members-only* documents
  // (docs/adr/036); it never withholds what the patch published. This page
  // used to refuse a follower the whole room over it — so a follower, who
  // has identified themselves, saw less than a passer-by, the exact
  // inversion docs/adr/050 ruled out. The server filters the listing per
  // document; the page shows whatever came back.

  let docs = $state([]);
  // Whether this listing held only what the patch published to everyone.
  // The server says it about the viewer, not about the documents, so it
  // discloses nothing about what is being withheld.
  let publishedOnly = $state(false);
  // Whether this patch decides its proposals elsewhere (docs/adr/052).
  // Only used to signpost: a patch that meets in a room has two doors here
  // and no label saying which is which.
  let decidesElsewhere = $state(false);
  let loading = $state(true);
  let error = $state('');

  // Unclaimed patches carry no governance (docs/adr/039) — this route is
  // already unreachable through the shell's own guard, but a bookmarked
  // or shared link should still bounce rather than show an empty
  // document list.
  $effect(() => {
    if (slug && isUnclaimed) {
      navigate(`/patches/${slug}/events`);
      return;
    }
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
      publishedOnly = data.published_only === true;
    } catch (e) {
      error = e.message || 'Failed to load governance documents';
      docs = [];
    } finally {
      loading = false;
    }
    try {
      const rules = await api(`nodes/${slug}/governance/rules`);
      decidesElsewhere = (rules?.proposal_venue || 'patchwork') === 'elsewhere';
    } catch {
      decidesElsewhere = false;
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
            <!-- Two doors on this page, and on a patch that decides in a
                 room they are easy to mistake for each other (F-057): the
                 minutes of a meeting are a document, while recording an
                 adopted text replaces a charter with the version a meeting
                 passed. Neither button can say that on its face. -->
            {#if decidesElsewhere}
              <p class="muted section-hint">
                Minutes of a meeting go in a document. Recording an adopted
                text, below, replaces a charter with the version a meeting
                adopted.
              </p>
            {/if}
          {:else if publishedOnly}
            <!-- What this listing is, said once, so the empty state below
                 only has to say which kind of empty it is. -->
            <p class="muted section-hint">
              Members-only documents are not listed here.
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
        <!-- "This patch has published nothing" and "you are not being shown
             what it has" are different facts and a reader must be able to
             tell which they are looking at. Neither sentence says whether
             anything is being withheld — that would be the disclosure the
             per-document choice exists to prevent (docs/adr/036). -->
        <p class="muted" style="padding: 2rem 0; text-align: center;">
          {publishedOnly ? 'Nothing published yet.' : 'No governance documents yet.'}
        </p>
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
                  <span class="muted">Updated {formatDay(doc.updated_at)}</span>
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

      <!-- Texts a meeting adopted (docs/adr/053). Here as well as on each
           charter because this is the only place a document Patchwork does
           not have yet can be named — a meeting can adopt a charter this
           instance was never templated with, and refusing it would mean a
           community may only record amendments to documents Patchwork
           happened to guess at. Renders nothing on a patch that votes here. -->
      <AdoptedElsewhere {slug} {isAdmin} onRecorded={loadDocs} />
    </div>
  </div>
  {/snippet}
</GovernanceShell>

<style>
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
