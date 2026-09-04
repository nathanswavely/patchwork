<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { formatDay as formatDate } from '../lib/datetime.js';
  import SegmentedControl from '../components/SegmentedControl.svelte';
  import ToggleSwitch from '../components/ToggleSwitch.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  // The noticeboard's two settings and its report queue (docs/adr/081).
  // The queue is the patch's own: reports about notices and replies come
  // here, never to the instance panel, and the actions are the kit's
  // three — dismiss, remove, switch replies off. Nothing else is offered.
  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

  let posting = $state('members');
  let repliesDefault = $state(true);
  let hydrated = false;
  $effect(() => {
    if (node && !hydrated) {
      posting = node.notice_posting || 'members';
      repliesDefault = node.notice_replies_default !== false;
      hydrated = true;
    }
  });

  async function save(body, done) {
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body });
      showToast(done, 'success');
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
      hydrated = false;
    }
  }

  function setPosting(v) {
    posting = v;
    save({ notice_posting: v }, v === 'admins' ? 'Only admins put up notices now' : 'Members can put up notices');
  }

  function setRepliesDefault(v) {
    repliesDefault = v;
    save({ notice_replies_default: v }, v ? 'New notices take replies' : 'New notices start with replies off');
  }

  // Report queue.
  let tab = $state('pending');
  let reports = $state([]);
  let loadingReports = $state(true);
  let resolvingId = $state(null);
  let action = $state('dismiss');
  let note = $state('');

  $effect(() => {
    if (slug) loadReports(tab);
  });

  async function loadReports(status) {
    loadingReports = true;
    try {
      const data = await api(`nodes/${slug}/reports?status=${status}`);
      reports = data.items || [];
    } catch {
      reports = [];
    } finally {
      loadingReports = false;
    }
  }

  function startResolve(r) {
    resolvingId = r.id;
    action = 'dismiss';
    note = '';
  }

  async function resolve() {
    try {
      await api(`nodes/${slug}/reports/${resolvingId}`, { method: 'PATCH', body: { action, resolution_note: note.trim() } });
      showToast('Report reviewed', 'success');
      resolvingId = null;
      await loadReports(tab);
    } catch (e) {
      showToast(e.message || 'Could not resolve the report', 'error');
    }
  }
</script>

<div class="page-fade">
  <h2>Noticeboard</h2>
  <p class="muted subtitle">Read by this patch's admins and members, and nobody else.</p>

  <div class="setting-row">
    <div class="setting-info">
      <span class="setting-label">Who puts up notices</span>
      <span class="setting-desc muted">Members too, or only this patch's admins. Anyone in the room can reply either way.</span>
    </div>
    <SegmentedControl
      options={[{ value: 'members', label: 'Members' }, { value: 'admins', label: 'Admins only' }]}
      value={posting}
      label="Who puts up notices"
      onchange={setPosting}
    />
  </div>

  <div class="setting-row">
    <div class="setting-info">
      <span class="setting-label">New notices take replies</span>
      <span class="setting-desc muted">The starting position. Each notice's author decides for their own, and can change it later.</span>
    </div>
    <ToggleSwitch checked={repliesDefault} label="New notices take replies" onchange={(e) => setRepliesDefault(e.currentTarget.checked)} />
  </div>

  <h3 class="queue-heading">Reports</h3>
  <p class="muted subtitle">
    Reports about notices and replies come to this patch's admins. Three things can be done with one: dismiss it, remove what was reported, or switch replies off on the notice.
  </p>

  <div class="tabs">
    {#each [['pending', 'Pending'], ['resolved', 'Resolved'], ['dismissed', 'Dismissed']] as [key, label]}
      <button class="tab-btn" class:active={tab === key} onclick={() => { tab = key; }}>{label}</button>
    {/each}
  </div>

  {#if loadingReports}
    <p class="muted">Loading...</p>
  {:else if reports.length === 0}
    <p class="muted">No {tab} reports.</p>
  {:else}
    <ul class="report-list">
      {#each reports as r (r.id)}
        <li class="report-row">
          <div class="report-head">
            <span class="badge">{r.entity_type}</span>
            <span class="muted">{formatDate(r.created_at)}</span>
            <span class="muted">reported by {r.reporter_name}</span>
          </div>
          <p class="report-target">
            {#if r.gone}
              <span class="muted">Already taken down.</span>
            {:else}
              <a href="/patches/{slug}/noticeboard/{r.notice_id}" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/noticeboard/${r.notice_id}`); }}>
                {r.target}
              </a>
            {/if}
          </p>
          <p class="report-reason">{r.reason}</p>
          {#if r.details}<p class="muted report-details">{r.details}</p>{/if}

          {#if tab === 'pending'}
            {#if resolvingId === r.id}
              <div class="resolve-form">
                <label class="form-label">
                  Action
                  <select bind:value={action}>
                    <option value="dismiss">Dismiss, leave it up</option>
                    {#if !r.gone}
                      <option value="remove">Remove the {r.entity_type}</option>
                      <option value="close_replies">Switch replies off on the notice</option>
                    {/if}
                  </select>
                </label>
                <label class="form-label">
                  Note
                  <textarea bind:value={note} rows="2" placeholder="Optional"></textarea>
                </label>
                <div class="resolve-actions">
                  {#if action === 'remove'}
                    <ConfirmAction label="Confirm" confirmLabel="Yes, remove it" variant="danger" onConfirm={resolve} />
                  {:else}
                    <button class="btn btn-primary btn-sm" onclick={resolve}>Confirm</button>
                  {/if}
                  <button class="btn btn-secondary btn-sm" onclick={() => resolvingId = null}>Cancel</button>
                </div>
              </div>
            {:else}
              <button class="btn btn-secondary btn-sm" onclick={() => startResolve(r)}>Review</button>
            {/if}
          {:else if r.resolution_note}
            <p class="muted report-details">Note: {r.resolution_note}</p>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  h2 { font-size: 1.2rem; margin-bottom: 0.25rem; }
  .subtitle { font-size: 0.85rem; margin-bottom: 1.25rem; }
  .setting-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--color-border);
  }
  .setting-info { display: flex; flex-direction: column; gap: 0.1rem; }
  .setting-label { font-size: 0.92rem; font-weight: 500; color: var(--color-text); }
  .setting-desc { font-size: 0.8rem; }
  .queue-heading { font-size: 1rem; margin: 1.75rem 0 0.25rem; }
  .tabs { display: flex; gap: 0.25rem; margin-bottom: 0.75rem; }
  .tab-btn {
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-text-muted);
    font-size: 0.8rem;
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
    cursor: pointer;
  }
  .tab-btn.active { color: var(--color-text); border-color: var(--color-primary); }
  .report-list { list-style: none; padding: 0; margin: 0; }
  .report-row { padding: 0.75rem 0; border-bottom: 1px solid var(--color-border); font-size: 0.9rem; }
  .report-head { display: flex; gap: 0.6rem; align-items: center; font-size: 0.8rem; margin-bottom: 0.25rem; }
  .report-target { margin: 0.2rem 0; }
  .report-target a { color: var(--color-text); font-weight: 500; }
  .report-reason { margin: 0.2rem 0; font-weight: 500; }
  .report-details { font-size: 0.82rem; margin: 0.2rem 0; }
  .resolve-form { display: flex; flex-direction: column; gap: 0.5rem; margin-top: 0.5rem; max-width: 30rem; }
  .form-label { display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.82rem; color: var(--color-text-muted); }
  .form-label select, .form-label textarea {
    padding: 0.4rem 0.55rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    font: inherit;
  }
  .resolve-actions { display: flex; gap: 0.5rem; }
</style>
