<script>
  import { getContext, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  let sources = $state([]);
  let loading = $state(true);
  let newUrl = $state('');
  let adding = $state(false);
  let syncing = $state({});
  let pollTimer = null;

  // Aggregators (docs/adr/056): names on instance-wide calendars this
  // patch can answer to. Mapping one is this patch's own act — nobody
  // else can point a city calendar at your events.
  let crosswalk = $state([]);
  let names = $state([]);
  let holds = $state([]);
  let mapping = $state({});
  let deciding = $state({});
  // Programs crediting this patch, and the offers they make
  // (docs/adr/063). An offer is an event somebody else owns whose title
  // matches a program credited here — nothing has happened to it, and
  // nothing will until someone here proposes the link and the owner
  // confirms.
  let programs = $state([]);
  let offers = $state([]);
  let offering = $state({});

  $effect(() => {
    if (slug) {
      loadSources();
      loadCrosswalk();
    }
  });

  onDestroy(() => clearTimeout(pollTimer));

  async function loadSources() {
    loading = true;
    try {
      const data = await api(`nodes/${slug}/event-sources`);
      sources = data.items || [];
      schedulePoll();
    } catch (e) {
      showToast('Failed to load event sources', 'error');
    } finally {
      loading = false;
    }
  }

  // A just-attached source syncs in the background; poll until it
  // settles so the row's status updates without a manual refresh.
  function schedulePoll() {
    clearTimeout(pollTimer);
    if (sources.some((s) => s.status === 'pending')) {
      pollTimer = setTimeout(async () => {
        try {
          const data = await api(`nodes/${slug}/event-sources`);
          sources = data.items || [];
        } catch (e) {
          // Next poll or manual action will surface it.
        }
        schedulePoll();
      }, 2000);
    }
  }

  async function addSource(e) {
    e.preventDefault();
    if (!newUrl.trim()) return;
    adding = true;
    try {
      await api(`nodes/${slug}/event-sources`, { method: 'POST', body: { url: newUrl.trim() } });
      newUrl = '';
      await loadSources();
    } catch (err) {
      showToast(err.message || 'Failed to attach feed', 'error');
    } finally {
      adding = false;
    }
  }

  async function removeSource(id) {
    try {
      await api(`nodes/${slug}/event-sources/${id}`, { method: 'DELETE' });
      showToast('Event source removed', 'info');
      await loadSources();
    } catch (e) {
      showToast('Failed to remove event source', 'error');
    }
  }

  async function syncNow(id) {
    syncing = { ...syncing, [id]: true };
    try {
      const updated = await api(`nodes/${slug}/event-sources/${id}/sync`, { method: 'POST' });
      sources = sources.map((s) => (s.id === id ? updated : s));
      if (updated.status === 'ok') {
        showToast('Synced', 'info');
      }
    } catch (err) {
      showToast(err.message || 'Sync failed', 'error');
      loadSources();
    } finally {
      syncing = { ...syncing, [id]: false };
    }
  }

  async function loadCrosswalk() {
    try {
      const [entries, available, held, progs] = await Promise.all([
        api(`nodes/${slug}/crosswalk`),
        api(`nodes/${slug}/aggregator-names`),
        api(`nodes/${slug}/aggregator-holds`),
        api(`nodes/${slug}/programs`),
      ]);
      crosswalk = entries.items || [];
      names = available.items || [];
      holds = held.items || [];
      programs = progs.items || [];
      offers = progs.offers || [];
    } catch (e) {
      crosswalk = [];
      names = [];
      holds = [];
      programs = [];
      offers = [];
    }
  }

  // Proposing is an ordinary event link (docs/adr/032) — the owning patch
  // still confirms. The program only found it.
  async function proposeLink(offer) {
    offering = { ...offering, [offer.event_id]: true };
    try {
      await api(`events/${offer.event_id}/links`, {
        method: 'POST',
        body: { target: slug },
      });
      showToast(`Asked ${offer.owner_name} to add you to “${offer.title}”`);
      await loadCrosswalk();
    } catch (err) {
      showToast(err.data?.error || 'Failed to propose that link', 'error');
    } finally {
      offering = { ...offering, [offer.event_id]: false };
    }
  }

  async function dismissOffer(offer) {
    offering = { ...offering, [offer.event_id]: true };
    try {
      await api(`nodes/${slug}/offers/dismiss`, {
        method: 'POST',
        body: { program_id: offer.program_id, event_id: offer.event_id },
      });
      await loadCrosswalk();
    } catch (err) {
      showToast(err.data?.error || 'Failed to dismiss that', 'error');
    } finally {
      offering = { ...offering, [offer.event_id]: false };
    }
  }

  async function stopProgram(program) {
    try {
      await api(`nodes/${slug}/programs/${program.id}`, { method: 'DELETE' });
      showToast('Stopped. Events you already joined stay yours.');
      await loadCrosswalk();
    } catch (err) {
      showToast(err.data?.error || 'Failed to stop that', 'error');
    }
  }

  async function mapName(name) {
    mapping = { ...mapping, [name.name_key]: true };
    try {
      await api(`nodes/${slug}/crosswalk`, {
        method: 'POST',
        body: { aggregator_id: name.aggregator_id, name_key: name.name_key },
      });
      showToast(`Events listed as "${name.display_name}" now arrive here`);
      await loadCrosswalk();
    } catch (err) {
      showToast(err.data?.error || 'Failed to map name', 'error');
    } finally {
      mapping = { ...mapping, [name.name_key]: false };
    }
  }

  async function unmap(entry) {
    try {
      await api(`nodes/${slug}/crosswalk/${entry.id}`, { method: 'DELETE' });
      showToast('Stopped. Its events are yours to edit now.');
      await loadCrosswalk();
    } catch (e) {
      showToast('Failed to unmap', 'error');
    }
  }

  async function decideHold(hold, decision) {
    deciding = { ...deciding, [hold.id]: true };
    try {
      await api(`aggregator-holds/${hold.id}/decide`, { method: 'POST', body: { decision } });
      showToast(decision === 'same' ? 'Kept yours' : 'Added as a separate event');
      await loadCrosswalk();
    } catch (err) {
      showToast(err.data?.error || 'Failed to record that', 'error');
    } finally {
      deciding = { ...deciding, [hold.id]: false };
    }
  }

  function whenOf(iso) {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString(undefined, {
      month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
    });
  }

  function hostOf(url) {
    try {
      return new URL(url).host;
    } catch (e) {
      return url;
    }
  }

  function relTime(iso) {
    if (!iso) return null;
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return null;
    const mins = Math.round((Date.now() - then) / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.round(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.round(hours / 24)}d ago`;
  }
</script>

<div class="page-fade">
  <h2>Event Sources</h2>
  <p class="muted subtitle">
    Calendar feeds this patch pulls events from. Paste an ICS address (a
    Google Calendar's secret address, the calendar feed from your website),
    an events page from Squarespace, Humanitix, and similar sites, or an
    atproto handle whose calendar you want to follow.
    Imported events publish directly and stay in step with the feed.
  </p>

  {#if loading}
    <p class="muted" style="padding: 2rem 0;">Loading...</p>
  {:else}
    {#if sources.length > 0}
      <ul class="source-list">
        {#each sources as source (source.id)}
          <li class="source-row">
            <div class="source-info">
              <span class="source-host">{hostOf(source.url)}</span>
              <span class="source-url muted">{source.url}</span>
              <span class="source-status">
                {#if source.status === 'pending'}
                  <span class="muted">First sync in progress…</span>
                {:else if source.status === 'error'}
                  <span class="status-error">Sync failed{source.last_error ? `: ${source.last_error}` : ''}</span>
                {:else}
                  <span class="muted">
                    {source.event_count} {source.event_count === 1 ? 'event' : 'events'}
                    {#if relTime(source.last_success_at)}· synced {relTime(source.last_success_at)}{/if}
                  </span>
                {/if}
              </span>
            </div>
            <div class="source-actions">
              <button
                class="btn btn-secondary btn-sm"
                disabled={syncing[source.id] || source.status === 'pending'}
                onclick={() => syncNow(source.id)}
              >{syncing[source.id] ? 'Syncing…' : 'Sync now'}</button>
              <ConfirmAction
                label="Remove"
                confirmLabel="Yes, remove it and its upcoming events"
                variant="danger"
                onConfirm={() => removeSource(source.id)}
              />
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="muted empty">No event sources yet.</p>
    {/if}

    <form class="add-form" onsubmit={addSource}>
      <!-- Deliberately type="text": an atproto handle is a bare domain
           (docs/adr/064), and type="url" makes the browser refuse to submit
           one before the server ever sees it. The server validates. -->
      <input
        type="text"
        inputmode="url"
        placeholder="https://…/basic.ics  or  an atproto handle"
        bind:value={newUrl}
        disabled={adding}
      />
      <button class="btn btn-primary" type="submit" disabled={adding || !newUrl.trim()}>
        {adding ? 'Attaching…' : 'Attach feed'}
      </button>
    </form>
    <p class="muted hint">
      Removing a source keeps past events and removes upcoming imported ones.
    </p>
  {/if}

  {#if holds.length > 0}
    <h2 class="section-head">Possible duplicates</h2>
    <p class="muted subtitle">
      Each of these lands at the same time as an event you already have.
      Yours is the one showing.
    </p>
    <ul class="hold-list">
      {#each holds as hold (hold.id)}
        <li class="hold-row">
          <div class="hold-pair">
            <span class="hold-when muted">{whenOf(hold.starts_at)}</span>
            <span class="hold-yours"><strong>Yours:</strong> {hold.rival_title}</span>
            <span class="hold-theirs">
              <strong>{hold.aggregator_name}:</strong> {hold.title}
              {#if hold.location}<span class="muted">{' · '}{hold.location}</span>{/if}
            </span>
          </div>
          <div class="hold-actions">
            <button
              class="btn btn-secondary btn-sm"
              disabled={deciding[hold.id]}
              onclick={() => decideHold(hold, 'same')}
            >Same event</button>
            <button
              class="btn btn-secondary btn-sm"
              disabled={deciding[hold.id]}
              onclick={() => decideHold(hold, 'different')}
            >Add it too</button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}

  {#if programs.length > 0}
    <h2 class="section-head">Programs credited to you</h2>
    <p class="muted subtitle">
      Recurring listings someone recognized as yours. They stay on the
      venue's calendar — these are offers to put your name on them too, and
      the venue confirms each one.
    </p>
    <ul class="program-list">
      {#each programs as p (p.id)}
        <li class="program-row">
          <div class="program-head">
            <span class="program-title">{p.display_title}</span>
            <span class="muted program-sub">
              {'listed under '}{p.display_name}{' on '}{p.aggregator_name}
            </span>
          </div>
          <button class="link-stop" onclick={() => stopProgram(p)}>Stop</button>
        </li>
        {@const mine = offers.filter((o) => o.program_id === p.id)}
        {#if mine.length === 0}
          <li class="offer-empty muted">
            {p.routed
              ? 'Nothing waiting — every listing is either linked or dismissed.'
              : `Nothing yet: “${p.display_name}” has not been mapped to a patch, so these listings are not events on this quilt.`}
          </li>
        {:else}
          {#each mine as o (o.event_id)}
            <li class="offer-row">
              <div class="offer-info">
                <span class="offer-when muted">{whenOf(o.starts_at)}</span>
                <span class="offer-title">{o.title}</span>
                <span class="muted offer-owner">{'on '}{o.owner_name}</span>
              </div>
              <div class="offer-actions">
                <button
                  class="btn btn-secondary btn-sm"
                  disabled={offering[o.event_id]}
                  onclick={() => proposeLink(o)}
                >Ask to be added</button>
                <button
                  class="btn btn-secondary btn-sm"
                  disabled={offering[o.event_id]}
                  onclick={() => dismissOffer(o)}
                >Not ours</button>
              </div>
            </li>
          {/each}
        {/if}
      {/each}
    </ul>
  {/if}

  <h2 class="section-head">Listing calendars</h2>
  <p class="muted subtitle">
    Calendars that list events across the community, like the city calendar.
    Answering to a name brings every event filed under it here from now on.
    Anything an admin points at you waits in your review queue first. Stop
    any of them here.
  </p>

  {#if crosswalk.length > 0}
    <ul class="source-list">
      {#each crosswalk as entry (entry.id)}
        <li class="source-row">
          <div class="source-info">
            <span class="source-host">{entry.display_name}</span>
            <span class="source-url muted">on {entry.aggregator_name}</span>
            <span class="source-status muted">
              {#if entry.suggests}
                Suggests into your review queue{#if entry.added_by_name}, set up by {entry.added_by_name}{/if}
                {#if entry.pending_count > 0}
                  {' · '}<a href={`/patches/${slug}/events?review=1`}>
                    {entry.pending_count} waiting
                  </a>
                {/if}
                {#if entry.event_count > 0}{' · '}{entry.event_count} approved{/if}
              {:else}
                {entry.event_count} {entry.event_count === 1 ? 'event' : 'events'}
                {#if relTime(entry.last_success_at)}{' · '}updated {relTime(entry.last_success_at)}{/if}
              {/if}
            </span>
          </div>
          <div class="source-actions">
            <ConfirmAction
              label="Stop"
              confirmLabel={entry.suggests
                ? 'Yes, stop. Keep what you approved, drop the rest.'
                : 'Yes, stop. Keep its events.'}
              variant="danger"
              onConfirm={() => unmap(entry)}
            />
          </div>
        </li>
      {/each}
    </ul>
  {/if}

  {#if names.length > 0}
    <ul class="name-list">
      {#each names as name (name.aggregator_id + name.name_key)}
        <li class="name-row">
          <div class="name-info">
            <span class="name-label">{name.display_name}</span>
            <span class="muted name-count">
              {name.count} {name.count === 1 ? 'listing' : 'listings'} on {' '}
              {name.aggregator_name || 'a listing calendar'}
            </span>
            {#if name.sample_titles?.length}
              <span class="muted samples">{name.sample_titles.join(' · ')}</span>
            {/if}
          </div>
          <button
            class="btn btn-secondary btn-sm"
            disabled={mapping[name.name_key]}
            onclick={() => mapName(name)}
          >{mapping[name.name_key] ? 'Mapping…' : "That's us"}</button>
        </li>
      {/each}
    </ul>
  {:else if crosswalk.length === 0}
    <p class="muted empty">
      No listing calendars are carrying a name this patch could answer to.
    </p>
  {/if}
</div>

<style>
  h2 {
    font-size: 1.2rem;
    margin-bottom: 0.25rem;
  }

  .subtitle {
    font-size: 0.85rem;
    margin-bottom: 1.5rem;
  }

  .source-list {
    list-style: none;
    padding: 0;
    margin: 0 0 1.5rem;
  }

  .source-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .source-info {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .source-host {
    font-size: 0.92rem;
    font-weight: 500;
  }

  .source-url {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 340px;
  }

  .source-status {
    font-size: 0.8rem;
  }

  .status-error {
    color: var(--color-danger, #c0392b);
  }

  .source-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .add-form {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .add-form input {
    flex: 1;
    min-width: 0;
  }

  .hint {
    font-size: 0.78rem;
    margin-top: 0.5rem;
  }

  .empty {
    padding: 1rem 0;
  }

  .section-head {
    margin-top: 2.25rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--color-border);
  }

  .name-list,
  .program-list { list-style: none; padding: 0; margin: 0 0 1.5rem; }
  .program-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 1rem;
    padding: 0.7rem 0 0.35rem;
    border-top: 1px solid var(--color-border);
    flex-wrap: wrap;
  }
  .program-head { display: flex; flex-direction: column; gap: 0.1rem; min-width: 0; }
  .program-title { font-weight: 500; }
  .program-sub { font-size: 0.82rem; }
  .link-stop {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.82rem;
    color: var(--color-text-muted);
    cursor: pointer;
  }
  .link-stop:hover { text-decoration: underline; }
  .offer-empty { font-size: 0.82rem; padding: 0.15rem 0 0.6rem 1rem; line-height: 1.5; }
  .offer-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.5rem 0 0.5rem 1rem;
    flex-wrap: wrap;
  }
  .offer-info { display: flex; flex-direction: column; gap: 0.1rem; min-width: 0; }
  .offer-when { font-size: 0.8rem; }
  .offer-owner { font-size: 0.8rem; }
  .offer-actions { display: flex; gap: 0.4rem; }

  .hold-list {
    list-style: none;
    padding: 0;
    margin: 0.75rem 0 0;
  }

  .name-row,
  .hold-row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    padding: 0.65rem 0;
    border-bottom: 1px solid var(--color-border);
    flex-wrap: wrap;
  }

  .name-row:last-child,
  .hold-row:last-child {
    border-bottom: none;
  }

  .name-info,
  .hold-pair {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
    flex: 1 1 16rem;
  }

  .name-label {
    font-weight: 500;
  }

  .name-count,
  .samples,
  .hold-when {
    font-size: 0.82rem;
  }

  .samples {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hold-yours,
  .hold-theirs {
    font-size: 0.9rem;
  }

  .hold-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
    flex-wrap: wrap;
  }
</style>
