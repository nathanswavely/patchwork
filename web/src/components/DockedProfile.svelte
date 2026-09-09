<script>
  /**
   * The docked profile (CONTEXT.md "Docked profile", docs/adr/094): a
   * patch's own profile, shown over the discovery surface a reader tapped
   * it from, so answering "what is that one?" does not spend their zoom
   * and pan.
   *
   * Two forms, and the room picks — the surface passes `form`, because it
   * is the one component that knows how much room it has:
   *
   *   sheet  — at the foot of a phone. Two heights and no third: at rest
   *            the profile's head, pulled up the whole thing, full screen.
   *   panel  — in the cards pane's slot. One height, no drag: that room
   *            already shows a surface and a profile at once, which is why
   *            this form sits beside the surface instead of over it.
   *
   * Never modal. In the panel form the canvas behind stays live — hover
   * still previews, panning still pans — so no scrim, and Escape rather
   * than a click-behind closes it, since a pan that begins with a click
   * would otherwise dismiss what the reader is reading.
   *
   * The sheet is one tall box translated down so only its head shows,
   * rather than a short box that grows. A transform animates for free and
   * a drag maps straight onto it; animating height would relayout the
   * profile on every frame of a pull.
   */
  import PatchProfileHead from './PatchProfileHead.svelte';
  import PatchProfileGlimpses from './PatchProfileGlimpses.svelte';
  import RemotePatch from '../pages/RemotePatch.svelte';
  import { X } from 'phosphor-svelte';

  let {
    slug = '',
    // Set when the tapped patch lives on another quilt: the container is
    // chosen by the room and its occupant by the address, so this one docks
    // a remote patch card (docs/adr/024, docs/adr/094 decision 7).
    host = null,
    // The quilt row the surface already has. It paints the first frame;
    // the head's own fetch completes it (docs/adr/094 decision 9).
    seed = null,
    form = 'sheet',
    onClose = () => {},
  } = $props();

  let isSheet = $derived(form === 'sheet');

  // The sheet's two heights. The panel has none, so it renders its
  // glimpses on open — there is no pull to defer them to.
  let expanded = $state(false);
  let loaded = $state(null);
  let showGlimpses = $derived(!isSheet || expanded);

  // How much of the foot the sheet must leave showing at rest, measured
  // rather than assumed: a cover plus a name that wraps to three lines is a
  // different height on every patch, and a guessed fraction would be wrong
  // for most of them.
  //
  // Measured from the sheet's own top edge to the foot of the head, not the
  // head's height — the same distinction IntroCard makes about publishing
  // what it occupies rather than what it is. The head's height alone left
  // the handle above it unaccounted for, which put the relationship row 25
  // pixels below the fold: a control the reader could see the top of and
  // not press.
  let sheetEl = $state(null);
  let headEl = $state(null);
  let restH = $state(0);
  $effect(() => {
    const head = headEl;
    const sheet = sheetEl;
    if (!head || !sheet || typeof ResizeObserver === 'undefined') return;
    const measure = () => {
      restH = Math.ceil(head.getBoundingClientRect().bottom - sheet.getBoundingClientRect().top);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(head);
    window.addEventListener('resize', measure);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', measure);
    };
  });

  // A new patch arrives in the same sheet (the surface replaces the address
  // rather than pushing one), so the height resets with it: a reader who
  // pulled one patch up has not asked to read the next one whole.
  $effect(() => {
    void slug;
    void host;
    expanded = false;
    dragY = 0;
  });

  // --- The pull ---------------------------------------------------------
  // Downward from full lands on the head; downward from the head dismisses,
  // which is the one pull that leaves. Upward from the head goes full and
  // stops there — a sheet that kept rising would promise a height it does
  // not have. Same third-of-the-way rule the docked card used, so a scroll
  // that starts on the handle by accident doesn't throw away what the
  // reader was about to read.
  let dragY = $state(0);
  let dragFrom = null;
  let dragPulled = false;

  function dragStart(e) {
    if (!isSheet) return;
    dragFrom = e.clientY;
    dragY = 0;
    dragPulled = false;
    e.currentTarget.setPointerCapture(e.pointerId);
  }

  function dragMove(e) {
    if (dragFrom === null) return;
    const dy = e.clientY - dragFrom;
    // Expanded: only downward. At rest: upward opens, downward dismisses.
    dragY = expanded ? Math.max(0, dy) : dy;
    if (Math.abs(dragY) > 4) dragPulled = true;
  }

  function dragEnd() {
    if (dragFrom === null) return;
    dragFrom = null;
    const travel = dragY;
    dragY = 0;
    const reach = Math.max(80, restH / 3);
    if (expanded) {
      if (travel > window.innerHeight / 4) expanded = false;
      return;
    }
    if (travel < -reach) expanded = true;
    else if (travel > reach) onClose();
  }

  // A pull that fell short still ends in a click on the handle, and a
  // handle that is also a button would dismiss what the reader had just
  // decided to keep.
  function handlePress() {
    if (dragPulled) { dragPulled = false; return; }
    if (isSheet && !expanded) expanded = true;
    else if (isSheet) expanded = false;
  }

  function onKeydown(e) {
    if (e.key === 'Escape') onClose();
  }

  // Where the sheet sits: fully up, or down by everything but its head.
  // While a finger is on it, wherever the finger says.
  let restOffset = $derived(restH > 0 ? `calc(100% - ${restH}px)` : '100%');
  let offset = $derived(
    !isSheet ? '0px'
      : dragY !== 0 ? `calc(${expanded ? '0px' : restOffset} + ${dragY}px)`
      : expanded ? '0px' : restOffset
  );
