<script>
  // Event links (docs/adr/032): confirmed links render as "with X" chips
  // for everyone; the handshake controls only appear for admins who can
  // act. Cross-quilt mentions are doorways — plain external links.
  import { LinkSimple, ArrowSquareOut, X, Check } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isAdmin, isTrustedContributor } from '../stores/auth.svelte.js';
  import { getMembershipRoles, getMemberships } from '../stores/memberships.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { patchPickerProvider } from '../lib/finderProviders.js';
  import WorkspaceSearch from './WorkspaceSearch.svelte';

  let { event, onChanged } = $props();

  let links = $derived(event?.links || []);
  let mentions = $derived(event?.mentions || []);
  let confirmed = $derived(links.filter((l) => l.status === 'confirmed'));
  let pending = $derived(links.filter((l) => l.status === 'pending'));

  // Mirrors the server's userSpeaksForNode. Standing attaches to the
  // patch, not the event: instance admins everywhere, patch admins on
  // their own, and a trusted contributor on any patch while it is
  // unclaimed (docs/adr/057). Keeping the two in step is what stops the
  // UI offering a control the API will 403.
  function speaksFor(slug, status) {
    if (isAdmin()) return true;
    if (getMembershipRoles().get(slug) === 'admin') return true;
    return isTrustedContributor() && status === 'unclaimed';
  }

  let ownerAdmin = $derived(speaksFor(event?.node_slug, event?.node_status));
  // Patches this person admins, minus the owner — the ones they could
  // request a link for from this side of the handshake. Carries the name,
  // because a slug is not what anyone calls their patch.
  let adminPatches = $derived.by(() => {
    const out = [];
    for (const m of getMemberships()) {
      if (m.role === 'admin' && m.status === 'active' && m.node_slug !== event?.node_slug) {
        out.push({ slug: m.node_slug, label: m.node_name || m.node_slug });
      }
    }
    return out;
  });
  // A trusted contributor may propose any unclaimed patch onto this
  // event, so their reach isn't enumerable from memberships alone.
  let reachesBeyondOwn = $derived(ownerAdmin || isTrustedContributor());
  let canAct = $derived(reachesBeyondOwn || adminPatches.length > 0);

  let adding = $state(false);
  let busy = $state(false);
  // The chosen target, held before it is submitted. The picker cannot
  // submit on select the way the aggregator's does: absorption is a human
  // choice made *between* choosing a patch and requesting the link
  // (docs/adr/032), so there has to be a moment where a target is chosen
  // and nothing has been written yet. The pasted-link path lands here too.
  //   { kind: 'patch',   slug, name }
  //   { kind: 'mention', host, slug, target }
  let staged = $state(null);
  // Duplicate absorption is a human choice in the flow (docs/adr/032):
  // when the acting admin speaks for the linked side, their patch's
  // same-week events are offered as optional replacements.
  let duplicates = $state([]);
  let absorbId = $state('');
  let confirmingNode = $state('');

  // Patches already spoken for on this event, by slug: the owner (which
  // the server refuses outright) and anything linked or pending.
  let takenSlugs = $derived.by(() => {
    const m = new Map();
    for (const l of links) m.set(l.node_slug, l.status);
    return m;
  });

  // The picker's corpus. Owner-side, every public patch is a legitimate
  // proposal. Otherwise only the patches this person may speak for, since
  // the server refuses the rest — a corpus that offers what will 403 is
  // worse than a short one. The owning patch is omitted rather than shown
  // refused: it isn't a rejected candidate, it's the subject of the
  // sentence. Already-linked patches ARE shown refused, with the reason —
  // that answers the question someone is asking when they search for a
  // patch whose chip they can't see (a pending link is invisible to
  // everyone but the two sides).
  function linkPatchProvider() {
    return patchPickerProvider((n) => {
      if (n.slug === event?.node_slug) return null;
      if (!ownerAdmin && !speaksFor(n.slug, n.status)) return null;
      const taken = takenSlugs.get(n.slug);
      return {
        type: n.status === 'unclaimed' ? 'Unclaimed patches' : 'Patches',
        sublabel: taken === 'confirmed'
          ? 'already linked'
          : taken === 'pending'
            ? 'link already requested'
            : n.description
              ? n.description.slice(0, 60)
              : '',
        disabled: !!taken,
      };
    });
  }

  // A pasted patch URL means "this one", not a search for its characters
  // — the same reading SocialShell gives it for the follow path
  // (docs/adr/024). Unlike that one it never acts: it stages, because
  // creating a public "with X" chip is not something a paste should do
  // behind your back. A local URL stages as an ordinary link proposal,
  // never a consent-free mention (docs/adr/032), and it reaches patches
  // the listing won't enumerate — holding the URL is how a private patch
  // was legitimately found (docs/adr/033).
  function recognizePatchLink(value) {
    const m = value.trim().match(/^https?:\/\/([^/]+)\/patches\/([a-z0-9-]+)\/?$/);
    if (!m) return false;
    const [target, host, slug] = m;
    if (host === location.host) {
      if (slug === event?.node_slug) {
        showToast('That is this event\u2019s own patch', 'error');
        return true;
      }
      stagePatch({ slug, label: slug });
    } else if (ownerAdmin) {
      staged = { kind: 'mention', host, slug, target };
    } else {
      showToast('Only this patch\u2019s admins can add a mention', 'error');
    }
    return true;
  }

  function stagePatch(item) {
    staged = { kind: 'patch', slug: item.slug, name: item.label };
    loadDuplicates(item.slug);
    // A pasted link arrives as a slug with no name — GetNode serves a
    // private patch by slug, so this is also how one gets a readable
    // chip. Best-effort: the slug already staged is a working label.
    if (item.label === item.slug) {
      api(`nodes/${item.slug}`)
        .then((n) => {
          if (n?.name && staged?.slug === item.slug) staged = { ...staged, name: n.name };
        })
        .catch(() => {});
    }
  }

  function suggestPatch(q) {
    navigate(`/submit?name=${encodeURIComponent(q)}`);
  }

  function resetAdd() {
    adding = false;
    staged = null;
    duplicates = [];
    absorbId = '';
  }

  async function loadDuplicates(slug) {
    duplicates = [];
    absorbId = '';
    if (!slug || !event?.starts_at || getMembershipRoles().get(slug) !== 'admin') return;
    try {
      const day = event.starts_at.slice(0, 10);
      const from = `${day}T00:00:00Z`;
      const to = new Date(new Date(`${day}T00:00:00Z`).getTime() + 2 * 86400000)
        .toISOString().slice(0, 19) + 'Z';
      const res = await api(`events?node_slug=${encodeURIComponent(slug)}&from=${from}&to=${to}`);
      duplicates = (res.items || []).filter((e) => e.id !== event.id && e.node_slug === slug);
    } catch {}
  }

  async function submitLink() {
    if (!staged || busy) return;
    busy = true;
    try {
      const body = { target: staged.kind === 'mention' ? staged.target : staged.slug };
      if (absorbId) body.absorb_event_id = absorbId;
      const res = await api(`events/${event.id}/links`, { method: 'POST', body });
      showToast(res?.host ? 'Mention added' : res?.status === 'confirmed' ? 'Linked' : 'Link requested');
      resetAdd();
      onChanged?.();
    } catch (e) {
      showToast(e.data?.error || e.message || 'Failed to request link', 'error');
    } finally {
      busy = false;
    }
  }

  function canConfirm(l) {
    // The side that didn't initiate confirms.
    if (l.initiated_by === 'owner') {
      return isAdmin() || getMembershipRoles().get(l.node_slug) === 'admin';
    }
    return ownerAdmin;
  }

  function canRemove(l) {
    return ownerAdmin || isAdmin() || getMembershipRoles().get(l.node_slug) === 'admin';
  }

  async function startConfirm(l) {
    confirmingNode = l.node_id;
    // Absorption is the linked side's call — offered only when this
    // confirmer speaks for the linked patch.
    if (l.initiated_by === 'owner') await loadDuplicates(l.node_slug);
  }

  async function confirmLink(l) {
    busy = true;
    try {
      const body = absorbId ? { absorb_event_id: absorbId } : {};
      await api(`events/${event.id}/links/${l.node_id}/confirm`, { method: 'POST', body });
      showToast(`Linked with ${l.node_name}`);
      confirmingNode = '';
      duplicates = [];
      absorbId = '';
      onChanged?.();
    } catch (e) {
      showToast(e.message || 'Failed to confirm link', 'error');
    } finally {
      busy = false;
    }
  }

  async function removeLink(l) {
    try {
      await api(`events/${event.id}/links/${l.node_id}`, { method: 'DELETE' });
      showToast(l.status === 'pending' ? 'Request removed' : `Removed ${l.node_name}`);
      onChanged?.();
    } catch (e) {
      showToast(e.message || 'Failed to remove link', 'error');
    }
  }

  async function removeMention(m) {
    try {
      await api(`events/${event.id}/mentions/${m.id}`, { method: 'DELETE' });
      onChanged?.();
    } catch (e) {
      showToast(e.message || 'Failed to remove mention', 'error');
    }
  }
