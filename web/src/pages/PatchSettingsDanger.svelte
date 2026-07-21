<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  async function archivePatch() {
    try {
      await api(`nodes/${slug}`, { method: 'DELETE' });
      showToast('Patch archived', 'info');
      navigate('/');
    } catch (e) {
      showToast(e.message || 'Failed to archive patch', 'error');
    }
  }
</script>

<div class="danger-zone">
  <h3 class="danger-heading">Danger Zone</h3>
  <div class="danger-card">
    <p class="danger-warning">
      Archiving this patch will hide it from the quilt and make it inaccessible to members. This action cannot be undone.
    </p>
    <ConfirmAction
      label="Archive Patch"
      confirmLabel="Yes, archive this patch"
      variant="danger"
      onConfirm={archivePatch}
    />
  </div>
</div>

<style>
  .danger-zone {
    max-width: 520px;
  }

  .danger-heading {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--color-error);
    margin-bottom: 0.75rem;
  }

  .danger-card {
    border: 1px solid var(--color-error);
    border-radius: 6px;
    padding: 1.25rem;
  }

  .danger-warning {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    line-height: 1.5;
    margin-bottom: 1rem;
  }
</style>
