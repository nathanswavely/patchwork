<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, isAdmin as isInstanceAdmin, getUser } from '../stores/auth.svelte.js';
  import { parseCsv, rowsToEvents, TEMPLATE_CSV } from '../lib/eventCsv.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let membershipRole = $derived(patch.value.membershipRole);
  let isUnclaimed = $derived(patch.value.isUnclaimed);
  let node = $derived(patch.value.node);

  let followerPermissions = $derived(patch.value.followerPermissions);
  let permissionDenied = $derived(membershipRole === 'follower' && followerPermissions?.events === false);

  // Review is owed to whoever owns the calendar (docs/adr/026): patch
  // admins here, and the instance admin for unclaimed patches.
  let canReview = $derived(patch.value.isAdmin || isInstanceAdmin());

  // Non-members may suggest events for review when the doors are open.
  let canSuggest = $derived.by(() => {
    if (!node || !isLoggedIn() || patch.value.isBanned) return false;
    if (!getSubmissionsEnabled()) return false;
    if (isUnclaimed) return true;
    if (patch.value.isAdmin || (isMember && membershipRole !== 'follower')) return false;
    return node.accept_event_suggestions === true;
  });

  let events = $state([]);
  let loading = $state(true);
  let submissions = $state([]);
  let decliningId = $state('');
  let declineNote = $state('');
  let reviewing = $state(false);

  $effect(() => {
    if (slug) loadEvents();
  });

  $effect(() => {
    if (slug && canReview) loadSubmissions();
  });

  async function loadEvents() {
    loading = true;
    try {
      const data = await api(`events?node_slug=${encodeURIComponent(slug)}`);
      events = data.items || data || [];
    } catch {
      events = [];
    } finally {
      loading = false;
    }
  }

  async function loadSubmissions() {
    try {
      const data = await api(`nodes/${slug}/event-submissions`);
      submissions = data.items || [];
    } catch {
      submissions = [];
    }
  }

  async function review(id, action, note = '') {
    reviewing = true;
    try {
      const body = { action };
      if (note.trim()) body.note = note.trim();
      await api(`events/${id}/review`, { method: 'PATCH', body });
      showToast(action === 'approve' ? 'Event approved' : 'Suggestion declined', 'success');
      decliningId = '';
      declineNote = '';
      await Promise.all([loadSubmissions(), loadEvents()]);
    } catch (e) {
      showToast(e.message || 'Failed to review', 'error');
    } finally {
      reviewing = false;
    }
  }

  // Bulk upload is an admin act: patch admins on active patches, the
  // instance admin and trusted contributors on unclaimed ones. Members
  // create events one at a time.
  let canBulkUpload = $derived(
    isUnclaimed
      ? (isInstanceAdmin() || !!getUser()?.trusted_contributor)
      : (patch.value.isAdmin || isInstanceAdmin())
  );

  let showUpload = $state(false);
  let uploadFileName = $state('');
  let uploadEvents = $state([]);
  let uploadErrors = $state([]);
  let uploading = $state(false);

  function onUploadFile(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    uploadFileName = file.name;
    const reader = new FileReader();
    reader.onload = () => {
      const { events: evts, errors } = rowsToEvents(parseCsv(String(reader.result)));
      uploadEvents = evts;
      uploadErrors = errors;
    };
    reader.readAsText(file);
  }

  async function confirmUpload() {
    uploading = true;
    try {
      const res = await api(`nodes/${slug}/events/bulk`, {
        method: 'POST',
        body: { events: uploadEvents },
      });
      showToast(
        res.skipped > 0
          ? `Created ${res.created} events (${res.skipped} already on the calendar)`
          : `Created ${res.created} events`
      );
      resetUpload();
      showUpload = false;
      await loadEvents();
    } catch (err) {
      showToast(err.message || 'Upload failed', 'error');
    } finally {
      uploading = false;
    }
  }

  function resetUpload() {
    uploadFileName = '';
    uploadEvents = [];
    uploadErrors = [];
  }

  function downloadTemplate() {
    const url = URL.createObjectURL(new Blob([TEMPLATE_CSV], { type: 'text/csv' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = 'events-template.csv';
    a.click();
    URL.revokeObjectURL(url);
  }

  function previewWhen(iso) {
    return new Date(iso).toLocaleString('en-US', {
      month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit',
    });
  }

  // Subscribable feeds exist only for public patches (docs/adr/031).
  let showSubscribe = $state(false);
  let feedAvailable = $derived(node?.visibility === 'public');
  let icsUrl = $derived(`${location.origin}/api/v1/nodes/${slug}/events.ics`);
  let rssUrl = $derived(`${location.origin}/api/v1/nodes/${slug}/events.rss`);

  async function copyUrl(url) {
    try {
      await navigator.clipboard.writeText(url);
      showToast('Copied');
    } catch {
      showToast('Copy failed. Select the address instead.', 'error');
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }

  function formatTime(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  }
</script>

{#if permissionDenied}
  <div class="permission-notice">
    <p>This content is only visible to members.</p>
    <p class="muted">Become a member to access events.</p>
  </div>
{:else}
<div class="events-page">
  <div class="events-header">
    <span class="muted">
      {events.length} events
      {#if isUnclaimed}
        <span class="badge">Community-submitted</span>
      {/if}
      {#if feedAvailable}
        <button class="subscribe-toggle" onclick={() => (showSubscribe = !showSubscribe)}>Subscribe</button>
      {/if}
    </span>
    {#if isMember && membershipRole !== 'follower'}
      <span class="header-buttons">
        {#if canBulkUpload}
          <button class="btn btn-secondary btn-sm" onclick={() => (showUpload = !showUpload)}>Upload events</button>
        {/if}
        <a
          href="/events/new"
          class="btn btn-primary btn-sm"
          onclick={(e) => { e.preventDefault(); navigate('/events/new'); }}
        >Create Event</a>
      </span>
    {:else if canBulkUpload}
      <button class="btn btn-secondary btn-sm" onclick={() => (showUpload = !showUpload)}>Upload events</button>
    {:else if canSuggest}
      <a
        href="/events/new?node={slug}"
        class="btn btn-primary btn-sm"
        onclick={(e) => { e.preventDefault(); navigate(`/events/new?node=${slug}`); }}
      >Suggest an event</a>
    {:else if membershipRole === 'follower'}
      <span class="role-prompt muted">Become a member to create events.</span>
    {:else}
      <span class="role-prompt muted">Join this patch to create events.</span>
    {/if}
  </div>

  {#if showUpload && canBulkUpload}
    <div class="upload-panel">
      <div class="upload-row">
        <input type="file" accept=".csv,text/csv" onchange={onUploadFile} disabled={uploading} />
        <button class="btn-link" onclick={downloadTemplate}>Download template</button>
      </div>

      {#if uploadErrors.length > 0}
        <ul class="upload-errors">
          {#each uploadErrors as err}
            <li>{err.row > 0 ? `Row ${err.row}: ` : ''}{err.message}</li>
          {/each}
        </ul>
      {/if}

      {#if uploadEvents.length > 0}
        <div class="upload-preview">
          <table>
            <thead><tr><th>When</th><th>Title</th><th>Location</th></tr></thead>
            <tbody>
              {#each uploadEvents.slice(0, 8) as ev}
                <tr>
                  <td>{previewWhen(ev.starts_at)}</td>
                  <td>{ev.title}</td>
                  <td>{ev.location || ''}</td>
                </tr>
              {/each}
            </tbody>
          </table>
          {#if uploadEvents.length > 8}
            <p class="muted upload-more">…and {uploadEvents.length - 8} more</p>
          {/if}
        </div>
        <div class="upload-actions">
          <button
            class="btn btn-primary btn-sm"
            onclick={confirmUpload}
            disabled={uploading || uploadErrors.length > 0}
          >{uploading ? 'Uploading…' : `Create ${uploadEvents.length} ${uploadEvents.length === 1 ? 'event' : 'events'}`}</button>
          <button class="btn btn-secondary btn-sm" onclick={() => { resetUpload(); showUpload = false; }} disabled={uploading}>Cancel</button>
          {#if uploadErrors.length > 0}
            <span class="muted upload-fix-note">Fix the rows above and re-pick the file.</span>
          {/if}
        </div>
      {:else if uploadFileName && uploadErrors.length === 0}
        <p class="muted">No events found in {uploadFileName}.</p>
      {/if}

      <p class="muted upload-hint">
        Times are read in your timezone. Rows matching an event already on the
        calendar are skipped, so re-uploading a corrected file is safe.
      </p>
    </div>
  {/if}

  {#if showSubscribe && feedAvailable}
    <div class="subscribe-panel">
      <div class="feed-row">
        <span class="feed-label">Calendar (ICS)</span>
        <code class="feed-url">{icsUrl}</code>
        <button class="btn btn-secondary btn-sm" onclick={() => copyUrl(icsUrl)}>Copy</button>
      </div>
      <div class="feed-row">
        <span class="feed-label">RSS</span>
        <code class="feed-url">{rssUrl}</code>
        <button class="btn btn-secondary btn-sm" onclick={() => copyUrl(rssUrl)}>Copy</button>
      </div>
      <p class="muted feed-hint">Paste the calendar address into your calendar app to follow this patch's events.</p>
    </div>
  {/if}

  {#if canReview && submissions.length > 0}
    <section class="suggested-section">
      <h3 class="suggested-title">Suggested events</h3>
      {#each submissions as sub (sub.id)}
        <div class="suggested-card">
          <div class="suggested-main">
            <span class="suggested-event-title">{sub.title}</span>
            <span class="suggested-meta muted">
              {formatDate(sub.starts_at)} &middot; {formatTime(sub.starts_at)}
              {#if sub.location} &middot; {sub.location}{/if}
            </span>
            {#if sub.description}
              <p class="suggested-desc muted">{sub.description}</p>
            {/if}
            <span class="suggested-meta muted">
              Suggested by {sub.submitter_display_name || sub.submitter_username || 'unknown'}
            </span>
          </div>
          {#if decliningId === sub.id}
            <div class="decline-form">
              <textarea
                rows="2"
                placeholder="Note to the submitter (optional)"
                bind:value={declineNote}
                disabled={reviewing}
              ></textarea>
              <div class="suggested-actions">
                <button class="btn btn-danger btn-sm" onclick={() => review(sub.id, 'reject', declineNote)} disabled={reviewing}>Decline</button>
                <button class="btn btn-secondary btn-sm" onclick={() => { decliningId = ''; declineNote = ''; }} disabled={reviewing}>Cancel</button>
              </div>
            </div>
          {:else}
            <div class="suggested-actions">
              <button class="btn btn-primary btn-sm" onclick={() => review(sub.id, 'approve')} disabled={reviewing}>Approve</button>
              <button class="btn btn-secondary btn-sm" onclick={() => { decliningId = sub.id; declineNote = ''; }} disabled={reviewing}>Decline</button>
            </div>
          {/if}
        </div>
      {/each}
    </section>
  {/if}

  {#if loading}
    <p class="muted">Loading events...</p>
  {:else if events.length === 0}
    <div class="empty-state">
      <p>No events scheduled.</p>
      <p class="muted">Members can create events for meetups, shows, or gatherings.</p>
    </div>
  {:else}
    <ul class="event-list">
      {#each events as event (event.id)}
        <li>
          <a
            href="/events/{event.id}"
            class="event-row"
            onclick={(e) => { e.preventDefault(); navigate(`/events/${event.id}`); }}
          >
            <span class="event-date">{formatDate(event.starts_at)}</span>
            <span class="event-info">
              <span class="event-title">{event.title}</span>
              <span class="event-detail">
                {formatTime(event.starts_at)}
                {#if event.location} &middot; {event.location}{/if}
              </span>
            </span>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</div>
{/if}

<style>
  .permission-notice {
    text-align: center;
    padding: 3rem 1rem;
  }

  .permission-notice p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .events-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
    font-size: 0.85rem;
  }

  .header-buttons {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .upload-panel {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 0.75rem 1rem;
    margin-bottom: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .upload-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .btn-link {
    background: none;
    border: none;
    padding: 0;
    font-size: 0.8rem;
    color: var(--color-primary);
    cursor: pointer;
  }

  .btn-link:hover {
    text-decoration: underline;
  }

  .upload-errors {
    margin: 0;
    padding-left: 1.1rem;
    font-size: 0.8rem;
    color: var(--color-danger, #c0392b);
  }

  .upload-preview {
    overflow-x: auto;
  }

  .upload-preview table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.8rem;
  }

  .upload-preview th,
  .upload-preview td {
    text-align: left;
    padding: 0.3rem 0.6rem 0.3rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .upload-preview th {
    font-weight: 600;
    color: var(--color-text-muted);
  }

  .upload-more {
    font-size: 0.75rem;
    margin: 0.25rem 0 0;
  }

  .upload-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .upload-fix-note {
    font-size: 0.78rem;
  }

  .upload-hint {
    font-size: 0.75rem;
    margin: 0;
  }

  .subscribe-toggle {
    background: none;
    border: none;
    padding: 0;
    margin-left: 0.5rem;
    font-size: 0.85rem;
    color: var(--color-primary);
    cursor: pointer;
  }

  .subscribe-toggle:hover {
    text-decoration: underline;
  }

  .subscribe-panel {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    margin-bottom: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .feed-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }

  .feed-label {
    font-size: 0.8rem;
    font-weight: 500;
    width: 96px;
    flex-shrink: 0;
  }

  .feed-url {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
    min-width: 0;
  }

  .feed-hint {
    font-size: 0.75rem;
    margin: 0;
  }

  .role-prompt {
    font-size: 0.85rem;
  }

  /* Suggested events queue (docs/adr/026) */
  .suggested-section {
    margin-bottom: 1.5rem;
  }

  .suggested-title {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  .suggested-card {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    margin-bottom: 0.5rem;
  }

  .suggested-main {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .suggested-event-title {
    font-size: 0.9rem;
    font-weight: 600;
  }

  .suggested-meta {
    font-size: 0.78rem;
  }

  .suggested-desc {
    font-size: 0.82rem;
    margin: 0.15rem 0;
    white-space: pre-wrap;
  }

  .suggested-actions {
    display: flex;
    gap: 0.5rem;
  }

  .decline-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .decline-form textarea {
    resize: vertical;
    font-size: 0.85rem;
  }

  .empty-state {
    text-align: center;
    padding: 2rem 0;
  }

  .empty-state p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .event-list {
    list-style: none;
    padding: 0;
  }

  .event-list li {
    border-bottom: 1px solid var(--color-border);
  }

  .event-list li:last-child {
    border-bottom: none;
  }

  .event-row {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
    padding: 0.6rem 0.25rem;
    text-decoration: none;
    color: var(--color-text);
    border-radius: 4px;
    transition: background 100ms ease;
  }

  .event-row:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .event-date {
    flex-shrink: 0;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-primary);
    min-width: 5rem;
    padding-top: 0.1rem;
  }

  .event-info {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .event-title {
    font-size: 0.9rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .event-detail {
    font-size: 0.78rem;
    color: var(--color-text-muted);
  }
</style>
