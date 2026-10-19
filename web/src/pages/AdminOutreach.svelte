<script>
  /**
   * Intro emails to the owners of unclaimed patches
   * (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md). Two
   * tabs. The backlog: every unclaimed patch, who has been written to and
   * who has not, each row leading to that patch's Intro section where the
   * send happens one at a time. And the template: the shipped text, which
   * an instance replaces part by part, the way the legal documents work.
   */
  import { api } from '../lib/api.js';
  import { ltTabs } from '../lib/lt.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import ToggleSwitch from '../components/ToggleSwitch.svelte';
  import { ArrowCounterClockwise } from 'phosphor-svelte';

  let loading = $state(true);
  let error = $state('');
  let data = $state(null);
  let active = $state('patches');

  // Editable copies of the template parts.
  let draft = $state({ subject: '', opener: '', calendar: {}, signoff: '', dm_opener: '' });
  let saving = $state(false);
  let resetting = $state(false);
  let togglingEnabled = $state(false);

  const situationLabels = {
    own: 'Their own calendar is syncing',
    aggregator: 'Events arrive through an aggregator',
    squarespace: 'Squarespace, one page type away',
    wordpress: 'WordPress, no feed',
    other: 'A site with nothing to subscribe to',
    none: 'No site to read',
  };

  $effect(() => { load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      data = await api('admin/outreach');
      const p = data.template?.parts || {};
      draft = {
        subject: p.subject || '',
        opener: p.opener || '',
        calendar: { ...(p.calendar || {}) },
        signoff: p.signoff || '',
        dm_opener: p.dm_opener || '',
      };
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  let dirty = $derived.by(() => {
    const p = data?.template?.parts;
    if (!p) return false;
    if (draft.subject !== p.subject || draft.opener !== p.opener || draft.signoff !== p.signoff || draft.dm_opener !== p.dm_opener) return true;
    return Object.keys(situationLabels).some((k) => (draft.calendar[k] || '') !== (p.calendar?.[k] || ''));
  });

  async function saveTemplate() {
    saving = true;
    try {
      await api('admin/outreach', { method: 'PATCH', body: { template: draft } });
      showToast('Intro template saved', 'success');
      await load();
    } catch (e) {
      showToast(e.message || 'Save failed', 'error');
    }
    saving = false;
  }

  async function restoreDefault() {
    if (!confirm('Replace the custom intro text with the default that ships with Patchwork? Your text will be gone.')) return;
    resetting = true;
    try {
      await api('admin/outreach/template', { method: 'DELETE' });
      showToast('Intro template restored to the shipped default', 'success');
      await load();
    } catch (e) {
      showToast(e.message || 'Reset failed', 'error');
    }
    resetting = false;
  }

  async function setEnabled(enabled) {
    togglingEnabled = true;
    try {
      await api('admin/outreach', { method: 'PATCH', body: { enabled } });
      data = { ...data, enabled };
      showToast(enabled ? 'Intro emails on' : 'Intro emails off', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    togglingEnabled = false;
  }

  function openPatch(e, slug) {
    e.preventDefault();
    navigate(`/patches/${slug}/settings/intro`);
  }

  function linkHost(href) {
    try { return new URL(href).host.replace(/^www\./, ''); } catch { return href; }
  }

  function when(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  }

  let uncontacted = $derived((data?.patches || []).filter((p) => !p.sends).length);
</script>

<div class="admin-outreach lt-fade-in">
  <header class="page-head">
    <h1>Outreach</h1>
    <p class="lt-muted">
      A page made for someone should tell them so. Each unclaimed patch can get one intro, sent by you after you have read it, to an address or a message link a person entered. Nothing here sends on its own.
    </p>
  </header>

  {#if loading}
    <Skeleton lines={8} />
  {:else if error}
    <ErrorState message={error} retry={load} />
  {:else}
    <div class="switch-row">
      <label class="lt-check">
        <ToggleSwitch checked={data.enabled} disabled={togglingEnabled} onchange={(e) => setEnabled(e.currentTarget.checked)} />
        <span>Intros on for this quilt</span>
      </label>
      {#if !data.smtp}
        <span class="lt-muted">No email relay is configured, so email intros are copied out and sent by hand, as messages on a link always are.</span>
      {/if}
    </div>

    <div class="doc-tabs" role="tablist" aria-label="Outreach" use:ltTabs={{ onSelect: (t) => { active = t.dataset.value; } }}>
      <button role="tab" aria-selected={active === 'patches'} class="doc-tab" class:active={active === 'patches'} data-value="patches">
        Patches <span class="doc-state">{uncontacted} not yet written to</span>
      </button>
      <button role="tab" aria-selected={active === 'template'} class="doc-tab" class:active={active === 'template'} data-value="template">
        Template <span class="doc-state" class:customized={data.template.customized}>{data.template.customized ? 'customized' : 'default'}</span>
      </button>
    </div>

    {#if active === 'patches'}
      {#if !data.patches.length}
        <p class="lt-muted">No unclaimed patches.</p>
      {:else}
        <table class="backlog">
          <thead>
            <tr><th>Patch</th><th>Calendar</th><th>Reach them</th><th>Intro</th></tr>
          </thead>
          <tbody>
            {#each data.patches as p (p.id)}
              <tr>
                <td><a href="/patches/{p.slug}/settings/intro" onclick={(e) => openPatch(e, p.slug)}>{p.name}</a></td>
                <td class="lt-muted">{p.own_source ? 'own site' : p.aggregator_source ? 'aggregator' : p.website ? 'not read' : 'no site'}</td>
                <td class="lt-muted">
                  {#if p.contact_email}<div>{p.contact_email}</div>{/if}
                  {#if p.contact_url}<div>{linkHost(p.contact_url)}</div>{/if}
                  {#if !p.contact_email && !p.contact_url}—{/if}
                </td>
                <td class="lt-muted">
                  {#if p.sends}
                    sent {when(p.last_sent_at)}{p.sends > 1 ? ` (${p.sends})` : ''}
                  {:else if p.open_claims}
                    claim open
                  {:else}
                    not yet
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    {:else}
      <p class="lt-muted parts-hint">
        Placeholders: <code>{'{patch_name}'}</code> <code>{'{patch_url}'}</code> <code>{'{claim_url}'}</code> <code>{'{quilt_name}'}</code> <code>{'{quilt_url}'}</code> <code>{'{sender_name}'}</code> <code>{'{event_count}'}</code> <code>{'{aggregator_name}'}</code>. A part left empty falls back to the shipped text.
      </p>

      <label class="lt-label" for="tpl-subject">Subject</label>
      <input class="lt-field subject" id="tpl-subject" bind:value={draft.subject} />

      <label class="lt-label" for="tpl-opener">Opener</label>
      <textarea class="doc-editor" id="tpl-opener" bind:value={draft.opener} rows="8" spellcheck="true"></textarea>

      {#each Object.entries(situationLabels) as [id, label] (id)}
        <label class="lt-label" for={`tpl-${id}`}>{label}</label>
        <textarea class="doc-editor" id={`tpl-${id}`} bind:value={draft.calendar[id]} rows="5" spellcheck="true"></textarea>
      {/each}

      <label class="lt-label" for="tpl-signoff">Sign-off</label>
      <textarea class="doc-editor" id="tpl-signoff" bind:value={draft.signoff} rows="2"></textarea>

      <label class="lt-label" for="tpl-dm-opener">Opener for a message on a link</label>
      <textarea class="doc-editor" id="tpl-dm-opener" bind:value={draft.dm_opener} rows="5" spellcheck="true"></textarea>
      <p class="lt-muted parts-hint">Used instead of the subject, opener and sign-off when the intro is a message on a link such as Instagram. The first two lines are all a message preview shows. The calendar paragraph follows it as in the email; the claim link is the ordinary claim page, never the one-click one.</p>

      <div class="doc-actions">
        <button class="lt-btn lt-btn-primary" onclick={saveTemplate} disabled={saving || !dirty}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        {#if data.template.customized}
          <button class="lt-btn lt-btn-secondary" onclick={restoreDefault} disabled={resetting}>
            <ArrowCounterClockwise size={14} weight="bold" />
            {resetting ? 'Restoring…' : 'Restore shipped default'}
          </button>
        {/if}
        {#if dirty}
          <span class="lt-muted unsaved">Unsaved changes</span>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .admin-outreach {
    max-width: var(--pw-measure);
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
  .switch-row {
    display: flex;
    gap: 12px;
    align-items: baseline;
    flex-wrap: wrap;
    margin-bottom: 16px;
    font-size: 0.9rem;
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
  .backlog {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.88rem;
  }
  .backlog th {
    text-align: left;
    font-weight: 600;
    padding: 6px 8px 6px 0;
    border-bottom: 1px solid var(--color-border);
  }
  .backlog td {
    padding: 8px 8px 8px 0;
    border-bottom: 1px solid var(--color-border);
    vertical-align: top;
  }
  .parts-hint {
    font-size: 0.82rem;
    margin: 0 0 8px;
  }
  .parts-hint code {
    font-size: 0.8em;
  }
  .lt-label {
    display: block;
    margin-top: 12px;
  }
  .subject {
    width: 100%;
  }
  .doc-editor {
    width: 100%;
    font-family: inherit;
    font-size: 0.9rem;
    line-height: 1.5;
    padding: 10px 12px;
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
  .doc-actions {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 16px;
  }
  .doc-actions .lt-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .unsaved {
    font-size: 0.82rem;
  }
</style>
