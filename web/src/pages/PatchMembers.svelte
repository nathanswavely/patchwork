<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import PersonCard from '../components/PersonCard.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);
  let followerPermissions = $derived(patch.value.followerPermissions);
  let permissionDenied = $derived(membershipRole === 'follower' && followerPermissions?.members === false);

  // What this patch publishes (docs/adr/095), straight from the listing
  // that applied it — so the page can tell "no members yet" from "the list
  // is not public", which want opposite copy. An outsider is anyone who is
  // not in the room; a follower counts as one here, which is the single
  // place this setting lands somewhere a reader might not expect, and the
  // reason the withheld state says so rather than claiming the patch is
  // empty.
  let publicList = $state('everyone');
  let insider = $derived(isAdmin || membershipRole === 'member' || membershipRole === 'admin');
  let rosterWithheld = $derived(!insider && publicList === 'nobody');
  let adminsOnly = $derived(!insider && publicList === 'admins');

  let members = $state([]);
  let nextCursor = $state('');
  let loadingMore = $state(false);
  let loading = $state(true);

  // The header's counts come from the server, never from the loaded array:
  // the listing is paged, so counting what happened to arrive would report
  // the page size as the patch's size.
  let memberCount = $state(0);
  let followerCount = $state(0);

  // Contact items (docs/adr/083) ride along only for a viewer who is in the
  // room, and only the items shared into this patch. A member sharing
  // nothing here is offered the way to, which is patch-first and lives in My
  // Patches — never on this page, because granting from a list of people is
  // not the decision anyone is making while reading one.
  //
  // Both facts come from the server, never the loaded array (#231): the
  // listing is paged, so asking `members` whether the viewer shares here
  // really asks "…among these twenty", and a member on page 4 was offered
  // nothing.
  let inRoom = $derived(membershipRole === 'member' || membershipRole === 'admin');
  let viewerSharesContact = $state(false);
  let anyContact = $state(false);
  let offerSharing = $derived(inRoom && !viewerSharesContact);

  $effect(() => {
    if (slug) loadMembers();
  });

  async function loadMembers(after = '') {
    if (after) loadingMore = true;
    else loading = true;
    try {
      const q = after ? `?after=${encodeURIComponent(after)}` : '';
      const data = await api(`nodes/${slug}/members${q}`);
      const items = data.items || [];
      members = after ? [...members, ...items] : items;
      nextCursor = data.next_cursor || '';
      // Admins plus members, never followers (CONTEXT.md) — the two are
      // counted apart by the server and never summed.
      memberCount = data.member_count || 0;
      followerCount = data.follower_count || 0;
      // Sent only to a viewer in the room; absent means nothing to offer.
      viewerSharesContact = !!data.viewer_shares_contact;
      anyContact = !!data.any_contact_shared;
      publicList = data.public_member_list || 'everyone';
    } catch {
      if (!after) {
        members = [];
        nextCursor = '';
        memberCount = 0;
        followerCount = 0;
        viewerSharesContact = false;
        anyContact = false;
        publicList = 'everyone';
      }
    } finally {
      loading = false;
      loadingMore = false;
    }
  }
</script>

{#if permissionDenied}
  <div class="permission-notice">
    <p>This content is only visible to members.</p>
    <p class="muted">Become a member to access the member list.</p>
  </div>
{:else}
<div class="members-page">
  {#if loading}
    <p class="muted">Loading members...</p>
  {:else if rosterWithheld}
    <!-- The count is deliberately still here (docs/adr/095 decision 3): the
         quilt sizes this patch's tile by it, so withholding it on this one
         page would hide nothing and only make the page read as evasive. -->
    <div class="empty-state">
      <p>This patch doesn't publish its member list.</p>
      <p class="muted">{memberCount === 1 ? '1 member' : `${memberCount} members`}.</p>
    </div>
  {:else if members.length === 0}
    <div class="empty-state">
      <p>No members yet.</p>
      <p class="muted">Share your patch's invite link to grow your community.</p>
    </div>
  {:else}
    <div class="members-header">
      <span class="muted">{memberCount === 1 ? '1 member' : `${memberCount} members`}{#if followerCount > 0}{' · '}{followerCount === 1 ? '1 following' : `${followerCount} following`}{/if}</span>
      {#if adminsOnly}
        <!-- Said once, above the list, because a list of three under a
             count of forty otherwise reads as a bug. -->
        <span class="muted roster-note">Only this patch's admins are listed publicly.</span>
      {/if}
    </div>
    {#if offerSharing}
      <p class="muted contact-offer">
        {#if anyContact}
          Contact details here are shared only with this patch's admins and members.
        {/if}
        Share yours with this patch in
        <a href="/settings/patches" onclick={(e) => { e.preventDefault(); navigate('/settings/patches'); }}>My Patches</a>.
      </p>
    {/if}
    <ul class="member-list">
      {#each members as member (member.user_id)}
        <li class="member-row">
          <div class="member-main">
            <!-- One rendering of a person, wherever they are named: the card
                 carries the shared items, so the list stays a list. -->
            <span class="member-name">
              <PersonCard person={member} role={member.role} />
            </span>
            <span class="member-marks">
              <span class="badge">{member.role}</span>
              {#if (member.contact || []).length > 0}
                <span class="reachable" title="Shares contact details with this patch">reachable</span>
              {/if}
            </span>
          </div>
        </li>
      {/each}
    </ul>
    {#if nextCursor}
      <button
        class="btn btn-secondary btn-sm load-more"
        disabled={loadingMore}
        onclick={() => loadMembers(nextCursor)}
      >{loadingMore ? 'Loading...' : 'Load more'}</button>
    {/if}
  {/if}
</div>
{/if}

<style>
  .permission-notice {
    text-align: center;
    padding: 3rem 1rem;
  }

  .permission-notice p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .empty-state {
    text-align: center;
    padding: 2rem 0;
  }

  .empty-state p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  .members-header {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    margin-bottom: 0.75rem;
    font-size: 0.85rem;
  }

  .roster-note {
    font-size: 0.8rem;
  }

  .member-list {
    list-style: none;
    padding: 0;
  }

  .member-row {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.6rem 0;
    border-bottom: 1px solid var(--color-border);
    font-size: 0.9rem;
  }

  /* Name on the left, every mark on the right: space-between across three
     children strands the middle one. */
  .member-main {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.5rem;
  }

  .member-marks {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 0 0 auto;
  }




  .contact-offer {
    font-size: 0.85rem;
    margin-bottom: 0.75rem;
  }

  .member-row:last-child {
    border-bottom: none;
  }

  .load-more {
    margin-top: 0.75rem;
  }

  .member-name {
    font-weight: 500;
    color: var(--color-text);
    text-decoration: none;
  }

  .member-name:hover {
    color: var(--color-primary);
  }

  /* A quiet mark that this person can be reached here, so the list still
     says who is reachable without spelling out how — the how is in the card
     (docs/adr/083). */
  .reachable {
    font-size: 0.75rem;
    color: var(--color-text-muted, #666);
    border: 1px solid var(--color-border);
    border-radius: 999px;
    padding: 0.05rem 0.4rem;
  }

</style>
