<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import InlineEdit from '../components/InlineEdit.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import TagPicker from '../components/TagPicker.svelte';
  import MapLocationPicker from '../components/MapLocationPicker.svelte';
  import { hasMapLocation, formatCoord } from '../lib/mapLocation.js';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

  // Map location (issue #4): a placed marker, independent of the address
  // prose above. Placement is a deliberate, explicit-save flow — the picker
  // opens, the admin drops/drags a marker, and only then does it save.
  let placingLocation = $state(false);
  let savingLocation = $state(false);
  let mapCenter = $state(null);
  let onMap = $derived(hasMapLocation(node?.latitude, node?.longitude));
  let locationReadout = $derived(
    onMap ? formatCoord(node.latitude, node.longitude) : ''
  );

  // Center the empty picker on the instance's configured area, the same
  // origin the discovery map uses.
  $effect(() => {
    if (mapCenter) return;
    api('instance')
      .then((inst) => {
        if (inst?.geography) {
          mapCenter = { lat: inst.geography.latitude, lng: inst.geography.longitude };
        }
      })
      .catch(() => {});
  });

  async function saveLocation(lat, lng) {
    savingLocation = true;
    try {
      await api(`nodes/${slug}`, {
        method: 'PATCH',
        body: { latitude: lat, longitude: lng },
      });
      showToast('Map location saved', 'success');
      placingLocation = false;
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to save map location', 'error');
    } finally {
      savingLocation = false;
    }
  }

  async function removeLocation() {
    savingLocation = true;
    try {
      await api(`nodes/${slug}`, {
        method: 'PATCH',
        body: { latitude: null, longitude: null },
      });
      showToast('Removed from map', 'success');
      placingLocation = false;
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to remove from map', 'error');
    } finally {
      savingLocation = false;
    }
  }

  // Links state (managed separately since InlineEdit doesn't handle arrays)
  let links = $state([]);
  let addingLink = $state(false);
  let newLinkUrl = $state('');
  let newLinkLabel = $state('');
  let savingLinks = $state(false);

  // Sync links from node data
  $effect(() => {
    if (node?.links) {
      links = Array.isArray(node.links) ? node.links.map(l => ({ ...l })) : [];
    } else {
      links = [];
    }
  });

  // Tags, in the stored priority order. Edits save on change; the last
  // write wins, so an instance admin can seed tags and the patch admin's
  // later edit simply replaces them (docs/adr/021).
  let tags = $state([]);
  let savingTags = $state(false);
  let tagsSynced = false;
  $effect(() => {
    if (node && !tagsSynced) {
      tags = Array.isArray(node.tags) ? [...node.tags] : [];
      tagsSynced = true;
    }
  });

  let tagsDirty = $derived(
    JSON.stringify(tags) !== JSON.stringify(Array.isArray(node?.tags) ? node.tags : [])
  );

  async function saveTags() {
    savingTags = true;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { tags } });
      showToast('Tags saved', 'success');
      tagsSynced = false;
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to save tags', 'error');
    } finally {
      savingTags = false;
    }
  }

  function resetTags() {
    tags = Array.isArray(node?.tags) ? [...node.tags] : [];
  }

  async function saveField(field, newValue) {
    await api(`nodes/${slug}`, { method: 'PATCH', body: { [field]: newValue } });
    showToast('Saved', 'success');
    patch.value.reload();
  }

  // Patch visibility. Each option carries the consequence, not just the word:
  // "Private" alone reads like a promise the patch page can't keep.
  const visibilityOptions = [
    {
      value: 'public',
      label: 'Public',
      description:
        'On the quilt. Anyone can find this patch through the quilt, search, the map, and public event feeds, and other quilts can follow it.',
    },
    {
      value: 'private',
      label: 'Private',
      description:
        'Off the quilt. Kept out of the quilt, search, the map, and public feeds, and it does not federate.',
    },
  ];
  let currentVisibility = $derived(node?.visibility || 'public');
  let savingVisibility = $state(false);

  async function saveVisibility(value) {
    if (value === currentVisibility) return;
    savingVisibility = true;
    try {
      await saveField('visibility', value);
    } catch (e) {
      showToast(e.message || 'Failed to save visibility', 'error');
    } finally {
      savingVisibility = false;
    }
  }

  // Event suggestions switch (docs/adr/026): whether non-members can
  // suggest events to this patch for admin review.
  let savingSuggestions = $state(false);
  async function saveSuggestions(enabled) {
    savingSuggestions = true;
    try {
      await saveField('accept_event_suggestions', enabled);
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    } finally {
      savingSuggestions = false;
    }
  }

  async function removeLink(index) {
    const updatedLinks = links.filter((_, i) => i !== index);
    savingLinks = true;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { links: updatedLinks } });
      links = updatedLinks;
      showToast('Link removed', 'success');
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to remove link', 'error');
    } finally {
      savingLinks = false;
    }
  }

  async function addLink() {
    if (!newLinkUrl.trim()) return;
    const updatedLinks = [...links, { url: newLinkUrl.trim(), label: newLinkLabel.trim() }];
    savingLinks = true;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { links: updatedLinks } });
      links = updatedLinks;
      newLinkUrl = '';
      newLinkLabel = '';
      addingLink = false;
      showToast('Link added', 'success');
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to add link', 'error');
    } finally {
      savingLinks = false;
    }
  }
