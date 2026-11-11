<script>
  import { api } from '../lib/api.js';
  import { allMembers } from '../lib/patchMembers.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from './PasskeyNotice.svelte';
  import { formatDay } from '../lib/datetime.js';
  import { navigate } from '../stores/router.svelte.js';

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

  // The record in force is the latest one nothing has corrected and nobody
  // has struck. Every other record stays readable below it: a correction
  // does not erase what it corrects, and a record entered in error is
  // crossed out rather than removed (F-078).
  let current = $derived(records.find((r) => !r.superseded_by && !r.entered_in_error) || null);
  let earlier = $derived(records.filter((r) => r !== current));

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
        // Every member, not the first page: a record can name anyone here.
        members = (await allMembers(slug)).filter((x) => x.role === 'admin' || x.role === 'member');
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
  // Who the decision removed (F-078). Optional, and it starts empty: most
  // meetings put a council in place without taking anybody off it.
  let downRows = $state([]);
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
      downRows = (base.stood_down || []).map((n) => ({ userId: n.user_id || '', name: n.display_name }));
    } else {
      rows = [{ userId: '', name: '' }];
      downRows = [];
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

  function addDownRow() {
    downRows = [...downRows, { userId: '', name: '' }];
  }

  function removeDownRow(i) {
    downRows = downRows.filter((_, idx) => idx !== i);
  }

  function namesFrom(list) {
    return list
      .map((r) => ({ user_id: r.userId || undefined, display_name: r.name.trim() || undefined }))
      .filter((n) => n.user_id || n.display_name);
  }

  async function save() {
    if (!decidedAt) {
      error = 'Say when the decision was made';
      return;
    }
    const names = namesFrom(rows);
    const stoodDown = namesFrom(downRows);
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
            stood_down: stoodDown,
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

  // ---- entered in error ----------------------------------------------------
  //
  // Crossing a record out (F-078). It stays listed and stops counting; where
  // it was the record in force, the one before it is in force again and
  // who holds admin follows it.

  let striking = $state('');
  let strikeError = $state('');

  async function strikeRecord(id) {
    strikeError = '';
    try {
      await withStepUp(() => api(`nodes/${slug}/attestations/${id}/strike`, { method: 'POST' }));
      striking = '';
      await load();
    } catch (e) {
      strikeError = e instanceof PasskeyRequiredError
        ? 'Marking a record entered in error needs a passkey. Enroll one in Security settings first.'
        : e.message || 'Failed to mark the record';
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
  <h4>Leadership decisions recorded here</h4>

  {#if loading}
    <p class="lt-muted small">Loading…</p>
  {:else if !current}
    <p class="lt-muted small">
      {records.length ? 'No record is in force. Every one here was entered in error.' : 'Nothing recorded yet.'}
    </p>
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
              <span class="lt-pill">hasn't joined</span>
              {#if isAdmin}
                {#if linking === n.id}
                  <span class="link-row">
                    <select class="lt-field" bind:value={linkChoice}>
                      <option value="">Choose a member</option>
                      {#each members as m}
                        <option value={m.user_id}>{m.display_name || m.username}</option>
                      {/each}
                    </select>
                    <button class="lt-btn lt-btn-sm" onclick={() => saveLink(n.id)} disabled={!linkChoice}>Link</button>
                    <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => { linking = ''; }}>Cancel</button>
                  </span>
                {:else}
                  <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => startLink(n.id)}>This is…</button>
                {/if}
              {/if}
            {/if}
          </li>
        {/each}
      </ul>
      {#if current.stood_down?.length}
        <p class="stood-down">Stood down: {current.stood_down.map((n) => n.display_name).join(', ')}</p>
      {/if}
      {#if linkError}<p class="err">{linkError}</p>{/if}
      {#if isAdmin}
        {@render strikeControl(current)}
      {/if}
    </div>
  {/if}
  {#if strikeError}<p class="err">{strikeError}</p>{/if}

  {#snippet strikeControl(r)}
    {#if striking === r.id}
      <div class="strike-confirm">
        <p class="form-note lt-muted">
          The record stays here, crossed out, and stops counting. If it is the one in force, the record before it takes its place.
        </p>
        <div class="form-actions">
          <button class="lt-btn lt-btn-sm lt-btn-danger" onclick={() => strikeRecord(r.id)}>Mark entered in error</button>
          <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => { striking = ''; }}>Cancel</button>
        </div>
      </div>
    {:else}
      <button class="lt-btn lt-btn-sm lt-btn-ghost strike-btn" onclick={() => { striking = r.id; strikeError = ''; }}>Entered in error</button>
    {/if}
  {/snippet}

  {#if earlier.length > 0}
    <details class="earlier">
      <summary>{earlier.length} earlier record{earlier.length > 1 ? 's' : ''}</summary>
      {#each earlier as r}
        <div class="record past" class:struck={r.entered_in_error}>
          <p class="record-head">
            Decided {formatDay(r.decided_at)}{' · '}{#if r.entered_in_error}entered in error{#if r.struck_by_name}, marked by {r.struck_by_name}{/if}{:else if r.superseded_by}corrected{:else}earlier decision{/if}
          </p>
          <div class="struck-body">
            {#if r.summary}<p class="record-summary">{r.summary}</p>{/if}
            <ul class="names">
              {#each r.names as n}
                <li class:unrealized={!n.realized}><span class="name">{n.display_name}</span></li>
              {/each}
            </ul>
            {#if r.stood_down?.length}
              <p class="stood-down">Stood down: {r.stood_down.map((n) => n.display_name).join(', ')}</p>
            {/if}
          </div>
          {#if isAdmin && !r.entered_in_error}
            {@render strikeControl(r)}
          {/if}
        </div>
      {/each}
    </details>
  {/if}

  {#if isAdmin}
    {#if formOpen}
      <div class="form">
        <p class="form-head">{correcting ? 'Correct the record' : 'Record a leadership decision'}</p>
        <!-- This records who holds the admin role and nothing else
             (docs/adr/052). Headed "Record a decision", it read as the
             place for any decision a meeting made (F-039). -->
        <p class="form-note lt-muted">
          This records who a meeting put in place as admins. Minutes and other
          decisions go in a document under
          <a href="/patches/{slug}/governance/docs" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs`); }}>Documents</a>.
        </p>

        <label for="att-date">When it was decided</label>
        <input class="lt-field" id="att-date" type="date" bind:value={decidedAt} disabled={saving} />

        {#if hasTerms}
          <label for="att-term">When this council's term ends</label>
          <input class="lt-field" id="att-term" type="date" bind:value={termEndsAt} disabled={saving} />
        {/if}

        <label for="att-summary">What happened</label>
        <input class="lt-field" id="att-summary" type="text" bind:value={summary} disabled={saving}
               placeholder="Annual meeting elected the council" />

        <span class="names-label">Who it put in place</span>
        {#each rows as row, i}
          <div class="name-row">
            <select class="lt-field" bind:value={row.userId} disabled={saving}>
              <option value="">Someone not here yet</option>
              {#each members as m}
                <option value={m.user_id}>{m.display_name || m.username}</option>
              {/each}
            </select>
            {#if !row.userId}
              <input class="lt-field" type="text" bind:value={row.name} disabled={saving} placeholder="Their name" />
            {/if}
            <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => removeRow(i)} disabled={saving}>Remove</button>
          </div>
        {/each}
        <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={addRow} disabled={saving}>Add another</button>

        <p class="form-note lt-muted">
          Anyone already here takes up the role. Anyone not here yet is recorded
          by name and holds nothing until they join.
        </p>

        <!-- The counterpart the form lacked (F-078): a meeting that took
             somebody out of the chair could only say so in the summary. -->
        <span class="names-label">Who stood down</span>
        {#each downRows as row, i}
          <div class="name-row">
            <select class="lt-field" bind:value={row.userId} disabled={saving}>
              <option value="">Someone not here</option>
              {#each members as m}
                <option value={m.user_id}>{m.display_name || m.username}</option>
              {/each}
            </select>
            {#if !row.userId}
              <input class="lt-field" type="text" bind:value={row.name} disabled={saving} placeholder="Their name" />
            {/if}
            <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => removeDownRow(i)} disabled={saving}>Remove</button>
          </div>
        {/each}
        <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={addDownRow} disabled={saving}>
          {downRows.length ? 'Add another' : 'Add someone who stood down'}
        </button>
        <p class="form-note lt-muted">
          Optional. An admin not put in place above stops holding the role either way. This puts their name on the record.
        </p>

        <PasskeyNotice show={!hasPasskey} action="record a leadership decision" />
        {#if error}<p class="err">{error}</p>{/if}

        <div class="form-actions">
          <button class="lt-btn lt-btn-primary lt-btn-sm" onclick={save} disabled={saving}>
            {saving ? 'Recording…' : 'Record'}
          </button>
          <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => { formOpen = false; }} disabled={saving}>Cancel</button>
        </div>
      </div>
    {:else}
      <div class="actions">
        <button class="lt-btn lt-btn-sm" onclick={() => openForm('')}>Record a leadership decision</button>
        {#if current}
          <button class="lt-btn lt-btn-sm lt-btn-ghost" onclick={() => openForm(current.id)}>Correct the last one</button>
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

  /* Crossed out, not removed (F-078). */
  .struck .struck-body { text-decoration: line-through; }

  .stood-down {
    font-size: 0.82rem;
    margin: 0.35rem 0 0;
    color: var(--color-text-muted);
  }

  .strike-btn { margin-top: 0.4rem; }

  .strike-confirm { margin-top: 0.4rem; }

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
    color: var(--color-error);
    margin: 0.3rem 0 0;
  }
</style>
