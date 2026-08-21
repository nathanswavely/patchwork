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
   * - Report — signed-in visitors who don't run the patch.
   */
  import { DotsThree } from 'phosphor-svelte';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import Modal from './Modal.svelte';
  import ReportButton from './ReportButton.svelte';
  import SubscribeFeeds from './SubscribeFeeds.svelte';

  let {
    slug = '',
    node = null,
    isAdmin = false,
    isUnclaimed = false,
    hasStanding = false,
  } = $props();

  let menuOpen = $state(false);
  let subscribeOpen = $state(false);
  let reportOpen = $state(false);

  // Subscribable feeds exist only for public patches (docs/adr/031).
  let feedAvailable = $derived(node?.visibility === 'public');
  // Unclaimed patches have no governance, so their workspace root is the
  // events calendar (docs/adr/039).
  let workspaceHref = $derived(`/patches/${slug}/${isUnclaimed ? 'events' : 'governance'}`);
  let canEnterWorkspace = $derived(hasStanding || isAdmin);

  let canReport = $derived(isLoggedIn() && !isAdmin && !!node?.id);

  function handleWindowClick(e) {
    if (menuOpen && !e.target.closest('.overflow-container')) menuOpen = false;
  }

</script>

<svelte:window onclick={handleWindowClick} />

{#if feedAvailable || canEnterWorkspace || canReport}
  <div class="overflow-container">
    <button
      class="overflow-trigger"
      onclick={() => { menuOpen = !menuOpen; }}
      aria-haspopup="menu"
      aria-expanded={menuOpen}
      aria-label="More actions"
      title="More actions"
    >
      <DotsThree size={20} weight="bold" />
    </button>

    {#if menuOpen}
      <div class="overflow-menu" role="menu">
        {#if feedAvailable}
          <button role="menuitem" onclick={() => { menuOpen = false; subscribeOpen = true; }}>Subscribe</button>
        {/if}
        {#if canEnterWorkspace}
          <a
            role="menuitem"
            href={workspaceHref}
            onclick={(e) => { e.preventDefault(); menuOpen = false; navigate(workspaceHref); }}
          >Workspace view</a>
        {/if}
        {#if canReport}
          <button role="menuitem" onclick={() => { menuOpen = false; reportOpen = true; }}>Report</button>
        {/if}
      </div>
    {/if}
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
  .overflow-container {
    position: relative;
    display: inline-flex;
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
  }

  .overflow-trigger:hover {
    color: var(--color-text);
    background: var(--color-overlay);
  }

  .overflow-menu {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 60;
    min-width: 10rem;
    padding: 0.25rem;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
  }

  .overflow-menu > button,
  .overflow-menu > a {
    display: block;
    width: 100%;
    padding: 0.4rem 0.6rem;
    font-size: 0.85rem;
    font-weight: 500;
    text-align: left;
    white-space: nowrap;
    color: var(--color-text);
    background: none;
    border: none;
    border-radius: calc(var(--radius) - 2px);
    text-decoration: none;
    cursor: pointer;
  }

  .overflow-menu > button:hover,
  .overflow-menu > a:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .subscribe-title {
    font-size: 1.15rem;
    font-weight: 700;
    margin-bottom: 0.75rem;
    padding-right: 1.5rem;
  }

</style>
