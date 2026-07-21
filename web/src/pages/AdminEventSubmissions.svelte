<script>
  /**
   * The instance admin's event queue (docs/adr/026): pending events on
   * unclaimed patches, whose calendars the instance admin holds in trust.
   * Submissions to active patches never land here — those belong to the
   * patch's own admins.
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';

  let submissions = $state([]);
  let loading = $state(true);
  let nextCursor = $state('');
  let decliningId = $state('');
  let declineNote = $state('');
  let reviewing = $state(false);

  $effect(() => { loadSubmissions(); });

  async function loadSubmissions(append = false) {
    if (!append) loading = true;
    try {
      const params = append && nextCursor ? `?after=${encodeURIComponent(nextCursor)}` : '';
      const data = await api(`admin/event-submissions${params}`);
      submissions = append ? [...submissions, ...(data.items || [])] : (data.items || []);
      nextCursor = data.next_cursor || '';
    } catch {
      if (!append) submissions = [];
    } finally {
      loading = false;
    }
  }

  async function review(id, action, note = '') {
    reviewing = true;
    try {
      const body = { action };
      if (note.trim()) body.note = note.trim();
      await api(`events/${id}/review`, { method: 'PATCH', body });
      showToast(action === 'approve' ? 'Event approved' : 'Event rejected', 'success');
      decliningId = '';
      declineNote = '';
      await loadSubmissions();
    } catch (e) {
      showToast(e.message || 'Failed', 'error');
    } finally {
      reviewing = false;
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
  }

  function formatTime(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  }
</script>

<div class="page-fade">
  <h1>Event Submissions</h1>
  <p class="muted" style="margin-bottom: 1.5rem;">
    Community-submitted events on unclaimed patches awaiting review.
  </p>

  {#if loading}
    <Skeleton lines={4} height="1rem" />
  {:else if submissions.length === 0}
    <p class="muted">No pending submissions.</p>
  {:else}
    <div class="submission-list">
      {#each submissions as sub (sub.id)}
        <div class="submission-card card">
          <div class="sub-header">
            <h3>{sub.title}</h3>
            <span class="muted">{formatDate(sub.starts_at)} &middot; {formatTime(sub.starts_at)}</span>
          </div>
          <p class="sub-patch">
            <a
              href="/patches/{sub.node_slug}"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${sub.node_slug}`); }}
            >{sub.node_name}</a>
          </p>
          {#if sub.description}
            <p class="sub-desc">{sub.description}</p>
          {/if}
          {#if sub.location}
            <p class="sub-location muted">{sub.location}</p>
          {/if}
          <div class="sub-meta muted">
            Submitted by {sub.submitter_display_name || sub.submitter_username || 'unknown'}
          </div>
          {#if decliningId === sub.id}
            <div class="decline-form">
              <textarea
                rows="2"
                placeholder="Note to the submitter (optional)"
                bind:value={declineNote}
                disabled={reviewing}
              ></textarea>
              <div class="sub-actions">
                <button class="btn btn-danger btn-sm" onclick={() => review(sub.id, 'reject', declineNote)} disabled={reviewing}>Reject</button>
                <button class="btn btn-secondary btn-sm" onclick={() => { decliningId = ''; declineNote = ''; }} disabled={reviewing}>Cancel</button>
              </div>
            </div>
          {:else}
            <div class="sub-actions">
              <button class="btn btn-primary btn-sm" onclick={() => review(sub.id, 'approve')} disabled={reviewing}>Approve</button>
              <button class="btn btn-danger btn-sm" onclick={() => { decliningId = sub.id; declineNote = ''; }} disabled={reviewing}>Reject</button>
            </div>
          {/if}
        </div>
      {/each}
    </div>

    {#if nextCursor}
      <div style="text-align: center; padding: 1rem 0;">
        <button class="btn btn-secondary" onclick={() => loadSubmissions(true)}>Load More</button>
      </div>
    {/if}
  {/if}
</div>

<style>
  h1 {
    font-size: 1.2rem;
    margin-bottom: 0.25rem;
  }

  .submission-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .sub-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.35rem;
    gap: 0.5rem;
  }

  .sub-header h3 {
    font-size: 1rem;
  }

  .sub-header .muted {
    font-size: 0.8rem;
    flex-shrink: 0;
  }

  .sub-patch {
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
  }

  .sub-desc {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
    white-space: pre-wrap;
  }

  .sub-location {
    font-size: 0.82rem;
    margin-bottom: 0.5rem;
  }

  .sub-meta {
    font-size: 0.78rem;
    margin-bottom: 0.5rem;
  }

  .sub-actions {
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

  .btn-sm {
    padding: 0.25rem 0.6rem;
    font-size: 0.75rem;
  }
</style>
