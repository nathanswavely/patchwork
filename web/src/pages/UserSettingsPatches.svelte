<script>
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { loadMemberships } from '../stores/memberships.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  let patches = $state([]);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    loadPatches();
  });

  async function loadPatches() {
    loading = true;
    error = '';
    try {
      const data = await api('me/nodes');
      patches = data.items || data || [];
    } catch (e) {
      error = e.message || 'Failed to load patches';
      patches = [];
    } finally {
      loading = false;
    }
  }

  let adminPatches = $derived(patches.filter(m => m.role === 'admin'));
  let memberPatches = $derived(patches.filter(m => m.role === 'member'));
  let followerPatches = $derived(patches.filter(m => m.role === 'follower'));

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  async function handleLeave(slug) {
    try {
      await api(`nodes/${slug}/leave`, { method: 'POST' });
      await loadMemberships();
      await loadPatches();
      showToast('Left patch', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to leave', 'error');
    }
  }

  async function handleBecomeMember(slug) {
    try {
      await api(`nodes/${slug}/join`, { method: 'POST' });
      await loadMemberships();
      await loadPatches();
      showToast('Joined as member', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to join', 'error');
    }
  }

  // One switch, both directions: hiding a membership removes it from your
  // profile AND from the patch's public member list. The patch's admins and
  // members still see you (docs/adr/006).
  async function toggleVisibility(m) {
    try {
      await api(`users/me/memberships/${m.node_id}`, {
        method: 'PATCH',
        body: { visible: !m.visible },
      });
      m.visible = !m.visible;
      showToast(m.visible ? 'Membership visible to the public' : 'Membership hidden from the public', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to update visibility', 'error');
    }
  }
</script>

{#snippet visibilityToggle(m)}
  <button
    class="btn btn-sm vis-toggle"
    class:vis-hidden={!m.visible}
    title={m.visible
      ? 'Shown on your profile and the patch\'s public member list. Click to hide from both.'
      : 'Hidden from your profile and the patch\'s public member list. Its admins and members still see you. Click to show.'}
    onclick={() => toggleVisibility(m)}
  >
    {m.visible ? 'Public' : 'Hidden'}
  </button>
{/snippet}

<div class="settings-patches">
  {#if loading}
    <p class="muted">Loading...</p>
  {:else if error}
    <p class="error-text">{error}</p>
  {:else if patches.length === 0}
    <p class="muted">You haven't joined any patches yet.</p>
  {:else}
    {#if adminPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Managing</h3>
        {#each adminPatches as m (m.node_slug)}
          <div class="patch-row">
            <div class="patch-info">
              <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                {m.node_name || m.node_slug}
              </a>
              <span class="badge">admin</span>
              <span class="muted joined-date">{formatDate(m.joined_at)}</span>
            </div>
            <div class="patch-actions">
              {@render visibilityToggle(m)}
            </div>
          </div>
        {/each}
      </section>
    {/if}

    {#if memberPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Member of</h3>
        {#each memberPatches as m (m.node_slug)}
          <div class="patch-row">
            <div class="patch-info">
              <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                {m.node_name || m.node_slug}
              </a>
              <span class="badge">member</span>
              <span class="muted joined-date">{formatDate(m.joined_at)}</span>
            </div>
            <div class="patch-actions">
              {@render visibilityToggle(m)}
              <ConfirmAction label="Leave" variant="warning" onConfirm={() => handleLeave(m.node_slug)} />
            </div>
          </div>
        {/each}
      </section>
    {/if}

    {#if followerPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Following</h3>
        {#each followerPatches as m (m.node_slug)}
          <div class="patch-row">
            <div class="patch-info">
              <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                {m.node_name || m.node_slug}
              </a>
              <span class="badge">following</span>
              <span class="muted joined-date">{formatDate(m.joined_at)}</span>
            </div>
            <div class="patch-actions">
              <button class="btn btn-primary btn-sm" onclick={() => handleBecomeMember(m.node_slug)}>Become Member</button>
              <ConfirmAction label="Unfollow" variant="default" onConfirm={() => handleLeave(m.node_slug)} />
            </div>
          </div>
        {/each}
      </section>
    {/if}
  {/if}
</div>

<style>
  .settings-patches {
    padding: 0;
  }

  .patch-section {
    margin-bottom: 1.5rem;
  }

  .patch-section:last-child {
    margin-bottom: 0;
  }

  .section-heading {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .patch-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.6rem 0;
    border-bottom: 1px solid var(--color-border);
    gap: 0.75rem;
  }

  .patch-info {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
    flex: 1;
  }

  .patch-name {
    font-size: 0.9rem;
    font-weight: 500;
    color: var(--color-text);
    text-decoration: none;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .patch-name:hover {
    color: var(--color-primary);
  }

  .joined-date {
    font-size: 0.75rem;
    flex-shrink: 0;
  }

  .patch-actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  .vis-toggle {
    border: 1px solid var(--color-border);
    background: var(--color-surface);
    color: var(--color-text-muted);
    border-radius: 999px;
  }

  .vis-toggle.vis-hidden {
    color: var(--color-warning, #b45309);
    border-color: currentColor;
  }
</style>
