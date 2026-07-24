<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import Skeleton from '../components/Skeleton.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  let members = $state([]);
  let loadingMembers = $state(true);
  let pendingMembers = $state([]);
  let loadingPending = $state(true);
  let bannedMembers = $state([]);
  let loadingBanned = $state(true);
  let showBanned = $state(false);

  // Track pending role changes per user (explicit save)
  let roleEdits = $state({});

  $effect(() => {
    if (slug) {
      loadMembers();
      loadPendingMembers();
      loadBannedMembers();
    }
  });

  async function loadMembers() {
    loadingMembers = true;
    try {
      const data = await api(`nodes/${slug}/members`);
      members = data.items || data || [];
      roleEdits = {};
    } catch {
      members = [];
    } finally {
      loadingMembers = false;
    }
  }

  async function loadPendingMembers() {
    loadingPending = true;
    try {
      const data = await api(`nodes/${slug}/members?status=pending`);
      pendingMembers = data.items || data || [];
    } catch {
      pendingMembers = [];
    } finally {
      loadingPending = false;
    }
  }

  async function loadBannedMembers() {
    loadingBanned = true;
    try {
      const data = await api(`nodes/${slug}/members?status=banned`);
      bannedMembers = data.items || data || [];
    } catch {
      bannedMembers = [];
    } finally {
      loadingBanned = false;
    }
  }

  async function banMember(userId) {
    try {
      await api(`nodes/${slug}/members/${userId}`, {
        method: 'PATCH',
        body: { status: 'banned' },
      });
      showToast('Member removed', 'success');
      await Promise.all([loadMembers(), loadBannedMembers()]);
    } catch (e) {
      showToast(e.message || 'Failed to remove member', 'error');
    }
  }

  async function reinstateMember(userId) {
    try {
      await api(`nodes/${slug}/members/${userId}`, {
        method: 'PATCH',
        body: { status: 'left' },
      });
      showToast('Member reinstated', 'success');
      await loadBannedMembers();
    } catch (e) {
      showToast(e.message || 'Failed to reinstate', 'error');
    }
  }

  async function approveMember(userId) {
    try {
      await api(`nodes/${slug}/members/${userId}`, {
        method: 'PATCH',
        body: { status: 'active' },
      });
      showToast('Member approved', 'success');
      await Promise.all([loadMembers(), loadPendingMembers()]);
    } catch (e) {
      showToast(e.message || 'Failed to approve member', 'error');
    }
  }

  async function rejectMember(userId) {
    try {
      await api(`nodes/${slug}/members/${userId}`, {
        method: 'PATCH',
        body: { status: 'left' },
      });
      showToast('Request rejected', 'success');
      await Promise.all([loadMembers(), loadPendingMembers()]);
    } catch (e) {
      showToast(e.message || 'Failed to reject member', 'error');
    }
  }

  function setRoleEdit(userId, newRole) {
    roleEdits = { ...roleEdits, [userId]: newRole };
  }

  async function saveRole(userId) {
    const newRole = roleEdits[userId];
    if (!newRole) return;
    try {
      await api(`nodes/${slug}/members/${userId}`, {
        method: 'PATCH',
        body: { role: newRole },
      });
      showToast('Role updated', 'success');
      const { [userId]: _, ...rest } = roleEdits;
      roleEdits = rest;
      await loadMembers();
    } catch (e) {
      showToast(e.message || 'Failed to change role', 'error');
    }
  }
</script>

