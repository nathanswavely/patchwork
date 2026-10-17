<script>
  /**
   * The intro email for an unclaimed patch
   * (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md). Two
   * things live here. The address, which an instance admin or a trusted
   * contributor whose grant reaches this patch may enter. And the send,
   * which is the instance admin's alone: the message is rendered whole,
   * read, edited if need be, then sent or copied out. One send per
   * address; a second is an explicit resend.
   */
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);
  // Instance admin, not patch admin: an unclaimed patch has no admins of
  // its own, and only the quilt's admin sends on the quilt's domain.
  let canSend = $derived(getUser()?.role === 'admin');

  // The address. Seeded from the patch payload, which carries it only for
  // a viewer allowed to enter it.
  let email = $state('');
  let seeded = false;
  $effect(() => {
    if (!seeded && patch.value.contactEmail !== undefined && !patch.value.loading) {
      email = patch.value.contactEmail || '';
      seeded = true;
    }
  });
  let emailDirty = $derived(email.trim().toLowerCase() !== (patch.value.contactEmail || ''));
  let savingEmail = $state(false);

  async function saveEmail() {
    savingEmail = true;
    try {
      const data = await api(`nodes/${slug}/contact`, { method: 'PUT', body: { email: email.trim() } });
      showToast(data.contact_email ? 'Contact address saved' : 'Contact address cleared', 'success');
      seeded = false;
      patch.value.reload();
      if (canSend) await loadState();
    } catch (e) {
      showToast(e.message || 'Failed to save the address', 'error');
    } finally {
      savingEmail = false;
    }
  }

  // Send state (admin only).
  let loading = $state(true);
  let error = $state('');
  let state = $state(null);
  $effect(() => {
    if (canSend && slug) loadState();
    else loading = false;
  });
  async function loadState() {
    loading = true;
    error = '';
    try {
      state = await api(`nodes/${slug}/intro`);
    } catch (e) {
      error = e.message || 'Failed to load';
    } finally {
      loading = false;
    }
  }

  let sends = $derived(state?.sends || []);
  let sentToCurrent = $derived(sends.some((s) => s.email === (state?.contact_email || '')));

  // The preview: the probe's answer, and the message as it would go.
  let previewing = $state(false);
  let preview = $state(null);
  let situation = $state('');
  let subject = $state('');
  let body = $state('');
  let sending = $state(false);
  let copied = $state(false);

  const situationLabels = {
    own: 'Their own calendar is syncing',
    aggregator: 'Events arrive through an aggregator',
    squarespace: 'Squarespace, one page type away',
    wordpress: 'WordPress, no feed',
    other: 'A site with nothing to subscribe to',
    none: 'No site to read',
  };

  async function runPreview(override) {
    previewing = true;
    try {
      const data = await api(`nodes/${slug}/intro/preview`, {
        method: 'POST',
        body: override ? { situation: override } : {},
      });
      preview = data;
      situation = data.situation;
      subject = data.subject;
      body = data.body;
      copied = false;
    } catch (e) {
      showToast(e.message || 'Preview failed', 'error');
    } finally {
      previewing = false;
    }
  }

  async function send(delivery) {
    const resend = sentToCurrent;
    if (resend && !confirm('An intro already went to this address. Send another one?')) return;
    sending = true;
    try {
      const data = await api(`nodes/${slug}/intro/send`, {
        method: 'POST',
        body: { subject, body, situation, delivery, resend },
      });
      showToast(delivery === 'smtp' ? 'Intro sent' : 'Recorded as sent', 'success');
      state = { ...state, sends: data.sends };
      preview = null;
    } catch (e) {
      showToast(e.message || 'Send failed', 'error');
    } finally {
      sending = false;
    }
  }

  async function copyMessage() {
    try {
      await navigator.clipboard.writeText(`Subject: ${subject}\n\n${body}`);
      copied = true;
    } catch {
      showToast('Could not copy; select the text instead', 'error');
    }
  }

  function when(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }
</script>

