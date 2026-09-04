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

  // Full form on a discovery surface — the first thing a cold landing sees,
  // and the only form that carries what this quilt actually promises ("no
  // ads, no personalized algorithm") along with Join and a worded decline.
  // A deep link (an event someone was sent, a patch profile) gets the compact
  // one-liner instead, so the card never competes with the content they came
  // for.
  //
  // That was always the intent; the set had drifted from it. The scope
  // variants of the two canvases were missing, and discovery mode and the
  // events list — both cold-landing surfaces in their own right — were
  // getting the deep-link treatment, which is how the surface built for
  // newcomers (docs/adr/075) came to show them the thinnest version of the
  // card.
  const FULL_ROUTES = new Set([
    'home', 'homeMy', 'map', 'mapMy', 'discover', 'eventList', 'eventListMy',
  ]);
  // The canvas views float their own chrome over the foot of the screen on
  // mobile — the Quilt/Map/List pill — so the card has further to rise there.
  const CANVAS_ROUTES = new Set(['home', 'homeMy', 'map', 'mapMy']);
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

  // The card floats over the foot of the screen on mobile, so the shell has
  // to leave room for it or it lands on whatever sits at the bottom of a
  // short page — on discovery mode, its primary button. The contract is that
  // this card may overlap dismissible content and never a control
  // (CONTEXT.md), so publish the measured height and let the shell pad by it.
  // Measured rather than assumed: the heading wraps to two lines or three
  // depending on the quilt's name, and a guessed number would be wrong for
  // every instance but this one.
  let cardEl = $state(null);
  $effect(() => {
    const root = document.body;
    if (!visible || !cardEl) {
      root.style.removeProperty('--intro-card-h');
      return;
    }
    const publish = () => {
      // How much of the foot the card occupies, offset included — not just
      // its own height. The shell pads by this, and padding by the height
      // alone would leave the card's own gap uncovered.
      const fromBottom = window.innerHeight - cardEl.getBoundingClientRect().top;
      root.style.setProperty('--intro-card-h', `${Math.max(0, Math.ceil(fromBottom))}px`);
    };
    publish();
    const ro = new ResizeObserver(publish);
    ro.observe(cardEl);
    window.addEventListener('resize', publish);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', publish);
      root.style.removeProperty('--intro-card-h');
    };
  });

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
  <div
    class="intro-card"
    class:compact={variant === 'compact'}
    class:canvas={CANVAS_ROUTES.has(routeName)}
    bind:this={cardEl}
  >
    {#if variant === 'full'}
      <h2 class="intro-heading">{instanceName} is a quilt of the communities around you.</h2>
      <p class="intro-body">
        Every tile is a real group, placed near the groups it shares people with. No ads, no personalized algorithm. Run by people here.
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
     (55), matching the label/filter sheets' layer. Both variants share one
     placement: the bottom-left corner, clear of the results pane, the
     Quilt/Map toggle, and the bottom-right corner (Label overlay, FABs,
     toasts). It may overlap dismissible content, never a control.

     The compact strip used to centre itself just under the global bar,
     which is empty space over a canvas and exactly where a page surface
     puts its heading — it covered 22px of every 30px <h1> on the events
     list and discovery mode. One corner for both variants is what
     CONTEXT.md already promises ("overlaid on a corner of the surface,
     never covering it"), and it is one rule rather than a margin every
     future page surface has to remember to leave. */
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

  .intro-card {
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
     fixed measure — deep links never compete with the card for space. */
  .intro-card.compact {
    display: flex;
    align-items: center;
    gap: 10px;
    max-width: min(92vw, 420px);
    padding: 10px 34px 10px 14px;
  }

  .intro-card.compact .intro-line {
    font-size: 0.82rem;
    font-weight: 600;
  }

  .intro-card.compact .intro-about {
    font-size: 0.82rem;
    white-space: nowrap;
  }

  /* Mobile: full width, and at the foot rather than under the bar. Under
     the bar is where a page surface puts its heading — the card covered the
     whole of it and the first row of controls beneath it — and the top is
     also where the canvas views keep their Quilt/Map/List pill, which is a
     control the card must never sit on.

     The offset clears the sidebar rail, which is a bottom tab bar at this
     width. It comes from app.css (`--pw-nav-h`, the bar's footprint, plus
     the canvas row's gap) precisely so every surface that floats something
     over the foot of the screen reads one number instead of guessing it.

     768px, not 640px, because that is where SocialShell turns the rail into
     that tab bar — and the two have to agree. They did not: between 641 and
     768 the card kept its desktop bottom-left placement while the rail had
     already become a bar beneath it, so the card sat on 35px of it. A card
     that may never cover a control cannot own a breakpoint the control does
     not share. */
  @media (max-width: 768px) {
    .intro-card {
      left: 12px;
      right: 12px;
      top: auto;
      /* app.css's standing offset for anything floating above the mobile tab
         bar, safe-area included — the same one the canvas chrome uses. */
      bottom: var(--pw-canvas-chrome-bottom, 12px);
      max-width: none;
    }

    /* On a canvas the Quilt/Map/List pill is already sitting at that offset,
       and a control is the one thing this card may never cover (CONTEXT.md).
       36px pill plus the same 12px gap it keeps from everything else. */
    .intro-card.canvas {
      bottom: calc(var(--pw-canvas-chrome-bottom, 12px) + 48px);
    }
  }
</style>
