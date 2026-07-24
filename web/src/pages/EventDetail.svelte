<script>
  import { CalendarBlank, MapPin, ArrowsClockwise, PencilSimple } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isAdmin, getUser } from '../stores/auth.svelte.js';
  import { getMembershipRoles } from '../stores/memberships.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import EventLinks from '../components/EventLinks.svelte';

  let { eventId = '' } = $props();

  let event = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    loadEvent(eventId);
  });

  async function loadEvent(id) {
    loading = true;
    error = '';
    try {
      event = await api(`events/${id}`);
    } catch (e) {
      event = null;
      error = e.message || 'Event not found';
    } finally {
      loading = false;
    }
  }

  // An imported event's source is authoritative (docs/adr/031): no
  // Edit — the admins who manage the feed get Detach instead.
  let imported = $derived(!!event?.source_id);
  let canManage = $derived(
    !!event && (isAdmin() || getMembershipRoles().get(event.node_slug) === 'admin')
  );

  // Editing is allowed for admins of the hosting patch, instance admins,
  // and the event's own creator, mirroring the backend check in UpdateEvent
  // (docs/adr/026: creators may always edit their own event).
  let canEdit = $derived(
    !!event && !imported && (
      canManage
      || (!!getUser() && event.created_by === getUser().id)
    )
  );

  async function detachEvent() {
    try {
      await api(`events/${event.id}/detach`, { method: 'POST' });
      showToast('Detached. This is an ordinary event now.');
      await loadEvent(event.id);
    } catch (e) {
      showToast(e.message || 'Failed to detach event', 'error');
    }
  }

  // Deleting mirrors the backend's DeleteEvent check: patch admins,
  // instance admins, and the event's own creator — removing your own
  // contribution is always free (docs/adr/026).
  async function deleteEvent() {
    try {
      await api(`events/${event.id}`, { method: 'DELETE' });
      showToast(event.status === 'pending_review' ? 'Submission withdrawn' : 'Event deleted');
      navigate('/events');
    } catch (e) {
      showToast(e.message || 'Failed to delete event', 'error');
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', {
      weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
    });
  }

  function formatTime(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  }

  let timeLabel = $derived.by(() => {
    if (!event?.starts_at) return '';
    const start = `${formatDate(event.starts_at)} · ${formatTime(event.starts_at)}`;
    if (!event.ends_at) return start;
    const sameDay = event.starts_at.slice(0, 10) === event.ends_at.slice(0, 10);
    return sameDay
      ? `${start} – ${formatTime(event.ends_at)}`
      : `${start} – ${formatDate(event.ends_at)} · ${formatTime(event.ends_at)}`;
  });

  const RECURRENCE_LABELS = {
    daily: 'Repeats daily',
    weekly: 'Repeats weekly',
    biweekly: 'Repeats every two weeks',
    monthly: 'Repeats monthly',
  };
</script>

