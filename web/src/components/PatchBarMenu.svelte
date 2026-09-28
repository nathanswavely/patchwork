<script>
  /**
   * A patch's workspace in the global bar, on a phone.
   *
   * Wide screens keep the context crumb, the public-profile eye and the
   * relationship cluster on its own row under the bar. On a phone those were
   * three rows of chrome with the patch's name cut to eight letters and a
   * whole row holding one button. Here the name is the menu: tapping it opens
   * a sheet with the viewer's standing and what it lets them do, the public
   * profile, posting an event, subscribing and reporting. The bar keeps the
   * way up (a section list, or the quilt) and nothing else of the patch's.
   * A visitor's next rung is in the sheet too: "Become a member" left the
   * name 0px wide beside it (ADR 042 keeps that word), a strip of its own
   * outweighed the page, and the public profile, where visitors arrive,
   * already offers it in full.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { isLoggedIn, isAdmin as isInstanceAdmin } from '../stores/auth.svelte.js';
  import { getInstanceName, getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import { eventPostingRight } from '../lib/patchWorkspace.js';
  import { identityColorForPatch } from '../lib/quiltTheme.js';
  import PatchRelationship from './PatchRelationship.svelte';
  import Modal from './Modal.svelte';
  import ReportButton from './ReportButton.svelte';
  import SubscribeFeeds from './SubscribeFeeds.svelte';
  import { CaretLeft, CaretDown, Eye, Plus, CalendarBlank, Flag } from 'phosphor-svelte';

  let {
    slug = '',
    node = null,
    isAdmin = false,
    isUnclaimed = false,
    isBanned = false,
    viewerTrusted = false,
    membershipRole = '',
    requestPending = false,
    invited = false,
    liningStatus = '',
    upLink = null,
    onChanged = () => {},
  } = $props();

  let sheetOpen = $state(false);
  let subscribeOpen = $state(false);
  let reportOpen = $state(false);

  let tileColor = $derived(node?.id ? identityColorForPatch(node) : 'var(--color-border)');
  let back = $derived(upLink || { href: '/', label: getInstanceName() });

  let postingRight = $derived(eventPostingRight({
    signedIn: isLoggedIn(),
    isInstanceAdmin: isInstanceAdmin(),
    viewerTrusted: viewerTrusted === true,
    isUnclaimed,
    isMemberOrAdmin: membershipRole === 'member' || membershipRole === 'admin',
    isBanned,
    submissionsEnabled: getSubmissionsEnabled(),
    acceptSuggestions: node?.accept_event_suggestions === true,
    hasMoved: !!node?.moved_to,
  }));
  let feedAvailable = $derived(node?.visibility === 'public');
  let canReport = $derived(isLoggedIn() && !isAdmin && !!node?.id);

  // Moving on closes the sheet, as it closes every other menu in the bar.
  let path = $derived(getPath());
  $effect(() => {
    void path;
    sheetOpen = false;
  });

  function go(e, href) {
    e.preventDefault();
    sheetOpen = false;
    navigate(href);
  }

  function onKeydown(e) {
    if (sheetOpen && e.key === 'Escape') sheetOpen = false;
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="patch-bar">
  <a
    href={back.href}
    class="patch-bar-back"
    aria-label="Back to {back.label}"
    title="Back to {back.label}"
    onclick={(e) => go(e, back.href)}
  >
    <CaretLeft size={18} weight="bold" />
  </a>

  <button
    type="button"
    class="patch-bar-name"
    aria-haspopup="dialog"
    aria-expanded={sheetOpen}
    onclick={() => { sheetOpen = !sheetOpen; }}
  >
    <span class="patch-bar-tile" style="background: {tileColor}" aria-hidden="true"></span>
    <span class="patch-bar-label">{node?.name || slug}</span>
    <span class="patch-bar-caret" aria-hidden="true"><CaretDown size={12} weight="bold" /></span>
  </button>

</div>

{#if sheetOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="patch-sheet-backdrop" onclick={() => { sheetOpen = false; }}></div>
  <div class="patch-sheet" role="dialog" aria-label={node?.name || slug}>
    <div class="patch-sheet-grip" aria-hidden="true"></div>
    <div class="patch-sheet-head">
      <span class="patch-sheet-tile" style="background: {tileColor}" aria-hidden="true"></span>
      <span class="patch-sheet-name">{node?.name || slug}</span>
    </div>
    <div class="patch-sheet-standing">
      <PatchRelationship
        {slug}
        {node}
        {isUnclaimed}
        {isBanned}
        {membershipRole}
        {requestPending}
        {invited}
        {liningStatus}
        {onChanged}
        size="sm"
      />
    </div>
    <nav class="patch-sheet-rows">
      <a href="/patches/{slug}" class="patch-sheet-row" onclick={(e) => go(e, `/patches/${slug}`)}>
        <Eye size={18} weight="duotone" />
        View the public profile
      </a>
      {#if postingRight !== 'none'}
        <a href="/events/new?node={slug}" class="patch-sheet-row" onclick={(e) => go(e, `/events/new?node=${slug}`)}>
          <Plus size={18} weight="bold" />
          {postingRight === 'direct' ? 'New event' : 'Suggest an event'}
        </a>
      {/if}
      {#if feedAvailable}
        <button type="button" class="patch-sheet-row" onclick={() => { sheetOpen = false; subscribeOpen = true; }}>
          <CalendarBlank size={18} weight="duotone" />
          Subscribe
        </button>
      {/if}
      {#if canReport}
        <button type="button" class="patch-sheet-row" onclick={() => { sheetOpen = false; reportOpen = true; }}>
          <Flag size={18} weight="duotone" />
          Report
        </button>
      {/if}
    </nav>
  </div>
{/if}

<!-- Outside the sheet: opening either closes the sheet, and a modal
     rendered inside it would be destroyed with it. -->
{#if canReport}
  <ReportButton
    entityType="node"
    entityId={node.id}
    entityName={node.name}
    variant="headless"
    bind:open={reportOpen}
  />
{/if}

<Modal open={subscribeOpen} label="Subscribe to {node?.name || 'this patch'}" onClose={() => { subscribeOpen = false; }}>
  {#snippet children()}
    <h2 class="subscribe-title">Subscribe</h2>
    <SubscribeFeeds {slug} />
  {/snippet}
</Modal>

<style>
  .patch-bar {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 1;
    min-width: 0;
  }

  .patch-bar-back {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    margin-left: -8px;
    flex-shrink: 0;
    color: var(--color-text-muted);
    border-radius: var(--radius);
    text-decoration: none;
  }

  .patch-bar-back:hover {
    color: var(--color-text);
    background: var(--color-overlay);
    text-decoration: none;
  }

  .patch-bar-name {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    min-width: 0;
    min-height: 40px;
    padding: 0 6px;
    border: none;
    border-radius: var(--radius);
    background: none;
    color: var(--color-text);
    text-align: left;
    cursor: pointer;
  }

  .patch-bar-name:hover,
  .patch-bar-name[aria-expanded='true'] {
    background: var(--color-overlay);
  }

  .patch-bar-tile,
  .patch-sheet-tile {
    width: 20px;
    height: 20px;
    flex-shrink: 0;
    border-radius: 4px;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.12);
  }

  .patch-bar-label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-display);
    font-size: 1rem;
    font-weight: 600;
  }

  .patch-bar-caret {
    display: flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .patch-sheet-backdrop {
    position: fixed;
    inset: 0;
    z-index: 210;
    background: var(--color-scrim);
  }

  .patch-sheet {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 211;
    max-height: 80vh;
    overflow-y: auto;
    padding: 8px 0 calc(12px + env(safe-area-inset-bottom, 0px));
    background: var(--color-surface);
    border-radius: 14px 14px 0 0;
    box-shadow: 0 -8px 24px var(--color-shadow);
    animation: sheet-up 180ms ease;
  }

  @keyframes sheet-up {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .patch-sheet {
      animation: none;
    }
  }

  .patch-sheet-grip {
    width: 36px;
    height: 4px;
    margin: 0 auto 10px;
    border-radius: 2px;
    background: var(--color-border);
  }

  .patch-sheet-head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 var(--pw-gutter) 10px;
  }

  .patch-sheet-tile {
    width: 32px;
    height: 32px;
    border-radius: 6px;
  }

  .patch-sheet-name {
    font-family: var(--font-display);
    font-size: 1.15rem;
    font-weight: 600;
    line-height: 1.25;
  }

  .patch-sheet-standing {
    padding: 0 var(--pw-gutter) 12px;
  }

  .patch-sheet-rows {
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--color-border);
  }

  .patch-sheet-row {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 52px;
    padding: 0 var(--pw-gutter);
    border: none;
    border-bottom: 1px solid var(--color-border);
    background: none;
    color: var(--color-text);
    font-size: 0.95rem;
    text-align: left;
    text-decoration: none;
    cursor: pointer;
  }

  .patch-sheet-row :global(svg) {
    color: var(--color-text-muted);
    flex-shrink: 0;
  }

  .patch-sheet-row:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .subscribe-title {
    font-size: 1.1rem;
    margin-bottom: 0.75rem;
  }
</style>
