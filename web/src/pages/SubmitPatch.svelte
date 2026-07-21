<script>
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  let name = $state('');
  let description = $state('');
  let website = $state('');
  let links = $state([{ url: '', label: '' }]);
  let address = $state('');
  let submitting = $state(false);
  let error = $state('');

  function addLink() {
    links = [...links, { url: '', label: '' }];
  }

  function removeLink(i) {
    links = links.filter((_, idx) => idx !== i);
  }

  async function handleSubmit() {
    if (!name.trim()) { error = 'Name is required'; return; }
    submitting = true;
    error = '';
    try {
      const cleanLinks = links.filter(l => l.url.trim()).map(l => ({
        url: l.url.trim(),
        label: l.label.trim() || undefined,
      }));
      const body = {
        name: name.trim(),
        description: description.trim(),
        website: website.trim() || undefined,
        links: cleanLinks.length > 0 ? cleanLinks : undefined,
        address: address.trim() || undefined,
      };
      const result = await api('submissions', { method: 'POST', body });
      if (result.node) {
        showToast('Patch added to the quilt!', 'success');
        navigate(`/patches/${result.node.slug}`);
      } else {
        showToast('Submitted for review. Thanks!', 'success');
        navigate('/');
      }
    } catch (e) {
      if (e.status === 409) {
        error = 'A patch with a similar name already exists.';
      } else {
        error = e.message || 'Something went wrong.';
      }
    } finally {
      submitting = false;
    }
  }
</script>

<div class="submit-page page-fade">
  <h1>Add a patch to the quilt</h1>
  <p class="muted">Know a place, group, or organization that should be on the map? Add it here. The real owner can claim it later.</p>

  <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
    <div class="field">
      <label for="name">Name <span class="required">*</span></label>
      <input id="name" type="text" bind:value={name} placeholder="e.g. Gallery Row" disabled={submitting} />
    </div>

    <div class="field">
      <label for="description">Description</label>
      <textarea id="description" bind:value={description} rows="3" placeholder="What is this place/group about?" disabled={submitting}></textarea>
    </div>

    <div class="field">
      <label for="website">Website</label>
      <input id="website" type="url" bind:value={website} placeholder="https://..." disabled={submitting} />
    </div>

    <div class="field">
      <label>Links</label>
      {#each links as link, i}
        <div class="link-row">
          <input type="url" bind:value={link.url} placeholder="https://..." disabled={submitting} />
          <input type="text" bind:value={link.label} placeholder="Label (optional)" disabled={submitting} />
          {#if links.length > 1}
            <button type="button" class="remove-btn" onclick={() => removeLink(i)} disabled={submitting}>&times;</button>
          {/if}
        </div>
      {/each}
      <button type="button" class="btn btn-secondary btn-sm" onclick={addLink} disabled={submitting}>+ Add link</button>
    </div>

    <div class="field">
      <label for="address">Address <span class="muted">(optional)</span></label>
      <input id="address" type="text" bind:value={address} placeholder="123 Main St, Lancaster, PA" disabled={submitting} />
    </div>

    {#if error}
      <p class="error-text">{error}</p>
    {/if}

    <div class="form-actions">
      <button type="submit" class="btn btn-primary" disabled={submitting}>
        {submitting ? 'Submitting...' : 'Submit'}
      </button>
      <button type="button" class="btn btn-secondary" onclick={() => navigate('/')} disabled={submitting}>Cancel</button>
    </div>
  </form>
</div>

<style>
  .submit-page {
    max-width: 600px;
  }

  h1 {
    font-size: 1.4rem;
    margin-bottom: 0.25rem;
  }

  h1 + .muted {
    margin-bottom: 1.5rem;
    font-size: 0.88rem;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
  }

  .required {
    color: var(--color-error);
  }

  .link-row {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    margin-bottom: 0.35rem;
  }

  .link-row input:first-child {
    flex: 2;
  }

  .link-row input:nth-child(2) {
    flex: 1;
  }

  .remove-btn {
    border: none;
    background: none;
    color: var(--color-text-muted);
    font-size: 1.2rem;
    cursor: pointer;
    padding: 0 0.25rem;
  }

  .remove-btn:hover {
    color: var(--color-error);
  }

  .form-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }
</style>
