<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ToggleSwitch from '../components/ToggleSwitch.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  let categories = $state([]);
  let loading = $state(true);

  $effect(() => {
    if (slug) loadConfig();
  });

  async function loadConfig() {
    loading = true;
    try {
      const data = await api(`nodes/${slug}/notification-config`);
      categories = data.categories || [];
    } catch (e) {
      showToast('Failed to load notification config', 'error');
    } finally {
      loading = false;
    }
  }

  async function toggle(catId) {
    const cat = categories.find(c => c.id === catId);
    if (!cat) return;
    cat.enabled = !cat.enabled;
    categories = [...categories];

    try {
      const payload = {};
      for (const c of categories) {
        payload[c.id] = c.enabled;
      }
      await api(`nodes/${slug}/notification-config`, { method: 'PUT', body: { categories: payload } });
    } catch (e) {
      showToast('Failed to save', 'error');
      cat.enabled = !cat.enabled;
      categories = [...categories];
    }
  }
</script>

<div class="page-fade">
  <h2>Notification Settings</h2>
  <p class="muted subtitle">Choose which notifications this patch sends its members.</p>

  {#if loading}
    <p class="muted" style="padding: 2rem 0;">Loading...</p>
  {:else}
    <div class="category-list">
      {#each categories as cat}
        <div class="category-row">
          <div class="category-info">
            <span class="category-label">{cat.label}</span>
            <span class="category-desc muted">{cat.description}</span>
          </div>
          <ToggleSwitch checked={cat.enabled} label={cat.label} onchange={() => toggle(cat.id)} />
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  h2 {
    font-size: 1.2rem;
    margin-bottom: 0.25rem;
  }

  .subtitle {
    font-size: 0.85rem;
    margin-bottom: 1.5rem;
  }

  .category-list {
    display: flex;
    flex-direction: column;
    gap: 0;
  }

  .category-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--color-border);
    gap: 1rem;
  }

  .category-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .category-label {
    font-size: 0.92rem;
    font-weight: 500;
    color: var(--color-text);
  }

  .category-desc {
    font-size: 0.8rem;
  }

</style>