</script>

<div class="settings-info">
  <InlineEdit
    label="Name"
    value={node?.name || ''}
    type="text"
    onSave={(v) => saveField('name', v)}
    placeholder="Patch name"
  />

  <InlineEdit
    label="Description"
    value={node?.description || ''}
    type="textarea"
    onSave={(v) => saveField('description', v)}
    placeholder="Describe this patch"
  />

  <InlineEdit
    label="Location"
    value={node?.address || ''}
    type="text"
    onSave={(v) => saveField('address', v)}
    placeholder="e.g. Lancaster, PA"
  />

  <!-- Map location section (issue #4). Separate from the address prose
       above: an address never implies a map position. -->
  <div class="links-section">
    <div class="links-header">
      <span class="links-label">Map location</span>
    </div>
    {#if onMap}
      <p class="location-state">
        On the map at <span class="location-coords">{locationReadout}</span>
      </p>
    {:else}
      <p class="muted">Not on the map.</p>
    {/if}
    <p class="muted tags-hint">
      Placing a marker adds this patch to the map. The marker can be set
      approximately — it is separate from the address above.
    </p>

    {#if placingLocation}
      <MapLocationPicker
        lat={node?.latitude ?? null}
        lng={node?.longitude ?? null}
        center={mapCenter}
        saving={savingLocation}
        onSave={saveLocation}
        onCancel={() => (placingLocation = false)}
      />
    {:else}
      <div class="location-actions">
        <button class="btn btn-secondary btn-sm" onclick={() => (placingLocation = true)}>
          {onMap ? 'Adjust map location' : 'Set map location'}
        </button>
        {#if onMap}
          <ConfirmAction
            label="Remove from map"
            confirmLabel="Remove"
            variant="danger"
            disabled={savingLocation}
            onConfirm={removeLocation}
          />
        {/if}
      </div>
    {/if}
  </div>

  <InlineEdit
    label="Website"
    value={node?.website || ''}
    type="text"
    onSave={(v) => saveField('website', v)}
    placeholder="https://..."
  />

  <!-- Visibility. The old control was a bare Public/Private select, which
       named a state without saying what it does — and said nothing about the
       one thing admins asked: whether it covers the patch's documents too
       (it doesn't; charters carry their own switch, docs/adr/035). -->
  <div class="links-section">
    <div class="links-header">
      <span class="links-label">Visibility</span>
    </div>
    <p class="muted tags-hint">
      This is about the patch itself — where it shows up. Events, members, and
      documents each carry their own visibility.
    </p>
    <div class="choice-list">
      {#each visibilityOptions as opt}
        <label class="choice" class:selected={currentVisibility === opt.value}>
          <input
            type="radio"
            name="patch-visibility"
            value={opt.value}
            checked={currentVisibility === opt.value}
            disabled={savingVisibility}
            onchange={() => saveVisibility(opt.value)}
          />
          <span class="choice-text">
            <span class="choice-label">{opt.label}</span>
            <span class="muted choice-desc">{opt.description}</span>
          </span>
        </label>
      {/each}
    </div>
    <p class="muted tags-hint caveat">
      Either way, anyone holding a direct link can open this patch's page. Private
      keeps it from being found, not from being visited.
    </p>
  </div>

  <!-- Tags section -->
  <div class="links-section">
    <div class="links-header">
      <span class="links-label">Tags</span>
    </div>
    <p class="muted tags-hint">
      Tags help people find this patch, and the quilt places patches with
      shared tags near each other. The first tag decides the default motif.
    </p>
    <TagPicker bind:selected={tags} disabled={savingTags} />
    {#if tagsDirty}
      <div class="tags-actions">
        <button class="btn btn-primary btn-sm" onclick={saveTags} disabled={savingTags}>
          {savingTags ? 'Saving...' : 'Save tags'}
        </button>
        <button class="btn btn-secondary btn-sm" onclick={resetTags} disabled={savingTags}>
          Cancel
        </button>
      </div>
    {/if}
  </div>

  <!-- Event suggestions section (docs/adr/026) -->
  <div class="links-section">
    <div class="links-header">
      <span class="links-label">Event suggestions</span>
    </div>
    <label class="toggle-row">
      <input
        type="checkbox"
        checked={node?.accept_event_suggestions === true}
        disabled={savingSuggestions}
        onchange={(e) => saveSuggestions(e.target.checked)}
      />
      <span>Let non-members suggest events to this patch</span>
    </label>
    <p class="muted tags-hint" style="margin-top: 0.35rem;">
      Suggestions wait in your events queue until an admin approves them.
    </p>
  </div>

  <!-- Links section -->
  <div class="links-section">
    <div class="links-header">
      <span class="links-label">Links</span>
    </div>

    {#if links.length > 0}
      <ul class="links-list">
        {#each links as link, i}
          <li class="links-item">
            <div class="links-item-info">
              <a href={link.url} target="_blank" rel="noopener noreferrer">{link.label || link.url}</a>
              {#if link.label}
                <span class="muted">{link.url}</span>
              {/if}
            </div>
            <ConfirmAction
              label="Remove"
              confirmLabel="Remove"
              variant="danger"
              disabled={savingLinks}
              onConfirm={() => removeLink(i)}
            />
          </li>
        {/each}
      </ul>
    {:else}
      <p class="muted">No links added.</p>
    {/if}

    {#if addingLink}
      <div class="add-link-form">
        <input
          type="url"
          class="link-input"
          placeholder="https://..."
          bind:value={newLinkUrl}
        />
        <input
          type="text"
          class="link-input"
          placeholder="Label (optional)"
          bind:value={newLinkLabel}
        />
        <div class="add-link-actions">
          <button class="btn btn-primary btn-sm" onclick={addLink} disabled={savingLinks || !newLinkUrl.trim()}>
            {savingLinks ? 'Saving...' : 'Save'}
          </button>
          <button class="btn btn-secondary btn-sm" onclick={() => { addingLink = false; newLinkUrl = ''; newLinkLabel = ''; }} disabled={savingLinks}>
            Cancel
          </button>
        </div>
      </div>
    {:else}
      <button class="btn btn-secondary btn-sm" onclick={() => { addingLink = true; }} style="margin-top: 0.5rem;">
        Add link
      </button>
    {/if}
  </div>

</div>

<style>
  .settings-info {
    max-width: 520px;
  }

  .links-section {
    padding: 0.6rem 0;
    border-top: 1px solid var(--color-border);
  }

  .links-header {
    margin-bottom: 0.4rem;
  }

  .links-label {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }

  .links-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .links-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.4rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .links-item:last-child {
    border-bottom: none;
  }

  .links-item-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }

  .links-item-info a {
    font-size: 0.9rem;
    word-break: break-all;
  }

  .links-item-info .muted {
    font-size: 0.78rem;
    word-break: break-all;
  }

  .add-link-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .link-input {
    width: 100%;
    padding: 0.45rem 0.6rem;
    font-size: 0.88rem;
    border: 1px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text);
    font-family: inherit;
  }

  .link-input:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  .add-link-actions {
    display: flex;
    gap: 0.4rem;
  }

  .toggle-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
    cursor: pointer;
  }

  .toggle-row input {
    accent-color: var(--color-primary);
  }

  .choice-list {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    margin-top: 0.5rem;
  }

  .choice {
    display: flex;
    gap: 0.55rem;
    align-items: flex-start;
    padding: 0.55rem 0.65rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .choice:hover {
    border-color: var(--color-primary);
  }

  .choice.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 6%, transparent);
  }

  .choice input {
    margin-top: 0.2rem;
    flex-shrink: 0;
  }

  .choice-text {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .choice-label {
    font-size: 0.9rem;
    font-weight: 500;
  }

  .choice-desc {
    font-size: 0.8rem;
    line-height: 1.45;
  }

  .caveat {
    margin-top: 0.5rem;
  }

  .tags-hint {
    font-size: 0.8rem;
    margin-bottom: 0.5rem;
  }

  .tags-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.5rem;
  }

  .location-state {
    font-size: 0.88rem;
    margin: 0 0 0.15rem;
  }

  .location-coords {
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
  }

  .location-actions {
    display: flex;
    gap: 0.4rem;
    align-items: center;
    margin-top: 0.5rem;
  }

</style>
