<script>
  import { X } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { toZonedInputValue, fromZonedInputValue, sameZoneAsViewer } from '../lib/datetime.js';
  import { navigate, getQuery } from '../stores/router.svelte.js';
  import VocabLabel from '../components/VocabLabel.svelte';
  import WorkspaceSearch from '../components/WorkspaceSearch.svelte';
  import { reachablePatchPickerProvider } from '../lib/finderProviders.js';
  import { isAdmin, isTrustedContributor } from '../stores/auth.svelte.js';
  import { getMemberships } from '../stores/memberships.svelte.js';
  import { getSubmissionsEnabled, getInstanceTimezone } from '../stores/quilt.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  let { eventId = '', nodeSlug = '' } = $props();
  let isEdit = $derived(!!eventId);

  // A ?node=slug query param (or nodeSlug prop) pre-scopes the form to one
  // patch — the suggest-an-event door (docs/adr/026). The user may not
  // belong to that patch, and the door already answered which patch this
  // is for, so the field states it rather than asking again.
  let lockSlug = $derived(nodeSlug || getQuery().get('node') || '');

  let title = $state('');
  let description = $state('');
  let nodeId = $state('');
  let location = $state('');
  let startsAt = $state('');
  let endsAt = $state('');
  let recurrence = $state('');
  // A flyer or show photo, held wherever the patch already keeps it
  // (docs/adr/007). The description is required alongside it, and the server
  // refuses the pair without one.
  let imageUrl = $state('');
  let imageAlt = $state('');
  let eventUrl = $state('');

  // The patch this event is for: chosen in the picker, or fixed by the door
  // that was walked in through. { id, name, slug, status }.
  let hostingPatch = $state(null);
  // Fixed means the field states a patch rather than asking for one: an edit
  // cannot move an event between patches, and the suggest door (docs/adr/026)
  // is already scoped to the patch it came from. Neither is a choice, so
  // neither gets a picker.
  let fixed = $derived(isEdit || !!lockSlug);

  // The zone the times in this form are written in (docs/adr/045). It
  // follows the hosting patch, because an event inherits its patch's zone
  // unless it says otherwise — so picking a patch is what sets the clock
  // the boxes below are read in.
  let timezone = $state('');
  // What the patch would give this event on its own. The control appears
  // only when the two differ — a touring band's out-of-town date — because
  // a field that must be filled for every event is a field filled wrong.
  let patchTimezone = $state('');
  let zoneOverridden = $state(false);

  // Whether a typed zone is one this browser can resolve. The server checks
  // too and is the authority; this is so a typo is visible before a save
  // round trip rather than after it.
  function isValidZone(tz) {
    try {
      new Intl.DateTimeFormat('en-US', { timeZone: tz });
      return true;
    } catch {
      return false;
    }
  }

  // What zone this event would get from its patch alone. Fetched rather
  // than assumed: an event payload's zone arrives already resolved, so an
  // inheriting event and one pinning its patch's zone are indistinguishable
  // in it, and guessing the instance's here would make every save freeze a
  // copy of a zone the event was happily inheriting.
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
  // Set when a submit came back pending_review — the form is replaced by
  // a confirmation instead of navigating to an event nobody else can see.
  let pendingReviewers = $state('');
  let loadingPatch = $state(true);
  let submitting = $state(false);
  let error = $state('');

  $effect(() => {
    if (isEdit) {
      loadEvent();
    } else if (lockSlug) {
      loadLockedNode();
    } else {
      // Nothing to fetch: the picker loads its corpus on first focus.
      loadingPatch = false;
    }
  });

  async function loadEvent() {
    loadingPatch = true;
    try {
      const event = await api(`events/${eventId}`);
      title = event.title || '';
      description = event.description || '';
      nodeId = event.node_id || '';
      location = event.location || '';
      imageUrl = event.image_url || '';
      imageAlt = event.image_alt || '';
      eventUrl = event.event_url || '';
      // Read in the event's zone, not the editor's: an organizer editing a
      // Lancaster show sees 8:00 PM whether they are in Lancaster or on
      // tour, because 8pm is the fact they are editing.
      timezone = event.timezone || '';
      startsAt = toZonedInputValue(event.starts_at, timezone);
      endsAt = toZonedInputValue(event.ends_at, timezone);
      recurrence = event.recurrence || '';
      hostingPatch = {
        id: event.node_id,
        name: event.node_name || '',
        slug: event.node_slug || '',
        status: event.node_status || '',
      };
      await loadPatchZone(event.node_slug);
      zoneOverridden = !!timezone && !!patchTimezone && timezone !== patchTimezone;
    } catch (e) {
      error = e.message || 'Failed to load event';
    } finally {
      loadingPatch = false;
    }
  }

  async function loadLockedNode() {
    loadingPatch = true;
    try {
      const data = await api(`nodes/${lockSlug}`);
      const node = data.node || data;
      nodeId = node.id;
      hostingPatch = {
        id: node.id,
        name: node.name,
        slug: node.slug,
        status: data.is_unclaimed ? 'unclaimed' : node.status || 'active',
      };
      patchTimezone = node.timezone || getInstanceTimezone() || '';
      if (!isEdit) timezone = patchTimezone;
    } catch (e) {
      error = e.message || 'Failed to load patch';
    } finally {
      loadingPatch = false;
    }
  }

  // Active memberships only, matching the server's userHasNodeRole. me/nodes
  // serves 'active' and 'pending' alike, so the raw role map calls someone
  // with an outstanding join request a member — the picker would label the
  // row "member", the button would say Create Event, and the server would
  // make a submission. That is the same lying button this field exists to
  // stop, arriving by a different route.
  let activeRoles = $derived.by(() => {
    const roles = new Map();
    for (const m of getMemberships()) {
      if (m.status === 'active') roles.set(m.node_slug, m.role);
    }
    return roles;
  });

  // Who may post straight to a patch, mirroring CreateEvent (docs/adr/026):
  // members and admins of an active patch, the instance admin anywhere, and
  // a trusted contributor on an unclaimed one. Everybody else may still
  // suggest, and the field says so rather than finding out on submit.
  function postsDirectly(status, slug) {
    if (isAdmin()) return true;
    if (status === 'unclaimed') return isTrustedContributor();
    const role = activeRoles.get(slug);
    return role === 'member' || role === 'admin';
  }

  // Why a suggestion would be refused, in the words the server would use.
  // A patch that will not take one is shown and refused with the reason
  // rather than hidden (CONTEXT.md "Patch picker") — absence would read as
  // "not on this quilt".
  function refusalFor(node) {
    if (!getSubmissionsEnabled()) return 'not taking suggestions';
    if (node.status === 'active' && !node.accept_event_suggestions) {
      return 'not taking suggestions';
    }
    return '';
  }

  function hostingPatchProvider() {
    return reachablePatchPickerProvider((n) => {
      const direct = postsDirectly(n.status, n.slug);
      const refusal = direct ? '' : refusalFor(n);
      return {
        type: n.status === 'unclaimed' ? 'Unclaimed patches' : 'Patches',
        // Your own patches wear the standing that lets you post to them;
        // everything else says what pressing the button will actually do.
        sublabel: direct
          ? activeRoles.get(n.slug) || ''
          : refusal || 'will be reviewed',
        disabled: !!refusal,
        id: n.id,
        status: n.status,
      };
    });
  }

  async function choosePatch(item) {
    hostingPatch = {
      id: item.id,
      name: item.label,
      slug: item.slug,
      status: item.status,
    };
    nodeId = item.id;
    // A new event follows the patch it is being posted to, so picking a
    // patch is what sets the clock this form is written in.
    await loadPatchZone(item.slug);
    if (!isEdit && !zoneOverridden) timezone = patchTimezone;
  }

  function clearPatch() {
    hostingPatch = null;
    nodeId = '';
    // Without a patch there is no zone to inherit, so the form falls back
    // to the editor's own clock rather than keeping the last patch's.
    if (!zoneOverridden) {
      timezone = '';
      patchTimezone = '';
    }
  }

  // A pick the server will hold for review. Drives the button, so the label
  // promises what the patch actually allows.
  let willReview = $derived(
    !!hostingPatch && !postsDirectly(hostingPatch.status, hostingPatch.slug)
  );
  let reviewers = $derived(
    hostingPatch?.status === 'unclaimed' ? 'quilt admins' : 'patch admins'
  );

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
        // Sent only when it differs from what the patch would supply, so an
        // ordinary event stays inheriting rather than freezing a copy.
        timezone: timezone && timezone !== patchTimezone ? timezone : undefined,
        recurrence: recurrence || undefined,
        image_url: imageUrl.trim(),
        image_alt: imageAlt.trim(),
        event_url: eventUrl.trim(),
      };
      if (isEdit) {
        const result = await api(`events/${eventId}`, { method: 'PATCH', body });
        if (result?.status === 'pending_review') {
          // A non-trusted creator's edit on an unclaimed patch goes back
          // through review (docs/adr/026).
          pendingReviewers = reviewers;
          showToast('Submitted for review', 'success');
        } else {
          showToast('Event updated', 'success');
          navigate(`/events/${eventId}`);
        }
      } else {
        const result = await api('events', { method: 'POST', body });
        if (result?.status === 'pending_review') {
          pendingReviewers = reviewers;
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
          Suggest an event{hostingPatch ? ` for ${hostingPatch.name}` : ''}. It will be reviewed before it appears.
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
          {#if fixed}
            <!-- Stated, not asked. An edit cannot move an event between
                 patches and a suggestion goes to the patch you came from,
                 so a control here would be a dropdown that refuses to open. -->
            {#if loadingPatch}
              <p class="fixed-patch muted">Loading…</p>
            {:else if hostingPatch}
              <p class="fixed-patch">{hostingPatch.name}</p>
            {:else}
              <p class="muted" style="font-size: 0.85rem;">
                {error || 'Could not load this patch.'}
              </p>
            {/if}
          {:else if hostingPatch}
            <span class="chosen-patch">
              {hostingPatch.name}
              <button
                type="button"
                class="chip-x"
                title="Choose a different patch"
                disabled={submitting}
                onclick={clearPatch}
              >
                <X size={11} />
              </button>
            </span>
          {:else}
            <WorkspaceSearch
              variant="picker"
              matchField
              browse
              inputId="node"
              placeholder="Find a patch…"
              provider={hostingPatchProvider}
              onSelect={choosePatch}
            />
          {/if}
          {#if !fixed && willReview}
            <p class="image-hint muted">
              You aren't a member of this patch, so the {reviewers} will look
              at your event before it appears.
            </p>
          {/if}
        </div>

        <div class="field">
          <label for="location">Location</label>
          <input id="location" type="text" bind:value={location} placeholder="Where is this happening?" disabled={submitting} />
        </div>

        <!-- Where the event already lives on the web: tickets, the venue's
             own listing, the RSVP form. Events pulled from a calendar feed
             fill this in from the feed (docs/adr/079). -->
        <div class="field">
          <label for="event-url">Event page</label>
          <input id="event-url" type="url" bind:value={eventUrl} disabled={submitting} placeholder="https://..." />
          <p class="image-hint muted">
            Where to buy tickets or read more, if this event has a page
            somewhere else.
          </p>
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
          <button type="submit" class="btn btn-primary" disabled={submitting || !nodeId}>
            {submitting ? 'Saving...' : isEdit ? 'Save Changes' : willReview ? 'Suggest Event' : 'Create Event'}
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

  .fixed-patch {
    margin: 0;
    padding: 0.1rem 0;
    font-weight: 500;
  }

  .chosen-patch {
    display: inline-flex;
    align-items: center;
    align-self: flex-start;
    gap: 0.3rem;
    padding: 0.15rem 0.55rem;
    border: 1px dashed var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .chip-x {
    display: inline-flex;
    align-items: center;
    padding: 0;
    border: none;
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .chip-x:hover:not(:disabled) {
    color: var(--color-text);
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
