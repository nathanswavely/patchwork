<script>
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { loadMemberships } from '../stores/memberships.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import JoinSheet from '../components/JoinSheet.svelte';
  import { formatDay as formatDate } from '../lib/datetime.js';

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

  // The contact card (docs/adr/083). This page is where sharing happens,
  // because granting is patch-first: you decide to be reachable standing in
  // a room with people, not while editing a form. The items themselves are
  // owned by Profile settings; here you only choose which of them a patch
  // may read.
  let user = $derived(getUser());
  let myItems = $state([]);
  let cardEmpty = $derived(myItems.length === 0);
  // The patch whose picker is open, and the tentative set inside it. Held
  // apart from the row so cancelling changes nothing.
  let sharingFor = $state(null);
  let sharingIds = $state([]);
  let sharingBusy = $state(false);

  const KIND_WORD = { phone: 'Phone', email: 'Email', handle: 'Handle', note: 'Note' };

  $effect(() => {
    api('users/me/contact-items')
      .then((data) => { myItems = data.items || []; })
      .catch(() => { myItems = []; });
  });

  async function openSharing(m) {
    sharingFor = m.node_slug;
    sharingIds = [];
    try {
      const data = await api(`nodes/${m.node_slug}/contact-shares`);
      sharingIds = (data.items || []).filter((it) => it.shared).map((it) => it.id);
    } catch (e) {
      showToast(e.message || 'Failed to load contact sharing', 'error');
      sharingFor = null;
    }
  }

  function toggleSharingId(id) {
    sharingIds = sharingIds.includes(id)
      ? sharingIds.filter((x) => x !== id)
      : [...sharingIds, id];
  }

  // The whole set, replaced: a partial update of a disclosure set is a
  // request to get it half-applied.
  async function saveSharing(m) {
    sharingBusy = true;
    try {
      await api(`nodes/${m.node_slug}/contact-shares`, {
        method: 'PUT',
        body: { item_ids: sharingIds },
      });
      m.contact_items_shared = sharingIds.length;
      sharingFor = null;
      showToast(
        sharingIds.length === 0
          ? `${m.node_name || m.node_slug} can no longer reach you`
          : `${m.node_name || m.node_slug} can reach you by ${sharingIds.length === 1 ? '1 item' : `${sharingIds.length} items`}`,
        'info',
      );
    } catch (e) {
      showToast(e.message || 'Failed to update contact sharing', 'error');
    } finally {
      sharingBusy = false;
    }
  }

  // me/nodes serves 'active' and 'pending', and a pending request is not a
  // membership: sorting by role alone filed a request you had not been
  // answered on under "Member of" and badged it 'member'.
  let adminPatches = $derived(patches.filter(m => m.role === 'admin' && m.status === 'active'));
  let memberPatches = $derived(patches.filter(m => m.role === 'member' && m.status === 'active'));
  let pendingPatches = $derived(patches.filter(m => m.status === 'pending'));
  let followerPatches = $derived(patches.filter(m => m.role === 'follower' && m.status === 'active'));

  /**
   * The member rung renders only where it can succeed (docs/adr/042), the
   * same rule the relationship row runs: an unclaimed patch takes followers
   * only and invite_only refuses the request outright (memberships.go), so
   * this list wore a blue button that answered every click with a 403. The
   * reason is worn as state on the row instead — a door that cannot open is
   * worse than no door, and its error message is addressed to a reader who
   * should never have been offered the click.
   */
  function rungFor(m) {
    if (m.node_status === 'unclaimed' || m.membership_policy === 'invite_only') return null;
    return m.membership_policy === 'approval_required' ? 'Send request' : 'Become a member';
  }

  const STATE_NOTE = {
    unclaimed: {
      label: 'unclaimed',
      title: 'No one runs this patch yet. User can follow, but not join.',
    },
    invite_only: {
      label: 'invite only',
      title: 'This patch adds members by invitation. An admin has to invite you.',
    },
  };

  function stateNote(m) {
    if (m.node_status === 'unclaimed') return STATE_NOTE.unclaimed;
    if (m.membership_policy === 'invite_only') return STATE_NOTE.invite_only;
    return null;
  }

  // Two verbs, two routes (docs/adr/088). Leaving exits a relationship;
  // withdrawing retracts a request that was never answered, and a
  // requester holds no relationship to exit. Routing on the row's status
  // is also what makes a stale page safe: if the request was approved
  // between load and click, /withdraw refuses rather than quietly
  // resigning a membership this person did not know they had.
  async function handleLeave(m) {
    const withdrawing = m.status === 'pending';
    try {
      await api(`nodes/${m.node_slug}/${withdrawing ? 'withdraw' : 'leave'}`, { method: 'POST' });
      await loadMemberships();
      await loadPatches();
      // One wording per event, matching the relationship row: unfollowing
      // is not leaving, and withdrawing a request is neither.
      const said = withdrawing
        ? 'Request withdrawn'
        : m.role === 'follower' ? 'Unfollowed patch' : 'Left patch';
      showToast(said, 'info');
    } catch (e) {
      showToast(e.message || (withdrawing ? 'Failed to withdraw request' : 'Failed to leave'), 'error');
    }
  }

  // The join ceremony is the join sheet (docs/adr/040), here as on the patch
  // page — an approval-required patch gets its intro message either way,
  // rather than a silent request from whichever surface you happened to
  // start from.
  let joinTarget = $state(null);
  let joining = $state(false);

  async function handleJoin(message) {
    const target = joinTarget;
    if (!target) return;
    joining = true;
    try {
      const result = await api(`nodes/${target.node_slug}/join`, {
        method: 'POST',
        body: message ? { message } : undefined,
      });
      await loadMemberships();
      await loadPatches();
      showToast(result.status === 'pending' ? 'Membership request sent' : 'You are now a member', 'success');
    } catch (e) {
      showToast(e.message || 'Could not join', 'error');
    } finally {
      joining = false;
      joinTarget = null;
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

{#snippet contactToggle(m)}
  <button
    class="btn btn-sm vis-toggle"
    class:contact-shared={m.contact_items_shared > 0}
    disabled={cardEmpty}
    title={cardEmpty
      ? 'Add something to your contact card first, under Profile.'
      : 'Choose which of your contact items this patch\'s admins and members can read. Its followers never can.'}
    onclick={() => (sharingFor === m.node_slug ? (sharingFor = null) : openSharing(m))}
  >
    {#if m.contact_items_shared > 0}
      Reachable by {m.contact_items_shared}
    {:else}
      Not reachable
    {/if}
  </button>
{/snippet}

{#snippet contactPicker(m)}
  {#if sharingFor === m.node_slug}
    <div class="contact-picker">
      <p class="muted contact-picker-hint">
        What {m.node_name || m.node_slug} can reach you by. Its admins and
        members see whatever you tick here, including anyone who joins later.
      </p>
      <ul class="contact-picker-list">
        {#each myItems as it (it.id)}
          <li>
            <label>
              <input
                type="checkbox"
                checked={sharingIds.includes(it.id)}
                disabled={sharingBusy}
                onchange={() => toggleSharingId(it.id)}
              />
              <span class="contact-picker-kind">{KIND_WORD[it.kind] || it.kind}</span>
              <span class="contact-picker-value">{it.value}</span>
              {#if it.label}<span class="muted">{' · '}{it.label}</span>{/if}
            </label>
          </li>
        {/each}
      </ul>
      <div class="contact-picker-actions">
        <button class="btn btn-primary btn-sm" disabled={sharingBusy} onclick={() => saveSharing(m)}>Save</button>
        <button class="btn btn-sm" disabled={sharingBusy} onclick={() => (sharingFor = null)}>Cancel</button>
      </div>
    </div>
  {/if}
{/snippet}

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
    {#if cardEmpty && (adminPatches.length > 0 || memberPatches.length > 0)}
      <p class="muted contact-hint">
        Your contact card is empty. Add the ways you are willing to be reached
        under
        <a href="/settings" onclick={(e) => { e.preventDefault(); navigate('/settings'); }}>Profile</a>,
        then give them to patches one at a time here.
      </p>
    {/if}
    {#if adminPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Managing</h3>
        {#each adminPatches as m (m.node_slug)}
          <div class="patch-entry">
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
                {@render contactToggle(m)}
              </div>
            </div>
            {@render contactPicker(m)}
          </div>
        {/each}
      </section>
    {/if}

    {#if memberPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Member of</h3>
        {#each memberPatches as m (m.node_slug)}
          <div class="patch-entry">
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
                {@render contactToggle(m)}
                <ConfirmAction label="Leave" variant="warning" onConfirm={() => handleLeave(m)} />
              </div>
            </div>
            {@render contactPicker(m)}
          </div>
        {/each}
      </section>
    {/if}

    {#if pendingPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Requested</h3>
        {#each pendingPatches as m (m.node_slug)}
          <div class="patch-row">
            <div class="patch-info">
              <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                {m.node_name || m.node_slug}
              </a>
              <span class="badge" title="This patch's admins have not answered your request yet.">awaiting approval</span>
              <span class="muted joined-date">asked {formatDate(m.joined_at)}</span>
            </div>
            <!-- No contact control: a pending request is not standing in the
                 room yet, so there is nothing to share into (docs/adr/083). -->
            <div class="patch-actions">
              <ConfirmAction label="Withdraw" variant="default" onConfirm={() => handleLeave(m)} />
            </div>
          </div>
        {/each}
      </section>
    {/if}

    {#if followerPatches.length > 0}
      <section class="patch-section">
        <h3 class="section-heading">Following</h3>
        {#each followerPatches as m (m.node_slug)}
          {@const rung = rungFor(m)}
          {@const note = stateNote(m)}
          <div class="patch-row">
            <div class="patch-info">
              <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                {m.node_name || m.node_slug}
              </a>
              <span class="badge">following</span>
              {#if note}
                <span class="badge state-badge" title={note.title}>{note.label}</span>
              {/if}
              <span class="muted joined-date">{formatDate(m.joined_at)}</span>
            </div>
            <div class="patch-actions">
              {#if rung}
                <button class="btn btn-primary btn-sm" onclick={() => { joinTarget = m; }} disabled={joining}>{rung}</button>
              {/if}
              <ConfirmAction label="Unfollow" variant="default" onConfirm={() => handleLeave(m)} />
            </div>
          </div>
        {/each}
      </section>
    {/if}
  {/if}
</div>

<JoinSheet
  open={!!joinTarget}
  onClose={() => { joinTarget = null; }}
  onConfirm={handleJoin}
  slug={joinTarget?.node_slug || ''}
  patchName={joinTarget?.node_name || joinTarget?.node_slug || ''}
  membershipPolicy={joinTarget?.membership_policy || 'open'}
  submitting={joining}
/>

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

  .vis-toggle.contact-shared {
    color: var(--color-primary);
    border-color: currentColor;
  }

  /* State is not a role: the reason a rung is absent is worn quietly next
     to the badge that says where you stand, never dressed as one. */
  .state-badge {
    background: transparent;
    border: 1px dashed var(--color-border);
    color: var(--color-text-muted);
    font-weight: 500;
  }

  .contact-hint {
    font-size: 0.85rem;
    margin-bottom: 1rem;
  }

  /* One patch and, when open, the picker for what it can reach you by
     (docs/adr/083). The picker sits under its row rather than in a dialog:
     the decision is about this patch, so it stays attached to it. */
  .patch-entry {
    border-bottom: 1px solid var(--color-border);
  }
  .patch-entry .patch-row {
    border-bottom: none;
  }
  .contact-picker {
    padding: 0 0 0.75rem 0;
  }
  .contact-picker-hint {
    margin: 0 0 0.5rem;
    font-size: 0.85rem;
  }
  .contact-picker-list {
    list-style: none;
    margin: 0 0 0.6rem;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .contact-picker-list label {
    display: flex;
    align-items: baseline;
    gap: 0.45rem;
    font-size: 0.9rem;
    cursor: pointer;
  }
  .contact-picker-kind {
    font-weight: 600;
    min-width: 4.5rem;
  }
  .contact-picker-value {
    word-break: break-word;
  }
  .contact-picker-actions {
    display: flex;
    gap: 0.4rem;
  }

</style>
