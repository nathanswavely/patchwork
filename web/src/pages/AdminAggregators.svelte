<script>
  // Aggregators and the crosswalk (docs/adr/056). An aggregator lists
  // events it does not own — a city calendar, a chamber of commerce —
  // so it owns nothing here either: attaching one creates no patch and
  // no event. Names become events only where a crosswalk entry
  // addresses them, and an active patch is mapped by its own admins,
  // never from this page.
  //
  // The names are the page. Attaching a feed happens once; mapping
  // names is the standing work, so it is the body rather than something
  // folded inside a feed row — the shape Event Submissions and Claims
  // already use for an admin's queue.
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import Modal from '../components/Modal.svelte';
  import WorkspaceSearch from '../components/WorkspaceSearch.svelte';
  import { patchPickerProvider } from '../lib/finderProviders.js';

  let aggregators = $state([]);
  let names = $state([]);
  // Programs credited so far (docs/adr/063). They sit in the same list
  // as the names: both are a key grouping listings that someone had to
  // read and judge, and splitting them would hide that a name has been
  // read already.
  let programs = $state([]);
  let crediting = $state({});
  let loading = $state(true);
  let showIgnored = $state(false);
  let ignoredCount = $state(0);

  // Opening a name shows what it actually carries. No summary settles
  // whether "West Art" is an organization or a room — you have to read
  // the listings, and sometimes go and look at the publisher's page.
  let openName = $state(null);
  let listings = $state([]);
  let listingsLoading = $state(false);
  let newName = $state('');
  let newUrl = $state('');
  let adding = $state(false);
  let error = $state('');

  let mapping = $state({});
  let pollTimer = null;

  async function load() {
    try {
      const [feeds, shown, ignoredSet, progs] = await Promise.all([
        api('admin/aggregators'),
        api(`admin/aggregator-names${showIgnored ? '?ignored=true' : ''}`),
        api('admin/aggregator-names?ignored=true'),
        api('admin/programs'),
      ]);
      aggregators = feeds.items || [];
      names = shown.items || [];
      ignoredCount = (ignoredSet.items || []).length;
      programs = progs.items || [];
      schedulePoll();
    } catch {
      aggregators = [];
      names = [];
      programs = [];
    } finally {
      loading = false;
    }
  }

  async function openListings(name) {
    openName = name;
    listingsLoading = true;
    listings = [];
    try {
      const data = await api(
        `admin/aggregator-listings?aggregator_id=${encodeURIComponent(name.aggregator_id)}` +
        `&name_key=${encodeURIComponent(name.name_key)}`,
      );
      listings = data.items || [];
    } catch {
      listings = [];
    } finally {
      listingsLoading = false;
    }
  }

  // The drawer groups by title, because that is what a program is: the
  // seven dates of one walking tour, not seven things to decide about
  // (docs/adr/063). The feed names no organizer, so the reader supplies
  // the answer — the grouping only saves them from typing it seven times.
  let titleGroups = $derived.by(() => {
    const by = new Map();
    for (const l of listings) {
      const key = l.title_key || l.title;
      if (!by.has(key)) by.set(key, { titleKey: l.title_key, title: l.title, items: [] });
      by.get(key).items.push(l);
    }
    return [...by.values()];
  });

  // Names and programs in one list, each program under the name it
  // was recognized beneath. A program whose name is already routed has
  // no row to sit under, so it goes last.
  let rows = $derived.by(() => {
    const out = [];
    const placed = new Set();
    for (const n of names) {
      out.push({ kind: 'name', key: `n:${n.aggregator_id}:${n.name_key}`, name: n });
      for (const p of programs) {
        if (p.aggregator_id === n.aggregator_id && p.name_key === n.name_key) {
          out.push({ kind: 'program', key: `p:${p.id}`, program: p });
          placed.add(p.id);
        }
      }
    }
    for (const p of programs) {
      if (!placed.has(p.id)) out.push({ kind: 'program', key: `p:${p.id}`, program: p });
    }
    return out;
  });

  function creditedFor(group) {
    if (!openName) return null;
    return programs.find(
      (p) =>
        p.aggregator_id === openName.aggregator_id &&
        p.name_key === openName.name_key &&
        p.title_key === group.titleKey,
    );
  }

  // Any patch on the quilt: an instance admin speaks for all of them, and
  // crediting asserts nothing about the venue whose event it is — that
  // patch's calendar changes only if someone proposes a link and its own
  // admins confirm (docs/adr/063).
  function creditProvider() {
    return patchPickerProvider((n) => ({
      type: n.status === 'unclaimed' ? 'Unclaimed' : 'Active',
      sublabel: n.description ? n.description.slice(0, 60) : '',
    }));
  }

  async function creditTo(group, patch) {
    crediting = { ...crediting, [group.titleKey]: true };
    try {
      await api(`nodes/${patch.slug}/programs`, {
        method: 'POST',
        body: {
          aggregator_id: openName.aggregator_id,
          name_key: openName.name_key,
          title_key: group.titleKey,
        },
      });
      showToast(`“${group.title}” credited to ${patch.label}.`);
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Failed to credit', 'error');
    } finally {
      crediting = { ...crediting, [group.titleKey]: false };
    }
  }

  async function uncredit(program) {
    try {
      await api(`nodes/${program.node_slug}/programs/${program.id}`, { method: 'DELETE' });
      showToast(`“${program.display_title}” is no longer credited to ${program.node_name}.`);
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Failed to remove credit', 'error');
    }
  }

  async function setIgnored(name, ignore) {
    try {
      await api(`admin/aggregator-names/${ignore ? 'ignore' : 'unignore'}`, {
        method: 'POST',
        body: { aggregator_id: name.aggregator_id, name_key: name.name_key },
      });
      if (openName?.name_key === name.name_key) openName = null;
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Failed to record that', 'error');
    }
  }

  // A just-attached aggregator fetches in the background.
  function schedulePoll() {
    clearTimeout(pollTimer);
    if (aggregators.some((a) => a.status === 'pending')) {
      pollTimer = setTimeout(load, 2000);
    }
  }

  async function handleAdd(e) {
    e.preventDefault();
    if (!newName.trim() || !newUrl.trim() || adding) return;
    adding = true;
    error = '';
    try {
      await api('admin/aggregators', {
        method: 'POST',
        body: { name: newName.trim(), url: newUrl.trim() },
      });
      showToast('Feed attached. Make sure you map any names to get the events flowing.');
      newName = '';
      newUrl = '';
      await load();
    } catch (err) {
      error = err.data?.error || 'Failed to attach feed.';
    } finally {
      adding = false;
    }
  }

  async function handleRemove(agg) {
    try {
      await api(`admin/aggregators/${agg.id}`, { method: 'DELETE' });
      showToast('Feed removed. Mapped patches kept their events.');
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Failed to remove', 'error');
    }
  }

  async function togglePause(agg) {
    try {
      await api(`admin/aggregators/${agg.id}`, {
        method: 'PATCH',
        body: { paused: !agg.paused },
      });
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Failed to update', 'error');
    }
  }

  async function syncNow(agg) {
    try {
      await api(`admin/aggregators/${agg.id}/sync`, { method: 'POST' });
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Sync failed', 'error');
    }
  }

  // The patch picker's corpus, fetched once per dropdown and filtered in
  // memory the way every other search here works (docs/adr/033). Three
  // groups, because where a name lands changes what arrives: an unclaimed
  // patch is held in trust so its events publish; a claimed patch that
  // accepts suggestions gets a review queue; one that doesn't is shown
  // and refused, since hiding it would read as "not on this quilt".
  function patchProvider() {
    return patchPickerProvider((n) => {
      const unclaimed = n.status === 'unclaimed';
      const open = n.accept_event_suggestions;
      return {
        type: unclaimed
          ? 'Unclaimed, events publish'
          : open
            ? 'Accepts suggestions, events await review'
            : 'Not accepting suggestions',
        sublabel: n.description ? n.description.slice(0, 60) : '',
        disabled: !unclaimed && !open,
        suggests: !unclaimed && open,
      };
    });
  }

  async function mapTo(name, patch) {
    mapping = { ...mapping, [name.name_key]: true };
    try {
      await api(`nodes/${patch.slug}/crosswalk`, {
        method: 'POST',
        body: { aggregator_id: name.aggregator_id, name_key: name.name_key },
      });
      showToast(
        patch.suggests
          ? `"${name.display_name}" now suggests to ${patch.label}. Their admins review it.`
          : `"${name.display_name}" now routes to ${patch.label}.`,
      );
      await load();
    } catch (err) {
      showToast(err.data?.error || 'Failed to map', 'error');
    } finally {
      mapping = { ...mapping, [name.name_key]: false };
    }
  }

  // No patch fits the name: make one. Prefilled with what the feed wrote,
  // not what the admin typed to find it.
  function suggestPatchFor(name) {
    navigate(`/submit?name=${encodeURIComponent(name.display_name)}`);
  }

  function whenOf(iso) {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString(undefined, {
      weekday: 'short', month: 'short', day: 'numeric',
      hour: 'numeric', minute: '2-digit',
    });
  }

  function hostOf(url) {
    try {
      return new URL(url).host;
    } catch {
      return 'the calendar';
    }
  }

  // Feed descriptions run long and carry their own line breaks; a few
  // lines is enough to tell a venue from a room.
  function snippet(text) {
    const clean = (text || '').replace(/\s+/g, ' ').trim();
    return clean.length > 320 ? `${clean.slice(0, 320)}…` : clean;
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

  $effect(() => { load(); });
</script>

<div class="admin-page">
  <h1>Aggregators</h1>
  <p class="page-desc">
    Add a feed here if you have a calendar link that doesn't belong to a
    specific patch. Things like city or chamber of commerce calendars.
  </p>

  <section class="feeds">
    <h2>Feeds</h2>
    {#if loading}
      <p class="muted">Loading…</p>
    {:else if aggregators.length === 0}
      <p class="muted">No feeds attached.</p>
    {:else}
      <ul class="feed-list">
        {#each aggregators as agg (agg.id)}
          <li class="feed-row">
            <div class="feed-info">
              <span class="feed-name">{agg.name}</span>
              <span class="feed-status muted">
                {#if agg.status === 'pending'}
                  First fetch in progress…
                {:else if agg.status === 'error'}
                  <span class="status-error">Fetch failed{agg.last_error ? `: ${agg.last_error}` : ''}</span>
                {:else}
                  {agg.listing_count} {agg.listing_count === 1 ? 'listing' : 'listings'}
                  {' · '}{agg.mapped_count} routed
                  {#if relTime(agg.last_success_at)}{' · '}fetched {relTime(agg.last_success_at)}{/if}
                {/if}
                {#if agg.paused}{' · '}<span class="paused-chip">paused</span>{/if}
              </span>
            </div>
            <div class="feed-actions">
              <button class="btn btn-secondary btn-sm" onclick={() => syncNow(agg)}>Sync now</button>
              <button class="btn btn-secondary btn-sm" onclick={() => togglePause(agg)}>
                {agg.paused ? 'Resume' : 'Pause'}
              </button>
              <ConfirmAction
                label="Remove"
                confirmLabel="Yes, remove it. Mapped patches keep their events."
                variant="danger"
                onConfirm={() => handleRemove(agg)}
              />
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <form class="add-form" onsubmit={handleAdd}>
      <input type="text" bind:value={newName} placeholder="Visit Lancaster City" disabled={adding} />
      <input type="url" bind:value={newUrl} placeholder="https://…/calendar.ics" disabled={adding} />
      <button type="submit" class="btn btn-secondary btn-sm" disabled={adding || !newName.trim() || !newUrl.trim()}>
        {adding ? 'Attaching…' : 'Attach feed'}
      </button>
    </form>
    {#if error}<p class="form-error">{error}</p>{/if}
  </section>

  <div class="names-head">
    <h2>
      {showIgnored ? 'Ignored names' : 'Unrouted names'}
      {#if names.length}<span class="muted count">{names.length}</span>{/if}
    </h2>
    {#if ignoredCount > 0 || showIgnored}
      <button
        class="btn btn-secondary btn-sm"
        onclick={() => { showIgnored = !showIgnored; load(); }}
      >{showIgnored ? 'Back to unrouted' : `Show ignored (${ignoredCount})`}</button>
    {/if}
  </div>

  {#if loading}
    <p class="muted" style="padding: 2rem 0;">Loading…</p>
  {:else if aggregators.length === 0}
    <p class="muted empty">Attach a feed to see the names it carries.</p>
  {:else if names.length === 0}
    <p class="muted empty">
      {showIgnored ? 'No names have been ignored.' : 'Every name your feeds carry is mapped or ignored.'}
    </p>
  {:else}
    <p class="muted names-note">
      {#if showIgnored}
        Hidden from this list only. Patch admins still see every name in
        their own picker.
      {:else}
        Names found in your feeds show up here, and you can map them to
        patches on your quilt.
      {/if}
    </p>
    <ul class="name-list">
      {#each rows as row (row.key)}
        {#if row.kind === 'program'}
          <li class="name-row program-row">
            <div class="name-info">
              <span class="name-label">{row.program.display_title}</span>
              <span class="muted name-count">
                {'Credited to '}{row.program.node_name}
                {' · '}{row.program.listing_count}
                {row.program.listing_count === 1 ? ' listing under ' : ' listings under '}
                {row.program.display_name}
              </span>
              {#if !row.program.routed}
                <span class="muted samples">
                  {`Waiting on “${row.program.display_name}” to be mapped — until it is, there are no events to offer.`}
                </span>
              {:else if row.program.offer_count > 0}
                <span class="muted samples">
                  {row.program.offer_count === 1
                    ? '1 offer waiting on their admins'
                    : `${row.program.offer_count} offers waiting on their admins`}
                </span>
              {/if}
            </div>
            <div class="name-map">
              <button class="link-ignore" onclick={() => uncredit(row.program)}>
                Remove credit
              </button>
            </div>
          </li>
        {:else}
          {@const name = row.name}
          <li class="name-row">
            <div class="name-info">
            <span class="name-label">{name.display_name}</span>
            <span class="muted name-count">
              <button class="link-count" onclick={() => openListings(name)}>
                {name.count} {name.count === 1 ? 'listing' : 'listings'}
              </button>
              {' on '}{name.aggregator_name}
            </span>
            {#if name.sample_titles?.length}
              <span class="muted samples">{name.sample_titles.join(' · ')}</span>
            {/if}
          </div>
          <div class="name-map">
            {#if showIgnored}
              <button class="btn btn-secondary btn-sm" onclick={() => setIgnored(name, false)}>
                Stop ignoring
              </button>
            {:else}
              <WorkspaceSearch
                variant="picker"
                placeholder="Find a patch…"
                provider={patchProvider}
                onSelect={(patch) => mapTo(name, patch)}
                suggestLabel={() => `Suggest “${name.display_name}” as a patch`}
                onSuggest={() => suggestPatchFor(name)}
                alwaysSuggest
              />
              <button class="link-ignore" onclick={() => setIgnored(name, true)}>
                Ignore this name
              </button>
            {/if}
          </div>
          </li>
        {/if}
      {/each}
    </ul>
  {/if}
</div>

<Modal
  open={!!openName}
  onClose={() => { openName = null; }}
  label={openName ? `Listings filed under ${openName.display_name}` : 'Listings'}
>
  {#if openName}
    <div class="listings-modal">
      <h2>{openName.display_name}</h2>
      <p class="muted modal-sub">
        {openName.count} {openName.count === 1 ? 'listing' : 'listings'} on {openName.aggregator_name}
      </p>

      {#if listingsLoading}
        <p class="muted">Loading…</p>
      {:else}
        <p class="muted modal-note">
          Mapping this name decides whose calendar these land on. If one is
          run by somebody else, credit it to them — the events stay here
          either way.
        </p>
        <ul class="listing-list">
          {#each titleGroups as g (g.titleKey)}
            {@const credited = creditedFor(g)}
            <li class="title-group">
              <div class="group-head">
                <span class="listing-title">{g.title}</span>
                <span class="muted group-count">
                  {g.items.length}{g.items.length === 1 ? ' date' : ' dates'}
                </span>
              </div>
              {#if credited}
                <p class="muted credited-note">
                  {'Credited to '}{credited.node_name}{'.'}
                  <button class="link-ignore" onclick={() => uncredit(credited)}>
                    Remove credit
                  </button>
                </p>
              {:else}
                <div class="group-credit">
                  <WorkspaceSearch
                    variant="picker"
                    placeholder="Who runs this?"
                    provider={creditProvider}
                    onSelect={(patch) => creditTo(g, patch)}
                    suggestLabel={() => `Suggest “${g.title}” as a patch`}
                    onSuggest={() => suggestPatchFor({ display_name: g.title })}
                    alwaysSuggest
                  />
                </div>
              {/if}
              <ul class="date-list">
                {#each g.items as l (l.uid + l.occurrence)}
                  <li class="listing">
                    <span class="listing-when muted">{whenOf(l.starts_at)}</span>
                    {#if l.location}<span class="muted listing-loc">{l.location}</span>{/if}
                    {#if l.description}
                      <p class="listing-desc">{snippet(l.description)}</p>
                    {/if}
                    {#if l.url}
                      <a href={l.url} target="_blank" rel="noopener noreferrer" class="listing-link">
                        View on {hostOf(l.url)} ↗
                      </a>
                    {/if}
                  </li>
                {/each}
              </ul>
            </li>
          {/each}
        </ul>
      {/if}

      <div class="modal-actions">
        {#if openName.ignored}
          <button class="btn btn-secondary btn-sm" onclick={() => setIgnored(openName, false)}>
            Stop ignoring
          </button>
        {:else}
          <button class="btn btn-secondary btn-sm" onclick={() => setIgnored(openName, true)}>
            Ignore this name
          </button>
        {/if}
        <button class="btn btn-secondary btn-sm" onclick={() => { openName = null; }}>Close</button>
      </div>
    </div>
  {/if}
</Modal>

<style>
  .admin-page { max-width: 60rem; }
  h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
  .page-desc { color: var(--color-text-muted); margin-bottom: 1.75rem; line-height: 1.6; }

  .feeds {
    border: 1px solid var(--color-border);
    border-radius: 8px;
    padding: 0.9rem 1rem 1rem;
    margin-bottom: 2.25rem;
    background: var(--color-bg-subtle, transparent);
  }
  .feeds h2 {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--color-text-muted);
    margin: 0 0 0.6rem;
  }
  .feed-list { list-style: none; padding: 0; margin: 0 0 0.75rem; }
  .feed-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--color-border);
    flex-wrap: wrap;
  }
  .feed-row:last-child { border-bottom: none; }
  .feed-info { display: flex; flex-direction: column; gap: 0.15rem; min-width: 0; }
  .feed-name { font-weight: 600; }
  .feed-status { font-size: 0.82rem; }
  .status-error { color: var(--color-danger); }
  .paused-chip { font-style: italic; }
  .feed-actions { display: flex; gap: 0.4rem; flex-wrap: wrap; }

  .add-form { display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: center; }
  .add-form input {
    flex: 1 1 12rem;
    min-width: 0;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: 6px;
    background: var(--color-bg);
    color: var(--color-text);
  }
  .form-error { color: var(--color-danger); margin: 0.5rem 0 0; }

  .names-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 0.35rem;
    flex-wrap: wrap;
  }
  .names-head h2 { font-size: 1.15rem; margin: 0; display: flex; align-items: baseline; gap: 0.5rem; }
  .names-head .count { font-size: 0.9rem; font-weight: 400; }

  /* The listing count is the door to the listings themselves. */
  .link-count,
  .link-ignore {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--color-link, var(--color-accent));
    cursor: pointer;
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .link-ignore {
    color: var(--color-text-muted);
    font-size: 0.8rem;
    align-self: flex-start;
    text-decoration: none;
  }
  .link-ignore:hover { text-decoration: underline; }

  .listings-modal { max-width: 34rem; }
  .listings-modal h2 { font-size: 1.15rem; margin: 0 0 0.15rem; }
  .modal-sub { font-size: 0.85rem; margin: 0 0 1rem; }
  .listing-list { list-style: none; padding: 0; margin: 0; max-height: 26rem; overflow-y: auto; }
  .listing {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: 0.7rem 0;
    border-bottom: 1px solid var(--color-border);
  }
  .listing:last-child { border-bottom: none; }
  .listing-when { font-size: 0.8rem; }
  .listing-title { font-weight: 500; }
  .listing-loc { font-size: 0.8rem; }
  .listing-desc {
    font-size: 0.85rem;
    line-height: 1.5;
    margin: 0.25rem 0 0;
    color: var(--color-text-muted);
  }
  .listing-link { font-size: 0.82rem; margin-top: 0.25rem; }
  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 1rem;
    padding-top: 0.85rem;
    border-top: 1px solid var(--color-border);
  }
  .names-note { font-size: 0.85rem; margin-bottom: 1rem; line-height: 1.5; }
  .empty { padding: 1.5rem 0; }

  .name-list { list-style: none; padding: 0; margin: 0; }
  .name-row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1.5rem;
    padding: 0.8rem 0;
    border-bottom: 1px solid var(--color-border);
    flex-wrap: wrap;
  }
  .name-row:last-child { border-bottom: none; }
  .name-info { display: flex; flex-direction: column; gap: 0.15rem; min-width: 0; flex: 1 1 18rem; }
  .name-label { font-weight: 500; font-size: 1rem; }
  .name-count { font-size: 0.85rem; }
  .samples { font-size: 0.8rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .name-map { display: flex; flex-direction: column; gap: 0.35rem; flex: 0 1 18rem; }
  .muted { color: var(--color-text-muted); }

  /* A program sits under the name it was recognized beneath, indented
     so the list still reads as names first (docs/adr/063). */
  .program-row {
    padding-left: 1rem;
    border-left: 2px solid var(--color-border);
    margin-left: 0.15rem;
  }
  .program-row .name-label { font-weight: 400; font-size: 0.95rem; }

  .title-group {
    padding: 0.85rem 0;
    border-bottom: 1px solid var(--color-border);
  }
  .title-group:last-child { border-bottom: none; }
  .group-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 0.75rem;
  }
  .group-count { font-size: 0.8rem; white-space: nowrap; }
  .group-credit { margin: 0.5rem 0 0.25rem; }
  .credited-note {
    font-size: 0.82rem;
    margin: 0.4rem 0 0.25rem;
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
  }
  .modal-note { font-size: 0.82rem; line-height: 1.5; margin: 0 0 0.75rem; }
  .date-list { list-style: none; padding: 0; margin: 0.35rem 0 0 0.85rem; }
  .date-list .listing { padding: 0.45rem 0; border-bottom: none; }
</style>
