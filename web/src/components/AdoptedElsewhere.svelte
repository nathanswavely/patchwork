<script>
  import { api } from '../lib/api.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from './PasskeyNotice.svelte';
  import { formatDay } from '../lib/datetime.js';

  // Texts a meeting adopted (docs/adr/053). Readable by anyone who can read
  // the charter itself: the record's whole value is that the people who were
  // in the room can check it.
  //
  // Two mounts, one component. On a charter's own page `docId` is set and this
  // shows that document's record and prefills the form to it. On the documents
  // index `docId` is empty, the list covers every document, and the form can
  // name one Patchwork does not have yet — a meeting can adopt a charter this
  // instance was never templated with.
  let {
    slug = '',
    docId = '',
    docTitle = '',
    // The git filename, which is the identity linking a charter to its file
    // (docs/adr/011) and what the list filters on. Taken from the document
    // payload rather than re-derived from the title here: slugifying is the
    // server's rule, and a second copy of it in JavaScript would drift.
    docFilename = '',
    isAdmin = false,
    onRecorded = () => {},
  } = $props();

  let records = $state([]);
  let loading = $state(true);
  let hasPasskey = $state(true);
  // Whether this patch decides its proposals elsewhere. Recording is offered
  // only where it does; without that gate "the community adopted this" becomes
  // a button beside a vote it could be used to go around. Read here rather
  // than passed in, so every mount is gated by the same fetch.
  let decidesElsewhere = $state(false);

  $effect(() => {
    if (slug) load();
  });

  async function load() {
    loading = true;
    try {
      const q = docFilename ? `?doc=${encodeURIComponent(docFilename)}` : '';
      const data = await api(`nodes/${slug}/amendment-attestations${q}`);
      records = data.items || [];
    } catch {
      records = [];
    } finally {
      loading = false;
    }
    try {
      const rules = await api(`nodes/${slug}/governance/rules`);
      decidesElsewhere = (rules?.proposal_venue || 'patchwork') === 'elsewhere';
    } catch {
      decidesElsewhere = false;
    }
    if (isAdmin && decidesElsewhere) {
      stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
    }
  }

  let formOpen = $state(false);
  let title = $state('');
  let decidedAt = $state('');
  let summary = $state('');
  let adoptedBody = $state('');
  let saving = $state(false);
  let error = $state('');

  function openForm() {
    title = docTitle || '';
    decidedAt = '';
    summary = '';
    adoptedBody = '';
    error = '';
    formOpen = true;
  }

  async function save() {
    if (!decidedAt) {
      error = 'Say when it was adopted';
      return;
    }
    if (!adoptedBody.trim()) {
      error = 'Paste the text the meeting adopted';
      return;
    }
    if (!docId && !title.trim()) {
      error = 'Name the document that was adopted';
      return;
    }
    saving = true;
    error = '';
    try {
      await withStepUp(() =>
        api(`nodes/${slug}/amendment-attestations`, {
          method: 'POST',
          body: {
            doc_id: docId || undefined,
            title: docId ? undefined : title.trim(),
            decided_at: decidedAt,
            summary: summary.trim(),
            adopted_body: adoptedBody,
          },
        })
      );
      formOpen = false;
      await load();
      onRecorded();
    } catch (e) {
      error = e instanceof PasskeyRequiredError
        ? 'Recording an adopted text needs a passkey. Enroll one in Security settings first.'
        : e.message || 'Failed to record';
    } finally {
      saving = false;
    }
  }
</script>