</script>

{#if confirmed.length || mentions.length || canAct}
  <div class="event-links">
    {#if confirmed.length || mentions.length}
      <div class="chips">
        <span class="with-label">with</span>
        {#each confirmed as l (l.id)}
          <span class="chip">
            <a
              href="/patches/{l.node_slug}"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${l.node_slug}`); }}
            >{l.node_name}</a>
            {#if canRemove(l)}
              <button class="chip-x" title="Remove link" onclick={() => removeLink(l)}>
                <X size={11} />
              </button>
            {/if}
          </span>
        {/each}
        {#each mentions as m (m.id)}
          <span class="chip mention">
            <a href="https://{m.host}/patches/{m.slug}" target="_blank" rel="noopener noreferrer">
              {m.name || m.slug}
              <ArrowSquareOut size={11} />
            </a>
            <span class="mention-host">{m.host}</span>
            {#if ownerAdmin}
              <button class="chip-x" title="Remove mention" onclick={() => removeMention(m)}>
                <X size={11} />
              </button>
            {/if}
          </span>
        {/each}
      </div>
    {/if}

    {#each pending as l (l.id)}
      <div class="pending-row">
        <span class="muted">
          {#if l.initiated_by === 'owner'}
            Waiting for <strong>{l.node_name}</strong> to confirm this link
          {:else}
            <strong>{l.node_name}</strong> asked to link to this event
          {/if}
        </span>
        {#if canConfirm(l)}
          {#if confirmingNode === l.node_id}
            {#if duplicates.length}
              <select bind:value={absorbId} class="absorb-select">
                <option value="">Keep all my events</option>
                {#each duplicates as d (d.id)}
                  <option value={d.id}>Replace "{d.title}"</option>
                {/each}
              </select>
            {/if}
            <button class="btn btn-sm" disabled={busy} onclick={() => confirmLink(l)}>
              <Check size={13} /> Confirm
            </button>
          {:else}
            <button class="btn btn-sm" onclick={() => startConfirm(l)}>Review</button>
          {/if}
        {/if}
        {#if canRemove(l)}
          <button class="btn btn-sm btn-secondary" onclick={() => removeLink(l)}>Remove</button>
        {/if}
      </div>
    {/each}

    {#if canAct}
      {#if adding}
        <div class="add-row">
          {#if staged}
            <span class="staged">
              {#if staged.kind === 'mention'}
                <ArrowSquareOut size={12} />
                {staged.slug}
                <span class="mention-host">{staged.host}</span>
              {:else}
                {staged.name}
              {/if}
              <button class="chip-x" title="Choose a different patch" onclick={() => { staged = null; duplicates = []; absorbId = ''; }}>
                <X size={11} />
              </button>
            </span>
            {#if duplicates.length}
              <select bind:value={absorbId} class="absorb-select">
                <option value="">Keep all my events</option>
                {#each duplicates as d (d.id)}
                  <option value={d.id}>Replace "{d.title}"</option>
                {/each}
              </select>
            {/if}
            <button class="btn btn-sm" disabled={busy} onclick={submitLink}>
              {staged.kind === 'mention' ? 'Add mention' : 'Request'}
            </button>
          {:else if reachesBeyondOwn}
            <div class="picker-slot">
              <WorkspaceSearch
                variant="picker"
                placeholder="Find a patch…"
                provider={linkPatchProvider}
                onSelect={stagePatch}
                intercept={recognizePatchLink}
                suggestLabel={(q) => `Suggest “${q}” as a patch`}
                onSuggest={getSubmissionsEnabled() ? suggestPatch : null}
              />
            </div>
          {:else}
            <!-- Not owner-side: the only patches this person may put on
                 someone else's event are the ones they admin. That is a
                 handful of known names, so it is buttons — a search field
                 over two items, which reveals nothing until you type,
                 would hide the whole choice behind a guess. -->
            {#each adminPatches as patch (patch.slug)}
              <button
                class="btn btn-sm btn-secondary"
                disabled={busy || takenSlugs.has(patch.slug)}
                onclick={() => stagePatch(patch)}
              >
                {patch.label}{#if takenSlugs.get(patch.slug) === 'pending'}{' — requested'}{:else if takenSlugs.get(patch.slug) === 'confirmed'}{' — linked'}{/if}
              </button>
            {/each}
          {/if}
          <button class="btn btn-sm btn-secondary" onclick={resetAdd}>Cancel</button>
        </div>
      {:else}
        <button class="link-btn" onclick={() => (adding = true)}>
          <LinkSimple size={13} weight="duotone" />
          Link a patch
        </button>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .event-links {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-bottom: 1.25rem;
  }

  .chips {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-wrap: wrap;
  }

  .with-label {
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.15rem 0.55rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    font-size: 0.85rem;
  }

  .chip a {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    color: var(--color-primary);
    font-weight: 600;
    text-decoration: none;
  }

  .chip a:hover {
    text-decoration: underline;
  }

  .mention-host {
    font-size: 0.72rem;
    color: var(--color-text-muted);
  }

  .chip-x {
    display: inline-flex;
    align-items: center;
    border: none;
    background: none;
    padding: 0;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .chip-x:hover {
    color: var(--color-danger, #b3261e);
  }

  .pending-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    padding: 0.5rem 0.75rem;
    border: 1px dashed var(--color-border);
    border-radius: var(--radius);
    font-size: 0.85rem;
  }

  .pending-row .muted {
    color: var(--color-text-muted);
  }

  .add-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-wrap: wrap;
  }

  /* Sized to the dropdown, not to the row. The picker variant anchors its
     panel to the field's right edge and gives it a 26rem floor, so a field
     allowed to fill the row leaves the panel visibly detached from the box
     that opened it. Matching the two makes them read as one control. */
  .picker-slot {
    flex: 0 1 min(26rem, 100%);
    min-width: 220px;
    position: relative;
  }

  /* A staged target reads as the chip it is about to become, so the
     confirm step shows the shape of its own outcome. */
  .staged {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.15rem 0.55rem;
    border: 1px dashed var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .absorb-select {
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 0.8rem;
    max-width: 240px;
  }

  .link-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    align-self: flex-start;
    border: none;
    background: none;
    padding: 0;
    font-size: 0.8rem;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .link-btn:hover {
    color: var(--color-primary);
  }
</style>
