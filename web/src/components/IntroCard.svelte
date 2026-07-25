<script>
  /**
   * Intro card (CONTEXT.md "Intro card", docs/adr/040): the non-blocking
   * card an anonymous visitor meets on their first landing on any public
   * surface. Mounted once in SocialShell so it follows every discovery
   * route; this component decides per-route whether to show its full or
   * compact form, whether to show at all (signed-in people never see it),
   * and whether it has already been dismissed for good.
   *
   * Corner-overlaid, never blocking — no backdrop, nothing under it is
   * inert. Dismissed once, gone forever (lib/introCard.js); the sidebar's
   * "What is Patchwork?" entry remains as the standing path to the About
   * page after this card is gone.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, isAuthChecked } from '../stores/auth.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import { isIntroDismissed, dismissIntro } from '../lib/introCard.js';

  let { routeName = 'home' } = $props();

  // Full form on the canvas views (home quilt and map) — the first thing a
  // cold landing sees. Every other public surface (a deep-linked event, a
  // patch profile, the events list) gets the compact one-liner so the card
  // never competes with the content someone actually came to see.
  const FULL_ROUTES = new Set(['home', 'map']);
  // No card where the card's own destinations render — pointing someone at
  // the page they're already reading is noise, not orientation.
  const SUPPRESSED_ROUTES = new Set(['about', 'lining', 'label', 'legalDoc']);
  let variant = $derived(FULL_ROUTES.has(routeName) ? 'full' : 'compact');

  let dismissed = $state(isIntroDismissed());
  let instanceName = $derived(getInstanceName());
  let visible = $derived(
    isAuthChecked() && !isLoggedIn() && !dismissed &&
    !SUPPRESSED_ROUTES.has(routeName)
  );

  function dismiss() {
    dismissed = true;
    dismissIntro();
  }

  function goAbout(e) {
    e.preventDefault();
    navigate('/about');
  }

  function goJoin(e) {
    e.preventDefault();
    navigate('/login');
  }
</script>

{#if visible}
  <div class="intro-card" class:compact={variant === 'compact'}>
    {#if variant === 'full'}
      <h2 class="intro-heading">{instanceName} is a quilt of the communities around you.</h2>
      <p class="intro-body">
        Every tile is a real group, placed near the groups it shares
        people with. No ads, no algorithm. Run by people here.
      </p>
      <div class="intro-actions">
        <a href="/about" class="intro-about" onclick={goAbout}>What is Patchwork?</a>
        <a href="/login" class="btn btn-primary intro-join" onclick={goJoin}>Join</a>
      </div>
      <!-- The full card's only dismissal, and deliberately a worded one: a
           × closes a box, this answers the card's actual question — you can
           read this quilt without an account. Leaving Join as the only named
           way forward implied otherwise. Dismisses for good, as the × did. -->
      <button class="intro-lurk" onclick={dismiss}>I'll lurk for now</button>
    {:else}
      <span class="intro-line">{instanceName} is a quilt of the communities around you.</span>
      <a href="/about" class="intro-about" onclick={goAbout}>What is Patchwork?</a>
      <button class="intro-dismiss" onclick={dismiss} aria-label="Dismiss">&times;</button>
    {/if}
  </div>
{/if}

<style>
  /* Overlay, never a modal — no backdrop, nothing beneath it is inert.
     Sits below the global bar (z-index 60) and above the sidebar rail
     (55), matching the label/filter sheets' layer. Desktop placement is
     per-variant: the full card sits bottom-left on the canvas views,
     clear of the results pane, the Quilt/Map toggle, and the bottom-right
     corner (Label overlay, FABs, toasts); the compact strip centers just
     under the global bar. It may overlap dismissible content, never a
     control. */
  .intro-card {
    position: fixed;
    z-index: 56;
    max-width: 320px;
    padding: 16px 18px;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 8px 24px var(--color-shadow);
  }

  .intro-card:not(.compact) {
    left: 16px;
    bottom: 16px;
  }

  /* Compact only. The one-line strip has no room for a worded decline, so
     it keeps the × as its dismissal; the full card's is `.intro-lurk`. */
  .intro-card.compact .intro-dismiss {
    position: absolute;
    top: 50%;
    right: 8px;
    transform: translateY(-50%);
    border: none;
    background: none;
    color: var(--color-text-muted);
    font-size: 1.1rem;
    line-height: 1;
    cursor: pointer;
    padding: 4px;
    opacity: 0.7;
  }

  .intro-card.compact .intro-dismiss:hover {
    opacity: 1;
  }

  .intro-heading {
    margin: 0 0 8px;
    font-size: 1rem;
    font-weight: 700;
  }

  .intro-body {
    margin: 0 0 14px;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    line-height: 1.5;
  }

  .intro-actions {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .intro-about {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .intro-about:hover {
    text-decoration: underline;
  }

  .intro-join {
    font-size: 0.85rem;
  }

  /* Quieter than both actions above it — declining should be easy to find
     and never compete with them for attention. Still the full card's only
     way out, so it can't recede any further than this. */
  .intro-lurk {
    display: block;
    margin: 10px 0 0;
    padding: 0;
    border: none;
    background: none;
    font-size: 0.8rem;
    color: var(--color-text-muted);
    cursor: pointer;
    text-align: left;
  }

  .intro-lurk:hover {
    color: var(--color-text);
    text-decoration: underline;
  }

  /* Compact: one line, wraps to fit content instead of the full card's
     fixed measure — deep links never compete with the card for space.
     Centered just under the global bar on desktop. */
  .intro-card.compact {
    display: flex;
    align-items: center;
    gap: 10px;
    max-width: 92vw;
    padding: 10px 34px 10px 14px;
    top: calc(56px + 12px);
    left: 50%;
    transform: translateX(-50%);
  }

  .intro-card.compact .intro-line {
    font-size: 0.82rem;
    font-weight: 600;
  }

  .intro-card.compact .intro-about {
    font-size: 0.82rem;
    white-space: nowrap;
  }

  /* Mobile keeps the full-width strip under the bar for both variants.
     Both selectors carry two classes so they outrank the desktop
     `.intro-card:not(.compact)` placement — a one-class override loses to
     it and leaves the full card pinned top *and* bottom, stretched to the
     whole viewport instead of hugging its text. */
  @media (max-width: 640px) {
    .intro-card:not(.compact),
    .intro-card.compact {
      left: 12px;
      right: 12px;
      top: calc(56px + 8px);
      bottom: auto;
      transform: none;
      max-width: none;
    }
  }
</style>
