<script>
  import { api } from '../lib/api.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from './PasskeyNotice.svelte';
  import { formatDay } from '../lib/datetime.js';

  // Decisions a community made somewhere Patchwork was not (docs/adr/052).
  // Shown to everyone: an attestation's whole value is that the people who
  // were in the room can check it.
  // `hasTerms` is the elected model only — the one model with terms
  // (docs/adr/051). Everywhere else the term field is neither shown nor sent.
  let { slug = '', isAdmin = false, hasTerms = false } = $props();

  let records = $state([]);
  let loading = $state(true);
  let members = $state([]);
  let hasPasskey = $state(true);

  // The record in force is the one nothing has superseded. Earlier ones stay
  // readable — a correction does not erase what it corrects.
  let current = $derived(records.find((r) => !r.superseded_by) || null);
  let earlier = $derived(records.filter((r) => r.superseded_by));

  // A term that has run out does not remove anybody (docs/adr/051): the
  // council keeps serving until a successor is elected. What it changes is
  // that the patch is visibly overdue, which is the accountability the
  // charter's "power rotates" is asking for.
  let termLapsed = $derived.by(() => {
    const t = current?.term_ends_at;
    if (!t) return false;
    const parts = /^(\d{4})-(\d{2})-(\d{2})$/.exec(t);
    const end = parts
      ? new Date(Number(parts[1]), Number(parts[2]) - 1, Number(parts[3]))
      : new Date(t);
    return end < new Date();
  });

  $effect(() => {
    if (slug) load();
  });

  async function load() {
    loading = true;
    try {
      const data = await api(`nodes/${slug}/attestations`);
      records = data.items || [];
    } catch {
      records = [];
    } finally {
      loading = false;
    }
    if (isAdmin) {
      try {
        const m = await api(`nodes/${slug}/members`);
        members = (m.items || m || []).filter((x) => x.role === 'admin' || x.role === 'member');
      } catch {
        members = [];
      }
      stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
    }
  }

  // ---- recording -----------------------------------------------------------

  let formOpen = $state(false);
  let decidedAt = $state('');
  let termEndsAt = $state('');
  let summary = $state('');
  let rows = $state([{ userId: '', name: '' }]);
  let correcting = $state('');
  let saving = $state(false);
  let error = $state('');

  function openForm(correctID = '') {
    correcting = correctID;
    decidedAt = '';
    termEndsAt = '';
    summary = '';
    // Correcting starts from what the record already says, so a one-name fix
    // doesn't mean retyping the council.
    const base = correctID ? records.find((r) => r.id === correctID) : null;
    if (base) {
      decidedAt = base.decided_at || '';
      termEndsAt = base.term_ends_at || '';
      summary = base.summary || '';
      rows = base.names.map((n) => ({ userId: n.user_id || '', name: n.display_name }));
      if (!rows.length) rows = [{ userId: '', name: '' }];
    } else {
      rows = [{ userId: '', name: '' }];
    }
    error = '';
    formOpen = true;
  }

  function addRow() {
    rows = [...rows, { userId: '', name: '' }];
  }

  function removeRow(i) {
    rows = rows.filter((_, idx) => idx !== i);
    if (!rows.length) rows = [{ userId: '', name: '' }];
  }

  async function save() {
    if (!decidedAt) {
      error = 'Say when the decision was made';
      return;
    }
    const names = rows
      .map((r) => ({ user_id: r.userId || undefined, display_name: r.name.trim() || undefined }))
      .filter((n) => n.user_id || n.display_name);
    if (!names.length) {
      error = 'Name at least one person the decision put in place';
      return;
    }
    saving = true;
    error = '';
    try {
      await withStepUp(() =>
        api(`nodes/${slug}/attestations`, {
          method: 'POST',
          body: {
            decided_at: decidedAt,
            term_ends_at: hasTerms && termEndsAt ? termEndsAt : undefined,
            summary: summary.trim(),
            supersedes_id: correcting || undefined,
            names,
          },
        })
      );
      formOpen = false;
      await load();
    } catch (e) {
      error = e instanceof PasskeyRequiredError
        ? 'Recording a decision needs a passkey. Enroll one in Security settings first.'
        : e.message || 'Failed to record';
    } finally {
      saving = false;
    }
  }

  // ---- linking an unrealized name -----------------------------------------

  let linking = $state('');
  let linkChoice = $state('');
  let linkError = $state('');

  function startLink(nameID) {
    linking = nameID;
    linkChoice = '';
    linkError = '';
  }

  async function saveLink(nameID) {
    if (!linkChoice) return;
    try {
      await withStepUp(() =>
        api(`nodes/${slug}/attestation-names/${nameID}`, { method: 'PATCH', body: { user_id: linkChoice } })
      );
      linking = '';
      await load();
    } catch (e) {
      linkError = e instanceof PasskeyRequiredError
        ? 'Linking a name needs a passkey. Enroll one in Security settings first.'
        : e.message || 'Failed to link';
    }
  }