<div class="event-detail">
  {#if loading}
    <p class="muted state-msg">Loading event...</p>
  {:else if error || !event}
    <div class="state-msg">
      <h2>Event not found</h2>
      <p class="muted">{error || "This event doesn't exist or was removed."}</p>
      <a href="/events" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate('/events'); }}>Back to Events</a>
    </div>
  {:else}
    <a href="/events" class="back-link" onclick={(e) => { e.preventDefault(); navigate('/events'); }}>&larr; All events</a>

    {#if event.status === 'pending_review'}
      <!-- Pending events are only visible to their submitter and reviewers
           (docs/adr/026), so this banner speaks to the submitter. -->
      <div class="review-banner">
        <strong>Awaiting review</strong>
        <span>
          The {event.node_status === 'unclaimed' ? 'quilt admins' : 'patch admins'}
          will look at this event before it appears on the calendar. Only you
          and the reviewers can see it for now.
        </span>
      </div>
    {/if}

    <div class="detail-header">
      <h1>{event.title}</h1>
      {#if canEdit || (imported && canManage)}
        <div class="header-actions">
          {#if canEdit}
            <a
              href="/events/{event.id}/edit"
              class="btn btn-sm btn-secondary edit-btn"
              onclick={(e) => { e.preventDefault(); navigate(`/events/${event.id}/edit`); }}
            >
              <PencilSimple size={13} weight="duotone" />
              Edit
            </a>
          {/if}
          {#if imported && canManage}
            <ConfirmAction
              label="Detach"
              confirmLabel="Yes, detach from the feed"
              onConfirm={detachEvent}
            />
          {/if}
          <ConfirmAction
            label={event.status === 'pending_review' ? 'Withdraw' : 'Delete'}
            confirmLabel={event.status === 'pending_review' ? 'Withdraw' : 'Delete'}
            variant="danger"
            onConfirm={deleteEvent}
          />
        </div>
      {/if}
    </div>

    {#if imported && canManage}
      <p class="imported-note muted">
        This event comes from one of
        {#if event.node_slug}
          <a
            href="/patches/{event.node_slug}/settings/sources"
            onclick={(e) => { e.preventDefault(); navigate(`/patches/${event.node_slug}/settings/sources`); }}
          >this patch's event sources</a>{:else}this patch's event sources{/if}
        and updates with it. To change it, edit the source calendar, or
        detach it to make it an ordinary event.
      </p>
    {/if}

    {#if event.node_name}
      <div class="host-row">
        <a
          href="/patches/{event.node_slug}"
          class="host-link"
          onclick={(e) => { e.preventDefault(); navigate(`/patches/${event.node_slug}`); }}
        >Hosted by {event.node_name}</a>
        {#if event.node_status === 'unclaimed'}
          <span class="badge">Community-submitted</span>
        {/if}
      </div>
    {/if}

    <EventLinks {event} onChanged={() => loadEvent(event.id)} />

    <div class="meta">
      <div class="meta-row">
        <CalendarBlank size={16} weight="duotone" />
        <span>{timeLabel}</span>
      </div>
      {#if event.location}
        <div class="meta-row">
          <MapPin size={16} weight="duotone" />
          <span>{event.location}</span>
        </div>
      {/if}
      {#if event.recurrence && RECURRENCE_LABELS[event.recurrence]}
        <div class="meta-row">
          <ArrowsClockwise size={16} weight="duotone" />
          <span>{RECURRENCE_LABELS[event.recurrence]}</span>
        </div>
      {/if}
    </div>

    {#if event.description}
      <p class="description">{event.description}</p>
    {/if}
  {/if}
</div>

<style>
  .event-detail {
    max-width: 680px;
    margin: 0 auto;
    /* Padding comes from SocialShell's .social-main container (issue #17). */
  }

  .state-msg {
    text-align: center;
    padding: 3rem 0;
  }

  .state-msg h2 {
    margin-bottom: 0.5rem;
  }

  .state-msg .muted {
    margin-bottom: 1rem;
  }

  .back-link {
    display: inline-block;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    margin-bottom: 1rem;
    text-decoration: none;
  }

  .back-link:hover {
    color: var(--color-primary);
  }

  .detail-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  .detail-header h1 {
    font-size: 1.5rem;
    font-weight: 700;
    margin-bottom: 0.25rem;
  }

  .header-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  .edit-btn {
    flex-shrink: 0;
  }

  .imported-note {
    font-size: 0.8rem;
    margin-bottom: 0.75rem;
  }

  .review-banner {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0.75rem 1rem;
    margin-bottom: 1rem;
    border: 1px solid var(--color-border);
    border-left: 3px solid var(--color-primary);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.85rem;
  }

  .review-banner span {
    color: var(--color-text-muted);
  }

  .host-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1.25rem;
    flex-wrap: wrap;
  }

  .host-link {
    display: inline-block;
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .host-link:hover {
    text-decoration: underline;
  }

  .meta {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    margin-bottom: 1.25rem;
  }

  .meta-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.9rem;
    color: var(--color-text);
  }

  .meta-row :global(svg) {
    color: var(--color-primary);
    flex-shrink: 0;
  }

  .description {
    font-size: 0.92rem;
    line-height: 1.6;
    white-space: pre-wrap;
  }
</style>
