<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { formatDay } from '../lib/datetime.js';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import Skeleton from '../components/Skeleton.svelte';
  import SegmentedControl from '../components/SegmentedControl.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

  // Who appears in the patch's public member list (docs/adr/095). A patch
  // can be findable without being enumerable, and this is where the group
  // says so once instead of asking every member to hide themselves one at
  // a time. It only ever subtracts: a member who hid stays hidden at
  // 'everyone' (docs/adr/006), and nothing here can put them back.
  let publicList = $state('everyone');
  let hydrated = false;
  $effect(() => {
    if (node && !hydrated) {
      publicList = node.public_member_list || 'everyone';
      hydrated = true;
    }
  });

  async function setPublicList(v) {
    const prev = publicList;
    publicList = v;
    try {
      await api(`nodes/${slug}`, { method: 'PATCH', body: { public_member_list: v } });
      showToast({
        everyone: 'Anyone can see who is in this patch',
        admins: 'Only admins are listed publicly now',
        nobody: 'The member list is no longer public',
      }[v], 'success');
      patch.value.reload();
    } catch (e) {
      publicList = prev;
      showToast(e.message || 'Failed to save', 'error');
    }
  }

  // Where the council is elected, this dropdown does not make an admin
  // (docs/adr/100). Two founders used it believing they were doing the
  // ordinary thing and ended up with three admins, no new seats and no
  // election, on a patch whose governance page says the community elects
  // its council. The Admin option is withdrawn here and the section says
  // what fills a seat instead.
  let council = $state(null);
  let electedCouncil = $derived(
    council?.rules?.leadership_model === 'elected' && council?.rules?.leadership_venue !== 'elsewhere'
  );
  let councilSeats = $derived(council?.seats || []);
  let vacantSeats = $derived(councilSeats.filter((seat) => seat.vacant).length);
  let nextContestOpens = $derived(council?.next_contest_opens || '');

  $effect(() => {
    if (slug) loadCouncil();
  });

  async function loadCouncil() {
    try {
      council = await api(`nodes/${slug}/governance/overview`);
    } catch {
      council = null;
    }
  }

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

  // Invitations (docs/adr/098): the door an invite-only patch never had.
  // An admin asks by username; the person answers. The list rides on the
  // members payload under its own key, for the patch's admins only.
  let invited = $state([]);
  let inviteUsername = $state('');
  let inviting = $state(false);

  // Followers are their own relationship, listed apart and with no role
  // control (docs/adr/2026-09-17-a-follower-is-not-a-quieter-member.md). They used to arrive in the same array as the
  // membership, under a heading that said Active Members, above a count that
  // excluded them — and every row got a role dropdown, so a follower could
  // be handed admin without ever passing through membership. The server
  // refuses that now; this is the same rule where a person can see it.
  let followers = $state([]);

  async function loadMembers() {
    loadingMembers = true;
    try {
      const [data, followerData] = await Promise.all([
        api(`nodes/${slug}/members`),
        api(`nodes/${slug}/members?role=follower`),
      ]);
      members = data.items || data || [];
      invited = data.invited || [];
      followers = followerData.items || [];
      roleEdits = {};
    } catch {
      members = [];
      invited = [];
      followers = [];
    } finally {
      loadingMembers = false;
    }
  }

  async function inviteMember(e) {
    e?.preventDefault?.();
    const username = inviteUsername.trim();
    if (!username || inviting) return;
    inviting = true;
    try {
      await api(`nodes/${slug}/invitations`, { method: 'POST', body: { username } });
      showToast('Invitation sent', 'success');
      inviteUsername = '';
      await loadMembers();
    } catch (err) {
      showToast(err.message || 'Could not invite', 'error');
    } finally {
      inviting = false;
    }
  }

  async function rescindInvitation(userId) {
    try {
      await api(`nodes/${slug}/invitations/${userId}`, { method: 'DELETE' });
      showToast('Invitation rescinded', 'success');
      await loadMembers();
    } catch (err) {
      showToast(err.message || 'Could not rescind invitation', 'error');
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
  <section class="members-section">
    <h3 class="section-heading">Public member list</h3>
    <div class="setting-row">
      <div class="setting-info">
        <!-- Two things the control cannot show on its face, and each of
             them changes whether an admin's choice does what they think. The
             count one matters most against the "Nobody" label, which would
             otherwise read as hiding the number too (docs/adr/095). -->
        <span class="setting-desc muted">
          Who a visitor sees listed. Admins and members always see everyone. The member count stays public either way.
        </span>
      </div>
      <SegmentedControl
        options={[
          { value: 'everyone', label: 'Everyone' },
          { value: 'admins', label: 'Admins only' },
          { value: 'nobody', label: 'Nobody' },
        ]}
        value={publicList}
        label="Who appears in the public member list"
        onchange={setPublicList}
      />
    </div>
  </section>

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
          <li class="member-row pending-row">
            <div class="member-row-main">
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
            </div>
            {#if member.join_message}
              <!-- The join sheet's optional intro note (docs/adr/040) — quoted, never editable here. -->
              <p class="join-message">&ldquo;{member.join_message}&rdquo;</p>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <!-- Invitations (docs/adr/098). Above the roster because it is how the
       roster grows on an invite-only patch; on any other it is one more
       way in, and costs the same. -->
  <section class="members-section">
    <h3 class="section-heading">Invite a member</h3>
    <form class="invite-form" onsubmit={inviteMember}>
      <input
        class="invite-input"
        type="text"
        placeholder="Username"
        aria-label="Username to invite"
        autocomplete="off"
        bind:value={inviteUsername}
        disabled={inviting}
      />
      <button class="btn btn-primary btn-sm" type="submit" disabled={inviting || !inviteUsername.trim()}>
        Invite
      </button>
    </form>
    <!-- The one thing the form cannot show: what pressing Invite does not
         do. It asks; it never admits. -->
    <p class="setting-desc muted">They're notified and join only if they accept.</p>

    {#if !loadingMembers && invited.length > 0}
      <h4 class="subheading">Invited</h4>
      <ul class="member-list">
        {#each invited as person (person.user_id)}
          <li class="member-row">
            <div class="member-info">
              <span class="member-name">{person.display_name || person.username}</span>
              <span class="badge badge-invited">invited</span>
            </div>
            <div class="member-actions">
              <ConfirmAction
                label="Rescind"
                confirmLabel="Rescind"
                variant="danger"
                onConfirm={() => rescindInvitation(person.user_id)}
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

    {#if electedCouncil}
      <p class="council-note muted">
        {#if vacantSeats > 0}
          This patch elects its council, so admins are not made here. {vacantSeats} seat{vacantSeats === 1 ? ' is' : 's are'} vacant: an admin nominates a member on the Governance page and the members ratify it.
        {:else if nextContestOpens}
          This patch elects its council, so admins are not made here. All {councilSeats.length} seat{councilSeats.length === 1 ? '' : 's'} {councilSeats.length === 1 ? 'is' : 'are'} held; the next contest opens {formatDay(nextContestOpens)}. To make room sooner, add a seat on the Governance page, or bring a seat's term end forward there.
        {:else}
          This patch elects its council, so admins are not made here. Every seat is held and no contest is scheduled. To make room, add a seat on the Governance page.
        {/if}
      </p>
    {/if}

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
                <!-- Demotion, not a rung to climb: a follower is never
                     promoted from here (docs/adr/2026-09-17-a-follower-is-not-a-quieter-member.md), so this option only
                     ever moves somebody out of the membership. -->
                <option value="follower">Follower</option>
                <option value="member">Member</option>
                <!-- Kept for somebody who already holds the seat, so the
                     control can show their role and demote them. Demotion is
                     untouched; only making an admin moved. -->
                {#if !electedCouncil || member.role === 'admin'}
                  <option value="admin">Admin</option>
                {/if}
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

  <section class="members-section">
    <h3 class="section-heading">Followers</h3>
    <p class="setting-desc muted">
      Followers see only this patch's events and public pages.
    </p>
    {#if loadingMembers}
      <Skeleton lines={2} height="0.9rem" />
    {:else if followers.length === 0}
      <p class="muted">No followers yet.</p>
    {:else}
      <ul class="member-list">
        {#each followers as follower (follower.user_id)}
          <li class="member-row">
            <div class="member-info">
              <span class="member-name">{follower.display_name || follower.username}</span>
            </div>
            <div class="member-actions">
              <ConfirmAction
                label="Remove"
                confirmLabel="Remove"
                variant="danger"
                onConfirm={() => banMember(follower.user_id)}
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
  /* The list control sits above the roster it governs: stacked, because the
     sentence explaining it is longer than a row can hold beside a control. */
  .setting-row {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.6rem;
  }
  .setting-info { display: flex; flex-direction: column; gap: 0.1rem; }
  .setting-desc { font-size: 0.8rem; max-width: 42rem; }

  .settings-members {
    max-width: var(--pw-measure-narrow);
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

  .pending-row {
    flex-direction: column;
    align-items: stretch;
    gap: 0.4rem;
  }

  .member-row-main {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .join-message {
    font-size: 0.85rem;
    font-style: italic;
    color: var(--color-text-muted);
    margin: 0;
    padding-left: 0.1rem;
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

  .council-note {
    font-size: 0.8rem;
    line-height: 1.5;
    margin: 0 0 0.75rem;
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

  /* Invitations: the field and its button on one line, the list below. */
  .invite-form {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    max-width: 24rem;
  }

  .invite-input {
    flex: 1;
    min-width: 0;
    font-size: 0.85rem;
    padding: 0.35rem 0.55rem;
    border: 1px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text);
    font-family: inherit;
  }

  .subheading {
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--color-text-muted);
    margin: 1rem 0 0.4rem;
  }

  /* Invited is a wait, not a standing: the muted overlay, not a colour. */
  .badge-invited {
    background: var(--color-overlay);
    color: var(--color-text-muted);
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
