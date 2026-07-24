<script>
  import { showToast } from '../stores/toast.svelte.js';

  let {
    label = '',
    value = '',
    type = 'text', // 'text' | 'textarea' | 'select'
    options = [],   // for select: [{ value, label }]
    onSave = async () => {},
    disabled = false,
    placeholder = '',
  } = $props();

  let editing = $state(false);
  let editValue = $state('');
  let saving = $state(false);

  function startEdit() {
    editValue = value || '';
    editing = true;
  }

  function cancelEdit() {
    editing = false;
  }

  async function handleSave() {
    if (editValue === value) {
      editing = false;
      return;
    }
    saving = true;
    try {
      await onSave(editValue);
      editing = false;
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    } finally {
      saving = false;
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') cancelEdit();
    if (e.key === 'Enter' && type === 'text') handleSave();
  }
</script>

<div class="inline-edit">
  <div class="inline-edit-header">
    <span class="inline-edit-label">{label}</span>
    {#if !editing && !disabled}
      <button class="edit-btn" onclick={startEdit}>Edit</button>
    {/if}
  </div>

  {#if editing}
    <div class="inline-edit-form">
      {#if type === 'textarea'}
        <textarea
          class="inline-input"
          bind:value={editValue}
          onkeydown={handleKeydown}
          rows="3"
          {placeholder}
        ></textarea>
      {:else if type === 'select'}
        <select class="inline-input" bind:value={editValue}>
          {#each options as opt}
            <option value={opt.value}>{opt.label}</option>
          {/each}
        </select>
      {:else}
        <input
          class="inline-input"
          type="text"
          bind:value={editValue}
          onkeydown={handleKeydown}
          {placeholder}
        />
      {/if}
      <div class="inline-edit-actions">
        <button class="btn btn-primary btn-sm" onclick={handleSave} disabled={saving}>
          {saving ? 'Saving...' : 'Save'}
        </button>
        <button class="btn btn-secondary btn-sm" onclick={cancelEdit} disabled={saving}>
          Cancel
        </button>
      </div>
    </div>
  {:else}
    <div class="inline-edit-value">
      {#if value}
        {#if type === 'select'}
          {options.find(o => o.value === value)?.label || value}
        {:else}
          {value}
        {/if}
      {:else}
        <span class="muted">Not set</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .inline-edit {
    padding: 0.6rem 0;
  }

  .inline-edit + .inline-edit {
    border-top: 1px solid var(--color-border);
  }

  .inline-edit-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.3rem;
  }

  .inline-edit-label {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }

  .edit-btn {
    border: none;
    background: none;
    font-size: 0.78rem;
    color: var(--color-primary);
    cursor: pointer;
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
    transition: background 100ms ease;
  }

  .edit-btn:hover {
    background: var(--color-overlay);
  }

  .inline-edit-value {
    font-size: 0.9rem;
    line-height: 1.5;
    color: var(--color-text);
  }

  .inline-edit-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .inline-input {
    width: 100%;
    padding: 0.45rem 0.6rem;
    font-size: 0.88rem;
    border: 1px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text);
    font-family: inherit;
  }

  .inline-input:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  textarea.inline-input {
    resize: vertical;
    min-height: 60px;
  }

  .inline-edit-actions {
    display: flex;
    gap: 0.4rem;
  }

</style>
