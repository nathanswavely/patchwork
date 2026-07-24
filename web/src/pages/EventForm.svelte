<script>
  import { api } from '../lib/api.js';
  import { navigate, getQuery } from '../stores/router.svelte.js';
  import VocabLabel from '../components/VocabLabel.svelte';
  import { showToast } from '../stores/toast.svelte.js';

  let { eventId = '', nodeSlug = '' } = $props();
  let isEdit = $derived(!!eventId);

  // A ?node=slug query param (or nodeSlug prop) pre-scopes the form to one
  // patch — the suggest-an-event door (docs/adr/026). The user may not
  // belong to that patch, so the select is locked to it.
  let lockSlug = $derived(nodeSlug || getQuery().get('node') || '');

  let title = $state('');
  let description = $state('');
  let nodeId = $state('');
  let location = $state('');
  let startsAt = $state('');
  let endsAt = $state('');
  let recurrence = $state('');

  let myNodes = $state([]);
  let lockedNode = $state(null);
  let lockedUnclaimed = $state(false);
  let eventNodeStatus = $state('');
  // Set when a submit came back pending_review — the form is replaced by
  // a confirmation instead of navigating to an event nobody else can see.
  let pendingReviewers = $state('');
  let loadingNodes = $state(true);
  let submitting = $state(false);
  let error = $state('');

  $effect(() => {
    if (isEdit) {
      loadEvent();
    } else if (lockSlug) {
      loadLockedNode();
    } else {
      loadMyNodes();
    }
  });

  async function loadEvent() {
    loadingNodes = true;
    try {
      const [event, nodesData] = await Promise.all([
        api(`events/${eventId}`),
        api('me/nodes').catch(() => ({ items: [] })),
      ]);
      title = event.title || '';
      description = event.description || '';
      nodeId = event.node_id || '';
      location = event.location || '';
      startsAt = event.starts_at ? event.starts_at.slice(0, 16) : '';
      endsAt = event.ends_at ? event.ends_at.slice(0, 16) : '';
      recurrence = event.recurrence || '';
      eventNodeStatus = event.node_status || '';
      myNodes = nodesData.items || nodesData || [];
      // Creators can edit events on patches they don't belong to; keep
      // the (disabled) select able to show the hosting patch's name.
      if (nodeId && !myNodes.some((n) => n.node_id === nodeId) && event.node_name) {
        myNodes = [...myNodes, { node_id: nodeId, node_name: event.node_name }];
      }
    } catch (e) {
      error = e.message || 'Failed to load event';
      myNodes = [];
    } finally {
      loadingNodes = false;
    }
  }

  async function loadLockedNode() {
    loadingNodes = true;
    try {
      const data = await api(`nodes/${lockSlug}`);
      lockedNode = data.node || data;
      lockedUnclaimed = data.is_unclaimed || false;
      nodeId = lockedNode.id;
      myNodes = [lockedNode];
    } catch (e) {
      error = e.message || 'Failed to load patch';
      myNodes = [];
    } finally {
      loadingNodes = false;
    }
  }

  async function loadMyNodes() {
    loadingNodes = true;
    try {
      const data = await api('me/nodes');
      myNodes = data.items || data || [];
    } catch {
      myNodes = [];
    } finally {
      loadingNodes = false;
    }
  }

  function validate() {
    if (!title.trim()) return 'Title is required';
    if (!nodeId) return 'Please select a patch';
    if (!startsAt) return 'Start date/time is required';
    return '';
  }

  async function handleSubmit() {
    const validationError = validate();
    if (validationError) {
      error = validationError;
      return;
    }

    error = '';
    submitting = true;
    try {
      const body = {
        title: title.trim(),
        description: description.trim() || undefined,
        node_id: nodeId,
        location: location.trim() || undefined,
        starts_at: new Date(startsAt).toISOString(),
        ends_at: endsAt ? new Date(endsAt).toISOString() : undefined,
        recurrence: recurrence || undefined,
      };
      if (isEdit) {
        const result = await api(`events/${eventId}`, { method: 'PATCH', body });
        if (result?.status === 'pending_review') {
          // A non-trusted creator's edit on an unclaimed patch goes back
          // through review (docs/adr/026).
          pendingReviewers = eventNodeStatus === 'unclaimed' ? 'quilt admins' : 'patch admins';
          showToast('Submitted for review', 'success');
        } else {
          showToast('Event updated', 'success');
          navigate(`/events/${eventId}`);
        }
      } else {
        const result = await api('events', { method: 'POST', body });
        if (result?.status === 'pending_review') {
          pendingReviewers = lockedUnclaimed ? 'quilt admins' : 'patch admins';
          showToast('Submitted for review', 'success');
        } else {
          showToast('Event created', 'success');
          navigate(`/events/${result.id}`);
        }
      }
    } catch (e) {
      error = e.message || (isEdit ? 'Failed to update event' : 'Failed to create event');
      showToast('Something went wrong. Please try again.', 'error');
    } finally {
      submitting = false;
    }
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <div>
      {#if pendingReviewers}
        <div class="card pending-confirm">
          <h2>Submitted for review</h2>
          <p class="muted">
            The {pendingReviewers} will look at it. Your event will appear on
            the calendar once it's approved.
          </p>
          <div class="field-actions">
            {#if lockSlug}
              <a
                href="/patches/{lockSlug}"
                class="btn btn-primary"
                onclick={(e) => { e.preventDefault(); navigate(`/patches/${lockSlug}`); }}
              >Back to patch</a>
            {:else}
              <a
                href="/events"
                class="btn btn-primary"
                onclick={(e) => { e.preventDefault(); navigate('/events'); }}
              >Back to events</a>
            {/if}
          </div>
        </div>
      {:else}
      <h1>{!isEdit && lockSlug ? 'Suggest an' : isEdit ? 'Edit' : 'Create'} <VocabLabel term="event" /></h1>
      <p class="muted" style="margin-bottom: 1.5rem;">
        {#if isEdit}
          Update your event details.
        {:else if lockSlug}
          Suggest an event{lockedNode ? ` for ${lockedNode.name}` : ''}. It will be reviewed before it appears.
        {:else}
          Schedule a new event for your community.
        {/if}
      </p>

      <form class="card" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
        <div class="field">
          <label for="title">Title <span class="required">*</span></label>
          <input id="title" type="text" bind:value={title} disabled={submitting} required />
        </div>

        <div class="field">
          <label for="description">Description</label>
          <textarea id="description" bind:value={description} rows="4" disabled={submitting}></textarea>
        </div>

        <div class="field">
          <label for="node">Hosting Patch <span class="required">*</span></label>
          {#if loadingNodes}
            <select id="node" disabled>
              <option>Loading patches...</option>
            </select>
          {:else if myNodes.length === 0}
            <p class="muted" style="font-size: 0.85rem;">
              {lockSlug ? error || 'Could not load this patch.' : 'You need to be a member of a patch to create an event.'}
            </p>
          {:else}
            <select id="node" bind:value={nodeId} disabled={submitting || isEdit || !!lockSlug}>
              <option value="">Select a patch</option>
              {#each myNodes as node (node.node_id)}
                <option value={node.node_id}>{node.node_name}</option>
              {/each}
            </select>
          {/if}
        </div>

        <div class="field">
          <label for="location">Location</label>
          <input id="location" type="text" bind:value={location} placeholder="Where is this happening?" disabled={submitting} />
        </div>

        <div class="field-row">
          <div class="field">
            <label for="starts-at">Starts At <span class="required">*</span></label>
            <input id="starts-at" type="datetime-local" bind:value={startsAt} disabled={submitting} required />
          </div>
          <div class="field">
            <label for="ends-at">Ends At</label>
            <input id="ends-at" type="datetime-local" bind:value={endsAt} disabled={submitting} />
          </div>
        </div>

        <div class="field">
          <label for="recurrence">Recurrence</label>
          <select id="recurrence" bind:value={recurrence} disabled={submitting}>
            <option value="">One-time</option>
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
            <option value="biweekly">Every Two Weeks</option>
            <option value="monthly">Monthly</option>
          </select>
        </div>

        {#if error}
          <p class="error-text">{error}</p>
        {/if}

        <div class="field-actions">
          <button type="submit" class="btn btn-primary" disabled={submitting || myNodes.length === 0}>
            {submitting ? 'Saving...' : isEdit ? 'Save Changes' : lockSlug ? 'Suggest Event' : 'Create Event'}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            onclick={() => navigate(isEdit ? `/events/${eventId}` : lockSlug ? `/patches/${lockSlug}` : '/dashboard')}
          >
            Cancel
          </button>
        </div>
      </form>
      {/if}
    </div>
  </div>
</div>

<style>
  h1 {
    margin-bottom: 0.25rem;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .pending-confirm h2 {
    font-size: 1.1rem;
    margin-bottom: 0.35rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    flex: 1;
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .required {
    color: var(--color-error);
  }

  textarea {
    resize: vertical;
    min-height: 80px;
  }

  .field-row {
    display: flex;
    gap: 1rem;
  }

  .field-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }

  @media (max-width: 640px) {
    .field-row {
      flex-direction: column;
    }
  }
</style>
