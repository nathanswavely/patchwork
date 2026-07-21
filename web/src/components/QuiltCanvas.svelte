<script>
  import { onMount, untrack } from 'svelte';
  import * as d3 from 'd3';
  import { api } from '../lib/api.js';
  import { identityColorForPatch, paletteForPatch, ghostPalette, darken, textOnColor } from '../lib/quiltTheme.js';
  import { renderBlock, renderGhostBlock } from '../lib/quiltBlocks.js';
  import { quiltLayout } from '../lib/quiltLayout.js';
  import { createMotifElement, createMyPatchStar, createFollowedHeart } from '../lib/patchIcons.js';
  import { buildRemoteGroups, composeGroupLayouts } from '../lib/quiltRegions.js';
  import { blockPageZoom } from '../lib/pageZoom.js';
  import {
    getRemoteFollows, fetchQuiltInfo, colorForQuilt, refreshFollowSnapshot,
  } from '../stores/multiQuilt.svelte.js';

  // Raw edge polygon variants — percentage coordinates from LT raw.css.
  // Each is 12 points: 4 corners + 8 edge midpoints with slight deviation.
  const RAW_EDGES = [
    [[0.5,1.2],[30,0.3],[70,0.8],[99.2,0.4],[99.6,35],[99.1,65],[99.5,98.8],[65,99.5],[35,99.1],[0.8,99.6],[0.3,60],[0.7,30]],
    [[0.8,0.4],[35,0.9],[65,0.2],[99.5,1.0],[99.1,40],[99.7,70],[99.3,99.2],[70,99.6],[30,99.8],[0.4,99.1],[0.9,55],[0.2,25]],
    [[1.0,0.6],[40,0.2],[60,1.1],[99.4,0.3],[99.8,30],[99.2,60],[99.6,99.5],[60,99.2],[40,99.7],[0.6,99.4],[0.2,65],[1.1,35]],
    [[0.3,0.9],[25,0.5],[75,1.0],[99.7,0.7],[99.3,45],[99.8,55],[99.1,99.3],[75,99.8],[25,99.4],[0.9,99.7],[0.5,50],[1.0,20]],
    [[0.7,0.3],[45,1.1],[55,0.4],[99.3,0.8],[99.5,25],[99.1,75],[99.7,99.1],[55,99.5],[45,99.3],[0.4,99.8],[1.1,70],[0.6,40]],
  ];

  /** Get SVG polygon points string for a raw edge variant at given size. */
  function rawEdgePoints(variantIndex, size) {
    const pts = RAW_EDGES[variantIndex % RAW_EDGES.length];
    return pts.map(([x, y]) => `${(x / 100 * size).toFixed(1)},${(y / 100 * size).toFixed(1)}`).join(' ');
  }

  /** Get CSS clip-path polygon string for a raw edge variant. */
  function rawEdgeClipPath(variantIndex) {
    const pts = RAW_EDGES[variantIndex % RAW_EDGES.length];
    return `polygon(${pts.map(([x, y]) => `${x}% ${y}%`).join(', ')})`;
  }

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
  } = $props();

  let containerEl = $state(null);
  let shadowsEl = $state(null);
  let labelsEl = $state(null);

  /** Get container dimensions (falls back to window if container not ready). */
  function getContainerSize() {
    if (containerEl) {
      return { vw: containerEl.clientWidth, vh: containerEl.clientHeight };
    }
    return { vw: window.innerWidth, vh: window.innerHeight };
  }

  // On-screen tile size at which a tile earns a label (see updateLabels).
  const LABEL_MIN_PX = 52;
  // Name badge shape comes from its name alone (CONTEXT.md "Name badge") —
  // constant text cap, no tie to tile size or screen position.
  const LABEL_TEXT_MAX = 140;
  // Minimum visible quilt between placed badges; a rival badge that can't
  // clear this gap stays hidden until a closer zoom.
  const LABEL_GAP = 12;
  const LABEL_FONT = '600 13px "Space Grotesk Variable", system-ui, sans-serif';
  // Below this container width the quilt is a phone-sized surface.
  const NARROW_VW = 700;

  // Measured badge text metrics, cached per name (names don't change
  // mid-session; the cache is busted once when the display font loads).
  let measureCtx = null;
  const measureCache = new Map();

  function measureBadgeText(name) {
    let m = measureCache.get(name);
    if (m) return m;
    if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
    measureCtx.font = LABEL_FONT;
    const full = measureCtx.measureText(name).width;
    if (full <= LABEL_TEXT_MAX) {
      m = { textW: Math.ceil(full), lines: 1 };
    } else {
      // Two lines, balanced: mirror the CSS `text-wrap: balance` by taking
      // the word split that minimizes the longer line, so the pill hugs the
      // balanced width instead of sitting at the cap. +2px slack absorbs
      // canvas-vs-layout rounding.
      const words = name.split(/\s+/);
      let best = full;
      for (let i = 1; i < words.length; i++) {
        const a = measureCtx.measureText(words.slice(0, i).join(' ')).width;
        const b = measureCtx.measureText(words.slice(i).join(' ')).width;
        best = Math.min(best, Math.max(a, b));
      }
      m = { textW: Math.min(Math.ceil(best) + 2, LABEL_TEXT_MAX), lines: 2 };
    }
    measureCache.set(name, m);
    return m;
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
  // Map of patch ID → { g, shadowDiv, tile, dist, visible } for per-tile animation.
  let tileMap = new Map();
  let layoutBuilt = false;
  // Stored references for relayout animation.
  let contentG_ref = null;
  let shadowContainer_ref = null;
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
      const q = searchQuery.toLowerCase();
      children = children.filter(n =>
        n.name?.toLowerCase().includes(q) || n.description?.toLowerCase().includes(q)
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

  // Update labels when selected patch changes.
  $effect(() => {
    const slug = selectedPatchSlug;
    untrack(() => {
      if (placedTiles.length > 0) updateLabels();
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

    // Hide labels and shadows during transition.
    if (labelsEl) labelsEl.style.opacity = '0';
    if (shadowsEl) shadowsEl.style.opacity = '0';

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
    const layout = quiltLayout(filteredChildren, affinityData, fixedSizes);

    // New positions for non-filler tiles.
    const newPosMap = new Map();
    for (const t of layout.tiles) {
      if (t.isFiller) continue;
      newPosMap.set(t.data.id, {
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
        item.g.transition().duration(dur).ease(d3.easeCubicInOut)
          .attr('transform', `translate(${newCx},${newCy}) scale(1)`);
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

    // After everything settles: snap shadows and show labels.
    setTimeout(() => {
      for (const [, item] of tileMap) {
        if (item.shadowDiv) {
          item.shadowDiv.style.transition = 'none';
          item.shadowDiv.style.left = item.tile.px + 'px';
          item.shadowDiv.style.top = item.tile.py + 'px';
          item.shadowDiv.style.opacity = item.visible ? '1' : '0';
        }
      }
      if (shadowContainer_ref) {
        const k = currentTransform.k;
        shadowContainer_ref.style.transition = 'none';
        shadowContainer_ref.style.transform = `translate(${currentTransform.x + canvasOffsetX * k}px,${currentTransform.y + canvasOffsetY * k}px) scale(${k})`;
        shadowContainer_ref.style.transformOrigin = '0 0';
      }
      if (shadowsEl) shadowsEl.style.opacity = '1';

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
        if (item.shadowDiv) item.shadowDiv.style.opacity = '1';
        item.visible = true;
      } else if (!show && item.visible) {
        item.g.transition().duration(250).ease(d3.easeBackIn.overshoot(0.5))
          .attr('transform', `translate(${cx},${cy}) scale(0)`)
          .style('opacity', 0);
        if (item.shadowDiv) item.shadowDiv.style.opacity = '0';
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
    lastBuiltW = vw;
    lastBuiltH = vh;
    currentTransform = d3.zoomIdentity;
    if (labelsEl) labelsEl.style.opacity = '0';
    tileMap = new Map();

    d3.select(containerEl).selectAll('svg').remove();

    const padLeft = 0;
    const padRight = Math.round(vw * insetRight);

    const allChildren = treeData.children;
    const contentSize = Math.min(vw - padLeft - padRight - 32, vh - 64);
    const n = allChildren.length;
    const estimatedCells = n * 3;
    const gridSide = Math.ceil(Math.sqrt(estimatedCells) * 1.3);
    baseUnit = Math.max(30, Math.min(80, Math.floor(contentSize / gridSide)));

    const layout = groupsMeta
      ? composeGroupLayouts(groupsMeta, new Map([['home', affinityData]]))
      : quiltLayout(allChildren, affinityData);
    const bu = baseUnit;

    const pixelTiles = layout.tiles.map(t => ({
      ...t,
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

    const shadowLayer = [];
    const tileGroups = []; // For staggered animation.

    // --- RENDER TILES ---
    for (const tile of pixelTiles) {
      const s = tile.pxSize;
      const tileCx = tile.px + s / 2;
      const tileCy = tile.py + s / 2;

      // Create group at tile center, scaled to 0 for pop-in.
      const g = contentG.append('g')
        .attr('class', tile.isFiller ? 'filler' : 'tile')
        .attr('transform', `translate(${tileCx},${tileCy}) scale(0)`)
        .style('cursor', tile.isFiller ? 'default' : 'pointer')
        .style('opacity', 0);

      // Inner group offset so content draws from top-left.
      const inner = g.append('g').attr('transform', `translate(${-s/2},${-s/2})`);

      // Distance from center for stagger ordering.
      const dist = Math.sqrt((tileCx - centerX) ** 2 + (tileCy - centerY) ** 2);

      if (tile.isFiller) {
        const palette = ghostPalette(parseInt(tile.id.split('-')[1]) || 0);
        const fillerRaw = (parseInt(tile.id.split('-')[1]) || 0) % RAW_EDGES.length;
        inner.append('clipPath').attr('id', `clip-${tile.id}`)
          .append('polygon').attr('points', rawEdgePoints(fillerRaw, s));
        const blockG = inner.append('g').attr('clip-path', `url(#clip-${tile.id})`);
        renderGhostBlock(blockG, s, parseInt(tile.id.split('-')[1]) || 0, palette);
        inner.append('polygon')
          .attr('points', rawEdgePoints(fillerRaw, s))
          .attr('fill', 'none')
          .attr('stroke', 'var(--lt-thread)')
          .attr('stroke-width', 1);
        tileGroups.push({ g, dist, tile, shadowDiv: null });
      } else {
        const palette = paletteForPatch(tile.data.id, tile.data.appearance);

        // Raw edge variant — deterministic from tile ID hash
        const rawVariant = tile.data.id.charCodeAt(0) % RAW_EDGES.length;

        inner.append('clipPath').attr('id', `clip-${tile.data.id}`)
          .append('polygon').attr('points', rawEdgePoints(rawVariant, s));
        const blockG = inner.append('g').attr('clip-path', `url(#clip-${tile.data.id})`);
        renderBlock(blockG, s, tile.data.id, palette, tile.data.appearance);

        // Thread seam around every tile — keeps near-black block palettes
        // from dissolving into the dark canvas (and vice versa on cotton).
        inner.append('polygon')
          .attr('points', rawEdgePoints(rawVariant, s))
          .attr('fill', 'none')
          .attr('stroke', 'var(--lt-thread-heavy)')
          .attr('stroke-width', 1.5);

        // Unclaimed patch icon overlay — small "?" badge in corner.
        // Deliberately NOT clipped by the tile's raw-edge clipPath: the badge
        // is a status overlay, not fabric (same as label badges), and a
        // userSpaceOnUse clipPath referenced from this translated group would
        // be evaluated in badge-local space, slicing the circle (issue #14).
        // The inset scales with tile size so the badge clears the raw edge's
        // inward wobble (max 1.2% of s) at every size.
        if (tile.data.is_unclaimed) {
          const iconSize = Math.max(14, Math.round(s * 0.12));
          const inset = Math.max(4, Math.round(s * 0.02));
          const iconG = inner.append('g')
            .attr('transform', `translate(${s - iconSize - inset}, ${inset})`);
          iconG.append('circle')
            .attr('cx', iconSize / 2).attr('cy', iconSize / 2).attr('r', iconSize / 2)
            .attr('fill', 'rgba(0,0,0,0.55)');
          iconG.append('text')
            .attr('x', iconSize / 2).attr('y', iconSize / 2 + 1)
            .attr('text-anchor', 'middle').attr('dominant-baseline', 'central')
            .attr('fill', '#fff').attr('font-size', iconSize * 0.65).attr('font-weight', '700')
            .text('?');
        }

        // Pillow depth + fabric texture overlay div.
        const shadowDiv = document.createElement('div');
        shadowDiv.className = 'tile-shadow';
        shadowDiv.style.position = 'absolute';
        shadowDiv.style.left = tile.px + 'px';
        shadowDiv.style.top = tile.py + 'px';
        shadowDiv.style.width = s + 'px';
        shadowDiv.style.height = s + 'px';
        shadowDiv.style.pointerEvents = 'none';
        shadowDiv.style.opacity = '0';
        shadowDiv.style.transition = 'opacity 200ms ease';
        shadowDiv.style.clipPath = rawEdgeClipPath(rawVariant);

        // Pillow shadow — seam darkening at edges + subtle highlight.
        const seam = Math.max(3, Math.round(s * 0.03));
        const pillow = Math.max(4, Math.round(s * 0.05));
        shadowDiv.style.boxShadow = [
          `inset 0 0 ${seam}px 0 rgba(0,0,0,0.15)`,
          `inset ${pillow}px ${pillow}px ${pillow * 2}px 0 rgba(255,255,255,0.04)`,
        ].join(', ');

        // Fabric surface texture — very subtle rumple.
        const hash = tile.data.id.charCodeAt(0) + tile.data.id.charCodeAt(tile.data.id.length - 1);
        const a1 = 120 + (hash % 60);
        const a2 = 240 + ((hash * 7) % 80);
        shadowDiv.style.backgroundImage = [
          `linear-gradient(${a1}deg, transparent 30%, rgba(255,255,255,0.025) 45%, transparent 60%)`,
          `linear-gradient(${a2}deg, transparent 35%, rgba(0,0,0,0.025) 50%, transparent 65%)`,
        ].join(', ');

        shadowLayer.push(shadowDiv);

        if (tile.data.slug === selectedPatchSlug) {
          shadowDiv.style.boxShadow = [
            `inset 0 0 ${seam}px 0 rgba(0,0,0,0.1)`,
            `inset 0 0 ${Math.round(s * 0.08)}px 0 var(--color-primary)`,
          ].join(', ');
        }

        // Hover overlay (starts transparent, darkens on hover).
        inner.append('rect').attr('class', 'overlay')
          .attr('width', s).attr('height', s)
          .attr('fill', 'transparent')
          .style('pointer-events', 'none');

        // Hover + click.
        g.on('mouseenter', function(event) {
          d3.select(this).select('.overlay').attr('fill', 'var(--color-overlay-hover)');
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
          if (tooltip) tooltip.style.display = 'none';
        })
        .on('click', function() {
          if (tile.data.slug) onPatchClick(tile.data.slug, tile.data._source || null);
        });

        tileGroups.push({ g, dist, tile, shadowDiv });
      }
    }

    // --- SHADOW LAYER ---
    if (shadowsEl) {
      shadowsEl.innerHTML = '';
      const shadowContainer = document.createElement('div');
      shadowContainer.className = 'shadow-content';
      shadowContainer.style.transform = `translate(${oX}px,${oY}px)`;
      shadowContainer.style.transformOrigin = '0 0';
      shadowContainer.style.position = 'absolute';
      shadowContainer.style.left = '0';
      shadowContainer.style.top = '0';
      for (const div of shadowLayer) {
        shadowContainer.appendChild(div);
      }
      shadowsEl.appendChild(shadowContainer);
      shadowContainer_ref = shadowContainer;
    }

    // --- STAGGERED POP-IN ANIMATION ---
    // Sort by distance from center: closest tiles pop in first.
    tileGroups.sort((a, b) => a.dist - b.dist);
    const maxDist = tileGroups.length > 0 ? tileGroups[tileGroups.length - 1].dist : 1;
    const totalAnimDuration = 600; // Total stagger window in ms.
    const tileAnimDuration = 350; // Each tile's pop duration.

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

      // Pop in shadow div too.
      if (item.shadowDiv) {
        setTimeout(() => { item.shadowDiv.style.opacity = '1'; }, delay);
      }
    });

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
    const labelsDelay = totalAnimDuration + tileAnimDuration;
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
        if (shadowContainer_ref) {
          shadowContainer_ref.style.transition = 'none';
          shadowContainer_ref.style.transform = `translate(${event.transform.x + canvasOffsetX * event.transform.k}px,${event.transform.y + canvasOffsetY * event.transform.k}px) scale(${event.transform.k})`;
          shadowContainer_ref.style.transformOrigin = '0 0';
        }
        updateLabels();
      });

    svgSelection = svg;
    svg.call(zoomBehavior);

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
    const unclaimedTag = data.is_unclaimed ? '<span class="tip-unclaimed">Community added</span>' : '';
    tooltip.innerHTML = `
      <strong>${escapeHtml(data.name)}</strong>
      ${unclaimedTag}
      ${desc ? `<div class="tip-desc">${desc}${(data.description?.length || 0) > 140 ? '\u2026' : ''}</div>` : ''}
      <div class="tip-meta">
        ${tags ? `<span class="tip-tags">${tags}</span>` : ''}
        <span>${data.is_unclaimed ? `${data.follower_count || 0} following` : `${data.member_count || 0} members`} &middot; ${data.event_count || 0} events</span>
      </div>`;
  }

  function updateLabels() {
    if (!labelsEl) return;

    const t = currentTransform;
    const k = t.k;
    const { vw, vh } = getContainerSize();

    labelsEl.innerHTML = '';
    labeledPatchIds = new Set();

    // Progressive reveal by ON-SCREEN size: a tile earns a label once it is
    // physically big enough to hold one. Keying off the world size instead
    // meant a quilt of uniformly-sized tiles (no member/event spread to grow
    // them) never crossed the threshold at any zoom level.
    const minShowPx = LABEL_MIN_PX;

    // Sort tiles by size descending so larger labels get priority in collision detection.
    const sortedTiles = [...placedTiles].sort((a, b) => b.pxSize - a.pxSize);

    // Track placed label bounding boxes for collision avoidance.
    const labelRects = [];

    for (const tile of sortedTiles) {
      const tilePx = tile.pxSize;
      const screenPx = tilePx * k;
      if (screenPx < minShowPx) continue;

      // World coordinates (pre-zoom) — offset includes the centering transform.
      const worldX = canvasOffsetX + tile.px + tilePx / 2;
      const worldY = canvasOffsetY + tile.py + tilePx / 2;

      // Apply d3 zoom transform: screen = transform.x + world * transform.k
      const screenX = t.x + worldX * t.k;
      const screenY = t.y + worldY * t.k;

      // Skip off-screen labels.
      if (screenX < -150 || screenX > vw + 150 ||
          screenY < -50 || screenY > vh + 50) continue;

      // Exact pill footprint from the measured name: motif badge (26) +
      // gap (6) + text + padding (3+8) + borders (2×2), plus the role mark
      // when the viewer has one here.
      const name = tile.data.name || '';
      const { textW, lines } = measureBadgeText(name);
      const role = quiltScope === 'local' ? myPatchRoles.get(tile.data.slug) : undefined;
      const labelW = 26 + 6 + textW + 15 + (role ? 18 : 0);
      // Height: padding (3+3) + borders (2×2) + max(badge 26, lines × 17).
      const labelH = 10 + Math.max(26, lines * 17);

      // Collision check against already-placed labels. Placed rects are
      // stored inflated by LABEL_GAP, so a rival only lands when there is
      // visible quilt between the pills.
      const rect = {
        x: screenX - labelW / 2,
        y: screenY - labelH / 2,
        w: labelW,
        h: labelH,
      };

      let collides = false;
      for (const existing of labelRects) {
        if (rect.x < existing.x + existing.w &&
            rect.x + rect.w > existing.x &&
            rect.y < existing.y + existing.h &&
            rect.y + rect.h > existing.y) {
          collides = true;
          break;
        }
      }

      if (collides) continue; // Skip — a higher-priority label is already here.

      labelRects.push({
        x: rect.x - LABEL_GAP,
        y: rect.y - LABEL_GAP,
        w: rect.w + LABEL_GAP * 2,
        h: rect.h + LABEL_GAP * 2,
      });
      labeledPatchIds.add(tile.data.id);

      // Create the label element.
      const label = document.createElement('div');
      label.className = 'patch-label lt-vellum';
      if (tile.data.slug === selectedPatchSlug) {
        label.classList.add('selected');
      }

      // Layered motif badge: white outline → colored bg → motif.
      // Identity color: the patch's palette primary, matching its tile.
      const badgeColor = identityColorForPatch(tile.data);
      const badge = document.createElement('div');
      badge.className = 'label-badge lt-resin lt-resin-tinted';
      badge.style.background = badgeColor;

      const icon = createMotifElement(tile.data, 16, '#fff');
      icon.setAttribute('class', 'label-icon');
      badge.appendChild(icon);
      label.appendChild(badge);

      // Text: name only, 2-line ellipsis. Wrapped names get an explicit
      // width — the measured balanced width — so the pill hugs the text
      // instead of every two-liner rendering at the full cap.
      const nameSpan = document.createElement('span');
      nameSpan.className = 'label-name';
      if (lines === 2) {
        nameSpan.style.width = textW + 'px';
      } else {
        nameSpan.style.maxWidth = LABEL_TEXT_MAX + 'px';
      }
      nameSpan.textContent = name;
      label.appendChild(nameSpan);

      // Role mark (CONTEXT.md): star = belonging (admin/member), never a
      // follow. A followed-only patch gets a small filled heart instead.
      if (role === 'admin' || role === 'member') {
        const star = document.createElement('span');
        star.className = 'my-patch-star';
        star.title = role === 'admin' ? 'Admin' : 'Member';
        star.appendChild(createMyPatchStar(12));
        label.appendChild(star);
      } else if (role === 'follower') {
        const heart = document.createElement('span');
        heart.className = 'my-patch-heart';
        heart.title = 'Following';
        heart.appendChild(createFollowedHeart(12));
        label.appendChild(heart);
      }

      label.style.left = screenX + 'px';
      label.style.top = screenY + 'px';

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
        if (tileData.slug) onPatchClick(tileData.slug, tileData._source || null);
      });
      // Forward wheel events to the SVG so zoom works while hovering labels.
      label.addEventListener('wheel', (event) => {
        const svg = containerEl?.querySelector('svg');
        if (svg) svg.dispatchEvent(new WheelEvent(event.type, event));
      }, { passive: true });

      labelsEl.appendChild(label);
    }
  }

  function handleResize() {
    if (!treeData?.children?.length) return;

    const { vw, vh } = getContainerSize();
    if (!vw || !vh) return; // Still collapsed — wait for a real size.
    // Sub-pixel reflows shouldn't restart the pop-in animation.
    if (Math.abs(vw - lastBuiltW) < 2 && Math.abs(vh - lastBuiltH) < 2) return;

    layoutBuilt = false;
    tileMap = new Map();
    buildLayout();
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
    // Portal the tooltip to <body>: .quilt-pane is a stacking context
    // (z-index: 0), so no z-index inside it can clear the shell UI — the
    // tooltip must live outside the pane to float over the patch list.
    if (tooltip) document.body.appendChild(tooltip);
    // Text measured before the display font loads used the fallback font's
    // metrics — remeasure once real metrics exist.
    document.fonts?.ready?.then(() => {
      measureCache.clear();
      updateLabels();
    });
    return () => tooltip?.remove();
  });

  // A pinch on the quilt is a quilt zoom, never a page zoom — including over
  // the labels layer, which sits on top of the svg d3 already guards.
  $effect(() => {
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
      {#if filterTags.length > 0}
        <button class="btn btn-secondary" onclick={onClearFilter}>Clear filter</button>
      {/if}
    {:else}
      <p>This quilt is empty.</p>
      <p class="muted">Create the first patch to get started.</p>
    {/if}
  </div>
{:else}
  <div class="canvas-container lt-fill-canvas lt-texture-grain" bind:this={containerEl}></div>
  <div class="shadows-layer" bind:this={shadowsEl}></div>
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
      {#if filterTags.length > 0}
        <button class="btn btn-secondary" onclick={onClearFilter}>Clear filter</button>
      {/if}
    </div>
  {/if}
{/if}

<div class="canvas-tooltip" bind:this={tooltip}></div>

<style>
  .my-patch-star,
  .my-patch-heart {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    margin-left: 3px;
    filter: drop-shadow(0 1px 1px rgba(0,0,0,0.3));
  }

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

  .shadows-layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    z-index: 0;
    pointer-events: none;
    overflow: hidden;
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
    padding: 3px 8px 3px 3px;
    border-radius: 6px;
    font-size: 13px;
    font-weight: 600;
    pointer-events: auto;
    cursor: pointer;
    touch-action: none;
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 6px;
    line-height: 1.3;
    border: 2px solid var(--lt-thread);
    font-family: 'Space Grotesk Variable', system-ui, sans-serif;
    color: var(--color-text);
  }

  :global(.patch-label.selected) {
    font-weight: 700;
    border-color: var(--color-primary);
  }

  :global(.patch-label .label-badge) {
    flex-shrink: 0;
    width: 26px;
    height: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    /* lt-resin handles border-radius, dome highlight, and depth */
  }

  :global(.patch-label .label-icon) {
    filter: drop-shadow(0 0 1px rgba(0,0,0,0.3));
  }

  :global(.patch-label .label-name) {
    font-size: 13px;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    text-overflow: ellipsis;
    /* Even two-line splits; mid-word breaks stay as the escape hatch for
       pathological unbroken strings only. */
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
