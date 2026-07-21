<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';

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
          <label class="toggle-label">
            <input type="checkbox" checked={cat.enabled} onchange={() => toggle(cat.id)} />
            <span class="toggle-track">
              <span class="toggle-thumb"></span>
            </span>
          </label>
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

  .toggle-label {
    cursor: pointer;
    flex-shrink: 0;
  }

  .toggle-label input { display: none; }

  .toggle-track {
    display: block;
    width: 38px;
    height: 22px;
    border-radius: 11px;
    background: var(--color-text-muted);
    position: relative;
    transition: background 150ms ease;
  }

  .toggle-label input:checked + .toggle-track {
    background: var(--color-primary);
  }

  .toggle-thumb {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--color-surface);
    transition: transform 150ms ease;
  }

  .toggle-label input:checked + .toggle-track .toggle-thumb {
    transform: translateX(16px);
  }
</style>
