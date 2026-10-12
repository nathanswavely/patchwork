<script>
  /**
   * A value edited where it stands: Liquid Textile's inline edit
   * (ltInlineEdit in lib/lt.js). Edit puts focus in the field and Save or
   * Cancel hands it back to Edit; Enter saves a line, Mod+Enter a textarea,
   * Escape cancels. The page owns the value: what shows after a save is
   * the page's, so an edit it turned away leaves the old value.
   */
  import { ltInlineEdit } from '../lib/lt.js';
  import { showToast } from '../stores/toast.svelte.js';

  let {
    label = '',
    value = '',
    type = 'text', // 'text' | 'textarea' | 'select'
    options = [],   // for select: [{ value, label }]
    onSave = async () => {},
    placeholder = '',
  } = $props();

  // Every word the edit shows is Patchwork's, written here where the copy
  // ledger reads it rather than left to Liquid Textile's defaults.
  const words = { save: 'Save', cancel: 'Cancel', saving: 'Saving...' };
  const empty = 'Not set';
  const onError = (e) => showToast(e.message || 'Failed to save', 'error');
</script>

<div
  class="lt-inline-edit"
  use:ltInlineEdit={{
    value,
    type,
    options,
    label,
    placeholder,
    onSave,
    onError,
    empty,
    words,
  }}
>
  <div class="lt-inline-edit-head">
    <span class="lt-eyebrow">{label}</span>
    <button type="button" class="lt-btn-link" data-edit>Edit</button>
  </div>
  <div data-value></div>
</div>
