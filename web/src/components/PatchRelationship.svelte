<script>
  /**
   * The relationship row (CONTEXT.md "Relationship row", docs/adr/042): the
   * one thing on a patch's public face that is a control rather than a
   * glimpse. It states where the viewer stands and offers the next rung,
   * and holds nothing else — navigation, contribution, and moderation all
   * live elsewhere on the page.
   *
   * At most two controls in any state: a standing control whose menu holds
   * the exit, and the next rung if there is one. Mounted by both the patch
   * profile and the workspace cluster, because the two surfaces each grew
   * their own copy of this logic and had already drifted apart.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import JoinSheet from './JoinSheet.svelte';
  import { CaretDown, Heart, UsersThree, Wrench } from 'phosphor-svelte';

  let {
    slug = '',
    node = null,
    isUnclaimed = false,
    isBanned = false,
    membershipRole = '',
    liningStatus = '',
    onChanged = () => {},
    size = 'md',
  } = $props();

  let joining = $state(false);
  let joinSheetOpen = $state(false);
  let menuOpen = $state(false);

  /**
   * Standing reads from the membership row and never from is_admin: an
   * instance admin can manage any patch without standing in it (nodes.go),
   * and this row states relationship, not permission. The role marks match
   * CONTEXT.md — heart / three users / wrench.
   */
  const STANDING = {
    follower: { label: 'Following', icon: Heart, exit: 'Unfollow' },
    member: { label: 'Member', icon: UsersThree, exit: 'Leave' },
    admin: { label: 'Admin', icon: Wrench, exit: 'Leave' },
  };
  let standing = $derived(STANDING[membershipRole] || null);

  // The rung renders only where it can succeed (docs/adr/042). Unclaimed
  // patches take followers only, and invite_only rejects the request
  // outright (memberships.go) — an absent door beats a 403 at the end of a
  // ceremony.
  let canBecomeMember = $derived(
    !isUnclaimed &&
    node?.membership_policy !== 'invite_only' &&
    membershipRole !== 'member' &&
    membershipRole !== 'admin'
  );

  let btnSize = $derived(size === 'sm' ? 'btn-sm' : '');

  function handleWindowClick(e) {
    if (menuOpen && !e.target.closest('.standing-container')) menuOpen = false;
  }

  function openJoinSheet() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    joinSheetOpen = true;
  }

  async function handleJoin(message) {
    joining = true;
    try {
      const result = await api(`nodes/${slug}/join`, { method: 'POST', body: message ? { message } : undefined });
      await onChanged();
      // One wording for one event — the profile and the shell used to
      // disagree here ("Now a member" vs "You are now a member").
      showToast(result.status === 'pending' ? 'Membership request sent' : 'You are now a member', 'success');
    } catch (e) {
      showToast(e.message || 'Could not join', 'error');
    } finally {
      joining = false;
      joinSheetOpen = false;
    }
  }

  async function handleFollow() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    joining = true;
    try {
      await api(`nodes/${slug}/join`, { method: 'POST', body: { role: 'follower' } });
      await onChanged();
      showToast('Following patch', 'success');
    } catch (e) {
      showToast(e.message || 'Could not follow', 'error');
    } finally {
      joining = false;
    }
  }

  // The exit lives in the standing menu, so it costs one deliberate step.
  // Leaving as a patch's only admin is refused by the server (409) — the
  // message it returns is the one worth showing.
  async function handleLeave() {
    const wasFollower = membershipRole === 'follower';
    menuOpen = false;
    joining = true;
    try {
      await api(`nodes/${slug}/leave`, { method: 'POST' });
      await onChanged();
      showToast(wasFollower ? 'Unfollowed patch' : 'Left patch', 'info');
    } catch (e) {
      showToast(e.message || 'Could not leave', 'error');
    } finally {
      joining = false;
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<div class="relationship-row">
  {#if isBanned}
    <span class="banned-notice">Removed from this community</span>
  {:else}
    {#if standing}
      {@const Icon = standing.icon}
      <div class="standing-container">
        <button
          class="standing"
          class:sm={size === 'sm'}
          onclick={() => { menuOpen = !menuOpen; }}
          disabled={joining}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
        >
          <!-- The role mark is decoration beside the word; phosphor renders
               role="img" with no name, which leaves the button unnamed to a
               screen reader unless the icons are hidden outright. -->
          <span class="standing-mark" aria-hidden="true"><Icon size={size === 'sm' ? 13 : 15} weight="duotone" /></span>
          <span>{standing.label}</span>
          <span class="standing-mark" aria-hidden="true"><CaretDown size={11} weight="bold" /></span>
        </button>
        {#if menuOpen}
          <div class="standing-menu" role="menu">
            <button role="menuitem" onclick={handleLeave} disabled={joining}>{standing.exit}</button>
          </div>
        {/if}
      </div>
    {:else}
      <button class="btn btn-primary {btnSize}" onclick={handleFollow} disabled={joining}>Follow</button>
    {/if}

    {#if canBecomeMember}
      <button
        class="btn {standing ? 'btn-primary' : 'btn-secondary'} {btnSize}"
        onclick={openJoinSheet}
        disabled={joining}
      >Become a member</button>
    {/if}
  {/if}
</div>

<JoinSheet
  open={joinSheetOpen}
  onClose={() => { joinSheetOpen = false; }}
  onConfirm={handleJoin}
  slug={slug}
  patchName={node?.name || ''}
  membershipPolicy={node?.membership_policy || 'open'}
  liningStatus={liningStatus}
  submitting={joining}
/>

<style>
  .relationship-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .banned-notice {
    font-size: 0.85rem;
    color: var(--color-error);
    font-weight: 500;
  }

  /* --- Standing control: the resting form of where you stand --- */
  .standing-container {
    position: relative;
  }

  .standing {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.45rem 0.7rem;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-text);
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: background 120ms ease;
  }

  .standing-mark {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
  }

  .standing.sm {
    padding: 0.3rem 0.55rem;
    font-size: 0.8rem;
  }

  .standing:hover {
    background: var(--color-border);
  }

  .standing-menu {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    min-width: 100%;
    z-index: 60;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
    padding: 0.25rem;
  }

  .standing-menu button {
    display: block;
    width: 100%;
    padding: 0.4rem 0.6rem;
    font-size: 0.85rem;
    text-align: left;
    white-space: nowrap;
    color: var(--color-text);
    background: none;
    border: none;
    border-radius: calc(var(--radius) - 2px);
    cursor: pointer;
  }

  .standing-menu button:hover {
    background: var(--color-overlay);
  }
</style>