</script>

<svelte:window onkeydown={onKeydown} />

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<section
  bind:this={sheetEl}
  class="dock"
  class:sheet={isSheet}
  class:panel={!isSheet}
  class:expanded
  class:dragging={dragY !== 0}
  style="--dock-y: {offset}"
  aria-label="Patch"
>
  {#if isSheet}
    <!-- The handle: the pull's target, and at full screen one of only
         three ways out (the dismiss and the browser's back are the others
         — nothing of the surface is left to tap). -->
    <button
      class="dock-handle"
      onpointerdown={dragStart}
      onpointermove={dragMove}
      onpointerup={dragEnd}
      onpointercancel={dragEnd}
      onclick={handlePress}
      aria-label={expanded ? 'Collapse' : 'Expand'}
      aria-expanded={expanded}
    ></button>
  {/if}

  <button class="dock-dismiss" onclick={onClose} aria-label="Dismiss">
    <X size={16} weight="bold" />
  </button>

  <div class="dock-scroll" class:scrollable={showGlimpses}>
    {#if host}
      <!-- Another quilt's patch: read-only, and every act on it a doorway
           (docs/adr/024). It has no glimpses to defer, so both heights
           hold the whole card. -->
      <RemotePatch {host} {slug} />
    {:else}
      <div class="dock-head" bind:this={headEl}>
        <PatchProfileHead {slug} {seed} onLoaded={(state) => { loaded = state; }} />
      </div>
      {#if showGlimpses}
        <PatchProfileGlimpses
          {slug}
          node={loaded?.node ?? null}
          isMember={loaded?.isMember ?? false}
          isAdmin={loaded?.isAdmin ?? false}
          isUnclaimed={loaded?.isUnclaimed ?? false}
          isBanned={loaded?.isBanned ?? false}
          membershipRole={loaded?.membershipRole ?? ''}
          followerPermissions={loaded?.followerPermissions ?? null}
        />
      {/if}
    {/if}
  </div>
</section>

<style>
  .dock {
    background: var(--color-surface);
    color: var(--color-text);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  /* ---- The sheet: a phone's foot ------------------------------------- */
  /* One tall box translated down to show only its head. Out at the root
     rather than inside the quilt pane, which is its own stacking context
     at z-index 0 — the lesson docs/adr/078 recorded when the filter button
     ended up on the card's description. */
  .dock.sheet {
    position: fixed;
    left: 0;
    right: 0;
    top: 0;
    height: 100vh;
    height: 100dvh;
    /* Over the canvas and its floating buttons (20), the rail (55) and the
       bar (60): it is the answer to the tap that is still on screen. */
    z-index: 65;
    border-radius: 14px 14px 0 0;
    box-shadow: 0 -4px 24px var(--color-shadow);
    transform: translateY(var(--dock-y));
    transition: transform 220ms cubic-bezier(0.32, 0.72, 0, 1);
  }

  /* A finger's sheet follows the finger, without a transition chasing it. */
  .dock.sheet.dragging {
    transition: none;
  }

  .dock.sheet.expanded {
    border-radius: 0;
  }

  @media (prefers-reduced-motion: reduce) {
    .dock.sheet { transition: none; }
  }

  .dock-handle {
    flex: none;
    display: block;
    width: 100%;
    height: 26px;
    padding: 0;
    border: none;
    background: none;
    cursor: grab;
    touch-action: none;
    position: relative;
  }

  .dock-handle::after {
    content: '';
    position: absolute;
    left: 50%;
    top: 10px;
    transform: translateX(-50%);
    width: 36px;
    height: 4px;
    border-radius: 2px;
    background: var(--color-text-muted);
    opacity: 0.4;
  }

  /* ---- The panel: the cards pane's slot ------------------------------ */
  /* Absolute inside the surface, in the pane's own rectangle, so the
     canvas's inset is unchanged and nothing reflows behind it: the marker
     the reader clicked stays where they clicked it. */
  .dock.panel {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 45%;
    /* Just above the cards pane (10) it covers, and far below the rail and
       the bar — it is a peer of the pane, not chrome. */
    z-index: 11;
    padding-top: 56px; /* clear the glass top bar, as the pane does */
    border-left: 1px solid var(--color-border);
    box-shadow: -4px 0 24px var(--color-shadow);
    animation: dock-slide-in 220ms cubic-bezier(0.32, 0.72, 0, 1);
  }

  @keyframes dock-slide-in {
    from { transform: translateX(12px); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .dock.panel { animation: none; }
  }

  /* Top-LEFT, in both forms: the cover's top-right corner is spoken for by
     Settings and the overflow, and a dismiss landing on the overflow is the
     collision docs/adr/078 recorded once already. The rects overlapped by
     six pixels on a sheet — invisible in a screenshot, and enough to eat
     the press. */
  .dock-dismiss {
    position: absolute;
    top: 10px;
    left: 10px;
    z-index: 2;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: none;
    border-radius: 999px;
    background: var(--color-glass);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    color: var(--color-text);
    cursor: pointer;
    opacity: 0.85;
  }

  .dock-dismiss:hover {
    opacity: 1;
  }

  /* The panel's dismiss goes to the left: the cover's own top-right corner
     is spoken for by Settings and the overflow, and a dismiss landing on
     the overflow is the collision docs/adr/078 already recorded once. On a
     sheet it stays right, where the handle band keeps it clear of both. */
  .dock.panel .dock-dismiss {
    top: 64px;
  }

  .dock-scroll {
    flex: 1;
    min-height: 0;
    overflow: hidden;
    padding: 0 var(--pw-gutter);
  }

  /* Only a sheet showing its glimpses scrolls. At rest there is nothing
     below the head to reach, and a scrollable rest state would swallow the
     pull. */
  .dock-scroll.scrollable {
    overflow-y: auto;
    overscroll-behavior: contain;
    -webkit-overflow-scrolling: touch;
    padding-bottom: calc(2rem + env(safe-area-inset-bottom, 0px));
  }

  .dock-head {
    /* The head's own top gap: the handle is above it on a sheet, the bar
       above that on a panel. */
    padding-top: 4px;
  }

  /* At rest the sheet covers the tab bar, so nothing below it is keeping
     the relationship row clear of a home indicator. Inside the head rather
     than on the sheet, because the rest height is measured to the head's
     foot and a gap outside it would not be reserved. */
  .dock.sheet:not(.expanded) .dock-head {
    padding-bottom: calc(10px + env(safe-area-inset-bottom, 0px));
  }
</style>