<!-- Self-gating, like the other panels that only apply to some patches: a
     patch that votes in Patchwork and has never recorded an adoption has
     nothing to say here, and a heading over an empty explanation on every
     charter page is noise. Records still show after a patch switches back,
     because a record of what happened does not stop being true. -->
{#if !loading && (decidesElsewhere || records.length > 0)}
<div class="adopted">
  <h4>Adopted elsewhere</h4>

  {#if records.length === 0}
    <p class="muted small">Nothing recorded yet.</p>
  {:else}
    <ul class="records">
      {#each records as r}
        <li>
          <p class="head">
            {#if !docId}<strong>{r.doc_title}</strong> ·{/if}
            <!-- The separator is an expression, not literal leading
                 whitespace: Svelte trims the space at the head of an {#if}
                 body, and "Mar 14, 2026· recorded by" is what that looks
                 like on the page. -->
            Adopted {formatDay(r.decided_at)}{#if r.recorder_name}{' · '}recorded by {r.recorder_name}{/if}
          </p>
          {#if r.summary}<p class="summary">{r.summary}</p>{/if}
          {#if r.text_hidden}
            <p class="muted small">The adopted text is visible to members.</p>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if isAdmin && decidesElsewhere}
    {#if formOpen}
      <div class="form">
        <p class="form-head">Record an adopted text</p>

        {#if docId}
          <p class="muted small">Recording an adoption of <strong>{docTitle}</strong>.</p>
        {:else}
          <label for="ad-title">Which document</label>
          <input id="ad-title" type="text" bind:value={title} disabled={saving}
                 placeholder="Bylaws" />
          <p class="muted small">
            If this patch has no document by that name, one is created.
          </p>
        {/if}

        <label for="ad-date">When it was adopted</label>
        <input id="ad-date" type="date" bind:value={decidedAt} disabled={saving} />

        <label for="ad-summary">What happened</label>
        <input id="ad-summary" type="text" bind:value={summary} disabled={saving}
               placeholder="Amended at the annual meeting" />

        <label for="ad-body">The text as adopted</label>
        <textarea id="ad-body" rows="12" bind:value={adoptedBody} disabled={saving}
                  placeholder="Paste the whole document as it now reads"></textarea>

        <p class="form-note muted">
          This replaces the whole document. Paste it as it now reads, not just
          the part that changed — Patchwork's copy is being corrected, not
          amended.
        </p>

        <PasskeyNotice show={!hasPasskey} action="record an adopted text" />
        {#if error}<p class="err">{error}</p>{/if}

        <div class="form-actions">
          <button class="btn btn-primary btn-sm" onclick={save} disabled={saving}>
            {saving ? 'Recording…' : 'Record'}
          </button>
          <button class="btn btn-sm btn-ghost" onclick={() => { formOpen = false; }} disabled={saving}>
            Cancel
          </button>
        </div>
      </div>
    {:else}
      <div class="actions">
        <button class="btn btn-sm" onclick={openForm}>Record an adopted text</button>
      </div>
    {/if}
  {/if}
</div>
{/if}

<style>
  .adopted {
    margin-top: 1.25rem;
    padding-top: 0.85rem;
    border-top: 1px solid var(--color-border);
  }

  h4 {
    font-size: 0.85rem;
    margin: 0 0 0.5rem;
  }

  .small { font-size: 0.8rem; }

  .records {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .head {
    font-size: 0.82rem;
    margin: 0;
    color: var(--color-text-muted);
  }

  .summary {
    font-size: 0.85rem;
    margin: 0.15rem 0 0;
  }

  .actions {
    margin-top: 0.6rem;
  }

  .form {
    margin-top: 0.7rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .form-head { font-size: 0.85rem; font-weight: 600; margin: 0 0 0.2rem; }

  .form label {
    font-size: 0.78rem;
    color: var(--color-text-muted);
  }

  .form textarea {
    font-family: var(--font-mono, monospace);
    font-size: 0.82rem;
    line-height: 1.5;
    resize: vertical;
  }

  .form-note { font-size: 0.76rem; margin: 0.2rem 0 0; }

  .form-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.3rem;
  }

  .err {
    font-size: 0.8rem;
    color: var(--color-danger, #c0392b);
    margin: 0.3rem 0 0;
  }
</style>
