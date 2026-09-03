<script>
  import { onMount, untrack } from 'svelte';
  import * as d3 from 'd3';
  import { api } from '../lib/api.js';
  import { identityColorForPatch, paletteForPatch, ghostPalette, darken, textOnColor } from '../lib/quiltTheme.js';
  import { renderBlock, blockPieces, getRotation, blockKeyAt } from '../lib/quiltBlocks.js';
  import {
    CLOTH, createLattice, warpPolygon, fabricGrain, bakeWrinkles, bakeWeave,
    WEAVES, weaveFor,
  } from '../lib/quiltCloth.js';
  import { quiltLayout } from '../lib/quiltLayout.js';
  import { createMotifGroup, createMyPatchStarGroup, createFollowedHeartGroup, createUnclaimedMarkGroup } from '../lib/patchIcons.js';
  import { buildRemoteGroups, composeGroupLayouts } from '../lib/quiltRegions.js';
  import { blockPageZoom } from '../lib/pageZoom.js';
  import { textMatches } from '../lib/textMatch.js';
  import {
    getRemoteFollows, fetchQuiltInfo, colorForQuilt, refreshFollowSnapshot,
  } from '../stores/multiQuilt.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import { navigate } from '../stores/router.svelte.js';

  // A search that named a group the quilt doesn't have. The filter-miss case
  // gets no such offer — a tag toggle hiding a patch isn't the patch missing.
  function suggestPatch() {
    navigate(`/submit?name=${encodeURIComponent(searchQuery.trim())}`);
  }
  let canSuggest = $derived(!!searchQuery.trim() && getSubmissionsEnabled());

  // The tile-to-tile boundary is a **seam** (CONTEXT.md), and it belongs to
  // the boundary, not to either tile. The five per-tile RAW_EDGES polygons
  // that used to live here gave each tile its own wobbly outline, so the gap
  // between two neighbours was the sum of two independent insets — it pinched
  // and flared along a single seam, and being world geometry it grew 20x
  // across the zoom range. The wobble now comes from a lattice both
  // neighbours read; see internal docs in lib/quiltCloth.js.
  let lattice = null;

  let {
    filterTags = [],
    searchQuery = '',
    selectedPatchSlug = null,
    onPatchClick = () => {},
    myPatchRoles = new Map(),
    quiltScope = 'local',
    // Fraction of the container width covered by overlaid UI on the right
    // (e.g. the floating patch-card list). The quilt centers and zoom-fits
    // within the remaining visible area while still rendering full-bleed.
    insetRight = 0,
    // Clears the tag filter from the filtered-to-nothing overlay. The store
    // stays out of this component — the parent owns the lens state.
    onClearFilter = () => {},
    // The in-view lens (docs/adr/074): reports which patches are inside the
    // viewport, so the cards list can narrow to what a person is looking at.
    // The canvas only ever *reports* — whether anything narrows is the
    // parent's decision, the same way the filter works.
    onInViewChange = null,
    // Static/compact mode (the About page hero, docs/adr/040): a live
    // miniature of the real quilt with no pan/zoom and no name badges. The
    // parent controls the footprint by sizing/positioning the wrapper —
    // this component just stops capturing gestures and stops drawing
    // chrome that a small non-interactive tile has no room for.
    interactive = true,
    showLabels = true,
    // Pointing at a patch previews it (docs/adr/078). The quilt reports what
    // the pointer is over and the parent decides what that means — here, the
    // patch's card highlighting in the pane beside it.
    onPatchHover = null,
  } = $props();

  let containerEl = $state(null);
  let labelsEl = $state(null);

  /** Get container dimensions (falls back to window if container not ready). */
  function getContainerSize() {
    if (containerEl) {
      return { vw: containerEl.clientWidth, vh: containerEl.clientHeight };
    }
    return { vw: window.innerWidth, vh: window.innerHeight };
  }

  // On-screen tile size at which a tile earns a label, and the smaller one at
  // which it finally gives that label back (see updateLabels).
  //
  // Two thresholds rather than one because a single one is a line that gets
  // crossed twice: a zoom-out is a stream of passes, the tile shrinks past 52
  // in the middle of it, and any wobble — a pinch that gives a pixel back, a
  // rebuild that re-rounds the tile — flips the name on and off.
  //
  // The band is narrow on purpose. A pill is already wider than a 52px tile,
  // so every pixel of hold past that is a name floating further from the tile
  // it belongs to: at 38 the seeded quilt kept names hovering over a thumbnail
  // long after the tiles under them had stopped reading as tiles. 44 is about
  // 15% — enough to swallow the wobble a gesture produces, not enough to hold
  // a name into a zoom with nothing to attach it to.
  //
  // 48 was tried, to clear the last few names from the far zoom-out, and
  // reverted. It does not clear them: at k≈0.58 and k≈0.54 the same three
  // names still sit over the thumbnail — and they sit there in a build with no
  // hysteresis at all, because a pill has always been wider than a tile at
  // that size. 48 only moves the boundary one notch, to k≈0.49, and the sweep
  // prices that notch at three badges lost at k≈1.6, a zoom people browse at.
  // A narrow notch for a narrow notch, and the wrong one. If the far zoom-out
  // ever needs fixing for real, the lever is not here: either drop every badge
  // below an absolute zoom, or scale the pill down so it stops outgrowing its
  // tile.
  //
  // This band is NOT what stops a badge blinking off and back on mid-zoom;
  // that is a collision, and LABEL_KEEP_GAP is what settles it — see there
  // for how completely it settles it at the value shipped. Over a 34-step
  // sweep of the seeded quilt this threshold alone changed no reappearance at
  // any value from 52 (no hysteresis) down to 30.
  const LABEL_MIN_PX = 52;
  const LABEL_KEEP_PX = 44;
  // A name badge's TEXT is its name's alone (CONTEXT.md "Name badge"); how
  // that text is broken across lines is the tile's, and the two constants
  // below bound the choice rather than making it. See updateLabels.
  //
  // The cap is deliberately narrow and the line budget deliberately deep.
  // Two adjacent tiles both earn a badge only when the tile pitch clears
  // `widest pill + LABEL_GAP`, so the WIDEST name governs how far in one
  // has to zoom before a row stops alternating label/no-label. Tiles are
  // square and a pill is wide and short, so wrapping spends the dimension
  // that has room: across the Lancaster instance's names, 110/3 lines puts
  // the widest pill at 126px against 158px for the old 140/2.
  //
  // The two move together or not at all. Lowering the cap alone does not
  // narrow the longest names — their balanced two-line split is already at
  // the ceiling — it ellipsises them. At 110/2 ten of 44 names clip; at
  // 110/3 none do, including one ("Long's Park Amphitheater Foundation")
  // that was already clipping at 140/2.
  // The width cap, as a multiple of the badge's own font size rather than a
  // pixel count: 8.5em is the 110px that was measured against the old 13px
  // type, and stated this way it follows --pw-label-font wherever that lands
  // instead of quietly becoming a different cap at a different size.
  const LABEL_TEXT_EM = 8.5;
  const LABEL_MAX_LINES = 3;
  // Minimum visible quilt between placed badges; a rival badge that can't
  // clear this gap stays hidden until a closer zoom, and a badge already on
  // screen holds its place down to LABEL_KEEP_GAP. Sized by eye against a
  // dense quilt: a name-only pill is ~65px narrower than the old motif-and-
  // role one, so at the pre-corner-mark gap of 12 many more badges cleared
  // collision and the quilt went back to being papered in them.
  //
  // Left at 32 when the cap above came down, having been measured rather
  // than assumed: against the narrowed pills, dropping this to 20 bought
  // exactly one more badge across the crowded zooms (25 against 24 at the
  // densest) and spent every pixel of breathing room to do it. The cap is
  // the lever here; this is not.
  const LABEL_GAP = 32;
  // What an incumbent owes instead — the collision half of the hysteresis
  // above, and the half that does the work. Badges drift together as the quilt
  // scales, and on one gap a badge that crosses 32px of separation is dropped,
  // then re-placed a step later when its rival is dropped or reshaped: the
  // blink both KEEP constants exist to stop.
  //
  // This is the most an incumbent is ever forgiven, reached only on a roomy
  // tile (LABEL_ROOMY_PX below); a badge nearer the size floor owes more.
  //
  // 26 is a deliberate trade, and it is not the value that stops every blink.
  // Measured over the same 34-step sweep: 32/32 left three names blinking out
  // and back, 32/26 two, 32/18 none, at 394 and 422 badges-on-screen
  // respectively. What the six extra pixels buy is breathing room — LABEL_GAP
  // above is 32 because a quilt papered in pills stops reading as a quilt, and
  // an incumbent allowed all the way down to 18 spends most of that. They also
  // cost a little wrapping: at 26 a few names stack a line deeper than they
  // need to, purely to clear the wider gap.
  //
  // Re-measured once the ramp below existed, since 18 won on flicker before
  // it: under the ramp it no longer does. 18 raises reappearances from 2 to 3
  // while adding twelve badges across the sweep. Whatever the flat gap needed
  // a low floor for, the ramp does better.
  const LABEL_KEEP_GAP = 26;
  // ...and only as far as its tile has room to justify. An incumbent on a
  // 52px tile owes the full 32; one on an 80px tile or bigger owes 26; between
  // those the gap slides. Without this the relaxation applies hardest exactly
  // where it looks worst — zoomed far out, where every tile is near the floor,
  // the whole quilt is a thumbnail and holding five names over it reads as
  // clutter rather than continuity (desktop far zoom: 5 names against 3).
  //
  // A ramp and not a threshold, which was the first attempt: a gap that flips
  // at a tile size is one more line to cross, and badges crossing it blink for
  // exactly the reason the KEEP constants exist. Measured over the 34-step
  // sweep, a hard cutoff at 78 or 95 took reappearances from 2 back up to 4,
  // while the ramp holds at 2. Nothing about a tile's size should be a cliff.
  const LABEL_ROOMY_PX = 80;
  // Corner marks: an on-screen size, like a name badge, not a share of the
  // tile — see updateCornerMarks. MARK_PX is the diameter one wants,
  // MARK_INSET its gap from the tile's corner, both in canonical units
  // that the counter-scale turns into screen pixels.
  const MARK_PX = 22;
  const MARK_INSET = 6;
  // A mark never eats more than this share of the tile it sits on, and below
  // MARK_MIN_PX on screen it's an illegible speck, so it goes. The share cap
  // is load-bearing now that two marks share the top edge: at LABEL_MIN_PX a
  // pair of full-size discs plus their insets would not fit on the tile.
  const MARK_TILE_SHARE = 0.3;
  const MARK_MIN_PX = 9;
  // The glyph inside a disc, and how far the shadow circle sits down-right of
  // it (matching --lt-shadow-x/y, so the light direction agrees with the
  // rest of the quilt).
  const MARK_GLYPH_PX = MARK_PX * 0.64;
  const MARK_SHADOW_OFFSET = 1;
  // Status marks (unclaimed, role) wear a neutral disc; identity (the motif)
  // wears the patch's own color. See CONTEXT.md "Corner mark".
  const MARK_STATUS_FILL = 'rgba(0,0,0,0.55)';
  // Where each slot's group anchors on the tile (a fraction of the tile's
  // side) and which way it draws inward from there.
  const MARK_CORNERS = {
    tl: { ax: 0, ay: 0, sx: 1, sy: 1 },
    tr: { ax: 1, ay: 0, sx: -1, sy: 1 },
    br: { ax: 1, ay: 1, sx: -1, sy: -1 },
  };
  // Below this container width the quilt is a phone-sized surface.
  const NARROW_VW = 700;
  // How far the ideal tile size may drift from the one on screen before a
  // resize earns a full relayout rather than a view adjustment (handleResize).
  const BASE_UNIT_DRIFT = 0.1;

  // Measured badge shapes, cached per name (names don't change
  // mid-session; the cache is busted when the display font loads, and when
  // the type metrics below move under it).
  let measureCtx = null;
  const measureCache = new Map();

  /**
   * The badge's own computed type and box, read from a throwaway element
   * wearing the class rather than restated here.
   *
   * These numbers exist in the stylesheet already, and a second copy in JS
   * is a copy that goes stale: the pill is rem-based, so its font size is
   * the reader's to change, and the width cap, the pill footprint, and the
   * collision test all hang off it. Read once and cached — getComputedStyle
   * is a layout read and updateLabels runs on every pan tick.
   */
  let labelType = null;

  function badgeType() {
    if (labelType) return labelType;
    const probe = document.createElement('div');
    probe.className = 'patch-label';
    probe.style.cssText = 'position:absolute;left:-9999px;top:-9999px;visibility:hidden';
    document.body.appendChild(probe);
    const cs = getComputedStyle(probe);
    const fontSize = parseFloat(cs.fontSize) || 13;
    const lineH = parseFloat(cs.lineHeight) || Math.round(fontSize * 1.3);
    labelType = {
      fontSize,
      font: `${cs.fontWeight} ${fontSize}px ${cs.fontFamily}`,
      lineH,
      textMax: Math.round(fontSize * LABEL_TEXT_EM),
      // Horizontal and vertical chrome: what the pill adds around its text.
      chromeX: (parseFloat(cs.paddingLeft) || 0) + (parseFloat(cs.paddingRight) || 0)
        + (parseFloat(cs.borderLeftWidth) || 0) + (parseFloat(cs.borderRightWidth) || 0),
      chromeY: (parseFloat(cs.paddingTop) || 0) + (parseFloat(cs.paddingBottom) || 0)
        + (parseFloat(cs.borderTopWidth) || 0) + (parseFloat(cs.borderBottomWidth) || 0),
    };
    probe.remove();
    return labelType;
  }

  /** Type moved under us — remeasure everything that was derived from it. */
  function resetBadgeType() {
    labelType = null;
    measureCache.clear();
  }

  /**
   * Catch the badge's type moving under us. Two ways it can: the reader
   * changes their default text size (the pill is in rem), or the pill's own
   * size becomes viewport-dependent — a narrow-screen badge is the obvious
   * next thing to want here, and it would move at a breakpoint the root font
   * size never notices.
   *
   * So compare what the badge actually computes to, not the root it derives
   * from. Neither event announces itself, but both arrive with the reflow a
   * resize reports. Compared rather than reset unconditionally: a drag-resize
   * is a stream of these, and remeasuring every name per frame is waste.
   */
  function syncBadgeType() {
    const was = labelType ? labelType.fontSize : 0;
    labelType = null;
    const now = badgeType().fontSize;
    if (was && Math.abs(now - was) < 0.5) return false;
    measureCache.clear();
    return true;
  }

  /**
   * Narrowest balanced width for `words` broken into exactly `lines` lines —
   * the width of the longest line under the best split, mirroring what CSS
   * `text-wrap: balance` will do inside the pill.
   *
   * Recursive with a memo rather than a scan of split points: at two lines
   * those are the same thing, but at three the choices interact, and taking
   * the greedy split first can strand a long tail word on its own line.
   */
  function balancedWidth(words, lines, from, memo) {
    if (lines === 1) return measureCtx.measureText(words.slice(from).join(' ')).width;
    // One slot per (start word, lines remaining); lines is small and bounded
    // by LABEL_MAX_LINES, so the stride only has to clear it.
    const key = from * 8 + lines;
    const hit = memo.get(key);
    if (hit !== undefined) return hit;
    let best = Infinity;
    // Leave at least one word for each remaining line.
    for (let end = from + 1; end <= words.length - (lines - 1); end++) {
      const head = measureCtx.measureText(words.slice(from, end).join(' ')).width;
      // A line already wider than the best full answer can't be beaten by
      // whatever follows it — max() is monotone in this term.
      if (head >= best) break;
      best = Math.min(best, Math.max(head, balancedWidth(words, lines - 1, end, memo)));
    }
    memo.set(key, best);
    return best;
  }

  /**
   * Every shape one name can wear, shallowest first: {textW, lines} for each
   * line count that actually buys a narrower pill.
   *
   * Measured per name and cached; which of them a badge wears is decided per
   * placement (updateLabels), because the same name wants a different answer
   * on a roomy tile than on one hemmed in by its neighbours' badges.
   */
  function badgeShapes(name) {
    let shapes = measureCache.get(name);
    if (shapes) return shapes;
    if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
    const { font, textMax } = badgeType();
    measureCtx.font = font;
    shapes = [];
    const full = measureCtx.measureText(name).width;
    if (full <= textMax) shapes.push({ textW: Math.ceil(full), lines: 1 });
    // Deeper splits, kept only while each one is still narrower than the last
    // — a line that buys no width is a line spent for nothing. +2px slack
    // absorbs canvas-vs-layout rounding.
    const words = name.split(/\s+/);
    let narrowest = full;
    for (let lines = 2; lines <= LABEL_MAX_LINES && words.length >= lines; lines++) {
      const best = balancedWidth(words, lines, 0, new Map());
      if (best >= narrowest) break;
      narrowest = best;
      if (best <= textMax) shapes.push({ textW: Math.ceil(best) + 2, lines });
    }
    if (shapes.length === 0) {
      // Nothing fits on word boundaries: one unbroken word, or more name than
      // the budget holds. Sit at the cap and let word-break/line-clamp take it
      // mid-word — estimating the line count from the run length rather than
      // always claiming the maximum, so a name that overshoots by a little
      // doesn't get a pill sized for one that overshoots by a lot.
      const wrapped = Math.min(Math.ceil(full / textMax), LABEL_MAX_LINES);
      shapes.push({ textW: textMax, lines: Math.max(2, wrapped) });
    }
    measureCache.set(name, shapes);
    return shapes;
  }

  /**
   * Pixel size of one grid cell. The only thing about the container the tile
   * geometry depends on: the packing itself (quiltLayout / composeGroupLayouts)
   * takes no size input, so two container sizes that agree here produce
   * identical tiles — which is what lets handleResize skip the rebuild.
   */
  function computeBaseUnit(vw, vh, n) {
    const contentSize = Math.min(vw - Math.round(vw * insetRight) - 32, vh - 64);
    const gridSide = Math.ceil(Math.sqrt(n * 3) * 1.3);
    return Math.max(30, Math.min(80, Math.floor(contentSize / gridSide)));
  }

  /** Fit padding — a phone can't spare the desktop gutters. */
  function fitInsets(vw) {
    const narrow = vw <= NARROW_VW;
    return { fitPadLeft: narrow ? 8 : 72, padding: narrow ? 16 : 60, narrow };
  }

  /**
   * Clamp a fit scale to the zoom extent, and on narrow viewports floor it so
   * the smallest tile still clears the label threshold. Fitting the whole
   * quilt into a phone screen leaves every tile too small to be labeled — the
   * quilt reads as anonymous confetti. Better to start legible and let the
   * person pan.
   */
  function clampFitScale(targetK, narrow) {
    const k = Math.max(0.3, Math.min(6, targetK));
    if (!narrow) return k;
    return Math.max(k, Math.min(2.4, (LABEL_MIN_PX + 8) / baseUnit));
  }
  let tooltip = $state(null);
  let treeData = $state(null);
  let affinityData = $state([]);
  let loading = $state(true);
  let error = $state('');

  let placedTiles = $state([]);
  let baseUnit = $state(50);
  let currentTransform = $state(d3.zoomIdentity);
  let canvasOffsetX = $state(0);
  let canvasOffsetY = $state(0);
  let labeledPatchIds = new Set();
  // Map of patch ID → the badge element on screen for it, so a pan moves badges
  // instead of rebuilding them (see updateLabels).
  let labelEls = new Map();
  // Patch ID → { tile, inner, role, slots } for every tile wearing corner
  // marks, so the zoom handler can counter-scale them back to a fixed screen
  // size and the myPatchRoles effect can add or drop the role slot in place
  // (see updateCornerMarks).
  let cornerMarks = new Map();
  // Map of patch ID → { g, tile, dist, visible } for per-tile animation.
  let tileMap = new Map();
  let layoutBuilt = false;
  // The staggered pop-in is an *entrance* — it belongs to the quilt arriving,
  // not to every layout pass. A rebuild (resize, a pane settling after the
  // first build) would otherwise tear down a half-finished entrance and
  // restart it from scale 0, which reads as the tiles spawning twice. Reset
  // in loadData, where the canvas really does leave and re-enter.
  let firstBuild = true;
  // Stored references for relayout animation.
  let contentG_ref = null;
  // Cloth layers: rebuilt with the layout, stood down while it moves.
  let clothG = null;
  let weaveG = null;
  let stitchG = null;
  let weaveParked = false;
  let weaveSettleTimer = null;
  let svgSelection = null;
  let zoomBehavior = null;
  // Container size the current layout was built against, so the observer can
  // ignore the reflows that don't actually change what we'd draw.
  let lastBuiltW = 0;
  let lastBuiltH = 0;
  let resizeTimer = null;
  // Bounded next-frame retry while the container still measures 0x0.
  let buildRetries = 0;
  let buildRetryFrame = null;

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // Determine which patch IDs should be visible based on filters.
  let visibleIds = $derived.by(() => {
    if (!treeData?.children) return new Set();
    let children = treeData.children;
    if (filterTags.length > 0) {
      children = children.filter(n => (n.tags || []).some(t => filterTags.includes(t)));
    }
    if (searchQuery.trim()) {
      children = children.filter(n =>
        textMatches(n.name, searchQuery) || textMatches(n.description, searchQuery)
      );
    }
    return new Set(children.map(c => c.id));
  });

  // Build the layout once both halves are ready. Track the container as well
  // as the data: the container only exists in the loaded branch, so it binds
  // *after* treeData arrives. Reading it untracked meant this effect never
  // re-ran for that second half, and the first paint was left to whichever
  // ResizeObserver tick happened to notice — a visibly empty canvas until
  // something else resized.
  $effect(() => {
    const td = treeData;
    const el = containerEl;
    untrack(() => {
      if (el && td?.children?.length > 0 && !layoutBuilt) {
        buildLayout();
      }
    });
  });

  // Relayout when filters/search change.
  $effect(() => {
    const ids = visibleIds;
    untrack(() => {
      if (tileMap.size > 0) {
        relayout(ids);
      }
    });
  });

  // --- The in-view lens (docs/adr/074) ---
  // Which patches are currently inside the viewport. The mapping is the one
  // the name badges already use: world = offset + tile position, and
  // screen = transform.x + world x transform.k.
  //
  // A tile counts as in view when its rectangle *intersects* the visible
  // region, not when its centre sits inside it. Zoom into a 4x4 and it can
  // fill the screen with its centre off it — "the big one I am staring at
  // is missing from the list" is the one result nobody would accept.
  //
  // The visible region stops short of the container's right edge on desktop:
  // the cards pane floats over that strip (insetRight), so a tile behind the
  // cards is on the canvas but not in view.
  function computeInView() {
    const t = currentTransform;
    const { vw, vh } = getContainerSize();
    if (!vw || !vh || !placedTiles.length) return null;
    const rightEdge = vw - vw * insetRight;
    const ids = [];
    for (const tile of placedTiles) {
      const id = tile.data?.id;
      if (!id) continue;
      const s = tile.pxSize * t.k;
      const x = t.x + (canvasOffsetX + tile.px) * t.k;
      const y = t.y + (canvasOffsetY + tile.py) * t.k;
      if (x + s < 0 || x > rightEdge || y + s < 0 || y > vh) continue;
      ids.push(id);
    }
    return ids;
  }

  // Reported on every zoom frame and every relayout, deduped. A pan is a
  // stream of transforms and most of them do not change which tiles are on
  // screen; without the comparison the parent re-derives its whole list on
  // every frame of a drag. An empty layout reports nothing at all — "no
  // layout yet" is not "nothing in view", and the difference is a list
  // that blinks empty on load.
  let lastInView = '';
  $effect(() => {
    const t = currentTransform;
    const tiles = placedTiles;
    const inset = insetRight;
    untrack(() => {
      if (!onInViewChange) return;
      void t; void tiles; void inset;
      const ids = computeInView();
      if (!ids) return;
      const key = ids.join(',');
      if (key === lastInView) return;
      lastInView = key;
      onInViewChange(ids);
    });
  });

  // Update labels when selected patch changes.
  $effect(() => {
    const slug = selectedPatchSlug;
    untrack(() => {
      if (placedTiles.length > 0) updateLabels();
    });
  });

  // Keep the role corner marks honest. The role mark lives on the tile now,
  // built once during the tile loop, so joining or following a patch would
  // leave it stale until the next full rebuild — updateLabels' __role
  // comparison used to be the only thing catching that, back when the mark
  // was part of the name badge.
  $effect(() => {
    const roles = myPatchRoles;
    const scope = quiltScope;
    untrack(() => {
      if (cornerMarks.size === 0) return;
      for (const entry of cornerMarks.values()) syncRoleMark(entry);
      updateCornerMarks();
    });
  });


  // Non-null when My Quilt has remote regions: [{key, name, color,
  // reachable, home, children}] — the composed layout draws sashing
  // around each (docs/adr/024).
  let groupsMeta = null;

  async function loadData() {
    loading = true;
    error = '';
    groupsMeta = null;
    firstBuild = true;
    try {
      const resp = await api(`nodes/tree${quiltScope === 'my' ? '?scope=my' : ''}`);
      if (resp.tree) {
        treeData = resp.tree;
        affinityData = resp.affinity || [];
      } else {
        treeData = resp;
        affinityData = [];
      }

      if (quiltScope === 'my') {
        const follows = getRemoteFollows();
        if (follows.length > 0) {
          const remoteGroups = await buildRemoteGroups(follows, {
            fetchInfo: fetchQuiltInfo,
            colorFor: colorForQuilt,
            onLiveNode: refreshFollowSnapshot,
          });
          if (remoteGroups.length > 0) {
            const homeOrigin = window.location.origin;
            const homeInfo = await fetchQuiltInfo(homeOrigin);
            groupsMeta = [
              {
                key: 'home',
                name: homeInfo?.name || 'This quilt',
                color: colorForQuilt(homeOrigin, homeInfo),
                reachable: true,
                home: true,
                children: treeData?.children || [],
              },
              ...remoteGroups,
            ];
            treeData = { children: groupsMeta.flatMap((g) => g.children) };
          }
        }
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function relayout(ids) {
    // Region mode: repacking would dissolve the per-quilt grouping, so
    // filters pop tiles in and out at their fixed positions instead.
    if (groupsMeta) {
      relayoutGrouped(ids);
      return;
    }
    const allChildren = treeData.children;
    const showAll = ids.size === allChildren.length;

    // If showing all and everything is already visible, no-op.
    if (showAll) {
      let allVisible = true;
      for (const [, item] of tileMap) {
        if (!item.tile.isFiller && !item.visible) { allVisible = false; break; }
      }
      if (allVisible) return;
    }

    const filteredChildren = showAll ? allChildren : allChildren.filter(c => ids.has(c.id));

    // Hide labels and cloth during the transition. The seam network belongs
    // to the layout, and mid-slide there is no layout for it to belong to.
    if (labelsEl) labelsEl.style.opacity = '0';
    if (clothG) clothG.style('display', 'none');

    if (filteredChildren.length === 0) {
      // Pop out everything.
      for (const [, item] of tileMap) {
        if (item.visible) {
          const s = item.tile.pxSize;
          const cx = item.tile.px + s / 2;
          const cy = item.tile.py + s / 2;
          item.g.transition().duration(250).ease(d3.easeBackIn.overshoot(0.5))
            .attr('transform', `translate(${cx},${cy}) scale(0)`)
            .style('opacity', 0);
          item.visible = false;
        }
      }
      setTimeout(() => { placedTiles = []; updateLabels(); }, 300);
      return;
    }

    // Build fixedSizes map from initial layout so tiles keep their original sizes.
    const fixedSizes = new Map();
    for (const [id, item] of tileMap) {
      if (!item.tile.isFiller) {
        fixedSizes.set(id, item.tile.currentSize);
      }
    }

    // Run layout with filtered set but fixed sizes.
    const bu = baseUnit;
    lattice = createLattice(bu);
    const layout = quiltLayout(filteredChildren, affinityData, fixedSizes);

    // New positions for non-filler tiles.
    const newPosMap = new Map();
    for (const t of layout.tiles) {
      if (t.isFiller) continue;
      newPosMap.set(t.data.id, {
        nc: t.gridPos.col - layout.minCol,
        nr: t.gridPos.row - layout.minRow,
        px: (t.gridPos.col - layout.minCol) * bu,
        py: (t.gridPos.row - layout.minRow) * bu,
      });
    }

    // New centering offset.
    const { vw, vh } = getContainerSize();
    const padLeft = 0;
    const padRight = Math.round(vw * insetRight);
    const totalW = (layout.maxCol - layout.minCol) * bu;
    const totalH = (layout.maxRow - layout.minRow) * bu;
    const newOX = padLeft + ((vw - padLeft - padRight) - totalW) / 2;
    const newOY = (vh - totalH) / 2;
    const dur = 500;

    // Animate content group centering.
    if (contentG_ref) {
      contentG_ref.transition().duration(dur).ease(d3.easeCubicInOut)
        .attr('transform', `translate(${newOX},${newOY})`);
    }
    canvasOffsetX = newOX;
    canvasOffsetY = newOY;

    // Animate each tile.
    for (const [id, item] of tileMap) {
      if (item.tile.isFiller) {
        // Pop out fillers.
        if (item.visible) {
          const s = item.tile.pxSize;
          const cx = item.tile.px + s / 2;
          const cy = item.tile.py + s / 2;
          item.g.transition().duration(200)
            .attr('transform', `translate(${cx},${cy}) scale(0)`)
            .style('opacity', 0);
          item.visible = false;
        }
        continue;
      }

      const newPos = newPosMap.get(id);

      if (newPos && item.visible) {
        // Slide to new position.
        const s = item.tile.pxSize;
        const newCx = newPos.px + s / 2;
        const newCy = newPos.py + s / 2;
        // Opacity travels with the slide even though a visible tile should
        // already be opaque: relayout can run at the tail of buildLayout (a
        // standing filter), where it cancels the still-pending pop-in — the
        // only transition that was going to raise opacity off its initial 0.
        // Without this the tile keeps its block invisible forever while its
        // label and shadow, both setTimeout-driven, show up as normal.
        item.g.transition().duration(dur).ease(d3.easeCubicInOut)
          .attr('transform', `translate(${newCx},${newCy}) scale(1)`)
          .style('opacity', 1);
        item.tile.nc = newPos.nc;
        item.tile.nr = newPos.nr;
        item.tile.px = newPos.px;
        item.tile.py = newPos.py;
      } else if (newPos && !item.visible) {
        // Pop back in at new position.
        const s = item.tile.pxSize;
        const newCx = newPos.px + s / 2;
        const newCy = newPos.py + s / 2;
        item.g.attr('transform', `translate(${newCx},${newCy}) scale(0)`).style('opacity', 0);
        item.g.transition().delay(200).duration(300).ease(d3.easeBackOut.overshoot(0.6))
          .attr('transform', `translate(${newCx},${newCy}) scale(1)`)
          .style('opacity', 1);
        item.tile.nc = newPos.nc;
        item.tile.nr = newPos.nr;
        item.tile.px = newPos.px;
        item.tile.py = newPos.py;
        item.visible = true;
      } else if (!newPos && item.visible) {
        // Pop out.
        const s = item.tile.pxSize;
        const cx = item.tile.px + s / 2;
        const cy = item.tile.py + s / 2;
        item.g.transition().duration(250).ease(d3.easeBackIn.overshoot(0.5))
          .attr('transform', `translate(${cx},${cy}) scale(0)`)
          .style('opacity', 0);
        item.visible = false;
      }
    }

    // Zoom to fit visible tiles — runs in parallel with tile slides.
    // Use the NEW positions (already set on item.tile.px/py above).
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const [, item] of tileMap) {
      if (!item.visible || item.tile.isFiller) continue;
      const s = item.tile.pxSize;
      minX = Math.min(minX, item.tile.px);
      minY = Math.min(minY, item.tile.py);
      maxX = Math.max(maxX, item.tile.px + s);
      maxY = Math.max(maxY, item.tile.py + s);
    }

    if (minX < Infinity && svgSelection && zoomBehavior) {
      const bw = maxX - minX;
      const bh = maxY - minY;
      const { fitPadLeft, padding, narrow } = fitInsets(vw);
      const fitPadRight = Math.round(vw * insetRight);
      const availW = vw - fitPadLeft - fitPadRight - padding * 2;
      const availH = vh - padding * 2;
      const targetK = Math.min(availW / bw, availH / bh, 2.0);
      const clampedK = clampFitScale(targetK, narrow);

      const bcx = newOX + (minX + maxX) / 2;
      const bcy = newOY + (minY + maxY) / 2;
      const tx = (vw + fitPadLeft - fitPadRight) / 2 - bcx * clampedK;
      const ty = vh / 2 - bcy * clampedK;

      const targetTransform = d3.zoomIdentity.translate(tx, ty).scale(clampedK);

      svgSelection.transition('zoomFit').duration(dur).ease(d3.easeCubicInOut)
        .call(zoomBehavior.transform, targetTransform);
    }

    // After everything settles: re-cut the fabric where it landed, then lay
    // the cloth back over it.
    setTimeout(() => {
      rebuildCloth(layout, bu);

      placedTiles = [...tileMap.values()]
        .filter(item => !item.tile.isFiller && item.visible)
        .map(item => item.tile);
      updateLabels();
      if (labelsEl) {
        labelsEl.style.transition = 'opacity 300ms ease';
        labelsEl.style.opacity = '1';
      }
    }, dur + 100);
  }

  // Simplified relayout for region mode: no repack, tiles keep their
  // positions inside their sashing frames.
  function relayoutGrouped(ids) {
    if (labelsEl) labelsEl.style.opacity = '0';
    for (const [id, item] of tileMap) {
      if (item.tile.isFiller) continue;
      const show = ids.has(id);
      const s = item.tile.pxSize;
      const cx = item.tile.px + s / 2;
      const cy = item.tile.py + s / 2;
      if (show && !item.visible) {
        item.g.transition().duration(300).ease(d3.easeBackOut.overshoot(0.6))
          .attr('transform', `translate(${cx},${cy}) scale(1)`)
          .style('opacity', 1);
        item.visible = true;
      } else if (!show && item.visible) {
        item.g.transition().duration(250).ease(d3.easeBackIn.overshoot(0.5))
          .attr('transform', `translate(${cx},${cy}) scale(0)`)
          .style('opacity', 0);
        item.visible = false;
      }
    }
    setTimeout(() => {
      placedTiles = [...tileMap.values()]
        .filter(item => !item.tile.isFiller && item.visible)
        .map(item => item.tile);
      updateLabels();
      if (labelsEl) {
        labelsEl.style.transition = 'opacity 300ms ease';
        labelsEl.style.opacity = '1';
      }
    }, 400);
  }

  function buildLayout() {
    if (!containerEl || !treeData?.children?.length) return;

    const { vw, vh } = getContainerSize();
    // The container is bound in the same flush that data arrives, so it can
    // still measure 0x0 here. Building against that produces a 0x0 svg that
    // nothing recovers from — bail and retry on the next frame, as soon as
    // the browser has laid the container out. (The ResizeObserver would also
    // catch it eventually, but only after its 150ms debounce, which reads as
    // a blank quilt.)
    if (!vw || !vh) {
      if (buildRetryFrame === null && buildRetries < 60) {
        buildRetries++;
        buildRetryFrame = requestAnimationFrame(() => {
          buildRetryFrame = null;
          if (!layoutBuilt) buildLayout();
        });
      }
      return;
    }
    buildRetries = 0;

    layoutBuilt = true;
    const animate = firstBuild;
    firstBuild = false;
    lastBuiltW = vw;
    lastBuiltH = vh;
    currentTransform = d3.zoomIdentity;
    if (labelsEl) labelsEl.style.opacity = '0';
    tileMap = new Map();

    d3.select(containerEl).selectAll('svg').remove();

    const padLeft = 0;
    const padRight = Math.round(vw * insetRight);

    const allChildren = treeData.children;
    baseUnit = computeBaseUnit(vw, vh, allChildren.length);

    const layout = groupsMeta
      ? composeGroupLayouts(groupsMeta, new Map([['home', affinityData]]))
      : quiltLayout(allChildren, affinityData);
    const bu = baseUnit;
    lattice = createLattice(bu);

    const pixelTiles = layout.tiles.map(t => ({
      ...t,
      nc: t.gridPos.col - layout.minCol,
      nr: t.gridPos.row - layout.minRow,
      px: (t.gridPos.col - layout.minCol) * bu,
      py: (t.gridPos.row - layout.minRow) * bu,
      pxSize: t.currentSize * bu,
    }));

    const totalW = (layout.maxCol - layout.minCol) * bu;
    const totalH = (layout.maxRow - layout.minRow) * bu;

    // Center in the visible (non-overlaid) area of the viewport.
    const oX = padLeft + ((vw - padLeft - padRight) - totalW) / 2;
    const oY = (vh - totalH) / 2;
    canvasOffsetX = oX;
    canvasOffsetY = oY;

    // Center of quilt in pixel space (for stagger distance calc).
    const centerX = totalW / 2;
    const centerY = totalH / 2;

    const svg = d3.select(containerEl).append('svg')
      .attr('width', vw).attr('height', vh);

    const zoomG = svg.append('g');
    const contentG = zoomG.append('g').attr('transform', `translate(${oX},${oY})`);
    contentG_ref = contentG;

    // --- SASHING (docs/adr/024) ---
    // The strip framing each source quilt's region in My Quilt, colored
    // by that quilt's own branding color. Quilts are peers: once two
    // regions exist, every region gets sashing — home included. A
    // single-region quilt draws none.
    if (layout.groups && layout.groups.length > 1) {
      const sashG = contentG.append('g').attr('class', 'sashing');
      for (const gr of layout.groups) {
        const pad = bu * 0.4;
        const x = gr.minCol * bu - pad;
        const y = gr.minRow * bu - pad;
        const w = (gr.maxCol - gr.minCol) * bu + pad * 2;
        const h = (gr.maxRow - gr.minRow) * bu + pad * 2;
        const strokeW = Math.max(5, Math.round(bu * 0.14));

        sashG.append('rect')
          .attr('x', x).attr('y', y).attr('width', w).attr('height', h)
          .attr('fill', 'none')
          .attr('stroke', gr.color)
          .attr('stroke-width', strokeW)
          .attr('rx', strokeW * 1.5)
          .attr('stroke-dasharray', gr.reachable ? null : `${strokeW * 2} ${strokeW * 1.4}`)
          .style('opacity', 0.85);

        // Name tab on the sash's top edge. An unreachable quilt says so
        // right on the frame — the region renders from snapshots.
        const labelText = gr.name + (gr.reachable ? '' : ' · unreachable');
        const tabH = Math.max(20, Math.round(bu * 0.42));
        const fontSize = Math.round(tabH * 0.52);
        const tabW = Math.round(labelText.length * fontSize * 0.62) + tabH;
        const tab = sashG.append('g')
          .attr('transform', `translate(${x + strokeW * 1.5},${y - tabH / 2})`);
        tab.append('rect')
          .attr('width', tabW).attr('height', tabH)
          .attr('rx', tabH / 2)
          .attr('fill', gr.color)
          .style('opacity', gr.reachable ? 1 : 0.7);
        tab.append('text')
          .attr('x', tabW / 2).attr('y', tabH / 2 + 1)
          .attr('text-anchor', 'middle').attr('dominant-baseline', 'central')
          .attr('fill', textOnColor(gr.color))
          .attr('font-size', fontSize)
          .attr('font-weight', 700)
          .attr('font-family', "'Space Grotesk Variable', system-ui, sans-serif")
          .text(labelText);
      }
    }

    const tileGroups = []; // For staggered animation.
    cornerMarks = new Map(); // Re-collected below; the old svg is already gone.

    // Cloth geometry, collected across tiles and emitted once per paint.
    // Grouped into spatial chunks because a path is only skipped when its
    // bounding box misses the viewport — one path spanning the whole quilt is
    // a path that rasterises on every frame however little of it is on screen.
    const weaveChunks = new Map();
    let quiltSilhouette = '';

    // --- RENDER TILES ---
    for (const tile of pixelTiles) {
      const s = tile.pxSize;
      const tileCx = tile.px + s / 2;
      const tileCy = tile.py + s / 2;
      const sz = tile.currentSize;

      // Create group at tile center — scaled to 0 for the pop-in, or already
      // placed when this build is a rebuild that shouldn't re-animate.
      const g = contentG.append('g')
        .attr('class', tile.isFiller ? 'filler' : 'tile')
        .attr('transform', `translate(${tileCx},${tileCy}) scale(${animate ? 0 : 1})`)
        .style('cursor', (!interactive || tile.isFiller) ? 'default' : 'pointer')
        .style('opacity', animate ? 0 : 1);

      // Inner group offset so content draws from top-left.
      const inner = g.append('g').attr('transform', `translate(${-s/2},${-s/2})`);

      // Distance from center for stagger ordering.
      const dist = Math.sqrt((tileCx - centerX) ** 2 + (tileCy - centerY) ** 2);

      const filler = tile.isFiller;
      quiltSilhouette += paintFabric(tile, inner, weaveChunks);

      if (filler) {
        tileGroups.push({ g, dist, tile });
      } else {
        // Corner marks (CONTEXT.md "Corner mark"). A static hero has no room
        // for them and no zoom to reveal them at — same reason it draws no
        // name badges (docs/adr/040).
        if (interactive) {
          const entry = { tile, inner, role: undefined, slots: new Map() };
          cornerMarks.set(tile.data.id, entry);
          // Identity: the patch's motif on its own color, on every tile —
          // the quilt says what its patches are at zoom levels where no name
          // badge survives a collision.
          const identity = identityColorForPatch(tile.data);
          drawCornerMark(entry, 'motif', 'tl', identity,
            createMotifGroup(tile.data, MARK_GLYPH_PX, textOnColor(identity)));
          // Status: unclaimed (docs/adr/030) — a broken chain link, a patch
          // on the quilt with nobody holding the other end.
          if (tile.data.is_unclaimed) {
            drawCornerMark(entry, 'unclaimed', 'tr', MARK_STATUS_FILL,
              createUnclaimedMarkGroup(MARK_GLYPH_PX, '#fff'));
          }
          syncRoleMark(entry);
        }

        // Hover overlay (starts transparent, darkens on hover).
        inner.append('rect').attr('class', 'overlay')
          .attr('width', s).attr('height', s)
          .attr('fill', 'transparent')
          .style('pointer-events', 'none');

        // Hover + click — a static hero has nowhere for a tooltip to land
        // and nothing to navigate to, so it skips gestures entirely rather
        // than swallowing them silently.
        if (interactive) {
          g.on('mouseenter', function(event) {
            d3.select(this).select('.overlay').attr('fill', 'var(--color-overlay-hover)');
            if (onPatchHover) onPatchHover(tile.data);
            if (tooltip && !labeledPatchIds.has(tile.data.id)) {
              showTooltip(tile.data, event.clientX, event.clientY);
            }
          })
          .on('mousemove', function(event) {
            if (tooltip && tooltip.style.display === 'block') {
              tooltip.style.left = event.clientX + 14 + 'px';
              tooltip.style.top = event.clientY - 10 + 'px';
            }
          })
          .on('mouseleave', function() {
            d3.select(this).select('.overlay').attr('fill', 'transparent');
            if (onPatchHover) onPatchHover(null);
            if (tooltip) tooltip.style.display = 'none';
          })
          .on('click', function() {
            if (tile.data.slug) onPatchClick(tile.data.slug, tile.data._source || null);
          });
        }

        tileGroups.push({ g, dist, tile });
      }
    }

    // --- CLOTH ---
    // Everything above the fabric and shared across tiles: the weave, the
    // folds, and the seam ink. These layers are rebuilt whenever the layout
    // is, and stand down while the quilt is moving.
    buildCloth(contentG, layout, quiltSilhouette, weaveChunks, bu);

    // --- STAGGERED POP-IN ANIMATION ---
    // Sort by distance from center: closest tiles pop in first.
    tileGroups.sort((a, b) => a.dist - b.dist);
    const maxDist = tileGroups.length > 0 ? tileGroups[tileGroups.length - 1].dist : 1;
    const totalAnimDuration = 600; // Total stagger window in ms.
    const tileAnimDuration = 350; // Each tile's pop duration.

    if (animate) {
      tileGroups.forEach((item, i) => {
        const delay = (item.dist / (maxDist || 1)) * totalAnimDuration;
        const s = item.tile.pxSize;
        const cx = item.tile.px + s / 2;
        const cy = item.tile.py + s / 2;

        item.g
          .transition()
          .delay(delay)
          .duration(tileAnimDuration)
          .ease(d3.easeBackOut.overshoot(0.6))
          .attr('transform', `translate(${cx},${cy}) scale(1)`)
          .style('opacity', 1);
      });

      // The cloth is a property of the whole quilt, so it has nothing to say
      // until the whole quilt is there. Drawn at full strength from the
      // start, the seams, folds and weave hang in empty space while the tiles
      // are still flying in — a finished surface over a quilt that isn't
      // sewn yet. It fades in on the same beat the labels do.
      if (clothG) {
        clothG.style('opacity', 0)
          .transition()
          .delay(totalAnimDuration + tileAnimDuration * 0.6)
          .duration(400)
          .style('opacity', 1);
      }
    }

    // Store in tileMap keyed by patch ID (or filler ID).
    for (const item of tileGroups) {
      const id = item.tile.isFiller ? item.tile.id : item.tile.data.id;
      item.visible = true;
      tileMap.set(id, item);
    }

    // Show labels after all tiles have popped in. Snapshot from tileMap's
    // visibility flags, not pixelTiles: a standing filter (applied by the
    // relayout call at the end of this function) may have hidden tiles by
    // the time this fires, and labeling hidden tiles would undo it.
    const labelsDelay = animate ? totalAnimDuration + tileAnimDuration : 0;
    setTimeout(() => {
      placedTiles = [...tileMap.values()]
        .filter(item => !item.tile.isFiller && item.visible)
        .map(item => item.tile);
      updateLabels();
      if (labelsEl) {
        labelsEl.style.transition = 'opacity 300ms ease';
        labelsEl.style.opacity = '1';
      }
    }, labelsDelay);

    // --- ZOOM ---
    zoomBehavior = d3.zoom()
      .scaleExtent([0.3, 6])
      .filter(event => event.type !== 'dblclick')
      .on('zoom', (event) => {
        zoomG.attr('transform', event.transform);
        currentTransform = event.transform;
        updateLabels();
        updateCornerMarks();
        updateSeamRidge();
        applyClothTiers();
        parkWeaveDuringZoom();
      });

    svgSelection = svg;
    // Static mode never binds the interaction listeners, but zoomBehavior
    // still owns the element's transform state, so the programmatic
    // fit-to-view call below (and any later relayout) works either way.
    if (interactive) svg.call(zoomBehavior);

    // Default zoom: fit the quilt to the visible area rather than starting at
    // identity. A small or uniformly-sized quilt otherwise renders too small
    // for any label to clear the reveal threshold.
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const tile of pixelTiles) {
      if (tile.isFiller) continue;
      minX = Math.min(minX, tile.px);
      minY = Math.min(minY, tile.py);
      maxX = Math.max(maxX, tile.px + tile.pxSize);
      maxY = Math.max(maxY, tile.py + tile.pxSize);
    }

    if (minX < Infinity) {
      const { fitPadLeft, padding, narrow } = fitInsets(vw);
      const fitPadRight = padRight;
      const availW = vw - fitPadLeft - fitPadRight - padding * 2;
      const availH = vh - padding * 2;
      const targetK = Math.min(availW / (maxX - minX), availH / (maxY - minY), 2.0);
      const clampedK = clampFitScale(targetK, narrow);

      const bcx = oX + (minX + maxX) / 2;
      const bcy = oY + (minY + maxY) / 2;
      const tx = (vw + fitPadLeft - fitPadRight) / 2 - bcx * clampedK;
      const ty = vh / 2 - bcy * clampedK;

      svg.call(zoomBehavior.transform,
        d3.zoomIdentity.translate(tx, ty).scale(clampedK));
    }

    // The fit above dispatches a zoom event, which sizes the marks — but a
    // quilt with no non-filler tiles never reaches it, and neither does one
    // built at identity. Cheap enough to just be sure.
    updateCornerMarks();

    // A filter or search can already be standing when the layout builds —
    // arriving from another discovery surface with tags active (the filter
    // persists, docs/adr/022), or a rebuild after resize. The relayout
    // effect can't catch this (visibleIds settled before tileMap existed),
    // so apply it here; relayout's transitions supersede the pop-in cleanly.
    const standingIds = visibleIds;
    if (standingIds.size !== allChildren.length) {
      relayout(standingIds);
    }
  }

  function showTooltip(data, x, y) {
    if (!tooltip) return;
    const desc = escapeHtml(data.description?.slice(0, 140));
    const tags = (data.tags || []).map(escapeHtml).join(', ');
    tooltip.style.display = 'block';
    tooltip.style.left = x + 14 + 'px';
    tooltip.style.top = y - 10 + 'px';
    // Same word as the mark on the tile (CONTEXT.md "Unclaimed mark") — the
    // chip names the patch's state, not the provenance of what's on it.
    const unclaimedTag = data.is_unclaimed ? '<span class="tip-unclaimed">Unclaimed</span>' : '';
    tooltip.innerHTML = `
      <strong>${escapeHtml(data.name)}</strong>
      ${unclaimedTag}
      ${desc ? `<div class="tip-desc">${desc}${(data.description?.length || 0) > 140 ? '\u2026' : ''}</div>` : ''}
      <div class="tip-meta">
        ${tags ? `<span class="tip-tags">${tags}</span>` : ''}
        <span>${data.is_unclaimed ? `${data.follower_count || 0} following` : `${data.member_count || 0} members`} &middot; ${data.event_count || 0} events</span>
      </div>`;
  }

  // --- LABEL DRAG GESTURES ---
  // Name badges are DOM elements stacked above the svg, so d3's zoom never
  // sees a press that lands on one — every badge was a dead spot: on a phone
  // the finger, and on a desktop the cursor, had to find bare fabric to pan.
  // Drive the zoom behavior by hand instead, and only count the sequence as a
  // tap/click if the pointer stayed put.
  //
  // The move/end listeners live on window, since the finger or cursor leaves
  // the badge immediately. For a mouse that is all it takes. Touch is stricter:
  // every event in a touch sequence is addressed to the element that received
  // the touchstart, for the life of the sequence, so it only reaches window
  // while that element is still in the document. A badge normally survives a
  // pan (updateLabels moves badges rather than rebuilding them), but any pass
  // can still drop one: it collides with a bigger label, leaves the viewport,
  // or loses its data to a refetch. When that happens to the badge under the
  // finger it stays on as a holdover — hidden, inert, removed when the finger
  // lifts. Without it the pan stops dead the moment the badge goes.
  const TAP_SLOP = 10; // px of travel still forgiven as a tap
  const DRAG_SLOP = 4; // a mouse is steadier than a finger — commit sooner
  let labelGesture = null;
  let labelGestureMoved = false;
  // Badges owed to a live touch sequence — one per finger that landed on one.
  let labelHoldovers = new Set();

  function touchById(list, id) {
    for (const t of list) if (t.identifier === id) return t;
    return null;
  }

  function touchDistance(a, b) {
    return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
  }

  function panQuiltBy(dxScreen, dyScreen) {
    if (!svgSelection || !zoomBehavior) return;
    const k = currentTransform.k || 1;
    svgSelection.interrupt('zoomFit');
    zoomBehavior.translateBy(svgSelection, dxScreen / k, dyScreen / k);
  }

  function zoomQuiltBy(ratio, clientX, clientY) {
    if (!svgSelection || !zoomBehavior) return;
    const node = svgSelection.node();
    if (!node) return;
    const rect = node.getBoundingClientRect();
    svgSelection.interrupt('zoomFit');
    zoomBehavior.scaleBy(svgSelection, ratio, [clientX - rect.left, clientY - rect.top]);
  }

  function onLabelTouchMove(event) {
    if (!labelGesture) return;
    if (labelGesture.mode === 'pan') {
      const t = touchById(event.touches, labelGesture.id);
      if (!t) return;
      const dx = t.clientX - labelGesture.x;
      const dy = t.clientY - labelGesture.y;
      // Under the slop the finger is still resolving into a tap — holding the
      // anchor here keeps a shaky tap from nudging the quilt.
      if (!labelGestureMoved && Math.hypot(dx, dy) < TAP_SLOP) return;
      labelGestureMoved = true;
      labelGesture.x = t.clientX;
      labelGesture.y = t.clientY;
      panQuiltBy(dx, dy);
    } else {
      const a = touchById(event.touches, labelGesture.ids[0]);
      const b = touchById(event.touches, labelGesture.ids[1]);
      if (!a || !b) return;
      const dist = touchDistance(a, b);
      if (labelGesture.dist > 0 && dist > 0) {
        zoomQuiltBy(dist / labelGesture.dist, (a.clientX + b.clientX) / 2, (a.clientY + b.clientY) / 2);
      }
      labelGesture.dist = dist;
    }
  }

  function onLabelMouseMove(event) {
    if (!labelGesture || labelGesture.mode !== 'drag') return;
    // The button went up somewhere we couldn't hear it (outside the window,
    // over a native menu) — don't keep dragging the quilt around.
    if (!(event.buttons & 1)) {
      endLabelGesture();
      return;
    }
    const dx = event.clientX - labelGesture.x;
    const dy = event.clientY - labelGesture.y;
    if (!labelGestureMoved && Math.hypot(dx, dy) < DRAG_SLOP) return;
    labelGestureMoved = true;
    labelGesture.x = event.clientX;
    labelGesture.y = event.clientY;
    if (tooltip) tooltip.style.display = 'none'; // a drag isn't a hover
    panQuiltBy(dx, dy);
  }

  // Re-attached by updateLabels on every rebuild that happens mid-gesture, so
  // the touch sequence addressed to this badge keeps reaching window.
  function holdLabelForGesture(el) {
    labelHoldovers.add(el);
  }

  // Called once the gesture is over: a badge that was only being kept alive for
  // it has no business on screen. A badge that never lost its rebuild race is
  // still the live one — leave it be.
  function releaseLabelHoldovers() {
    for (const el of labelHoldovers) {
      if (el.classList.contains('gesture-holdover')) el.remove();
    }
    labelHoldovers.clear();
  }

  function detachLabelGestureListeners() {
    labelGesture = null;
    window.removeEventListener('touchmove', onLabelTouchMove);
    window.removeEventListener('touchend', endLabelGesture);
    window.removeEventListener('touchcancel', endLabelGesture);
    window.removeEventListener('mousemove', onLabelMouseMove);
    window.removeEventListener('mouseup', endLabelGesture);
  }

  function endLabelGesture() {
    detachLabelGestureListeners();
    releaseLabelHoldovers();
  }

  function attachLabelGestures(el) {
    el.addEventListener('touchstart', (event) => {
      // Only the listeners are reset here, not the holdovers: a second finger
      // landing on another badge promotes the gesture to a pinch, and the first
      // finger's badge is still the address its moves are delivered to.
      detachLabelGestureListeners();
      const touches = event.touches;
      if (touches.length === 1) {
        labelGesture = {
          mode: 'pan',
          id: touches[0].identifier,
          x: touches[0].clientX,
          y: touches[0].clientY,
        };
        labelGestureMoved = false;
      } else if (touches.length === 2) {
        labelGesture = {
          mode: 'pinch',
          ids: [touches[0].identifier, touches[1].identifier],
          dist: touchDistance(touches[0], touches[1]),
        };
        labelGestureMoved = true; // a pinch is never a tap
      } else {
        return; // Nothing to drive, so nothing to keep alive either.
      }
      holdLabelForGesture(el);
      window.addEventListener('touchmove', onLabelTouchMove, { passive: true });
      window.addEventListener('touchend', endLabelGesture, { passive: true });
      window.addEventListener('touchcancel', endLabelGesture, { passive: true });
    }, { passive: true });

    el.addEventListener('mousedown', (event) => {
      if (event.button !== 0) return; // left button only, as d3's zoom does
      endLabelGesture();
      labelGesture = { mode: 'drag', x: event.clientX, y: event.clientY };
      labelGestureMoved = false;
      // Keeps the drag from turning into a text selection of the patch name.
      // The click still fires on mouseup, which is where the tap/drag split
      // gets settled.
      event.preventDefault();
      window.addEventListener('mousemove', onLabelMouseMove);
      window.addEventListener('mouseup', endLabelGesture);
    });
  }

  /**
   * Wear one of the name's shapes: the line clamp it was measured at, and for
   * a wrapped name the measured balanced width, so the pill hugs its text
   * instead of every wrapped name rendering at the full cap. The clamp has to
   * travel with the measurement: a name measured at three lines and clamped at
   * two renders an ellipsis inside a pill built tall enough to hold it.
   *
   * Applied on every pass rather than baked in at build time, because the
   * shape is the tile's call and the tile is a zoom away from a different one.
   * Written in place rather than by rebuilding the badge: a rebuild during a
   * pinch drops the element the touch sequence is being delivered to.
   */
  function applyBadgeShape(label, textW, lines) {
    if (label.__textW === textW && label.__lines === lines) return;
    const nameSpan = label.firstChild;
    nameSpan.style.webkitLineClamp = String(lines);
    nameSpan.style.width = lines > 1 ? textW + 'px' : '';
    nameSpan.style.maxWidth = lines > 1 ? '' : badgeType().textMax + 'px';
    label.__textW = textW;
    label.__lines = lines;
  }

  // Build one badge. Everything in here is fixed for as long as the patch's data
  // and the viewer's role in it are: there are six listeners to bind, which is
  // why updateLabels reuses these rather than building a fresh set on every
  // zoom tick.
  //
  // The badge is a name and nothing else — the motif and the role mark are
  // corner marks on the tile itself now, where they show on every tile rather
  // than only the ones that win a label collision.
  function createLabelElement(tile, textW, lines, role) {
    const label = document.createElement('div');
    label.className = 'patch-label lt-vellum';

    // Text: name only. The shape it wears is applyBadgeShape's, here and on
    // every later pass.
    const nameSpan = document.createElement('span');
    nameSpan.className = 'label-name';
    nameSpan.textContent = tile.data.name || '';
    label.appendChild(nameSpan);
    applyBadgeShape(label, textW, lines);

    // Label hover → tooltip + click → select patch.
    const tileData = tile.data;
    label.addEventListener('mouseenter', (event) => {
      showTooltip(tileData, event.clientX, event.clientY);
    });
    label.addEventListener('mousemove', (event) => {
      if (tooltip && tooltip.style.display === 'block') {
        tooltip.style.left = event.clientX + 14 + 'px';
        tooltip.style.top = event.clientY - 10 + 'px';
      }
    });
    label.addEventListener('mouseleave', () => {
      if (tooltip) tooltip.style.display = 'none';
    });
    label.addEventListener('click', () => {
      // A pan that started on this badge still ends in a click (synthesized
      // on touch, native on mouseup) — dragging the quilt shouldn't open
      // whatever happened to be under the pointer.
      if (labelGestureMoved) {
        labelGestureMoved = false;
        return;
      }
      if (tileData.slug) onPatchClick(tileData.slug, tileData._source || null);
    });
    // Touch: pan and pinch the quilt even when the finger lands on a badge.
    attachLabelGestures(label);
    // Forward wheel events to the SVG so zoom works while hovering labels.
    label.addEventListener('wheel', (event) => {
      const svg = containerEl?.querySelector('svg');
      if (svg) svg.dispatchEvent(new WheelEvent(event.type, event));
    }, { passive: true });

    // What the badge was built from, so a later pass can tell whether it still
    // matches (compared by identity — a refetch hands out new data objects).
    // __role no longer changes what the badge draws; it stays as a reuse
    // marker so nothing built under one relationship outlives it.
    label.__data = tile.data;
    label.__role = role;
    return label;
  }

  // A badge that is no longer wanted on screen. Normally it goes; if a finger is
  // still on it, it stays in the tree as a holdover instead (see LABEL DRAG
  // GESTURES) so the touch sequence keeps arriving somewhere live.
  function dropLabelElement(el) {
    if (labelHoldovers.has(el)) el.classList.add('gesture-holdover');
    else el.remove();
  }

  function dropAllLabels() {
    for (const el of labelEls.values()) dropLabelElement(el);
    labelEls.clear();
  }

  function updateLabels() {
    if (!labelsEl) return;
    if (!showLabels) { dropAllLabels(); return; }

    const t = currentTransform;
    const k = t.k;
    const { vw, vh } = getContainerSize();

    // Who held a badge on the last pass. A zoom is a stream of these, and the
    // set that wins collision is not stable across it: two tiles a few pixels
    // apart in size swap priority as the quilt scales, and each swap reads on
    // screen as one name blinking out and another blinking in. An incumbent
    // therefore goes first and keeps its spot for as long as it can still hold
    // a badge at all — it still loses one to the size floor or the viewport
    // edge, which are the honest reasons to lose one.
    const heldBadge = labeledPatchIds;
    labeledPatchIds = new Set();

    // Progressive reveal by ON-SCREEN size: a tile earns a label once it is
    // physically big enough to hold one. Keying off the world size instead
    // meant a quilt of uniformly-sized tiles (no member/event spread to grow
    // them) never crossed the threshold at any zoom level.
    //
    // Earning one and keeping one are different sizes — see LABEL_KEEP_PX.

    // Incumbents first, then by size descending so larger labels get priority
    // in collision detection.
    const sortedTiles = [...placedTiles].sort((a, b) =>
      (heldBadge.has(b.data.id) ? 1 : 0) - (heldBadge.has(a.data.id) ? 1 : 0)
      || b.pxSize - a.pxSize);

    // Track placed label bounding boxes for collision avoidance.
    const labelRects = [];

    for (const tile of sortedTiles) {
      const tilePx = tile.pxSize;
      const screenPx = tilePx * k;
      const held = heldBadge.has(tile.data.id);
      if (screenPx < (held ? LABEL_KEEP_PX : LABEL_MIN_PX)) continue;

      // World coordinates (pre-zoom) — offset includes the centering transform.
      const worldX = canvasOffsetX + tile.px + tilePx / 2;
      const worldY = canvasOffsetY + tile.py + tilePx / 2;

      // Apply d3 zoom transform: screen = transform.x + world * transform.k
      const screenX = t.x + worldX * t.k;
      const screenY = t.y + worldY * t.k;

      // Skip off-screen labels.
      if (screenX < -150 || screenX > vw + 150 ||
          screenY < -50 || screenY > vh + 50) continue;

      // Nothing but the name is in the pill — motif and role mark are corner
      // marks on the tile.
      const name = tile.data.name || '';
      const role = quiltScope === 'local' ? myPatchRoles.get(tile.data.slug) : undefined;
      // Footprint from the pill's own computed box, not restated constants —
      // it is rem-based and the reader can move it.
      const { chromeX, chromeY, lineH } = badgeType();

      // How much visible quilt this badge owes its neighbours: the full gap to
      // move in, and to stay, a gap that slides toward LABEL_KEEP_GAP as its
      // tile grows past the floor. Same hysteresis as the size floor above and
      // for the same reason — the badges on screen drift together as the quilt
      // scales, and a name should not blink off the moment two pills come a
      // pixel closer than a fresh pair would be allowed — but a name only
      // earns that leniency while there is a tile under it worth reading.
      const relax = held
        ? Math.max(0, Math.min(1, (screenPx - LABEL_MIN_PX) / (LABEL_ROOMY_PX - LABEL_MIN_PX)))
        : 0;
      const gap = LABEL_GAP + (LABEL_KEEP_GAP - LABEL_GAP) * relax;

      // Which shape the name wears is decided HERE, against the badges already
      // on screen, rather than by the name alone: a pill that doesn't fit
      // stacks onto another line and asks again, and only a name that can't
      // fit at any depth gives up its badge.
      //
      // Wrapping is the right currency to pay in because the two dimensions
      // are not equally scarce. Tiles are square, a pill is wide and short, so
      // width is what runs out first; a line costs ~17px of the height nobody
      // was using and can buy 40+px of the width everybody wants. Shallowest
      // first, so nothing wraps that didn't have to: a big tile at a close
      // zoom still wears the flat single-line pill it wears today.
      let shape = null;
      let rect = null;
      for (const candidate of badgeShapes(name)) {
        const labelW = candidate.textW + chromeX;
        const labelH = chromeY + candidate.lines * lineH;
        const box = {
          x: screenX - labelW / 2,
          y: screenY - labelH / 2,
          w: labelW,
          h: labelH,
        };

        // Collision check against already-placed labels, each of which only
        // clears when there is `gap` of quilt between the two pills.
        let collides = false;
        for (const existing of labelRects) {
          if (box.x - gap < existing.x + existing.w &&
              box.x + box.w + gap > existing.x &&
              box.y - gap < existing.y + existing.h &&
              box.y + box.h + gap > existing.y) {
            collides = true;
            break;
          }
        }
        if (collides) continue; // Try a deeper, narrower stack of the name.
        shape = candidate;
        rect = box;
        break;
      }

      // No shape of this name clears its neighbours — a higher-priority label
      // is already here.
      if (!shape) continue;
      const { textW, lines } = shape;

      labelRects.push(rect);
      labeledPatchIds.add(tile.data.id);

      // Reuse this patch's badge if it still matches what it was built from.
      // A pan is a stream of these passes, and on all but the first the answer
      // is yes for every badge on screen — leaving position as the only thing
      // that has to be written.
      const id = tile.data.id;
      let label = labelEls.get(id);
      if (label && (label.__data !== tile.data || label.__role !== role)) {
        dropLabelElement(label); // may be under a finger, so not a plain remove
        labelEls.delete(id);
        label = null;
      }
      if (!label) {
        label = createLabelElement(tile, textW, lines, role);
        labelEls.set(id, label);
      }
      if (label.parentNode !== labelsEl) labelsEl.appendChild(label);

      applyBadgeShape(label, textW, lines);
      label.classList.toggle('selected', tile.data.slug === selectedPatchSlug);
      label.style.left = screenX + 'px';
      label.style.top = screenY + 'px';
    }

    // Badges whose patch didn't earn one this pass: too small now, collided
    // with a bigger one, or panned off the edge.
    for (const [id, el] of labelEls) {
      if (labeledPatchIds.has(id)) continue;
      labelEls.delete(id);
      dropLabelElement(el);
    }
  }

  /**
   * Draw one corner mark: a disc in a tile corner carrying a glyph.
   *
   * Deliberately NOT clipped by the tile's raw-edge clipPath: a mark is a
   * status overlay, not fabric (same as label badges), and a userSpaceOnUse
   * clipPath referenced from this translated group would be evaluated in
   * badge-local space, slicing the circle (issue #14).
   *
   * The group is anchored *on* its corner and draws inward, so
   * updateCornerMarks can hold it at a fixed screen size by counter-scaling
   * around that anchor — the corner is the one point that must not drift.
   */
  /**
   * Everything above the fabric: the weave, the folds, and the seam ink.
   *
   * All of it is shared between tiles rather than owned by any one of them,
   * which is the point — a seam is a property of a boundary, a fold spans the
   * whole quilt, and a weave belongs to a fabric. Each layer is split into
   * spatial chunks so its paths carry bounding boxes small enough to be
   * skipped when they scroll off screen.
   */
  /**
   * Draw one tile's fabric: its block, warped onto the shared lattice.
   *
   * Kept out of the build loop because a filter repacks the quilt, and a tile
   * that lands in a different grid cell has to be re-warped there — its baked
   * edge was cut to fit the seams of the cell it left, and carrying it across
   * by transform would open the very gaps this system exists to close.
   *
   * Returns the tile's outline so the caller can accumulate the quilt's
   * silhouette, and adds this tile's cuts to the weave chunks.
   */
  function paintFabric(tile, inner, weaveChunks) {
    const sz = tile.currentSize;
    const filler = tile.isFiller;
    const fillerIdx = filler ? (parseInt(tile.id.split('-')[1]) || 0) : 0;
    const palette = filler
      ? ghostPalette(fillerIdx)
      : paletteForPatch(tile.data.id, tile.data.appearance);
    const appearance = filler ? { block: blockKeyAt(fillerIdx) } : tile.data.appearance;
    const patchId = filler ? tile.id : tile.data.id;
    const rotation = filler ? 0 : getRotation(tile.data.id, tile.data.appearance);

    // The block stretches onto the lattice rather than being drawn square and
    // clipped. Clipping leaves the fabric short wherever a seam bows outward
    // — measured at 23% of the tile boundary uncovered — and the clip path
    // itself was one masking operation per tile, on every frame.
    const warp = lattice.makeWarp(tile.nc, tile.nr, sz, rotation);
    const pieces = blockPieces(patchId, palette, appearance);

    // Batch by fabric within the tile: pieces cut from one fabric become one
    // path, so an edge interior to that fabric stops being an edge — and the
    // seal stroke below is paid once per fabric rather than once per piece.
    const byFabric = new Map();
    for (const piece of pieces) {
      const d = warpPolygon(piece.pts, warp, sz, CLOTH.quality);
      byFabric.set(piece.fill, (byFabric.get(piece.fill) || '') + d);
      if (!filler && weaveChunks) {
        const wi = weaveFor(piece.fill);
        const key = `${lattice.chunkKey(tile.nc, tile.nr)}|${wi}`;
        const cut = weaveChunks.get(key);
        if (cut) cut.d += d;
        else weaveChunks.set(key, { wi, grain: fabricGrain(piece.fill), d });
      }
    }

    inner.selectAll('.fabric').remove();
    // Warped geometry is in world coordinates; this group carries it back
    // into the tile-local space the corner marks are anchored in.
    const blockG = inner.insert('g', ':first-child')
      .attr('class', 'fabric')
      .attr('transform', `translate(${-tile.px},${-tile.py})`);
    if (filler) blockG.attr('opacity', 0.15);
    for (const [fill, d] of byFabric) {
      blockG.append('path')
        .attr('d', d)
        .attr('fill', fill)
        // Two fabrics meeting each cover part of the boundary pixel, so the
        // ground shows through the remainder and draws a hairline grid over
        // the block. Growing each fill by half a non-scaling stroke of its
        // own colour closes it at any zoom.
        .attr('stroke', fill)
        .attr('stroke-width', 1)
        .attr('stroke-linejoin', 'round')
        .attr('vector-effect', 'non-scaling-stroke');
    }

    return lattice.tilePath(tile.nc, tile.nr, sz);
  }

  /**
   * Re-cut every visible tile against the layout it just landed in, then
   * rebuild the cloth over it.
   *
   * A repack moves tiles between grid cells, and a tile's edge was cut to fit
   * the seams of the cell it left. Sliding it across by transform would carry
   * the old cut with it and open exactly the gaps the shared lattice exists
   * to close, so the fabric is re-warped rather than moved.
   */
  function rebuildCloth(layout, bu) {
    if (!contentG_ref || !lattice) return;
    const weaveChunks = new Map();
    let silhouette = '';
    const placed = [];
    for (const [, item] of tileMap) {
      if (!item.visible) continue;
      const inner = item.g.select('g');
      if (inner.empty()) continue;
      silhouette += paintFabric(item.tile, inner, weaveChunks);
      placed.push({
        gridPos: { col: item.tile.nc, row: item.tile.nr },
        currentSize: item.tile.currentSize,
      });
    }
    if (!placed.length) {
      if (clothG) clothG.style('display', 'none');
      return;
    }
    let maxCol = 0, maxRow = 0;
    for (const t of placed) {
      maxCol = Math.max(maxCol, t.gridPos.col + t.currentSize);
      maxRow = Math.max(maxRow, t.gridPos.row + t.currentSize);
    }
    buildCloth(contentG_ref, { tiles: placed, minCol: 0, minRow: 0, maxCol, maxRow },
      silhouette, weaveChunks, bu);
    // Clear opacity as well as display: a rebuild can land mid-way through
    // the intro fade, and inheriting a half-finished value would leave the
    // quilt wearing no cloth with nothing left to finish the transition.
    if (clothG) clothG.style('display', null).style('opacity', null);
  }

  function buildCloth(contentG, layout, silhouette, weaveChunks, bu) {
    contentG.selectAll('.cloth, .cloth-defs').remove();
    // The cloth sits above every tile and covers the whole quilt — the weave
    // over each piece, the fold image over all of it, the seam strokes along
    // every boundary. Painted elements hit-test by default, so without this
    // the cloth swallows every hover and click and the only thing left
    // pointing at a patch is its name badge, which lives in the HTML layer
    // above. A tile with no badge would become unreachable.
    clothG = contentG.append('g')
      .attr('class', 'cloth')
      .style('pointer-events', 'none');
    weaveG = null;

    const cols = layout.maxCol - layout.minCol;
    const rows = layout.maxRow - layout.minRow;
    const seams = lattice.seamGeometry(
      layout.tiles.map(t => ({
        col: t.gridPos.col - layout.minCol,
        row: t.gridPos.row - layout.minRow,
        size: t.currentSize,
      })),
    );

    /** One stroke layer, one path per chunk. */
    const strokeChunks = (parent, chunks, attrs) => {
      for (const d of chunks.values()) {
        const path = parent.append('path').attr('d', d).attr('fill', 'none');
        for (const [k, v] of Object.entries(attrs)) path.attr(k, v);
      }
    };

    // --- Weave -------------------------------------------------------------
    // One baked swatch, reused by every fabric; they differ by the transform
    // on the pattern rather than by another image, so the GPU uploads one
    // texture however many fabrics the quilt has.
    if (weaveChunks.size) {
      const defs = contentG.append('defs').attr('class', 'cloth-defs');
      for (let wi = 0; wi < WEAVES; wi++) {
        const scale = CLOTH.weaveScale * bu;
        defs.append('pattern')
          .attr('id', `pw-weave-${wi}`)
          .attr('width', scale).attr('height', scale)
          .attr('patternUnits', 'userSpaceOnUse')
          .attr('patternTransform', `rotate(${wi * 31})`)
          .append('image')
          .attr('href', bakeWeave(wi))
          .attr('width', scale).attr('height', scale)
          .attr('preserveAspectRatio', 'none');
      }
      weaveG = clothG.append('g').attr('class', 'weave');
      for (const cut of weaveChunks.values()) {
        weaveG.append('path')
          .attr('d', cut.d)
          .attr('fill', `url(#pw-weave-${cut.wi})`)
          .attr('opacity', CLOTH.weaveDepth * cut.grain);
      }
    }

    // --- Folds -------------------------------------------------------------
    // One baked image masked to the quilt's own silhouette, so folds stop at
    // the ragged edge. Ordinary alpha rather than a blend mode: a blend would
    // force the whole quilt into an isolated compositing buffer.
    if (CLOTH.wrinkleAmt > 0 && cols > 0 && rows > 0) {
      const maskId = `pw-quiltmask-${Math.round(bu)}-${cols}x${rows}`;
      contentG.append('defs').attr('class', 'cloth-defs')
        .append('clipPath').attr('id', maskId)
        .append('path').attr('d', silhouette);
      clothG.append('g').attr('clip-path', `url(#${maskId})`)
        .append('image')
        .attr('href', bakeWrinkles(cols, rows))
        .attr('x', 0).attr('y', 0)
        .attr('width', cols * bu).attr('height', rows * bu)
        .attr('preserveAspectRatio', 'none');
    }

    // --- Bevel and puff ----------------------------------------------------
    // A single wide stroke centred on the boundary darkens both sides at
    // once, which is what an inner lip on each of two neighbours added up to
    // anyway. Three stacked widths stand in for a soft falloff without a
    // filter. This is the ground the light sits on, so it goes first.
    if (CLOTH.bevel > 0) {
      const lip = CLOTH.bevel * bu * 2;
      const bevelG = clothG.append('g').attr('class', 'bevel');
      for (const [mult, alpha] of [[3.2, 0.05], [1.9, 0.07], [1.0, 0.13]]) {
        strokeChunks(bevelG, seams.chunks, {
          stroke: `rgba(0,0,0,${(alpha * (1 + CLOTH.puff * 2)).toFixed(3)})`,
          'stroke-width': (lip * mult).toFixed(2),
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
        });
      }
    }

    // --- Doming ------------------------------------------------------------
    // A quilted surface under one lamp shows its curvature as asymmetric
    // shading either side of every seam: lit on the side turning toward the
    // lamp, shadowed on the side turning away. Two offset strokes for the
    // whole quilt say that, where a per-tile gradient needed a fill each —
    // and randomising the *angle* per tile, as the old shadow layer did,
    // cancels the illusion outright.
    if (CLOTH.lightAmt > 0) {
      const off = CLOTH.bevel * bu * 1.1 || 4;
      const domeG = clothG.append('g').attr('class', 'doming');
      for (const [shift, colour, weight] of [
        [off, '255,252,244', 1], [-off, '0,0,0', 0.85],
      ]) {
        strokeChunks(domeG, seams.chunks, {
          stroke: `rgba(${colour},${(CLOTH.lightAmt * 1.7 * weight).toFixed(3)})`,
          'stroke-width': (off * 1.6).toFixed(2),
          transform: `translate(${shift.toFixed(2)},${shift.toFixed(2)})`,
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
        });
      }
    }

    // --- Seam ink ----------------------------------------------------------
    // Screen pixels, not world units: the stitch reads the same at 0.3x and
    // 6x. The gap it replaces grew twentyfold across that range.
    const seamG = clothG.append('g').attr('class', 'seam');
    if (CLOTH.ridge > 0) {
      strokeChunks(seamG, seams.chunks, {
        stroke: 'var(--lt-seam-ridge)',
        'stroke-width': CLOTH.seamW,
        'vector-effect': 'non-scaling-stroke',
        'stroke-linecap': 'round',
        transform: `translate(${CLOTH.ridge / currentTransform.k},${CLOTH.ridge / currentTransform.k})`,
        'data-ridge': '1',
      });
    }
    strokeChunks(seamG, seams.chunks, {
      stroke: 'var(--lt-seam-ink)',
      'stroke-width': CLOTH.seamW,
      'vector-effect': 'non-scaling-stroke',
      'stroke-linecap': 'round',
      'stroke-linejoin': 'round',
    });

    // Topstitch and binding: detail that only exists above their tiers.
    stitchG = clothG.append('g').attr('class', 'topstitch');
    strokeChunks(stitchG, seams.chunks, {
      stroke: 'var(--lt-seam-stitch)',
      'stroke-width': 1,
      'vector-effect': 'non-scaling-stroke',
      'stroke-dasharray': '4 4',
      'stroke-linecap': 'round',
    });

    strokeChunks(clothG.append('g').attr('class', 'binding'), seams.outer, {
      stroke: 'var(--lt-seam-ink)',
      'stroke-width': CLOTH.seamW * 4,
      'vector-effect': 'non-scaling-stroke',
      'stroke-linecap': 'square',
      opacity: 0.9,
    });

    applyClothTiers();
  }

  /**
   * The detail that only exists close up. Toggling a layer's display is one
   * style write; deciding it per tile would be thousands.
   */
  function applyClothTiers() {
    const k = currentTransform.k || 1;
    if (stitchG) stitchG.style('display', k >= CLOTH.stitchFrom ? null : 'none');
    if (weaveG && !weaveParked) {
      weaveG.style('display', k >= CLOTH.weaveFrom ? null : 'none');
    }
  }

  /**
   * A pattern fill is the one paint that has to be re-tiled when the scale
   * changes: free to pan across, and the dominant cost of every zoom — 12ms
   * of a 41ms frame at 269 patches, where the bevel, the subdivision and the
   * doming each cost under a millisecond. Nobody can read a thread count
   * while the view is moving, so it stands down for the gesture.
   */
  function parkWeaveDuringZoom() {
    if (!weaveG) return;
    if (!weaveParked) {
      weaveParked = true;
      weaveG.style('display', 'none');
    }
    clearTimeout(weaveSettleTimer);
    weaveSettleTimer = setTimeout(() => {
      weaveParked = false;
      applyClothTiers();
    }, CLOTH.weaveSettleMs);
  }

  /** The ridge offset is measured in screen pixels, so it tracks the scale. */
  function updateSeamRidge() {
    if (!clothG) return;
    const o = (CLOTH.ridge / (currentTransform.k || 1)).toFixed(3);
    clothG.selectAll('[data-ridge]').attr('transform', `translate(${o},${o})`);
  }

  function drawCornerMark(entry, slot, corner, discFill, glyph) {
    const { ax, ay, sx, sy } = MARK_CORNERS[corner];
    const g = entry.inner.append('g')
      .attr('class', `corner-mark corner-mark-${slot}`)
      .style('pointer-events', 'none');
    const cx = sx * (MARK_INSET + MARK_PX / 2);
    const cy = sy * (MARK_INSET + MARK_PX / 2);

    // Drop shadow drawn as an offset circle rather than a CSS filter: these
    // transforms are rewritten on every zoom tick, up to three per tile across
    // the whole quilt, and for a flat circle an offset circle *is* the shadow
    // at zero compositor cost. Hardcoded black in both themes, like the tile's
    // own pillow shadow — --lt-shadow-color inverts with the theme because it
    // is built for chrome on the page canvas, and on arbitrary fabric a pale
    // halo either disappears or reads as a glow.
    g.append('circle')
      .attr('cx', cx + MARK_SHADOW_OFFSET).attr('cy', cy + MARK_SHADOW_OFFSET)
      .attr('r', MARK_PX / 2)
      .attr('fill', 'rgba(0,0,0,0.28)');

    // Seam ring, the same stroke the tile's edge wears. Not decorative: the
    // identity color IS the palette primary and the block is drawn from that
    // same palette, so a fair number of tiles have primary fabric directly
    // under this corner and the disc would otherwise vanish into itself.
    // (No resin dome highlight — that is a pill-scale effect, and these are
    // too small to carry it.)
    g.append('circle')
      .attr('cx', cx).attr('cy', cy).attr('r', MARK_PX / 2)
      .attr('fill', discFill)
      .attr('stroke', 'var(--lt-thread-heavy)')
      .attr('stroke-width', 1.5);

    glyph.setAttribute('transform',
      `translate(${cx - MARK_GLYPH_PX / 2},${cy - MARK_GLYPH_PX / 2}) ` +
      glyph.getAttribute('transform'));
    g.node().appendChild(glyph);

    entry.slots.set(slot, { g, corner });
  }

  /**
   * Bring a tile's role slot in line with the viewer's current relationship to
   * that patch: gold star for belonging (admin/member), red heart for a follow,
   * nothing otherwise. Adds and removes the slot, so a join or a follow lands
   * without a relayout — the role mark used to live on the name badge, where
   * updateLabels' __role comparison was the only thing rebuilding it.
   */
  function syncRoleMark(entry) {
    const role = quiltScope === 'local' ? myPatchRoles.get(entry.tile.data.slug) : undefined;
    if (entry.role === role && entry.slots.has('role') === !!role) return;
    entry.role = role;
    const existing = entry.slots.get('role');
    if (existing) {
      existing.g.remove();
      entry.slots.delete('role');
    }
    if (role === 'admin' || role === 'member') {
      drawCornerMark(entry, 'role', 'br', MARK_STATUS_FILL,
        createMyPatchStarGroup(MARK_GLYPH_PX));
    } else if (role === 'follower') {
      drawCornerMark(entry, 'role', 'br', MARK_STATUS_FILL,
        createFollowedHeartGroup(MARK_GLYPH_PX));
    }
  }

  /**
   * Hold every corner mark at a fixed on-screen size, the way a name badge
   * sits at 13px whatever the zoom. Marks are drawn inside the tile's group,
   * so they inherit the zoom scale k; dividing it back out is what makes them
   * static — otherwise a status pip becomes a dinner plate at 6× zoom.
   */
  function updateCornerMarks() {
    const k = currentTransform.k;
    for (const { tile, slots } of cornerMarks.values()) {
      // One size for every slot on this tile, so a tile's marks appear and
      // vanish together — sizing them slot by slot leaves zoom levels where a
      // tile wears a heart but no motif. Fixed, except on a tile too small to
      // host a mark: one covering a third of its own patch has stopped being
      // an annotation.
      const px = Math.min(MARK_PX, tile.pxSize * k * MARK_TILE_SHARE);
      const hidden = px < MARK_MIN_PX;
      for (const { g, corner } of slots.values()) {
        if (hidden) {
          g.style('display', 'none');
          continue;
        }
        const { ax, ay } = MARK_CORNERS[corner];
        g.style('display', null)
          .attr('transform',
            `translate(${ax * tile.pxSize},${ay * tile.pxSize}) scale(${px / MARK_PX / k})`);
      }
    }
  }

  function handleResize() {
    if (!treeData?.children?.length) return;

    const { vw, vh } = getContainerSize();
    if (!vw || !vh) return; // Still collapsed — wait for a real size.
    // Badge type first: if it moved, every measured footprint below is stale.
    const typeMoved = syncBadgeType();
    // Sub-pixel reflows shouldn't restart the pop-in animation — unless the
    // type moved, in which case the badges on screen are the wrong size.
    if (!typeMoved && Math.abs(vw - lastBuiltW) < 2 && Math.abs(vh - lastBuiltH) < 2) return;
    if (typeMoved) { dropAllLabels(); updateLabels(); }

    // A rebuild is only warranted when the tiles would come out meaningfully
    // different — that is, when baseUnit drifts. The packing ignores the
    // container entirely, so everything else a resize touches (svg dimensions,
    // where the quilt sits on screen) is a view concern the zoom transform
    // already expresses.
    //
    // Drift, not equality: baseUnit is a heuristic target, floored and clamped,
    // and the fit scales the quilt's bounding box to the available area — so a
    // baseUnit a pixel or two off is cancelled by a compensating zoom scale and
    // looks identical. Equality would still rebuild for a scrollbar, since one
    // baseUnit step costs only ~16px of width on a typical quilt. BASE_UNIT_DRIFT
    // sits above the reflows (a pane settling, a scrollbar) and below a real
    // window resize, which moves it many times over.
    const idealUnit = computeBaseUnit(vw, vh, treeData.children.length);
    if (svgSelection && zoomBehavior &&
        Math.abs(idealUnit - baseUnit) <= baseUnit * BASE_UNIT_DRIFT) {
      resizeViewport(vw, vh);
      return;
    }

    layoutBuilt = false;
    tileMap = new Map();
    buildLayout();
  }

  /**
   * Grow or shrink the canvas without touching the layout: stretch the svg to
   * the new container, then shift the view by half the delta so the quilt
   * stays put relative to the viewport's center rather than pinned to its
   * top-left. Zoom level and the viewer's panning survive intact.
   */
  function resizeViewport(vw, vh) {
    const dx = (vw - lastBuiltW) / 2;
    const dy = (vh - lastBuiltH) / 2;
    lastBuiltW = vw;
    lastBuiltH = vh;

    svgSelection.attr('width', vw).attr('height', vh);

    // Routing through zoomBehavior.transform keeps d3's internal transform in
    // step with ours and fires the zoom handler, which repositions the shadow
    // layer and the labels for us.
    const t = currentTransform;
    svgSelection.call(
      zoomBehavior.transform,
      d3.zoomIdentity.translate(t.x + dx, t.y + dy).scale(t.k),
    );
  }

  // Watch the container itself rather than the window: it also catches the
  // 0x0 -> real-size transition on first paint, and layout changes that never
  // fire a window resize (sidebar collapse, patch list opening).
  $effect(() => {
    const el = containerEl;
    if (!el || typeof ResizeObserver === 'undefined') return;

    const ro = new ResizeObserver(() => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(handleResize, 150);
    });
    ro.observe(el);

    return () => {
      ro.disconnect();
      clearTimeout(resizeTimer);
      if (buildRetryFrame !== null) {
        cancelAnimationFrame(buildRetryFrame);
        buildRetryFrame = null;
      }
    };
  });

  onMount(() => {
    loadData();
    // The tooltip lives on <body>, not in this component's markup:
    // .quilt-pane is a stacking context (z-index: 0), so no z-index inside
    // it can clear the shell UI — the tooltip has to be outside the pane to
    // float over the patch list. It's built here rather than declared in
    // the template and moved, because moving a component's *last* node out
    // of its own fragment breaks Svelte's teardown: destroying the canvas
    // then sweeps every following sibling, including the {#if} anchor its
    // parent needs to render the next branch (quilt → map left the pane
    // permanently empty). Its styles are already :global.
    if (interactive) {
      tooltip = document.createElement('div');
      tooltip.className = 'canvas-tooltip';
      document.body.appendChild(tooltip);
    }
    // Text measured before the display font loads used the fallback font's
    // metrics — remeasure once real metrics exist.
    document.fonts?.ready?.then(() => {
      resetBadgeType();
      // The badges on screen were sized from those metrics, and reuse can't see
      // that the numbers moved — so make this pass build them again.
      dropAllLabels();
      updateLabels();
    });
    return () => {
      tooltip?.remove();
      tooltip = null;
      endLabelGesture(); // a gesture in flight when the canvas leaves
      labelEls.clear();
    };
  });

  // A pinch on the quilt is a quilt zoom, never a page zoom — including over
  // the labels layer, which sits on top of the svg d3 already guards. A
  // static hero has no zoom to protect, and must not steal the page's own
  // scroll/pinch gestures out from under it.
  $effect(() => {
    if (!interactive) return;
    const cleanups = [containerEl, labelsEl].map(blockPageZoom);
    return () => cleanups.forEach(fn => fn());
  });

  // Reload data when the quilt scope or (in My Quilt) the set of remote
  // follows changes.
  let prevScope = quiltScope;
  let prevFollowCount = -1;
  $effect(() => {
    const scope = quiltScope;
    const followCount = scope === 'my' ? getRemoteFollows().length : -1;
    if (scope !== prevScope ||
        (prevFollowCount !== -1 && followCount !== -1 && followCount !== prevFollowCount)) {
      prevScope = scope;
      prevFollowCount = followCount;
      layoutBuilt = false;
      tileMap.clear();
      loadData();
    } else {
      prevFollowCount = followCount;
    }
  });
