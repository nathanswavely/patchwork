<script>
  /**
   * Admin editing for the legal documents (docs/adr/028): the privacy
   * policy and user agreement. Defaults ship with the software; editing
   * here replaces a document wholesale, and "restore default" returns to
   * the shipped template (which tracks the quilt's current name on its
   * own — a rename needs no edit here).
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import { ArrowSquareOut, ArrowCounterClockwise } from 'phosphor-svelte';

  let loading = $state(true);
  let error = $state('');
  let docs = $state([]);
  let active = $state('privacy');
  let drafts = $state({}); // path -> textarea contents
  let saving = $state(false);
  let resetting = $state(false);

  $effect(() => { load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      const data = await api('admin/legal');
      docs = data.docs || [];
      const next = {};
      for (const d of docs) next[d.doc] = d.markdown;
      drafts = next;
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  let current = $derived(docs.find((d) => d.doc === active));
  let dirty = $derived(current ? drafts[active] !== current.markdown : false);

  async function save() {
    saving = true;
    try {
      await api(`admin/legal/${active}`, {
        method: 'PUT',
        body: { markdown: drafts[active] },
      });
      showToast(`${current.title} saved, now live at /${active}`, 'success');
      await load();
    } catch (e) {
      showToast(e.message || 'Save failed', 'error');
    }
    saving = false;
  }

  async function restoreDefault() {
    if (!confirm(`Replace the custom ${current.title} with the default that ships with Patchwork? Your custom text will be gone.`)) {
      return;
    }
    resetting = true;
    try {
      await api(`admin/legal/${active}`, { method: 'DELETE' });
      showToast(`${current.title} restored to the shipped default`, 'success');
      await load();
    } catch (e) {
      showToast(e.message || 'Reset failed', 'error');
    }
    resetting = false;
  }

  function viewPublic(e) {
    e.preventDefault();
    navigate(`/${active}`);
  }
</script>

<div class="admin-legal page-fade">
  <header class="page-head">
    <h1>Legal documents</h1>
    <p class="muted">
      The privacy policy and user agreement, public at /privacy and /terms
      and linked from every signup form. Patchwork ships honest defaults
      that describe what the software actually does. Anything you write
      here replaces the default entirely, so keep it just as honest.
    </p>
  </header>

  {#if loading}
    <Skeleton lines={8} />
  {:else if error}
    <ErrorState message={error} retry={load} />
  {:else}
    <div class="doc-tabs" role="tablist">
      {#each docs as d (d.doc)}
        <button
          role="tab"
          aria-selected={active === d.doc}
          class="doc-tab"
          class:active={active === d.doc}
          onclick={() => { active = d.doc; }}
        >
          {d.title}
          <span class="doc-state" class:customized={d.customized}>
            {d.customized ? 'customized' : 'default'}
          </span>
        </button>
      {/each}
    </div>

    {#if current}
      <div class="doc-meta">
        {#if current.customized}
          <span class="muted">
            Custom text, last saved {current.updated_at?.slice(0, 10) || 'recently'}.
          </span>
        {:else}
          <span class="muted">
            The default that ships with Patchwork. It fills in this quilt's
            name by itself and starts with a note saying it's the default.
            Saving any edit removes that note along with it.
          </span>
        {/if}
        <a href={`/${active}`} onclick={viewPublic} class="view-link">
          <ArrowSquareOut size={14} weight="bold" /> View public page
        </a>
      </div>

      <textarea
        class="doc-editor"
        bind:value={drafts[active]}
        rows="24"
        spellcheck="true"
        aria-label={`${current.title} markdown`}
      ></textarea>
      <p class="hint muted">Markdown. Links like [the Label](/label) work.</p>

      <div class="doc-actions">
        <button class="btn btn-primary" onclick={save} disabled={saving || !dirty}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        {#if current.customized}
          <button class="btn btn-secondary" onclick={restoreDefault} disabled={resetting}>
            <ArrowCounterClockwise size={14} weight="bold" />
            {resetting ? 'Restoring…' : 'Restore shipped default'}
          </button>
        {/if}
        {#if dirty}
          <span class="muted unsaved">Unsaved changes</span>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .admin-legal {
    max-width: 760px;
  }

  .page-head {
    margin-bottom: 20px;
  }
  .page-head h1 {
    margin: 0 0 6px;
    font-size: 1.4rem;
  }
  .page-head p {
    margin: 0;
    font-size: 0.9rem;
    max-width: 60ch;
  }

  .doc-tabs {
    display: flex;
    gap: 6px;
    margin-bottom: 12px;
  }
  .doc-tab {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 0.9rem;
    cursor: pointer;
  }
  .doc-tab.active {
    border-color: var(--color-accent);
    font-weight: 600;
  }
  .doc-state {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 1px 6px;
    border-radius: 99px;
    background: var(--color-overlay);
    color: var(--color-text-muted);
  }
  .doc-state.customized {
    background: var(--color-accent);
    color: var(--color-surface);
  }

  .doc-meta {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    font-size: 0.85rem;
    margin-bottom: 8px;
  }
  .view-link {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
  }

  .doc-editor {
    width: 100%;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.85rem;
    line-height: 1.5;
    padding: 12px;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-surface);
    color: var(--color-text);
    resize: vertical;
  }
  .doc-editor:focus {
    outline: none;
    border-color: var(--color-accent);
  }

  .hint {
    font-size: 0.78rem;
    margin: 6px 0 14px;
  }

  .doc-actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .doc-actions .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .unsaved {
    font-size: 0.82rem;
  }
</style>
