<script>
  /**
   * Unlock panel (CONTEXT.md "Unlock panel", docs/adr/040): meets a new
   * member on their first workspace visits with what membership just made
   * visible — members-only charters, proposals and their vote, the member
   * list. A panel, not a wizard: it never blocks the tabs, and it says
   * nothing about documents the viewer wasn't already allowed to read.
   *
   * Shown only to active members with role exactly 'member' (admins get
   * the setup checklist instead; followers never joined anything) whose
   * joined_at falls within the last 30 days, and only until dismissed for
   * this patch. Mounted once in PatchShell, above the tab content.
   *
   * The workspace node payload carries no joined_at, so this reads the
   * viewer's own memberships store instead (lib/stores/memberships) and
   * refreshes it on mount — a join made moments ago elsewhere in this same
   * session should not have to wait for a reload to unlock this panel.
   */
  import { getContext } from 'svelte';
  import { X } from 'phosphor-svelte';
  import { navigate } from '../stores/router.svelte.js';
  import { isAuthChecked, isLoggedIn, getUser } from '../stores/auth.svelte.js';
  import { loadMemberships, getMemberships, isMembershipsLoaded } from '../stores/memberships.svelte.js';
  import { isUnlockPanelDismissed, dismissUnlockPanel } from '../lib/onboarding.js';

  const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

  let lastSlug = '';
  $effect(() => {
    if (slug && slug !== lastSlug && isLoggedIn()) {
      lastSlug = slug;
      loadMemberships();
    }
  });

  let membership = $derived(getMemberships().find((m) => m.node_slug === slug));

  let withinWindow = $derived.by(() => {
    if (!membership?.joined_at) return false;
    const ms = Date.now() - new Date(membership.joined_at).getTime();
    return ms >= 0 && ms <= THIRTY_DAYS_MS;
  });

  let dismissed = $state(false);
  $effect(() => {
    dismissed = isUnlockPanelDismissed(getUser()?.id, node?.id);
  });

  let visible = $derived(
    !!node &&
    isAuthChecked() &&
    isMembershipsLoaded() &&
    membership?.role === 'member' &&
    withinWindow &&
    !dismissed
  );

  function dismiss() {
    dismissed = true;
    dismissUnlockPanel(getUser()?.id, node?.id);
  }

  function goTo(path) {
    return (e) => {
      e.preventDefault();
      navigate(path);
    };
  }
</script>

{#if visible}
  <div class="unlock-panel">
    <button class="unlock-dismiss" onclick={dismiss} aria-label="Dismiss">
      <X size={14} weight="bold" />
    </button>
    <p class="unlock-heading">You're a member of {node.name}.</p>
    <ul class="unlock-list">
      <li>
        <a href="/patches/{slug}/governance/docs" onclick={goTo(`/patches/${slug}/governance/docs`)}>Members-only charters</a>
        are now readable.
      </li>
      <li>
        <a href="/patches/{slug}/governance/proposals" onclick={goTo(`/patches/${slug}/governance/proposals`)}>Proposals</a>
        are open to you, including your vote.
      </li>
      <li>
        You're on the <a href="/patches/{slug}/members" onclick={goTo(`/patches/${slug}/members`)}>member list</a>.
      </li>
    </ul>
  </div>
{/if}

<style>
  .unlock-panel {
    position: relative;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    padding: 0.9rem 2.25rem 0.9rem 1rem;
    /* Full width of the hosting pane — the Overview container owns the
       gutters now (the old var(--pw-gutter) margins date from shell-level
       mounting and double-inset the panel here). */
    margin: 0 0 1.25rem;
  }

  .unlock-dismiss {
    position: absolute;
    top: 8px;
    right: 8px;
    width: 26px;
    height: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
    border-radius: var(--radius);
    transition: background 100ms ease, color 100ms ease;
  }

  .unlock-dismiss:hover {
    background: var(--color-overlay);
    color: var(--color-text);
  }

  .unlock-heading {
    font-size: 0.92rem;
    font-weight: 700;
    margin: 0 0 0.4rem;
  }

  .unlock-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    line-height: 1.5;
  }

  .unlock-list a {
    color: var(--color-primary);
    text-decoration: none;
    font-weight: 500;
  }

  .unlock-list a:hover {
    text-decoration: underline;
  }
</style>