</script>

<!-- Belt and braces: the observer is the accurate signal, but this keeps
     resize working if ResizeObserver is unavailable. handleResize no-ops
     when the size hasn't actually changed, so the overlap is harmless. -->
<svelte:window onresize={handleResize} />

{#if loading}
  <div class="canvas-state" style="padding-right: {insetRight * 100}%">
    <div class="loading-spinner"></div>
    <p>Loading quilt...</p>
  </div>
{:else if error}
  <div class="canvas-state" style="padding-right: {insetRight * 100}%">
    <p>{error}</p>
    <button class="btn btn-secondary" onclick={loadData}>Try Again</button>
  </div>
{:else if !treeData?.children?.length}
  <div class="canvas-state" style="padding-right: {insetRight * 100}%">
    {#if filterTags.length > 0 || searchQuery.trim()}
      <p>
        No patches match your
        {filterTags.length > 0 && searchQuery.trim() ? 'search and filter'
          : filterTags.length > 0 ? 'filter' : 'search'}{quiltScope === 'my' ? ' in My Quilt' : ''}.
      </p>
      {#if filterTags.length > 0 || canSuggest}
        <div class="empty-actions">
          {#if filterTags.length > 0}
            <button class="btn btn-secondary" onclick={onClearFilter}>Clear filter</button>
          {/if}
          {#if canSuggest}
            <button class="btn btn-secondary" onclick={suggestPatch}>Suggest a patch</button>
          {/if}
        </div>
      {/if}
    {:else}
      <p>This quilt is empty.</p>
      <p class="muted">Create the first patch to get started.</p>
    {/if}
  </div>
{:else}
  <div class="canvas-container lt-fill-canvas lt-texture-grain" class:non-interactive={!interactive} bind:this={containerEl}></div>
  <div class="labels-layer" bind:this={labelsEl}></div>
  {#if visibleIds.size === 0 && (filterTags.length > 0 || searchQuery.trim())}
    <!-- Every tile filtered out. Name the lenses (docs/adr/022) — on mobile
         this overlay is the only explanation the quilt view gets. -->
    <div class="canvas-state canvas-empty-overlay" style="padding-right: {insetRight * 100}%">
      <p>
        No patches match your
        {filterTags.length > 0 && searchQuery.trim() ? 'search and filter'
          : filterTags.length > 0 ? 'filter' : 'search'}{quiltScope === 'my' ? ' in My Quilt' : ''}.
      </p>
      {#if filterTags.length > 0 || canSuggest}
        <div class="empty-actions">
          {#if filterTags.length > 0}
            <button class="btn btn-secondary" onclick={onClearFilter}>Clear filter</button>
          {/if}
          {#if canSuggest}
            <button class="btn btn-secondary" onclick={suggestPatch}>Suggest a patch</button>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
{/if}

<style>
  .canvas-container {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    z-index: 0;
    background: var(--color-bg);
    overflow: hidden;
    /* Touches here belong to the quilt's own pan/zoom, not to the page. */
    touch-action: none;
  }

  .canvas-container :global(svg) {
    display: block;
    cursor: grab;
  }

  .canvas-container :global(svg:active) {
    cursor: grabbing;
  }

  /* Static/compact mode (docs/adr/040): no gestures to invite, and the
     page's own scroll/pinch must pass through rather than be captured. */
  .canvas-container.non-interactive {
    touch-action: auto;
  }

  .canvas-container.non-interactive :global(svg),
  .canvas-container.non-interactive :global(svg:active) {
    cursor: default;
  }


  .labels-layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    z-index: 1;
    pointer-events: none;
    overflow: hidden;
  }

  :global(.patch-label) {
    position: absolute;
    /* Content-driven width: without this, an absolutely-positioned pill
       near the container's right edge lays out shrink-to-fit against the
       edge and re-wraps. Badges keep one shape everywhere and simply clip
       at the viewport (the layer's overflow: hidden), Google-Maps style. */
    width: max-content;
    transform: translate(-50%, -50%);
    color: var(--color-label-text);
    /* Padding and radius in em so the chip keeps its proportions at whatever
       size --pw-label-font resolves to; the seam stays a hairline, which is
       what a 20px pill can carry without the border reading as the loudest
       thing in it. */
    padding: 0.2em 0.4em;
    border-radius: 0.5em;
    font-size: var(--pw-label-font);
    font-weight: 600;
    pointer-events: auto;
    cursor: pointer;
    touch-action: none;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    text-align: center;
    line-height: 1.3;
    border: 1px solid var(--lt-thread);
    font-family: 'Space Grotesk Variable', system-ui, sans-serif;
    color: var(--color-text);
  }

  /* A badge drags the quilt like bare fabric does, so it borrows the svg's
     grabbing cursor while the button is down. */
  :global(.patch-label:active) {
    cursor: grabbing;
  }

  /* A badge outliving its own rebuild because a finger is still on it: it holds
     the touch sequence's delivery address and nothing else. Out of sight, out
     of the hit-testing, gone as soon as the finger lifts. */
  :global(.patch-label.gesture-holdover) {
    visibility: hidden;
    pointer-events: none;
  }

  :global(.patch-label.selected) {
    font-weight: 700;
    border-color: var(--color-primary);
  }

  :global(.patch-label .label-name) {
    /* Inherits the pill's font-size deliberately — badgeType() measures the
       pill and the text has to be the size that measurement assumed. A second
       declaration here is how the box and its contents drift apart. */
    display: -webkit-box;
    /* Overridden per badge to the line count it was measured at. */
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    text-overflow: ellipsis;
    /* Even splits across however many lines the name was measured at;
       mid-word breaks stay as the escape hatch for pathological unbroken
       strings only. */
    text-wrap: balance;
    word-break: break-word;
    min-width: 0;
  }

  .canvas-state {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    z-index: 0;
    background: var(--lt-canvas, var(--color-bg));
    color: var(--color-text-muted);
    gap: 0.75rem;
  }

  /* Filtered-to-nothing: floats over the (empty) canvas layers without
     hiding the textured background, and without eating pan gestures. */
  .canvas-empty-overlay {
    background: none;
    z-index: 5;
    pointer-events: none;
  }

  .canvas-empty-overlay .btn {
    pointer-events: auto;
  }

  /* Clear filter and Suggest a patch sit side by side when a search-and-filter
     miss raises both; wraps rather than crowding a narrow canvas. */
  .empty-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 0.5rem;
  }

  .loading-spinner {
    width: 32px;
    height: 32px;
    border: 3px solid var(--color-border);
    border-top-color: var(--color-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

  :global(.canvas-tooltip) {
    display: none;
    position: fixed;
    z-index: 200;
    background: var(--color-surface);
    border: 2px solid var(--lt-thread, #ccc);
    border-radius: 6px;
    padding: 0.6rem 0.8rem;
    max-width: 280px;
    font-size: 0.85rem;
    filter: drop-shadow(var(--lt-shadow-x, 3px) var(--lt-shadow-y, 3px) 0 var(--lt-shadow-color, rgba(0,0,0,0.15)));
    pointer-events: none;
    font-family: 'Space Grotesk Variable', system-ui, sans-serif;
  }

  :global(.canvas-tooltip .tip-desc) {
    color: var(--color-text-muted);
    margin-top: 0.25rem;
    font-size: 0.8rem;
    line-height: 1.4;
  }

  :global(.canvas-tooltip .tip-meta) {
    margin-top: 0.35rem;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  :global(.canvas-tooltip .tip-tags) {
    display: block;
    font-style: italic;
    margin-bottom: 0.15rem;
  }

  :global(.canvas-tooltip .tip-unclaimed) {
    display: inline-block;
    font-size: 0.65rem;
    color: var(--color-text-muted);
    border: 1px solid var(--color-border);
    border-radius: 3px;
    padding: 0 4px;
    margin-top: 2px;
  }
</style>
