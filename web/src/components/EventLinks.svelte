<script>
  // Event links (docs/adr/032): confirmed links render as "with X" chips
  // for everyone; the handshake controls only appear for admins who can
  // act. Cross-quilt mentions are doorways — plain external links.
  import { LinkSimple, ArrowSquareOut, X, Check } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isAdmin } from '../stores/auth.svelte.js';
  import { getMembershipRoles } from '../stores/memberships.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  let { event, onChanged } = $props();

  let links = $derived(event?.links || []);
  let mentions = $derived(event?.mentions || []);
  let confirmed = $derived(links.filter((l) => l.status === 'confirmed'));
  let pending = $derived(links.filter((l) => l.status === 'pending'));

  let ownerAdmin = $derived(
    isAdmin() || getMembershipRoles().get(event?.node_slug) === 'admin'
  );
  // Patches this person admins, minus the owner — the ones they could
  // request a link for from this side of the handshake.
  let adminSlugs = $derived.by(() => {
    const out = [];
    for (const [slug, role] of getMembershipRoles()) {
      if (role === 'admin' && slug !== event?.node_slug) out.push(slug);
    }
    return out;
  });
  let canAct = $derived(ownerAdmin || adminSlugs.length > 0);

  let adding = $state(false);
  let target = $state('');
  let busy = $state(false);
  // Duplicate absorption is a human choice in the flow (docs/adr/032):
  // when the acting admin speaks for the linked side, their patch's
  // same-week events are offered as optional replacements.
  let duplicates = $state([]);
  let absorbId = $state('');
  let confirmingNode = $state('');

  function targetSlug(value) {
    const v = value.trim();
    if (!v.startsWith('http://') && !v.startsWith('https://')) return v;
    try {
      const u = new URL(v);
      const parts = u.pathname.split('/').filter(Boolean);
      if (parts.length === 2 && parts[0] === 'patches') {
        return u.hostname === location.hostname ? parts[1] : '';
      }
    } catch {}
    return '';
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
    if (!target.trim() || busy) return;
    busy = true;
    try {
      const body = { target: target.trim() };
      if (absorbId) body.absorb_event_id = absorbId;
      const res = await api(`events/${event.id}/links`, { method: 'POST', body });
      showToast(res?.host ? 'Mention added' : res?.status === 'confirmed' ? 'Linked' : 'Link requested');
      adding = false;
      target = '';
      duplicates = [];
      absorbId = '';
      onChanged?.();
    } catch (e) {
      showToast(e.message || 'Failed to request link', 'error');
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
            Link proposed to <strong>{l.node_name}</strong> — awaiting their confirmation
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
          <input
            type="text"
            placeholder="Patch name or pasted patch link"
            bind:value={target}
            onchange={() => loadDuplicates(targetSlug(target))}
            onkeydown={(e) => e.key === 'Enter' && submitLink()}
          />
          {#if duplicates.length}
            <select bind:value={absorbId} class="absorb-select">
              <option value="">Keep all my events</option>
              {#each duplicates as d (d.id)}
                <option value={d.id}>Replace "{d.title}"</option>
              {/each}
            </select>
          {/if}
          <button class="btn btn-sm" disabled={busy || !target.trim()} onclick={submitLink}>Request</button>
          <button class="btn btn-sm btn-secondary" onclick={() => { adding = false; target = ''; duplicates = []; absorbId = ''; }}>Cancel</button>
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

  .add-row input {
    flex: 1;
    min-width: 220px;
    padding: 0.35rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 0.85rem;
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
