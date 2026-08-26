<script>
  import { api } from '../lib/api.js';
  import { getInstanceTimezone } from '../stores/quilt.svelte.js';
  import { toZonedInputValue, fromZonedInputValue, sameZoneAsViewer } from '../lib/datetime.js';
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
  // The zone the times in this form are written in (docs/adr/045). On an
  // edit it is the event's resolved zone; on a new event it is the chosen
  // patch's, which is why it follows nodeId. Empty means the form is
  // reading and writing the editor's own clock, which is the honest
  // fallback before a patch is picked.
  let timezone = $state('');
  // What the patch would give this event on its own. The zone control
  // appears only when the two differ — a touring band's out-of-town date
  // — because a field that must be filled for every event is a field that
  // will be filled wrong.
  let patchTimezone = $state('');
  // Whether the editor has asked for the zone control. Hidden by default:
  // the only case that justifies asking is an event whose zone differs
  // from its patch's, and that is rare enough to be opt-in.
  let zoneOverridden = $state(false);

  // Whether a typed zone is one this browser can resolve. The server
  // checks too and is the authority; this is so a typo is visible before
  // a save round trip rather than after it.
  function isValidZone(tz) {
    try {
      new Intl.DateTimeFormat('en-US', { timeZone: tz });
      return true;
    } catch {
      return false;
    }
  }
  let recurrence = $state('');
  // A flyer or show photo, held wherever the patch already keeps it
  // (docs/adr/007). The description is required alongside it, and the server
  // refuses the pair without one.
  let imageUrl = $state('');
  let imageAlt = $state('');

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
      imageUrl = event.image_url || '';
      imageAlt = event.image_alt || '';
      // Read in the event's zone, not the editor's: an organizer editing a
      // Lancaster show sees 8:00 PM whether they are in Lancaster or on
      // tour, because 8pm is the fact they are editing.
      timezone = event.timezone || '';
      // The event payload's zone is already resolved, so an inheriting
      // event and one that pins its patch's zone look identical here.
      // Comparing against the patch's own answer is what keeps the
      // control hidden for the ordinary case — and what stops a plain
      // save from freezing a copy of a zone the event was inheriting.
      await loadPatchZone(event.node_slug);
      zoneOverridden = !!timezone && !!patchTimezone && timezone !== patchTimezone;
      startsAt = toZonedInputValue(event.starts_at, timezone);
      endsAt = toZonedInputValue(event.ends_at, timezone);
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
      patchTimezone = lockedNode.timezone || getInstanceTimezone() || '';
      if (!isEdit) timezone = patchTimezone;
      myNodes = [lockedNode];
    } catch (e) {
      error = e.message || 'Failed to load patch';
      myNodes = [];
    } finally {
      loadingNodes = false;
    }
  }

  // What zone this event would get from its patch alone. Fetched rather
  // than assumed: me/nodes carries membership rows, not patch fields, and
  // guessing the instance's here would make every save pin a zone the
  // event was happily inheriting.
  async function loadPatchZone(slug) {
    const fallback = getInstanceTimezone() || '';
    if (!slug) {
      patchTimezone = fallback;
      return;
    }
    try {
      const data = await api(`nodes/${slug}`);
      const node = data.node || data;
      patchTimezone = node.timezone || fallback;
    } catch {
      patchTimezone = fallback;
    }
  }

  // A new event follows the patch it is being posted to, so picking a
  // patch is what sets the clock the form is written in.
  async function pickPatchZone() {
    if (isEdit || !nodeId) return;
    const chosen = myNodes.find((n) => n.node_id === nodeId);
    await loadPatchZone(chosen?.node_slug || chosen?.slug);
    timezone = patchTimezone;
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
    if (timezone && !isValidZone(timezone)) return 'Timezone must be an IANA name, like America/New_York';
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
        starts_at: fromZonedInputValue(startsAt, timezone),
        ends_at: fromZonedInputValue(endsAt, timezone),
        // Sent only when it differs from what the patch would supply, so
        // an ordinary event stays inheriting rather than freezing a copy.
        timezone: timezone && timezone !== patchTimezone ? timezone : undefined,
        recurrence: recurrence || undefined,
        image_url: imageUrl.trim(),
        image_alt: imageAlt.trim(),
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

      <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
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
            <select id="node" bind:value={nodeId} onchange={pickPatchZone} disabled={submitting || isEdit || !!lockSlug}>
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

        <div class="field">
          <label for="image-url">Image address</label>
          <input id="image-url" type="url" bind:value={imageUrl} disabled={submitting} placeholder="https://..." />
          <p class="image-hint muted">
            Link a flyer or photo you already have online. Patchwork points at
            it and never keeps a copy.
          </p>
        </div>

        <!-- The description only appears once there is something to describe.
             Asking for alt text beside an empty address is a field with no
             referent. -->
        {#if imageUrl.trim()}
          <div class="field">
            <label for="image-alt">Describe the image</label>
            <input id="image-alt" type="text" bind:value={imageAlt} disabled={submitting} placeholder="Flyer for the March show" />
            <p class="image-hint muted">
              Read aloud by screen readers, and shown if the image stops
              loading.
            </p>
          </div>
        {/if}

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

        <!--
          The zone the boxes above are written in. Said out loud only when
          the editor is somewhere else, because an organizer posting a show
          at their own venue in their own city is not doing a conversion and
          does not need to be told about one (docs/adr/045).
        -->
        {#if timezone && !sameZoneAsViewer(timezone)}
          <p class="zone-note muted">
            Times are in {timezone.replace(/_/g, ' ')}{zoneOverridden ? '' : ", this patch's timezone"}.
          </p>
        {/if}

        <div class="field">
          {#if zoneOverridden}
            <label for="event-timezone">Timezone</label>
            <input
              id="event-timezone"
              type="text"
              bind:value={timezone}
              disabled={submitting}
              placeholder={patchTimezone || 'America/New_York'}
            />
            <p class="muted">
              An IANA name. Clear it to go back to
              {patchTimezone ? patchTimezone.replace(/_/g, ' ') : "the patch's timezone"}.
              {#if timezone && !isValidZone(timezone)}
                <span class="zone-invalid">Not a timezone this quilt knows.</span>
              {/if}
            </p>
          {:else}
            <button type="button" class="link-button" onclick={() => (zoneOverridden = true)} disabled={submitting}>
              This event is in a different timezone
            </button>
          {/if}
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
  .zone-note {
    margin: -0.25rem 0 0.75rem;
    font-size: 0.9rem;
  }
  .zone-invalid {
    color: var(--color-danger, #e05252);
  }
  .link-button {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.9rem;
    color: var(--color-primary);
    cursor: pointer;
    text-decoration: underline;
  }
  .link-button:disabled {
    cursor: default;
    opacity: 0.6;
  }

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

  .image-hint {
    font-size: 0.78rem;
    line-height: 1.5;
    margin: 0.3rem 0 0;
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
