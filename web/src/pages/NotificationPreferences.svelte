<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import UserSettingsShell from '../components/UserSettingsShell.svelte';
  import ToggleSwitch from '../components/ToggleSwitch.svelte';

  let categories = $state([]);
  let channels = $state([]);
  let loading = $state(true);
  let saveTimeout = $state(null);

  $effect(() => {
    loadPreferences();
  });

  async function loadPreferences() {
    loading = true;
    try {
      const data = await api('notifications/preferences');
      channels = data.channels || ['in_app'];
      categories = data.categories || [];
    } catch (e) {
      showToast('Failed to load preferences', 'error');
    } finally {
      loading = false;
    }
  }

  function toggle(typeStr, channel) {
    // Update local state.
    for (const cat of categories) {
      for (const t of cat.types) {
        if (t.type === typeStr) {
          t.channels[channel] = !t.channels[channel];
        }
      }
    }
    categories = [...categories]; // Trigger reactivity.
    debouncedSave();
  }

  function debouncedSave() {
    if (saveTimeout) clearTimeout(saveTimeout);
    saveTimeout = setTimeout(savePreferences, 500);
  }

  async function savePreferences() {
    const prefs = [];
    for (const cat of categories) {
      for (const t of cat.types) {
        for (const ch of channels) {
          prefs.push({ type: t.type, channel: ch, enabled: t.channels[ch] ?? true });
        }
      }
    }
    try {
      await api('notifications/preferences', { method: 'PUT', body: { preferences: prefs } });
    } catch (e) {
      showToast('Failed to save preferences', 'error');
    }
  }

  function channelLabel(ch) {
    const labels = { in_app: 'In-App', email: 'Email' };
    return labels[ch] || ch;
  }
</script>

<UserSettingsShell>
  <div class="page-fade">
    <h2>Notification Preferences</h2>
    <p class="muted subtitle">Choose how you want to be notified about activity in your patches.</p>

    {#if loading}
      <p class="muted" style="padding: 2rem 0;">Loading...</p>
    {:else}
      <div class="prefs-table">
        <!-- Column headers -->
        <div class="prefs-header">
          <span class="prefs-label-col"></span>
          {#each channels as ch}
            <span class="prefs-channel-col">{channelLabel(ch)}</span>
          {/each}
        </div>

        {#each categories as cat}
          <div class="prefs-category">
            <div class="category-heading">
              <span class="category-label">{cat.label}</span>
              <span class="category-desc muted">{cat.description}</span>
            </div>
            {#each cat.types as t}
              <div class="prefs-row">
                <span class="prefs-type-label">{t.label}</span>
                {#each channels as ch}
                  <span class="prefs-toggle">
                    <ToggleSwitch
                      checked={t.channels[ch]}
                      label="{t.label} — {channelLabel(ch)}"
                      onchange={() => toggle(t.type, ch)}
                    />
                  </span>
                {/each}
              </div>
            {/each}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</UserSettingsShell>

<style>
  h2 {
    font-size: 1.2rem;
    margin-bottom: 0.25rem;
  }

  .subtitle {
    font-size: 0.85rem;
    margin-bottom: 1.5rem;
  }

  .prefs-table {
    display: flex;
    flex-direction: column;
  }

  .prefs-header {
    display: flex;
    align-items: center;
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--color-border);
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .prefs-label-col {
    flex: 1;
  }

  .prefs-channel-col {
    width: 70px;
    text-align: center;
    flex-shrink: 0;
  }

  .prefs-category {
    margin-bottom: 0.5rem;
  }

  .category-heading {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    padding: 0.75rem 0 0.25rem;
  }

  .category-label {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--color-text);
  }

  .category-desc {
    font-size: 0.78rem;
  }

  .prefs-row {
    display: flex;
    align-items: center;
    padding: 0.4rem 0;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border) 50%, transparent);
  }

  .prefs-type-label {
    flex: 1;
    font-size: 0.85rem;
    color: var(--color-text);
    padding-left: 0.5rem;
  }

  .prefs-toggle {
    width: 70px;
    display: flex;
    justify-content: center;
    flex-shrink: 0;
  }

  @media (max-width: 640px) {
    .prefs-channel-col, .prefs-toggle {
      width: 50px;
    }
  }
</style>