</script>

<div class="records">
  <h4>Decisions recorded here</h4>

  {#if loading}
    <p class="muted small">Loading…</p>
  {:else if !current}
    <p class="muted small">Nothing recorded yet.</p>
  {:else}
    <div class="record">
      <p class="record-head">
        <!-- The separator is an expression, not literal leading whitespace:
             Svelte trims the space at the head of an {#if} body, so this
             rendered as "Mar 14, 2026· recorded by". -->
        Decided {formatDay(current.decided_at)}{#if current.recorder_name}{' · '}recorded by {current.recorder_name}{/if}
      </p>
      {#if current.term_ends_at}
        <p class="term-line" class:lapsed={termLapsed}>
          {#if termLapsed}
            Term ended {formatDay(current.term_ends_at)}. The council serves until a successor is elected.
          {:else}
            Term ends {formatDay(current.term_ends_at)}
          {/if}
        </p>
      {/if}
      {#if current.summary}<p class="record-summary">{current.summary}</p>{/if}

      <ul class="names">
        {#each current.names as n}
          <li class:unrealized={!n.realized}>
            <span class="name">{n.display_name}</span>
            {#if !n.realized}
              <span class="tag">hasn't joined</span>
              {#if isAdmin}
                {#if linking === n.id}
                  <span class="link-row">
                    <select bind:value={linkChoice}>
                      <option value="">Choose a member</option>
                      {#each members as m}
                        <option value={m.user_id}>{m.display_name || m.username}</option>
                      {/each}
                    </select>
                    <button class="btn btn-sm" onclick={() => saveLink(n.id)} disabled={!linkChoice}>Link</button>
                    <button class="btn btn-sm btn-ghost" onclick={() => { linking = ''; }}>Cancel</button>
                  </span>
                {:else}
                  <button class="btn btn-sm btn-ghost" onclick={() => startLink(n.id)}>This is…</button>
                {/if}
              {/if}
            {/if}
          </li>
        {/each}
      </ul>
      {#if linkError}<p class="err">{linkError}</p>{/if}
    </div>
  {/if}

  {#if earlier.length > 0}
    <details class="earlier">
      <summary>{earlier.length} earlier record{earlier.length > 1 ? 's' : ''}</summary>
      {#each earlier as r}
        <div class="record past">
          <p class="record-head">
            Decided {formatDay(r.decided_at)} · corrected
          </p>
          {#if r.summary}<p class="record-summary">{r.summary}</p>{/if}
          <ul class="names">
            {#each r.names as n}
              <li class:unrealized={!n.realized}><span class="name">{n.display_name}</span></li>
            {/each}
          </ul>
        </div>
      {/each}
    </details>
  {/if}

  {#if isAdmin}
    {#if formOpen}
      <div class="form">
        <p class="form-head">{correcting ? 'Correct the record' : 'Record a decision'}</p>

        <label for="att-date">When it was decided</label>
        <input id="att-date" type="date" bind:value={decidedAt} disabled={saving} />

        {#if hasTerms}
          <label for="att-term">When this council's term ends</label>
          <input id="att-term" type="date" bind:value={termEndsAt} disabled={saving} />
        {/if}

        <label for="att-summary">What happened</label>
        <input id="att-summary" type="text" bind:value={summary} disabled={saving}
               placeholder="Annual meeting elected the council" />

        <span class="names-label">Who it put in place</span>
        {#each rows as row, i}
          <div class="name-row">
            <select bind:value={row.userId} disabled={saving}>
              <option value="">Someone not here yet</option>
              {#each members as m}
                <option value={m.user_id}>{m.display_name || m.username}</option>
              {/each}
            </select>
            {#if !row.userId}
              <input type="text" bind:value={row.name} disabled={saving} placeholder="Their name" />
            {/if}
            <button class="btn btn-sm btn-ghost" onclick={() => removeRow(i)} disabled={saving}>Remove</button>
          </div>
        {/each}
        <button class="btn btn-sm btn-ghost" onclick={addRow} disabled={saving}>Add another</button>

        <p class="form-note muted">
          Anyone already here takes up the role. Anyone not here yet is recorded
          by name and holds nothing until they join.
        </p>

        <PasskeyNotice show={!hasPasskey} action="record a decision" />
        {#if error}<p class="err">{error}</p>{/if}

        <div class="form-actions">
          <button class="btn btn-primary btn-sm" onclick={save} disabled={saving}>
            {saving ? 'Recording…' : 'Record'}
          </button>
          <button class="btn btn-sm btn-ghost" onclick={() => { formOpen = false; }} disabled={saving}>Cancel</button>
        </div>
      </div>
    {:else}
      <div class="actions">
        <button class="btn btn-sm" onclick={() => openForm('')}>Record a decision</button>
        {#if current}
          <button class="btn btn-sm btn-ghost" onclick={() => openForm(current.id)}>Correct the last one</button>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .records {
    margin-top: 0.85rem;
    padding-top: 0.85rem;
    border-top: 1px solid var(--color-border);
  }

  h4 {
    font-size: 0.85rem;
    margin: 0 0 0.5rem;
  }

  .small { font-size: 0.82rem; }

  .record-head {
    font-size: 0.82rem;
    margin: 0;
    color: var(--color-text-muted);
  }

  .term-line {
    font-size: 0.82rem;
    margin: 0.2rem 0 0;
  }

  .term-line.lapsed {
    color: var(--color-accent, #b8860b);
    font-weight: 600;
  }

  .record-summary {
    font-size: 0.85rem;
    margin: 0.2rem 0 0;
  }

  .names {
    list-style: none;
    padding: 0;
    margin: 0.4rem 0 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .names li {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.4rem;
    font-size: 0.85rem;
  }

  .unrealized .name { color: var(--color-text-muted); }

  .tag {
    font-size: 0.72rem;
    padding: 0.05rem 0.35rem;
    border-radius: var(--radius);
    background: var(--color-overlay);
    color: var(--color-text-muted);
  }

  .link-row {
    display: flex;
    gap: 0.3rem;
    flex-wrap: wrap;
  }

  .earlier {
    margin-top: 0.6rem;
    font-size: 0.82rem;
  }

  .earlier summary { cursor: pointer; color: var(--color-text-muted); }

  .past { margin-top: 0.5rem; opacity: 0.8; }

  .actions {
    margin-top: 0.6rem;
    display: flex;
    gap: 0.4rem;
    flex-wrap: wrap;
  }

  .form {
    margin-top: 0.7rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .form-head { font-size: 0.85rem; font-weight: 600; margin: 0 0 0.2rem; }

  .form label,
  .names-label {
    font-size: 0.78rem;
    color: var(--color-text-muted);
  }

  .name-row {
    display: flex;
    gap: 0.35rem;
    flex-wrap: wrap;
  }

  .name-row select,
  .name-row input { flex: 1 1 8rem; min-width: 0; }

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