<div class="settings-members">
  <!-- Pending Requests -->
  <section class="members-section">
    <h3 class="section-heading">Pending Requests</h3>

    {#if loadingPending}
      <Skeleton lines={2} height="0.9rem" />
    {:else if pendingMembers.length === 0}
      <p class="muted">No pending requests.</p>
    {:else}
      <ul class="member-list">
        {#each pendingMembers as member (member.user_id)}
          <li class="member-row">
            <div class="member-info">
              <span class="member-name">{member.display_name || member.username}</span>
              <span class="badge badge-pending">pending</span>
            </div>
            <div class="member-actions">
              <ConfirmAction
                label="Approve"
                confirmLabel="Approve"
                variant="default"
                onConfirm={() => approveMember(member.user_id)}
              />
              <ConfirmAction
                label="Reject"
                confirmLabel="Reject"
                variant="danger"
                onConfirm={() => rejectMember(member.user_id)}
              />
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <!-- Active Members -->
  <section class="members-section">
    <h3 class="section-heading">Active Members</h3>

    {#if loadingMembers}
      <Skeleton lines={4} height="0.9rem" />
    {:else if members.length === 0}
      <p class="muted">No members yet.</p>
    {:else}
      <ul class="member-list">
        {#each members as member (member.user_id)}
          <li class="member-row">
            <div class="member-info">
              <span class="member-name">{member.display_name || member.username}</span>
              <span class="badge">{member.role}</span>
            </div>
            <div class="member-actions">
              <select
                class="role-select"
                value={roleEdits[member.user_id] ?? member.role}
                onchange={(e) => setRoleEdit(member.user_id, e.target.value)}
              >
                <option value="follower">Follower</option>
                <option value="member">Member</option>
                <option value="admin">Admin</option>
              </select>
              {#if roleEdits[member.user_id] && roleEdits[member.user_id] !== member.role}
                <button class="btn btn-primary btn-sm" onclick={() => saveRole(member.user_id)}>
                  Save
                </button>
              {/if}
              <ConfirmAction
                label="Remove"
                confirmLabel="Remove"
                variant="danger"
                onConfirm={() => banMember(member.user_id)}
              />
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <!-- Banned Members -->
  {#if bannedMembers.length > 0 || showBanned}
    <section class="members-section">
      <button class="section-toggle" onclick={() => showBanned = !showBanned}>
        <h3 class="section-heading">
          Removed Members ({bannedMembers.length})
          <span class="toggle-caret">{showBanned ? '\u25B2' : '\u25BC'}</span>
        </h3>
      </button>

      {#if showBanned}
        {#if loadingBanned}
          <Skeleton lines={2} height="0.9rem" />
        {:else if bannedMembers.length === 0}
          <p class="muted">No removed members.</p>
        {:else}
          <ul class="member-list">
            {#each bannedMembers as member (member.user_id)}
              <li class="member-row">
                <div class="member-info">
                  <span class="member-name">{member.display_name || member.username}</span>
                  <span class="badge badge-banned">removed</span>
                </div>
                <div class="member-actions">
                  <ConfirmAction
                    label="Reinstate"
                    confirmLabel="Reinstate"
                    variant="default"
                    onConfirm={() => reinstateMember(member.user_id)}
                  />
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    </section>
  {/if}
</div>

<style>
  .settings-members {
    max-width: 600px;
  }

  .members-section {
    margin-bottom: 2rem;
  }

  .section-heading {
    font-size: 0.9rem;
    font-weight: 600;
    margin-bottom: 0.75rem;
    color: var(--color-text);
  }

  .member-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .member-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.6rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .member-row:last-child {
    border-bottom: none;
  }

  .member-info {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .member-name {
    font-weight: 500;
    font-size: 0.9rem;
  }

  .member-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .role-select {
    font-size: 0.82rem;
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text);
    font-family: inherit;
  }

  .badge-pending {
    background: var(--color-warning);
    color: var(--color-on-warning);
  }

  .badge-banned {
    background: var(--color-error);
    color: var(--color-on-error);
  }

  .section-toggle {
    border: none;
    background: none;
    padding: 0;
    cursor: pointer;
    width: 100%;
    text-align: left;
  }

  .toggle-caret {
    font-size: 0.7rem;
    color: var(--color-text-muted);
    margin-left: 0.25rem;
  }

</style>
