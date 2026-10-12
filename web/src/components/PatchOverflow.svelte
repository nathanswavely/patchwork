<script>
  /**
   * The patch profile's overflow (docs/adr/042): the acts that are real but
   * rare, kept out of the relationship row and off the glimpses.
   *
   * - Subscribe — the per-patch ICS/RSS feeds, which until now were
   *   reachable only from inside the workspace's Events tab, hiding a
   *   public feature from the public page.
   * - Workspace view — the one place the UI says "workspace" out loud, and
   *   the one door that names a container rather than a room. Redundant by
   *   construction (a glimpse renders whenever its viewer may enter it), so
   *   it is a fallback, not the way in.
   * - View as a visitor — the patch's own admins, checking what they are
   *   publishing. The only way to answer that used to be to sign out
   *   (F-126), and one board member did: "signing out is how I lost my
   *   account this afternoon."
   * - Report — signed-in visitors who don't run the patch.
   */
  import { DotsThree } from 'phosphor-svelte';
  import { navigate, getQuery } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { signedInForPatchView } from '../lib/preview.js';
  import Modal from './Modal.svelte';
  import { ltMenu } from '../lib/lt.js';
  import ReportButton from './ReportButton.svelte';
  import SubscribeFeeds from './SubscribeFeeds.svelte';

  let {
    slug = '',
    node = null,
    isAdmin = false,
    isUnclaimed = false,
    hasStanding = false,
  } = $props();

  // Already previewing: the banner's own control is the way back, and a
  // menu item offering to start again would be a dead end.
  let previewing = $derived(getQuery().get('as') === 'visitor');

  let menuEl = $state(null);
  let subscribeOpen = $state(false);
  let reportOpen = $state(false);

  // Subscribable feeds exist only for public patches (docs/adr/031).
  let feedAvailable = $derived(node?.visibility === 'public');
  // Unclaimed patches have no governance, so their workspace root is the
  // events calendar (docs/adr/039).
  let workspaceHref = $derived(`/patches/${slug}/${isUnclaimed ? 'events' : 'governance'}`);
  let canEnterWorkspace = $derived(hasStanding || isAdmin);

  // Reporting is a signed-in act, so it is not on the menu a visitor
  // sees, so it is not on the menu a preview shows (F-126).
  let canReport = $derived(signedInForPatchView(isLoggedIn()) && !isAdmin && !!node?.id);

  // Offered to the people who set what this patch publishes. A member can
  // already see roughly what a visitor sees by having no admin surfaces;
  // an admin cannot, and is the one being asked to decide.
  let canPreview = $derived(isAdmin && !previewing);
  let previewHref = $derived(`/patches/${slug}?as=visitor`);

</script>

{#if feedAvailable || canEnterWorkspace || canReport || canPreview}
  <div class="overflow-container">
    <button
      class="overflow-trigger"
      aria-label="More actions"
      use:ltMenu={{ list: menuEl, align: 'end' }}
      title="More actions"
    >
      <DotsThree size={20} weight="bold" />
    </button>

    <div class="lt-popover lt-menu overflow-menu" bind:this={menuEl}>
        {#if feedAvailable}
          <button class="lt-menu-item" onclick={() => { subscribeOpen = true; }}>Subscribe</button>
        {/if}
        {#if canEnterWorkspace}
          <a
            class="lt-menu-item"
            href={workspaceHref}
            onclick={(e) => { e.preventDefault(); navigate(workspaceHref); }}
          >Workspace view</a>
        {/if}
        {#if canPreview}
          <!-- A real page load, not navigate(): the components that fetch
               this patch key their effects on its slug, which does not
               change when only the query does, so a soft navigation would
               leave half the page holding an admin's answers under a
               visitor's banner. Letting the browser follow the href also
               makes middle-click and open-in-new-tab do the obvious
               thing, which for "show me the visitor view" is worth
               having. -->
          <a class="lt-menu-item" href={previewHref}>View as a visitor</a>
        {/if}
        {#if canReport}
          <button class="lt-menu-item" onclick={() => { reportOpen = true; }}>Report</button>
        {/if}
    </div>
  </div>

  <!-- Mounted outside the menu on purpose: opening the modal closes the
       menu, and a modal rendered inside it would be destroyed with it. -->
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
{/if}

<style>
  /* Liquid Textile's menu (ltMenu): its list is the popover below. */
  .overflow-container {
    position: relative;
    display: inline-flex;
    --lt-z-menu: 60;
  }

  .overflow-trigger {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    color: var(--color-text-muted);
    background: none;
    border: none;
    border-radius: var(--radius);
    cursor: pointer;
    transition: background 150ms ease, color 150ms ease;
    min-width: var(--lt-target);
    min-height: var(--lt-target);
  }

  .overflow-trigger:hover {
    color: var(--color-text);
    background: var(--color-overlay);
  }

  .subscribe-title {
    font-size: 1.15rem;
    font-weight: 700;
    margin-bottom: 0.75rem;
    padding-right: 1.5rem;
  }
</style>
