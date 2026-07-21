<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import InlineEdit from '../components/InlineEdit.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import TagPicker from '../components/TagPicker.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

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

  <InlineEdit
    label="Website"
    value={node?.website || ''}
    type="text"
    onSave={(v) => saveField('website', v)}
    placeholder="https://..."
  />

  <InlineEdit
    label="Visibility"
    value={node?.visibility || 'public'}
    type="select"
    options={[
      { value: 'public', label: 'Public' },
      { value: 'private', label: 'Private' },
    ]}
    onSave={(v) => saveField('visibility', v)}
  />

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

  .btn-sm {
    padding: 0.25rem 0.6rem;
    font-size: 0.78rem;
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

  .tags-hint {
    font-size: 0.8rem;
    margin-bottom: 0.5rem;
  }

  .tags-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.5rem;
  }

</style>
