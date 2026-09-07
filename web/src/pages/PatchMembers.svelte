<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';

  const patch = getContext('patch');
  let me = $derived(getUser());
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);
  let followerPermissions = $derived(patch.value.followerPermissions);
  let permissionDenied = $derived(membershipRole === 'follower' && followerPermissions?.members === false);

  let members = $state([]);
  let nextCursor = $state('');
  let loadingMore = $state(false);
  let loading = $state(true);

  // The header's counts come from the server, never from the loaded array:
  // the listing is paged, so counting what happened to arrive would report
  // the page size as the patch's size.
  let memberCount = $state(0);
  let followerCount = $state(0);

  // Contact cards (docs/adr/080) ride along only for a viewer who is in
  // the room. A member whose own row carries none is offered the switch.
  let inRoom = $derived(membershipRole === 'member' || membershipRole === 'admin');
  let myRow = $derived(me ? members.find((m) => m.user_id === me.id) : null);
  let offerSharing = $derived(inRoom && myRow && !myRow.contact);
  let anyContact = $derived(members.some((m) => m.contact));

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
    } catch {
      if (!after) {
        members = [];
        nextCursor = '';
        memberCount = 0;
        followerCount = 0;
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
  {:else if members.length === 0}
    <div class="empty-state">
      <p>No members yet.</p>
      <p class="muted">Share your patch's invite link to grow your community.</p>
    </div>
  {:else}
    <div class="members-header">
      <span class="muted">{memberCount === 1 ? '1 member' : `${memberCount} members`}{#if followerCount > 0}{' · '}{followerCount === 1 ? '1 following' : `${followerCount} following`}{/if}</span>
    </div>
    {#if offerSharing}
      <p class="muted contact-offer">
        {#if anyContact}
          Contact cards here are shared only with this patch's admins and members.
        {/if}
        Share yours with this patch in
        <a href="/settings/patches" onclick={(e) => { e.preventDefault(); navigate('/settings/patches'); }}>My Patches</a>.
      </p>
    {/if}
    <ul class="member-list">
      {#each members as member (member.user_id)}
        <li class="member-row">
          <div class="member-main">
            <a
              href="/users/{member.username}"
              class="member-name"
              onclick={(e) => { e.preventDefault(); navigate(`/users/${member.username}`); }}
            >
              {member.display_name || member.username}
            </a>
            <span class="badge">{member.role}</span>
          </div>
          {#if member.contact}
            <!-- Shown to this patch's admins and members only (docs/adr/080). -->
            <div class="member-contact">
              {#if member.contact.phone}
                <a href="tel:{member.contact.phone.replace(/[^+\d]/g, '')}" class="contact-item">{member.contact.phone}</a>
              {/if}
              {#if member.contact.email}
                <a href="mailto:{member.contact.email}" class="contact-item">{member.contact.email}</a>
              {/if}
              {#if member.contact.note}
                <span class="contact-item contact-note muted">{member.contact.note}</span>
              {/if}
            </div>
          {/if}
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
    margin-bottom: 0.75rem;
    font-size: 0.85rem;
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

  .member-main {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .member-contact {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1rem;
    font-size: 0.85rem;
  }

  .contact-item {
    color: var(--color-text);
    text-decoration: none;
  }

  a.contact-item:hover {
    color: var(--color-primary);
    text-decoration: underline;
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
</style>
