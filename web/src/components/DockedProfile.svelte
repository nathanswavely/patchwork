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
   *            the profile's head with the first glimpse starting under
   *            it and cut by the fold, pulled up the whole thing, full
   *            screen. The cut is the invitation: a sheet that ended
   *            cleanly at its buttons looked finished, and nothing said
   *            there was anything to pull up for.
   *   panel  — in the cards pane's slot, in the list's place. One height,
   *            no drag: that room already shows a surface and a list at
   *            once, and the profile takes the list's box rather than
   *            floating over it. The card the reader clicked grows into
   *            it (`origin`).
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
    // The rectangle the panel grows out of — the clicked card's, measured
    // by the surface before the list gave way. Null when there was no card
    // (a click on the canvas), and the panel simply arrives.
    origin = null,
    onClose = () => {},
  } = $props();

  let isSheet = $derived(form === 'sheet');

  // The sheet's two heights. The glimpses mount in both — at rest the
  // first one shows under the fold — but they only *fetch* once the sheet
  // is pulled up (or in the panel, which has no pull): a tap on the quilt
  // stays one request (docs/adr/094 decision 4).
  let expanded = $state(false);
  let loaded = $state(null);
  let glimpsesActive = $derived(!isSheet || expanded);

  // How much of what follows the head shows at rest, past the head's foot:
  // enough for the first section's rule, its title and a line of it, cut
  // off. The cut is doing the work, so the amount is a constant rather than
  // a measurement — it is the same invitation on every patch.
  const PEEK = 96;

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
      restH = Math.ceil(head.getBoundingClientRect().bottom - sheet.getBoundingClientRect().top) + PEEK;
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

  // Escape closes the dock — unless a dialog is open above it, which owns
  // the key: both listen on the window, so without this one Escape closed
  // the join sheet and the profile it was joining from together.
  function onKeydown(e) {
    if (e.key !== 'Escape') return;
    if (document.querySelector('[role="dialog"][aria-modal="true"]')) return;
    onClose();
  }

  // --- Opening in the pane's slot ---------------------------------------
  // The card grows into the profile: the profile's box is first drawn at
  // the card's rectangle and animates to its own, so the reader's eye is
  // carried from the thing they clicked to the thing it became. Without a
  // card to grow from it arrives with a short fade. A transform on the
  // whole box, so nothing inside relayouts; skipped for a reader who asked
  // for less motion.
  $effect(() => {
    const el = sheetEl;
    const from = origin;
    if (!el || isSheet || typeof el.animate !== 'function') return;
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    const to = el.getBoundingClientRect();
    const easing = 'cubic-bezier(0.32, 0.72, 0, 1)';
    if (from && to.width > 0 && to.height > 0) {
      el.style.transformOrigin = 'top left';
      el.animate(
        [
          {
            transform: `translate(${from.left - to.left}px, ${from.top - to.top}px) scale(${from.width / to.width}, ${from.height / to.height})`,
            opacity: 0.4,
          },
          { transform: 'none', opacity: 1 },
        ],
        { duration: 260, easing },
      );
    } else {
      el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 160, easing });
    }
  });

  // What a screen reader calls this region: the patch's own name, which the
  // seed carries and the slug backs up. A label naming the *kind* of thing
  // ("Patch") would be a new string in the copy ledger — words a person has
  // to own — to say less than the name does, and the head's own <h1> is
  // already the name a sighted reader sees.
  let regionName = $derived(seed?.name || slug);

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
  aria-label={regionName}
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

  <div class="dock-scroll" class:scrollable={glimpsesActive}>
    {#if host}
      <!-- Another quilt's patch: read-only, and every act on it a doorway
           (docs/adr/024). It has no glimpses to defer, so both heights
           hold the whole card. -->
      <RemotePatch {host} {slug} />
    {:else}
      <div class="dock-head" bind:this={headEl}>
        <PatchProfileHead
          {slug}
          {seed}
          layout="docked"
          onLoaded={(state) => { loaded = state; }}
        />
      </div>
      <!-- The gap between the row and the first glimpse's rule is the
           container's (PatchProfileHead ends at its row), so it is set
           here, the way the page sets its own. At rest a tap on the cut-off
           content — not on a link in it — pulls the sheet up: it is the
           thing the reader was reaching for. -->
      <!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
      <div
        class="dock-glimpses"
        onclick={(e) => {
          if (isSheet && !expanded && !e.target.closest('a, button')) expanded = true;
        }}
      >
          <PatchProfileGlimpses
            {slug}
            active={glimpsesActive}
            node={loaded?.node ?? null}
            isMember={loaded?.isMember ?? false}
            isAdmin={loaded?.isAdmin ?? false}
            isUnclaimed={loaded?.isUnclaimed ?? false}
            isBanned={loaded?.isBanned ?? false}
            membershipRole={loaded?.membershipRole ?? ''}
            followerPermissions={loaded?.followerPermissions ?? null}
          />
      </div>
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
    /* The head's cover bleeds to the sheet's edges (PatchProfileHead's
       sheet layout), and the sheet's corners are what clip it. */
    overflow: hidden;
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

  /* The handle lies over the cover, where a thumb arrives, rather than in a
     band above it: the band kept the cover off the sheet's own corners. It
     is sized to its hit area, not to the bar it draws, and centred rather
     than full width so the cover's own corners — dismiss on the left,
     Settings and the overflow on the right — stay pressable under it. Out
     of the flow, so the rest height (measured to the head's foot from the
     sheet's top) is unchanged by it. */
  .dock-handle {
    position: absolute;
    top: 0;
    left: 50%;
    transform: translateX(-50%);
    z-index: 3;
    width: 72px;
    height: 24px;
    padding: 0;
    border: none;
    background: none;
    cursor: grab;
    touch-action: none;
  }

  /* Filled and shadowed like the dismiss beside it, not glass: the bar lies
     on the patch's own fabric, which is any colour a patch chose, and a
     translucent one disappears on half of them. */
  .dock-handle::after {
    content: '';
    display: block;
    width: 36px;
    height: 4px;
    margin: 8px auto 0;
    border-radius: 999px;
    background: var(--color-surface);
    box-shadow: 0 1px 3px var(--color-shadow);
    opacity: 0.9;
  }

  /* ---- The panel: the cards pane's slot ------------------------------ */
  /* A card where the list was. The pane is transparent and the cards float
     over the canvas, so the profile is a card too: the list's own margins,
     a surface, an edge and the cards' shadow. It fills the pane's height
     and scrolls inside itself, the way the list did. The canvas's inset is
     unchanged and nothing reflows behind it: the marker the reader clicked
     stays where they clicked it. */
  .dock.panel {
    position: relative;
    flex: 1;
    min-height: 0;
    margin: 12px 16px 16px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 2px 10px var(--color-shadow);
    /* The head's cover bleeds to the card's edges; the corners clip it. */
    overflow: hidden;
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

  .dock-scroll {
    flex: 1;
    min-height: 0;
    overflow: hidden;
    padding: 0 var(--pw-gutter);
  }

  /* Only a sheet pulled up scrolls. At rest what shows below the head is
     the peek, reached by the pull, and a scrollable rest state would
     swallow it. */
  .dock-scroll.scrollable {
    overflow-y: auto;
    overscroll-behavior: contain;
    -webkit-overflow-scrolling: touch;
    padding-bottom: calc(2rem + env(safe-area-inset-bottom, 0px));
  }

  /* No top gap in either form: the cover starts at the box's own top edge,
     with the dismiss (and on a sheet the handle) over it. */
  .dock-head {
    padding-top: 0;
  }

  /* At rest the sheet ends just under the relationship row, so its standing
     menu opens upward: downward is below the fold, where a "Leave" nobody
     can see is a menu that appears to do nothing. The panel has room below
     and keeps the menu's own direction. */
  .dock.sheet .dock-head :global(.standing-menu) {
    top: auto;
    bottom: calc(100% + 4px);
  }

  .dock-glimpses {
    margin-top: 1.5rem;
  }

  /* At rest the sheet covers the tab bar, so nothing below it keeps the
     relationship row clear of a home indicator — the peek does: the row
     sits PEEK pixels above the fold, further than any inset reaches. */
</style>
