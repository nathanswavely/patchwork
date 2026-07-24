<script>
  /**
   * Join sheet (CONTEXT.md "Join sheet", docs/adr/040): the statement shown
   * between clicking Join and standing as a member or requester. A lens
   * over what this viewer could already see — never a bypass of document
   * visibility (docs/adr/036) — and never a signature: no checkbox, joining
   * informed is the agreement.
   *
   * Presentational only. It fetches the patch's own published-charter list
   * (the governance endpoint already filters to what this viewer may see)
   * but leaves the actual join call, its toasts, and post-join state
   * refresh to the caller via onConfirm — those already exist and this
   * component has no reason to duplicate them.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import Modal from './Modal.svelte';

  let {
    open = false,
    onClose = () => {},
    onConfirm = () => {},
    slug = '',
    patchName = '',
    membershipPolicy = 'open',
    liningStatus = '',
    submitting = false,
  } = $props();

  let isApproval = $derived(membershipPolicy === 'approval_required');

  let message = $state('');
  let charters = $state([]);
  let loadingCharters = $state(true);

  $effect(() => {
    if (open && slug) {
      message = '';
      loadCharters();
    }
  });

  async function loadCharters() {
    loadingCharters = true;
    try {
      const data = await api(`nodes/${slug}/governance`);
      const docs = data.items || data || [];
      // Published charters only — the lining's own state gets its own line
      // above, so it never shows twice. The server already restricted this
      // list to what this viewer may see (docs/adr/036); the visibility
      // check here just keeps this section honest if that ever changes.
      charters = docs.filter((d) => d.kind !== 'lining' && d.visibility === 'public');
    } catch {
      charters = [];
    } finally {
      loadingCharters = false;
    }
  }

  function goTo(path) {
    return (e) => {
      e.preventDefault();
      onClose();
      navigate(path);
    };
  }

  function handleConfirm() {
    const trimmed = message.trim();
    onConfirm(isApproval && trimmed ? trimmed : undefined);
  }
</script>

<Modal open={open} label="Join {patchName}" onClose={onClose}>
  {#snippet children()}
    <h2 class="join-title">Join {patchName}</h2>

    <p class="join-line">
      {#if isApproval}
        Membership is admin-approved — this sends a request.
      {:else}
        Membership is open — joining makes you a member.
      {/if}
    </p>

    {#if liningStatus === 'diverged'}
      <p class="join-line">
        This patch has amended the lining —
        <a href="/patches/{slug}/governance" onclick={goTo(`/patches/${slug}/governance`)}>see the changes</a>.
      </p>
    {:else if liningStatus === 'pristine' || liningStatus === 'stale'}
      <p class="join-line">
        Starts from the quilt's <a href="/lining" onclick={goTo('/lining')}>lining</a>.
      </p>
    {/if}

    {#if !loadingCharters && charters.length > 0}
      <div class="join-charters">
        <span class="join-charters-label">Published charters</span>
        <ul class="join-charters-list">
          {#each charters as doc (doc.id)}
            <li>
              <a
                href="/patches/{slug}/governance/docs/{doc.id}"
                onclick={goTo(`/patches/${slug}/governance/docs/${doc.id}`)}
              >{doc.title}</a>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if isApproval}
      <label class="join-message-label" for="join-message">Introduce yourself to the admins (optional)</label>
      <textarea
        id="join-message"
        class="join-message-input"
        bind:value={message}
        maxlength="500"
        rows="3"
        placeholder="A line or two about why you'd like to join…"
      ></textarea>
    {/if}

    <div class="join-actions">
      <button class="btn btn-secondary" onclick={onClose} disabled={submitting}>Cancel</button>
      <button class="btn btn-primary" onclick={handleConfirm} disabled={submitting}>
        {isApproval ? 'Request to join' : 'Join'}
      </button>
    </div>
  {/snippet}
</Modal>

<style>
  .join-title {
    font-size: 1.15rem;
    font-weight: 700;
    margin-bottom: 0.75rem;
    padding-right: 1.5rem;
  }

  .join-line {
    font-size: 0.88rem;
    color: var(--color-text);
    line-height: 1.6;
    margin-bottom: 0.6rem;
  }

  .join-line a {
    color: var(--color-primary);
  }

  .join-charters {
    margin: 0.75rem 0;
    padding-top: 0.75rem;
    border-top: 1px solid var(--color-border);
  }

  .join-charters-label {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 0.4rem;
  }

  .join-charters-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .join-charters-list a {
    font-size: 0.85rem;
    color: var(--color-primary);
    text-decoration: none;
  }

  .join-charters-list a:hover {
    text-decoration: underline;
  }

  .join-message-label {
    display: block;
    font-size: 0.82rem;
    font-weight: 500;
    margin: 0.75rem 0 0.4rem;
  }

  .join-message-input {
    width: 100%;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    font-family: inherit;
    font-size: 0.85rem;
    resize: vertical;
  }

  .join-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 1.25rem;
  }
</style>
