<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isMember = $derived(patch.value.isMember);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);
  let followerPermissions = $derived(patch.value.followerPermissions);
  let permissionDenied = $derived(membershipRole === 'follower' && followerPermissions?.members === false);

  let members = $state([]);
  let loading = $state(true);

  $effect(() => {
    if (slug) loadMembers();
  });

  async function loadMembers() {
    loading = true;
    try {
      const data = await api(`nodes/${slug}/members`);
      members = data.items || data || [];
    } catch {
      members = [];
    } finally {
      loading = false;
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
      <span class="muted">{members.length} members</span>
    </div>
    <ul class="member-list">
      {#each members as member (member.user_id)}
        <li class="member-row">
          <a
            href="/users/{member.username}"
            class="member-name"
            onclick={(e) => { e.preventDefault(); navigate(`/users/${member.username}`); }}
          >
            {member.display_name || member.username}
          </a>
          <span class="badge">{member.role}</span>
        </li>
      {/each}
    </ul>
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
    justify-content: space-between;
    align-items: center;
    padding: 0.6rem 0;
    border-bottom: 1px solid var(--color-border);
    font-size: 0.9rem;
  }

  .member-row:last-child {
    border-bottom: none;
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
