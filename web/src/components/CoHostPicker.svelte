<script>
  // Co-hosts, chosen on the event form
  // (docs/adr/2026-10-19-co-hosts-read-as-equals-and-the-owner-can-share-the-pen.md).
  // Choosing writes nothing. The form sends one link request per row when it
  // is saved, through the same two-consent handshake the event page uses
  // (docs/adr/032), so until then these rows are a plan. A row that came
  // from an existing link carries it as `link`, which is how the form tells
  // a new co-host from a changed one.
  //   { id, slug, name, status, can_edit, link }
  import { X } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { patchPickerProvider } from '../lib/finderProviders.js';
  import { parsePatchLink } from '../lib/patchLink.js';
  import { showToast } from '../stores/toast.svelte.js';
  import WorkspaceSearch from './WorkspaceSearch.svelte';

  let { rows = $bindable([]), hostSlug = '', disabled = false } = $props();

  let chosen = $derived(new Set(rows.map((r) => r.slug)));

  // The hosting patch is the subject of the sentence, not a candidate, so it
  // is left out rather than shown refused. A patch already chosen is shown
  // refused with the reason, as the event page's picker does.
  function provider() {
    return patchPickerProvider((n) => {
      if (n.slug === hostSlug) return null;
      return {
        type: n.status === 'unclaimed' ? 'Unclaimed patches' : 'Patches',
        sublabel: chosen.has(n.slug) ? 'already a co-host' : '',
        disabled: chosen.has(n.slug),
        id: n.id,
        status: n.status,
      };
    });
  }

  function add(item) {
    if (!item?.slug || item.slug === hostSlug || chosen.has(item.slug)) return;
    rows = [
      ...rows,
      { id: item.id || '', slug: item.slug, name: item.label || item.slug, status: item.status || '', can_edit: false, link: null },
    ];
  }

  // A pasted patch URL means "this one", the same reading the event page
  // gives it. It reaches patches the listing won't enumerate, since holding
  // the URL is how a private patch was legitimately found (docs/adr/033). A
  // patch on another quilt is a mention, which only the event page adds.
  function recognizePatchLink(value) {
    const parsed = parsePatchLink(value);
    if (!parsed) return false;
    if (parsed.host !== location.host) {
      showToast('A patch on another quilt is added from the event page, as a mention', 'error');
      return true;
    }
    const slug = parsed.slug;
    if (slug === hostSlug) {
      showToast('That is the hosting patch', 'error');
      return true;
    }
    api(`nodes/${slug}`)
      .then((data) => {
        const n = data.node || data;
        add({ id: n.id, slug: n.slug || slug, label: n.name || slug, status: data.is_unclaimed ? 'unclaimed' : n.status });
      })
      .catch(() => showToast('No patch at that address', 'error'));
    return true;
  }

  function remove(slug) {
    rows = rows.filter((r) => r.slug !== slug);
  }

  function setCanEdit(slug, value) {
    rows = rows.map((r) => (r.slug === slug ? { ...r, can_edit: value } : r));
  }

  function stateOf(r) {
    if (r.link?.status !== 'pending') return '';
    return r.link.initiated_by === 'owner'
      ? 'Waiting for them to confirm'
      : 'Asked to co-host. Confirm on the event page.';
  }
</script>

<div class="co-hosts">
  {#if rows.length}
    <ul class="co-host-rows">
      {#each rows as r (r.slug)}
        <li class="co-host-row">
          <span class="co-host-name">{r.name}</span>
          {#if stateOf(r)}
            <span class="lt-muted co-host-state">{stateOf(r)}</span>
          {/if}
          <label class="lt-check co-host-edit">
            <input
              type="checkbox"
              checked={r.can_edit}
              {disabled}
              onchange={(e) => setCanEdit(r.slug, e.currentTarget.checked)}
            />
            Can edit
          </label>
          <button
            type="button"
            class="chip-x"
            title="Remove {r.name}"
            {disabled}
            onclick={() => remove(r.slug)}
          >
            <X size={12} />
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="picker-slot">
    <WorkspaceSearch
      variant="picker"
      matchField
      browse
      inputId="co-hosts"
      placeholder="Add a patch…"
      {provider}
      onSelect={add}
      intercept={recognizePatchLink}
    />
  </div>
</div>

<style>
  .co-hosts {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .co-host-rows {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .co-host-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem 0.75rem;
    padding: 0.35rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    font-size: 0.9rem;
  }

  .co-host-name {
    font-weight: 600;
  }

  .co-host-state {
    font-size: 0.8rem;
  }

  /* The name and its state read left to right; the controls sit at the end
     of the row, where the eye finds them in every row at once. */
  .co-host-edit {
    margin-left: auto;
    font-size: 0.85rem;
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
    color: var(--color-error);
  }

  .picker-slot {
    position: relative;
    max-width: 26rem;
  }

  @media (pointer: coarse) {
    .chip-x {
      min-width: 36px;
      min-height: 36px;
      justify-content: center;
    }
  }
</style>
