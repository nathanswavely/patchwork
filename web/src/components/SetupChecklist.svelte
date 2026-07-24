<script>
  /**
   * Setup checklist (CONTEXT.md "Setup checklist", docs/adr/040): the
   * derived-state list a patch admin sees until their patch has its
   * footing. State, not stored progress — every item (except the two noted
   * below) is computed from the patch payload or a cheap existing
   * endpoint, so it can never lie about or nag over already-done work.
   * Mounted once in PatchShell, above the tab content, admins only.
   *
   * Two items have no derivable state and fall back to a per-user,
   * per-patch localStorage flag (lib/onboarding.js): "Share your patch"
   * (clicking Copy Link is the only signal there is), and the optional
   * "Decide how you govern" item, which also derives from real state (any
   * published charter beyond the lining) but falls back to "the admin has
   * visited the governance hub" (set from GovernanceOverview.svelte) so a
   * patch that never amends anything doesn't nag forever.
   *
   * Never blocks anything; collapses entirely once every item is done.
   */
  import { getContext } from 'svelte';
  import { Check, Circle } from 'phosphor-svelte';
  import { api } from '../lib/api.js';
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { hasMapLocation } from '../lib/mapLocation.js';
  import {
    isSetupChecklistDismissed,
    dismissSetupChecklist,
    isPatchLinkShared,
    markPatchLinkShared,
    isGovernanceHubVisited,
  } from '../lib/onboarding.js';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);
  // Patch-role admins only — deliberately NOT the workspace's isAdmin,
  // which includes the instance-admin override. An instance admin passing
  // through another patch's workspace has no setup to do there
  // (CONTEXT.md "Setup checklist": the panel is for a patch's own admins).
  let isAdmin = $derived(patch.value.membershipRole === 'admin');
  let isUnclaimed = $derived(patch.value.isUnclaimed);

  let hasEvent = $state(false);
  let hasCharterBeyondLining = $state(false);
  let checksLoaded = $state(false);

  let lastCheckedSlug = '';
  $effect(() => {
    if (slug && isAdmin && !isUnclaimed && slug !== lastCheckedSlug) {
      lastCheckedSlug = slug;
      loadChecks();
    }
  });

  async function loadChecks() {
    checksLoaded = false;
    const [eventData, docData] = await Promise.all([
      api(`events?node_slug=${encodeURIComponent(slug)}&limit=1`).catch(() => ({ items: [] })),
      api(`nodes/${slug}/governance`).catch(() => ({ items: [] })),
    ]);
    const events = eventData.items || eventData || [];
    const docs = docData.items || docData || [];
    hasEvent = events.length > 0;
    hasCharterBeyondLining = docs.some((d) => d.kind !== 'lining');
    checksLoaded = true;
  }

  let userId = $derived(getUser()?.id);

  let dismissed = $state(false);
  $effect(() => {
    dismissed = isSetupChecklistDismissed(userId, node?.id);
  });

  let shared = $state(false);
  $effect(() => {
    shared = isPatchLinkShared(userId, node?.id);
  });

  // Items, in display order. `done` and `optional`/`skippable` drive
  // rendering below; each carries its own navigation target.
  let items = $derived.by(() => {
    if (!node) return [];
    // Re-read on every route change (getPath()) — visiting the governance
    // hub sets a localStorage flag that isn't itself reactive, so this is
    // what notices the admin came back from there without a full reload.
    getPath();
    const hasTile = node.appearance != null;
    const hasTags = Array.isArray(node.tags) && node.tags.length > 0;
    const hasWhereabouts = !!(node.address && node.address.trim()) || hasMapLocation(node.latitude, node.longitude);
    const governanceDecided = hasCharterBeyondLining || isGovernanceHubVisited(userId, node.id);

    return [
      {
        id: 'tile',
        label: 'Design your tile',
        done: hasTile,
        href: `/patches/${slug}/settings/appearance`,
      },
      {
        id: 'tags',
        label: 'Tag your patch',
        done: hasTags,
        href: `/patches/${slug}/settings/info`,
      },
      {
        id: 'whereabouts',
        label: 'Say where you are',
        hint: 'not every patch is a place',
        done: hasWhereabouts,
        skippable: true,
        href: `/patches/${slug}/settings/info`,
      },
      {
        id: 'event',
        label: 'Post your first event',
        done: hasEvent,
        href: `/patches/${slug}/events`,
      },
      {
        id: 'share',
        label: 'Share your patch',
        done: shared,
        action: 'share',
      },
      {
        id: 'governance',
        label: 'Decide how you govern',
        hint: 'a band never needs this',
        done: governanceDecided,
        skippable: true,
        href: `/patches/${slug}/governance`,
      },
    ];
  });

  let allDone = $derived(checksLoaded && items.length > 0 && items.every((i) => i.done));
  let visible = $derived(isAdmin && !isUnclaimed && !!node && checksLoaded && !dismissed && !allDone);

  function dismiss() {
    dismissed = true;
    dismissSetupChecklist(userId, node?.id);
  }

  function goTo(href) {
    return (e) => {
      e.preventDefault();
      navigate(href);
    };
  }

  async function shareLink() {
    const url = `${location.origin}/patches/${slug}`;
    try {
      await navigator.clipboard.writeText(url);
      showToast('Copied');
    } catch {
      showToast('Copy failed — select the address instead', 'error');
      return;
    }
    shared = true;
    markPatchLinkShared(userId, node?.id);
  }
</script>

{#if visible}
  <div class="setup-checklist">
    <div class="checklist-header">
      <h2 class="checklist-title">Getting set up</h2>
      <button class="checklist-dismiss" onclick={dismiss} aria-label="Dismiss">&times;</button>
    </div>
    <ul class="checklist-items">
      {#each items as item (item.id)}
        <li class="checklist-item" class:done={item.done}>
          <span class="checklist-icon">
            {#if item.done}
              <Check size={14} weight="bold" />
            {:else}
              <Circle size={14} />
            {/if}
          </span>
          {#if item.action === 'share'}
            <button class="checklist-label checklist-action" onclick={shareLink}>
              {item.label}
            </button>
          {:else}
            <a class="checklist-label" href={item.href} onclick={goTo(item.href)}>{item.label}</a>
          {/if}
          {#if item.hint}
            <span class="checklist-hint">({item.hint})</span>
          {/if}
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .setup-checklist {
    position: relative;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    padding: 0.9rem 1rem;
    margin: 1rem var(--pw-gutter) 0;
  }

  .checklist-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }

  .checklist-title {
    font-size: 0.92rem;
    font-weight: 700;
    margin: 0;
  }

  .checklist-dismiss {
    border: none;
    background: none;
    color: var(--color-text-muted);
    font-size: 1.1rem;
    line-height: 1;
    cursor: pointer;
    padding: 4px;
    opacity: 0.7;
  }

  .checklist-dismiss:hover {
    opacity: 1;
  }

  .checklist-items {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .checklist-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
  }

  .checklist-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    border-radius: 999px;
    color: var(--color-text-muted);
  }

  .checklist-item.done .checklist-icon {
    color: var(--color-success);
  }

  .checklist-label {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--color-text);
    text-decoration: none;
    cursor: pointer;
    text-align: left;
  }

  .checklist-label:hover {
    text-decoration: underline;
  }

  .checklist-item.done .checklist-label {
    color: var(--color-text-muted);
    text-decoration: line-through;
  }

  .checklist-hint {
    font-size: 0.78rem;
    color: var(--color-text-muted);
  }
</style>
