<script>
  import { api } from '../lib/api.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import TrustScopePicker from '../components/TrustScopePicker.svelte';
  import { formatDay as formatDate } from '../lib/datetime.js';

  let pendingRoles = $state({});

  let users = $state([]);
  let loading = $state(true);
  let error = $state('');
  let searchInput = $state('');
  let searchQuery = $state('');
  let nextCursor = $state('');
  let searchTimeout = $state(null);

  // Promoting someone to admin and setting someone's email both need a
  // passkey confirmation. Surfaced on load so an admin without one learns it
  // here, not mid-action.
  let hasPasskey = $state(true);

  // Which user's email is being edited, and the address typed so far. One at
  // a time: this is a repair, not data entry.
  let editingEmailFor = $state('');
  let emailDraft = $state('');
  let savingEmail = $state(false);

  $effect(() => {
    void searchQuery;
    loadUsers();
  });

  $effect(() => {
    stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
  });

  // Trust requests (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-
  // carries-its-calendar, decision 7). A request is answered, never merely
  // seen: it arrives here with the scope that was asked for already in the
  // picker, and the admin grants wider or narrower before clicking.
  let trustRequests = $state([]);
  let requestScopes = $state({});
  let requestNotes = $state({});
  let requestBusy = $state('');

  $effect(() => { loadTrustRequests(); });

  async function loadTrustRequests() {
    try {
      const data = await api('admin/trust-requests');
      const items = data.items || [];
      const scopes = {};
      const notes = {};
      for (const req of items) {
        scopes[req.id] = {
          all: req.scope === 'all',
          selected: (req.nodes || []).map((n) => ({ id: n.id, slug: n.slug, name: n.name })),
        };
        notes[req.id] = '';
      }
      requestScopes = scopes;
      requestNotes = notes;
      trustRequests = items;
    } catch {
      trustRequests = [];
    }
  }

  async function approveRequest(req) {
    const scope = requestScopes[req.id];
    if (!scope.all && scope.selected.length === 0) {
      showToast('Pick at least one patch, or every unclaimed patch.', 'error');
      return;
    }
    const body = { action: 'approve', scope: scope.all ? 'all' : 'patches' };
    if (!scope.all) body.node_ids = scope.selected.map((n) => n.id);
    requestBusy = req.id;
    try {
      await api(`admin/trust-requests/${req.id}`, { method: 'PATCH', body });
      showToast('Trust granted', 'success');
      await loadTrustRequests();
      loadUsers();
    } catch (e) {
      showToast(e.message, 'error');
    } finally {
      requestBusy = '';
    }
  }

  async function declineRequest(req) {
    requestBusy = req.id;
    try {
      await api(`admin/trust-requests/${req.id}`, {
        method: 'PATCH',
        body: { action: 'decline', note: (requestNotes[req.id] || '').trim() },
      });
      showToast('Trust request declined', 'success');
      await loadTrustRequests();
    } catch (e) {
      showToast(e.message, 'error');
    } finally {
      requestBusy = '';
    }
  }

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

  // The per-patch grant: the same standing on one unclaimed patch and
  // nothing else. Granted and revoked here because this is where the
  // quilt-wide toggle already lives.
  let addingTrustFor = $state('');
  let addTrustSelected = $state([]);
  let savingTrustPatches = $state(false);

  function startTrustPatches(user) {
    addingTrustFor = user.id;
    addTrustSelected = [];
  }

  function cancelTrustPatches() {
    addingTrustFor = '';
    addTrustSelected = [];
  }

  async function saveTrustPatches(user) {
    if (addTrustSelected.length === 0) {
      showToast('Pick at least one patch.', 'error');
      return;
    }
    savingTrustPatches = true;
    try {
      for (const node of addTrustSelected) {
        await api(`admin/users/${user.id}/trusted-patches`, {
          method: 'POST',
          body: { node_id: node.id },
        });
      }
      showToast(
        addTrustSelected.length === 1
          ? `Trusted on ${addTrustSelected[0].name}`
          : `Trusted on ${addTrustSelected.length} patches`,
        'success'
      );
      cancelTrustPatches();
      loadUsers();
    } catch (e) {
      showToast(e.message, 'error');
    } finally {
      savingTrustPatches = false;
    }
  }

  async function revokeTrustedPatch(user, node) {
    try {
      await api(`admin/users/${user.id}/trusted-patches/${node.id}`, { method: 'DELETE' });
      showToast(`Trust on ${node.name} revoked`, 'success');
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

  function startEmailEdit(user) {
    editingEmailFor = user.id;
    emailDraft = user.email || '';
  }

  function cancelEmailEdit() {
    editingEmailFor = '';
    emailDraft = '';
  }

  // Setting an address points the account at a mailbox, and whoever holds it
  // can magic-link in — so it takes a passkey confirmation the way promotion
  // does (docs/adr/017, docs/adr/072).
  async function saveEmail(user) {
    const email = emailDraft.trim();
    if (!email) return;
    savingEmail = true;
    try {
      const data = await withStepUp(() => api(`admin/users/${user.id}/email`, {
        method: 'PUT',
        body: { email },
      }));
      // Show the address as stored, not as typed: sign-in matches it exactly,
      // so this is what the person has to enter.
      showToast(`Email set to ${data.email}`, 'success');
      cancelEmailEdit();
      loadUsers();
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      showToast(e.message, 'error');
    } finally {
      savingEmail = false;
    }
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

  <PasskeyNotice show={!hasPasskey} action="promote someone to instance admin or set their email address" />

  {#if trustRequests.length > 0}
    <section class="requests-section card">
      <h2>Trust requests</h2>
      <p class="muted">
        People asking to add events on unclaimed patches without review. Grant
        the scope you judge right, wider or narrower than the one asked for.
      </p>
      <div class="request-list">
        {#each trustRequests as req (req.id)}
          <div class="request">
            <div class="request-head">
              <a
                href="/users/{req.user.username}"
                class="user-link"
                onclick={(e) => { e.preventDefault(); navigate(`/users/${req.user.username}`); }}
              >{req.user.display_name || req.user.username}</a>
              <span class="muted">@{req.user.username}</span>
              <span class="muted">{formatDate(req.created_at)}</span>
            </div>
            {#if req.message}
              <p class="request-message">{req.message}</p>
            {/if}
            <TrustScopePicker
              bind:all={requestScopes[req.id].all}
              bind:selected={requestScopes[req.id].selected}
              disabled={requestBusy === req.id}
              label="Asked for"
            />
            <label class="request-note" for="trust-note-{req.id}">Note to the requester (optional)</label>
            <textarea
              id="trust-note-{req.id}"
              rows="2"
              placeholder="Sent with a decline"
              bind:value={requestNotes[req.id]}
            ></textarea>
            <div class="request-actions">
              <button
                class="btn btn-primary btn-sm"
                disabled={requestBusy === req.id}
                onclick={() => approveRequest(req)}
              >Approve</button>
              <button
                class="btn btn-secondary btn-sm"
                disabled={requestBusy === req.id}
                onclick={() => declineRequest(req)}
              >Decline</button>
            </div>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  <section class="invite-section card">
    <h2>Invite Links</h2>
    <p class="muted">
      Generate a link to invite someone to this Patchwork. Share it wherever your community talks: a message, a flyer, word of mouth.
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
            <th>Email</th>
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
                {#if editingEmailFor === u.id}
                  <form
                    class="email-edit"
                    onsubmit={(e) => { e.preventDefault(); saveEmail(u); }}
                  >
                    <!-- svelte-ignore a11y_autofocus -->
                    <input
                      type="email"
                      autofocus
                      bind:value={emailDraft}
                      placeholder="name@example.com"
                      onkeydown={(e) => { if (e.key === 'Escape') cancelEmailEdit(); }}
                    />
                    <button type="submit" class="btn btn-primary btn-sm" disabled={savingEmail || !emailDraft.trim()}>
                      {savingEmail ? 'Saving...' : 'Save'}
                    </button>
                    <button type="button" class="btn btn-secondary btn-sm" onclick={cancelEmailEdit}>
                      Cancel
                    </button>
                  </form>
                {:else}
                  <span class="email-address" class:muted={!u.email}>{u.email || '--'}</span>
                  <button class="btn btn-secondary btn-sm" onclick={() => startEmailEdit(u)}>
                    {u.email ? 'Change' : 'Set'}
                  </button>
                {/if}
              </td>
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
                {#if (u.trusted_nodes || []).length > 0}
                  <div class="patch-grants">
                    {#each u.trusted_nodes as node (node.id)}
                      <span class="patch-grant">
                        Trusted on {node.name}
                        <button
                          type="button"
                          class="grant-remove"
                          title="Revoke"
                          aria-label="Revoke trust on {node.name}"
                          onclick={() => revokeTrustedPatch(u, node)}
                        >×</button>
                      </span>
                    {/each}
                  </div>
                {/if}
                {#if addingTrustFor === u.id}
                  <div class="patch-grant-add">
                    <TrustScopePicker
                      bind:selected={addTrustSelected}
                      allowAll={false}
                      disabled={savingTrustPatches}
                      label="Patches"
                    />
                    <div class="grant-add-actions">
                      <button
                        class="btn btn-primary btn-sm"
                        disabled={savingTrustPatches || addTrustSelected.length === 0}
                        onclick={() => saveTrustPatches(u)}
                      >{savingTrustPatches ? 'Saving...' : 'Grant'}</button>
                      <button class="btn btn-secondary btn-sm" onclick={cancelTrustPatches}>Cancel</button>
                    </div>
                  </div>
                {:else}
                  <button class="btn btn-secondary btn-sm grant-add-btn" onclick={() => startTrustPatches(u)}>
                    Add a patch
                  </button>
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

  .invite-section,
  .requests-section {
    margin-bottom: 1.5rem;
    padding: 1.25rem;
  }

  .requests-section h2 {
    font-size: 1rem;
    margin-bottom: 0.25rem;
  }

  .requests-section .muted {
    font-size: 0.82rem;
  }

  .request-list {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    margin-top: 0.9rem;
  }

  .request {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--color-border);
  }

  .request:first-child {
    padding-top: 0;
    border-top: none;
  }

  .request-head {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 0.5rem;
    font-size: 0.88rem;
  }

  .request-head .muted {
    font-size: 0.78rem;
  }

  .request-message {
    font-size: 0.85rem;
    color: var(--color-text-muted);
    margin: 0;
  }

  .request-note {
    font-size: 0.78rem;
    font-weight: 500;
  }

  .request textarea {
    max-width: 420px;
    font-size: 0.85rem;
  }

  .request-actions {
    display: flex;
    gap: 0.5rem;
  }

  .patch-grants {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    margin-top: 0.35rem;
  }

  .patch-grant {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.15rem 0.45rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    font-size: 0.72rem;
    white-space: nowrap;
  }

  .grant-remove {
    border: none;
    background: none;
    padding: 0;
    color: inherit;
    cursor: pointer;
    font-size: 0.85rem;
    line-height: 1;
    opacity: 0.75;
  }

  .grant-remove:hover {
    opacity: 1;
  }

  .patch-grant-add {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    margin-top: 0.4rem;
    min-width: 16rem;
  }

  .grant-add-actions {
    display: flex;
    gap: 0.4rem;
  }

  .grant-add-btn {
    margin-top: 0.35rem;
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

  /* .table-wrapper and .data-table are the shared table component (app.css). */
  .data-table tr.suspended {
    opacity: 0.7;
  }

  .data-table select {
    padding: 0.35rem 0.5rem;
    font-size: 0.85rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .email-edit {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .email-edit input {
    min-width: 12rem;
    padding: 0.35rem 0.5rem;
    font-size: 0.85rem;
  }

  .email-address {
    margin-right: 0.4rem;
    word-break: break-all;
  }

  .badge-trusted {
    color: var(--color-primary);
    border-color: var(--color-primary);
    background: none;
    margin-right: 0.35rem;
  }

  .badge-suspended {
    background: var(--color-danger-bg);
    color: var(--color-error);
    border-color: var(--color-error);
  }

  .badge-active {
    background: var(--color-success-bg);
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
