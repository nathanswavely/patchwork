<script>
  /**
   * The one-time prompt for the patch-level AI declaration
   * (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md, consequences).
   * Existing patches carry no answer, and nobody is labelled by default, so
   * a patch's own admins are asked here until one of them answers. State,
   * not a dismissal: answering makes `ai_work` non-null and the prompt goes,
   * and nothing else does. Mounted beside the setup checklist, and like it
   * shown to patch-role admins only, never on an unclaimed patch, which
   * carries no declaration.
   */
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import AIWorkQuestion from './AIWorkQuestion.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);
  let isAdmin = $derived(patch.value.membershipRole === 'admin');
  let isUnclaimed = $derived(patch.value.isUnclaimed);

  let saving = $state(false);
  let visible = $derived(
    isAdmin && !isUnclaimed && !!node && node.ai_work !== true && node.ai_work !== false
  );

  async function answer(value) {
    saving = true;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { ai_work: value } });
      showToast('Saved', 'success');
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    } finally {
      saving = false;
    }
  }
</script>

{#if visible}
  <div class="ai-work-prompt">
    <AIWorkQuestion value={null} disabled={saving} onchange={answer} />
    <p class="lt-hint">
      You can change this later in
      <a
        href="/patches/{slug}/settings/info"
        onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/settings/info`); }}
      >Patch Settings</a>.
    </p>
  </div>
{/if}

<style>
  .ai-work-prompt {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    padding: 0.9rem 1rem;
    margin: 0 0 1.25rem;
  }
</style>
