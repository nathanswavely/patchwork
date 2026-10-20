<script>
  /**
   * The intro for an unclaimed patch
   * (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md, amended
   * 2026-10-19). Two things live here. Where to reach them: an email
   * address, a link to message them on, or both, which an instance admin
   * or a trusted contributor whose grant reaches this patch may enter.
   * And the send, which is the instance admin's alone: the message is
   * rendered whole, read, edited if need be, then sent by email or copied
   * out and sent by hand. One send per destination; a second is an
   * explicit resend.
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

  // Where to reach them. Seeded from the patch payload, which carries both
  // only for a viewer allowed to enter them.
  let email = $state('');
  let url = $state('');
  let seeded = false;
  $effect(() => {
    if (!seeded && patch.value.contactEmail !== undefined && !patch.value.loading) {
      email = patch.value.contactEmail || '';
      url = patch.value.contactUrl || '';
      seeded = true;
    }
  });
  let emailDirty = $derived(email.trim().toLowerCase() !== (patch.value.contactEmail || ''));
  let urlDirty = $derived(url.trim() !== (patch.value.contactUrl || ''));
  let savingEmail = $state(false);
  let savingUrl = $state(false);

  // The patch's own links, offered as places to message it. The
  // Instagram is often the only door an organization has.
  let links = $derived((node?.links || []).filter((l) => /^https?:\/\//.test(l.url || '')));

  async function saveContact(field) {
    if (field === 'email') savingEmail = true; else savingUrl = true;
    try {
      const body = field === 'email' ? { email: email.trim() } : { url: url.trim() };
      const data = await api(`nodes/${slug}/contact`, { method: 'PUT', body });
      if (field === 'email') showToast(data.contact_email ? 'Contact address saved' : 'Contact address cleared', 'success');
      else showToast(data.contact_url ? 'Message link saved' : 'Message link cleared', 'success');
      seeded = false;
      patch.value.reload();
      if (canSend) await loadState();
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    } finally {
      savingEmail = false;
      savingUrl = false;
    }
  }

  function useLink(href) {
    url = href;
    saveContact('url');
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
  let hasContact = $derived(!!(state?.contact_email || state?.contact_url));
  let bothChannels = $derived(!!(state?.contact_email && state?.contact_url));

  // The preview: the probe's answer, and the message as it would go.
  let previewing = $state(false);
  let preview = $state(null);
  let channel = $state('');
  let situation = $state('');
  let subject = $state('');
  let body = $state('');
  let sending = $state(false);
  let copied = $state(false);

  let destination = $derived(preview?.destination || '');
  let sentToDestination = $derived(sends.some((s) => s.email === destination));
  let isDM = $derived(channel === 'dm');

  const situationLabels = {
    own: 'Their own calendar is syncing',
    aggregator: 'Events arrive through an aggregator',
    squarespace: 'Squarespace, one page type away',
    wordpress: 'WordPress, no feed',
    other: 'A site with nothing to subscribe to',
    none: 'No site to read',
  };

  async function runPreview(opts = {}) {
    previewing = true;
    try {
      const req = {};
      if (opts.situation) req.situation = opts.situation;
      if (opts.channel) req.channel = opts.channel;
      const data = await api(`nodes/${slug}/intro/preview`, { method: 'POST', body: req });
      preview = data;
      channel = data.channel;
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
    const resend = sentToDestination;
    if (resend && !confirm('An intro already went there. Send another one?')) return;
    sending = true;
    try {
      const data = await api(`nodes/${slug}/intro/send`, {
        method: 'POST',
        body: { channel, subject, body, situation, delivery, resend },
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
      await navigator.clipboard.writeText(isDM ? body : `Subject: ${subject}\n\n${body}`);
      copied = true;
    } catch {
      showToast('Could not copy; select the text instead', 'error');
    }
  }

  function when(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  function host(href) {
    try { return new URL(href).host.replace(/^www\./, ''); } catch { return href; }
  }
</script>

<div class="intro-settings">
  <section class="block">
    <h2>Where to reach them</h2>
    <p class="lt-muted">An email address, a link to message them on, or both. Enter what you chose to write to; neither shows publicly.</p>

    <label class="lt-label" for="contact-email">Email address</label>
    <div class="field-row">
      <input class="lt-field" id="contact-email" type="email" bind:value={email} placeholder="someone@{patch.value.verificationDomain || 'example.org'}" />
      <button class="lt-btn lt-btn-primary" onclick={() => saveContact('email')} disabled={savingEmail || !emailDirty}>
        {savingEmail ? 'Saving…' : 'Save'}
      </button>
    </div>
    {#if patch.value.verificationDomain && email.trim().toLowerCase().endsWith('@' + patch.value.verificationDomain)}
      <p class="lt-hint">At the verified domain, so the intro can carry a one-click claim link.</p>
    {/if}

    <label class="lt-label" for="contact-url">Link to message them on</label>
    <div class="field-row">
      <input class="lt-field" id="contact-url" type="url" bind:value={url} placeholder="https://instagram.com/…" />
      <button class="lt-btn lt-btn-primary" onclick={() => saveContact('url')} disabled={savingUrl || !urlDirty}>
        {savingUrl ? 'Saving…' : 'Save'}
      </button>
    </div>
    {#if links.length}
      <div class="link-chips">
        <span class="lt-muted">On the patch:</span>
        {#each links as l (l.url)}
          <button type="button" class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => useLink(l.url)} disabled={savingUrl || l.url === (patch.value.contactUrl || '')}>
            {l.label || host(l.url)}
          </button>
        {/each}
      </div>
    {/if}
    <p class="lt-hint">A message on a link is copied out and sent by hand, and carries the ordinary claim page rather than a one-click link: an account there proves nothing about the domain.</p>
  </section>

  {#if canSend}
    <section class="block">
      <h2>Intro</h2>
      {#if loading}
        <Skeleton lines={3} height="1rem" />
      {:else if error}
        <p class="lt-error-text">{error}</p>
      {:else if state && !state.enabled}
        <p class="lt-muted">Intros are turned off for this quilt. <a href="/admin/settings/outreach" onclick={(e) => { e.preventDefault(); navigate('/admin/settings/outreach'); }}>Outreach settings</a></p>
      {:else if !hasContact}
        <p class="lt-muted">Save somewhere to reach them first.</p>
      {:else}
        {#if sends.length}
          <ul class="sends">
            {#each sends as s (s.id)}
              <li>
                <span>{when(s.created_at)}</span>
                <span>{s.channel === 'dm' ? 'message on' : 'to'} {s.email}</span>
                <span class="lt-muted">{situationLabels[s.situation] || s.situation}{s.delivery === 'copied' ? ', sent by hand' : ''}{s.claim_link ? ', with claim link' : ''}</span>
              </li>
            {/each}
          </ul>
        {/if}

        {#if !preview}
          <button class="lt-btn lt-btn-primary" onclick={() => runPreview()} disabled={previewing}>
            {previewing ? 'Reading the site…' : sends.length ? 'Preview another intro' : 'Preview intro'}
          </button>
          <p class="lt-hint">Reads the website once to pick the calendar paragraph, then shows the whole message before anything is sent.</p>
        {:else}
          {#if bothChannels}
            <div class="probe">
              <span class="lt-label">Send it</span>
              <div class="channel-row">
                <label class="lt-check"><input type="radio" name="channel" value="email" checked={channel === 'email'} onchange={() => runPreview({ channel: 'email', situation })} /> <span>by email, to {state.contact_email}</span></label>
                <label class="lt-check"><input type="radio" name="channel" value="dm" checked={channel === 'dm'} onchange={() => runPreview({ channel: 'dm', situation })} /> <span>as a message on {host(state.contact_url)}</span></label>
              </div>
            </div>
          {/if}

          <div class="probe">
            <label class="lt-label" for="situation">Calendar paragraph</label>
            <select class="lt-field" id="situation" bind:value={situation} onchange={() => runPreview({ situation, channel })}>
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

          {#if !isDM}
            <label class="lt-label" for="intro-subject">Subject</label>
            <input class="lt-field subject" id="intro-subject" bind:value={subject} />
          {/if}
          <label class="lt-label" for="intro-body">Message</label>
          <textarea class="lt-field body" id="intro-body" bind:value={body} rows={isDM ? 10 : 18} spellcheck="true"></textarea>
          {#if preview.claim_shortcut}
            <p class="lt-hint">The claim link is a one-click claim for this address, since it is at the verified domain. Cut the link and it goes without one.</p>
          {/if}

          <div class="lt-actions">
            {#if !isDM && state.smtp}
              <button class="lt-btn lt-btn-primary" onclick={() => send('smtp')} disabled={sending || !subject.trim() || !body.trim()}>
                {sending ? 'Sending…' : `Send to ${destination}`}
              </button>
            {/if}
            <button class="lt-btn {isDM || !state.smtp ? 'lt-btn-primary' : 'lt-btn-secondary'}" onclick={copyMessage}>{copied ? 'Copied' : 'Copy message'}</button>
            <button class="lt-btn lt-btn-secondary" onclick={() => send('copied')} disabled={sending || !body.trim()}>I sent it myself</button>
            <button class="lt-btn lt-btn-secondary" onclick={() => { preview = null; }}>Cancel</button>
          </div>
          {#if isDM}
            <p class="lt-hint">Copy it, send it on <a href={destination} target="_blank" rel="noopener noreferrer">{host(destination)}</a>, then record it here.</p>
          {:else if !state.smtp}
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
  .link-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    align-items: center;
    margin-top: 0.5rem;
    font-size: 0.85rem;
  }
  .channel-row {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
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