<div class="intro-settings">
  <section class="block">
    <h2>Contact address</h2>
    <p class="lt-muted">Where an intro about this page goes. Enter one you chose to write to; it never shows publicly.</p>
    <div class="field-row">
      <input class="lt-field" type="email" bind:value={email} placeholder="someone@{patch.value.verificationDomain || 'example.org'}" aria-label="Contact address" />
      <button class="lt-btn lt-btn-primary" onclick={saveEmail} disabled={savingEmail || !emailDirty}>
        {savingEmail ? 'Saving…' : 'Save'}
      </button>
    </div>
    {#if patch.value.verificationDomain && email.trim().toLowerCase().endsWith('@' + patch.value.verificationDomain)}
      <p class="lt-hint">At the verified domain, so the intro can carry a one-click claim link.</p>
    {/if}
  </section>

  {#if canSend}
    <section class="block">
      <h2>Intro</h2>
      {#if loading}
        <Skeleton lines={3} height="1rem" />
      {:else if error}
        <p class="lt-error-text">{error}</p>
      {:else if state && !state.enabled}
        <p class="lt-muted">Intro emails are turned off for this quilt. <a href="/admin/settings/outreach" onclick={(e) => { e.preventDefault(); navigate('/admin/settings/outreach'); }}>Outreach settings</a></p>
      {:else if !state?.contact_email}
        <p class="lt-muted">Save a contact address first.</p>
      {:else}
        {#if sends.length}
          <ul class="sends">
            {#each sends as s (s.id)}
              <li>
                <span>{when(s.created_at)}</span>
                <span>to {s.email}</span>
                <span class="lt-muted">{situationLabels[s.situation] || s.situation}{s.delivery === 'copied' ? ', sent elsewhere' : ''}{s.claim_link ? ', with claim link' : ''}</span>
              </li>
            {/each}
          </ul>
        {/if}

        {#if !preview}
          <button class="lt-btn lt-btn-primary" onclick={() => runPreview()} disabled={previewing}>
            {previewing ? 'Reading the site…' : sentToCurrent ? 'Preview another intro' : 'Preview intro'}
          </button>
          <p class="lt-hint">Reads the website once to pick the calendar paragraph, then shows the whole message before anything is sent.</p>
        {:else}
          <div class="probe">
            <label class="lt-label" for="situation">Calendar paragraph</label>
            <select class="lt-field" id="situation" bind:value={situation} onchange={() => runPreview(situation)}>
              {#each Object.entries(situationLabels) as [id, label] (id)}
                <option value={id}>{label}</option>
              {/each}
            </select>
            <p class="lt-hint">
              Picked because {preview.classification?.reason}.
              {#if preview.classification?.situation === 'attachable'}
                Attach it under <a href="/patches/{slug}/settings/sources" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/settings/sources`); }}>Event Sources</a> first and the intro will say so.
              {/if}
            </p>
          </div>

          <label class="lt-label" for="intro-subject">Subject</label>
          <input class="lt-field subject" id="intro-subject" bind:value={subject} />
          <label class="lt-label" for="intro-body">Message</label>
          <textarea class="lt-field body" id="intro-body" bind:value={body} rows="18" spellcheck="true"></textarea>
          {#if preview.claim_shortcut}
            <p class="lt-hint">The claim link is a one-click claim for this address, since it is at the verified domain. Cut the link and it goes without one.</p>
          {/if}

          <div class="lt-actions">
            {#if state.smtp}
              <button class="lt-btn lt-btn-primary" onclick={() => send('smtp')} disabled={sending || !subject.trim() || !body.trim()}>
                {sending ? 'Sending…' : `Send to ${state.contact_email}`}
              </button>
            {/if}
            <button class="lt-btn lt-btn-secondary" onclick={copyMessage}>{copied ? 'Copied' : 'Copy message'}</button>
            {#if !state.smtp || copied}
              <button class="lt-btn lt-btn-secondary" onclick={() => send('copied')} disabled={sending}>I sent it myself</button>
            {/if}
            <button class="lt-btn lt-btn-secondary" onclick={() => { preview = null; }}>Cancel</button>
          </div>
          {#if !state.smtp}
            <p class="lt-hint">This quilt has no email relay, so copy the message into your own mail and record it here.</p>
          {/if}
        {/if}
      {/if}
    </section>
  {/if}
</div>

<style>
  .intro-settings {
    max-width: var(--pw-measure);
    display: flex;
    flex-direction: column;
    gap: 2rem;
  }
  .block h2 {
    font-size: 1.05rem;
    margin: 0 0 0.25rem;
  }
  .block > .lt-muted {
    margin: 0 0 0.75rem;
    font-size: 0.9rem;
  }
  .field-row {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .field-row input {
    flex: 1;
  }
  .sends {
    list-style: none;
    padding: 0;
    margin: 0 0 1rem;
    font-size: 0.88rem;
  }
  .sends li {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
    padding: 0.35rem 0;
    border-bottom: 1px solid var(--color-border);
  }
  .probe {
    margin-bottom: 1rem;
  }
  .lt-label {
    display: block;
    margin-top: 0.75rem;
  }
  .subject {
    width: 100%;
  }
  .body {
    width: 100%;
    font-family: inherit;
    line-height: 1.5;
    resize: vertical;
  }
  .lt-actions {
    margin-top: 1rem;
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
</style>
