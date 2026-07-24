<script>
  import { api } from '../lib/api.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  let pendingRoles = $state({});

  let users = $state([]);
  let loading = $state(true);
  let error = $state('');
  let searchInput = $state('');
  let searchQuery = $state('');
  let nextCursor = $state('');
  let searchTimeout = $state(null);

  // Promoting someone to admin needs a passkey confirmation. Surfaced on load
  // so an admin without one learns it here, not mid-promotion.
  let hasPasskey = $state(true);

  $effect(() => {
    void searchQuery;
    loadUsers();
  });

  $effect(() => {
    stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
  });

  function handleSearch(e) {
    searchInput = e.target.value;
    if (searchTimeout) clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
      searchQuery = searchInput;
    }, 300);
  }

  async function loadUsers(append = false) {
    if (!append) {
      loading = true;
      users = [];
      nextCursor = '';
    }
    error = '';
    try {
      const params = new URLSearchParams();
      if (searchQuery) params.set('search', searchQuery);
      if (append && nextCursor) params.set('after', nextCursor);
      const data = await api(`admin/users?${params}`);
      if (append) {
        users = [...users, ...data.items];
      } else {
        users = data.items;
      }
      nextCursor = data.next_cursor || '';
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  async function toggleSuspension(user) {
    try {
      const suspended = user.suspended_at ? '' : 'now';
      await api(`admin/users/${user.id}`, {
        method: 'PATCH',
        body: { suspended_at: suspended },
      });
      showToast(user.suspended_at ? 'User unsuspended' : 'User suspended', 'success');
      loadUsers();
    } catch (e) {
      showToast(e.message, 'error');
    }
  }

  // Trusted contributor (docs/adr/026): an explicit instance-level grant
  // that lets someone record events on unclaimed patches without review.
  async function toggleTrusted(user) {
    try {
      await api(`admin/users/${user.id}`, {
        method: 'PATCH',
        body: { trusted_contributor: !user.trusted_contributor },
      });
      showToast(
        user.trusted_contributor ? 'Trusted contributor revoked' : 'Marked as trusted contributor',
        'success'
      );
      loadUsers();
    } catch (e) {
      showToast(e.message, 'error');
    }
  }

  function handleRoleSelect(user, newRole) {
    if (newRole === user.role) {
      const { [user.id]: _, ...rest } = pendingRoles;
      pendingRoles = rest;
    } else {
      pendingRoles = { ...pendingRoles, [user.id]: newRole };
    }
  }

  async function setRole(user) {
    const newRole = pendingRoles[user.id];
    if (!newRole || user.role === newRole) return;
    try {
      // Promotion to admin needs a fresh passkey confirmation (docs/adr/017)
      // — it hands someone the wipe button. Demotion goes through untouched.
      await withStepUp(() => api(`admin/users/${user.id}`, {
        method: 'PATCH',
        body: { role: newRole },
      }));
      showToast(`Role updated to ${newRole}`, 'success');
      const { [user.id]: _, ...rest } = pendingRoles;
      pendingRoles = rest;
      loadUsers();
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      showToast(e.message, 'error');
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  let inviteMaxUses = $state(1);
  let inviteExpiresHrs = $state(72);
  let inviteUrl = $state('');
  let generatingInvite = $state(false);

  async function generateInvite() {
    generatingInvite = true;
    inviteUrl = '';
    try {
      const data = await api('auth/invite-link', {
        method: 'POST',
        body: {
          max_uses: Number(inviteMaxUses) || 1,
          expires_in_hours: Number(inviteExpiresHrs) || 0,
        },
      });
      inviteUrl = data.url;
    } catch (e) {
      showToast(e.message, 'error');
    } finally {
      generatingInvite = false;
    }
  }

  async function copyInviteUrl() {
    try {
      await navigator.clipboard.writeText(inviteUrl);
      showToast('Invite link copied', 'success');
    } catch {
      showToast('Could not copy. Select the link and copy it manually.', 'error');
    }
  }
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>User Management</h1>
  </div>

  <PasskeyNotice show={!hasPasskey} action="promote someone to instance admin" />

  <section class="invite-section card">
    <h2>Invite Links</h2>
    <p class="muted">
      Generate a link to invite someone to this Patchwork. Share it wherever
      your community talks: a message, a flyer, word of mouth. No email required.
    </p>
    <form class="invite-form" onsubmit={(e) => { e.preventDefault(); generateInvite(); }}>
      <label>
        Max uses
        <input type="number" min="1" max="100" bind:value={inviteMaxUses} />
      </label>
      <label>
        Expires in (hours, 0 = never)
        <input type="number" min="0" max="8760" bind:value={inviteExpiresHrs} />
      </label>
      <button type="submit" class="btn btn-primary" disabled={generatingInvite}>
        {generatingInvite ? 'Generating...' : 'Generate Invite Link'}
      </button>
    </form>
    {#if inviteUrl}
      <div class="invite-result">
        <input type="text" readonly value={inviteUrl} onfocus={(e) => e.target.select()} />
        <button class="btn btn-secondary" onclick={copyInviteUrl}>Copy</button>
      </div>
      <p class="muted invite-note">
        This link is shown once, so copy it now. Anyone with it can create an account.
      </p>
    {/if}
  </section>

  <div class="search-bar">
    <input
      type="search"
      placeholder="Search by name or email..."
      value={searchInput}
      oninput={handleSearch}
    />
  </div>

  {#if loading}
    <Skeleton lines={5} />
  {:else if error}
    <ErrorState message={error} retry={() => loadUsers()} />
  {:else if users.length === 0}
    <p class="muted" style="text-align: center; padding: 2rem 0;">No users found.</p>
  {:else}
    <div class="table-wrapper">
      <table class="data-table">
        <thead>
          <tr>
            <th>Username</th>
            <th>Display Name</th>
            <th>Role</th>
            <th>Trusted Contributor</th>
            <th>Joined</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each users as u (u.id)}
            <tr class:suspended={u.suspended_at}>
              <td>
                <a
                  href="/users/{u.username}"
                  class="user-link"
                  onclick={(e) => { e.preventDefault(); navigate(`/users/${u.username}`); }}
                >
                  {u.username}
                </a>
              </td>
              <td>{u.display_name || '--'}</td>
              <td>
                <select
                  value={pendingRoles[u.id] ?? u.role}
                  onchange={(e) => handleRoleSelect(u, e.target.value)}
                >
                  <option value="member">member</option>
                  <option value="admin">admin</option>
                </select>
                {#if pendingRoles[u.id]}
                  <ConfirmAction
                    label="Change Role"
                    confirmLabel="Yes, change role"
                    variant="warning"
                    onConfirm={() => setRole(u)}
                  />
                {/if}
              </td>
              <td>
                {#if u.trusted_contributor}
                  <span class="badge badge-trusted">Trusted</span>
                  <ConfirmAction
                    label="Revoke"
                    confirmLabel="Yes, revoke"
                    variant="warning"
                    onConfirm={() => toggleTrusted(u)}
                  />
                {:else}
                  <ConfirmAction
                    label="Grant"
                    confirmLabel="Yes, grant"
                    variant="warning"
                    onConfirm={() => toggleTrusted(u)}
                  />
                {/if}
              </td>
              <td class="muted">{formatDate(u.created_at)}</td>
              <td>
                {#if u.suspended_at}
                  <span class="badge badge-suspended">Suspended</span>
                {:else}
                  <span class="badge badge-active">Active</span>
                {/if}
              </td>
              <td>
                {#if u.suspended_at}
                  <ConfirmAction
                    label="Unsuspend"
                    confirmLabel="Yes, unsuspend"
                    variant="warning"
                    onConfirm={() => toggleSuspension(u)}
                  />
                {:else}
                  <ConfirmAction
                    label="Suspend"
                    confirmLabel="Yes, suspend"
                    variant="danger"
                    onConfirm={() => toggleSuspension(u)}
                  />
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if nextCursor}
      <div style="text-align: center; padding: 1rem 0;">
        <button class="btn btn-secondary" onclick={() => loadUsers(true)}>Load More</button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .page-header {
    padding: 1.5rem 0 1rem;
  }

  .invite-section {
    margin-bottom: 1.5rem;
    padding: 1.25rem;
  }

  .invite-section h2 {
    font-size: 1rem;
    margin-bottom: 0.25rem;
  }

  .invite-form {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 0.75rem;
    margin-top: 0.75rem;
  }

  .invite-form label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .invite-form input {
    width: 10rem;
  }

  .invite-result {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.75rem;
  }

  .invite-result input {
    flex: 1;
    font-family: monospace;
    font-size: 0.85rem;
  }

  .invite-note {
    margin-top: 0.4rem;
    font-size: 0.8rem;
  }

  .search-bar {
    margin-bottom: 1rem;
  }

  .search-bar input {
    width: 100%;
    max-width: 400px;
  }

  .table-wrapper {
    overflow-x: auto;
  }

  .data-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.9rem;
  }

  .data-table th,
  .data-table td {
    padding: 0.6rem 0.75rem;
    text-align: left;
    border-bottom: 1px solid var(--color-border);
  }

  .data-table th {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .data-table tr.suspended {
    opacity: 0.7;
  }

  .data-table select {
    padding: 0.25rem 0.5rem;
    font-size: 0.85rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .badge-trusted {
    color: var(--color-primary);
    border-color: var(--color-primary);
    background: none;
    margin-right: 0.35rem;
  }

  .badge-suspended {
    background: #fdf2f2;
    color: var(--color-error);
    border-color: var(--color-error);
  }

  .badge-active {
    background: #f0faf3;
    color: var(--color-success);
    border-color: var(--color-success);
  }

  .user-link {
    color: var(--color-text);
    text-decoration: none;
  }

  .user-link:hover {
    color: var(--color-primary);
    text-decoration: underline;
  }
</style>
