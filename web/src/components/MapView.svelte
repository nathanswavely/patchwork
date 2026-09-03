<script>
  import { untrack } from 'svelte';
  import L from 'leaflet';
  import 'leaflet/dist/leaflet.css';
  import { getResolvedTheme } from '../stores/theme.svelte.js';
  import { identityColorForPatch, textOnColor } from '../lib/quiltTheme.js';
  import { blockPageZoom } from '../lib/pageZoom.js';
  import { addBasemap, BASEMAP_MAX_ZOOM } from '../lib/basemap.js';
  import { clusterNodes, patchActivity } from '../lib/mapClusters.js';
  import { placeLabels } from '../lib/mapLabels.js';
  import { createMotifGroup } from '../lib/patchIcons.js';
  import { getTagVocabulary } from '../stores/quilt.svelte.js';

  // How close two markers must be before they collapse into one disc. Sized
  // against the teardrop (26px wide) plus room to read between them.
  const CLUSTER_RADIUS_PX = 44;

  // insetRight (0–1): fraction of width covered by the floating cards panel
  // on desktop, so markers fit into the visible left portion instead of
  // hiding behind the cards. 0 on mobile (cards are a separate pane).
  // onInViewChange reports which patches are inside the visible map, for the
  // in-view lens (docs/adr/074). The map only reports; the parent decides
  // whether anything narrows.
  let {
    nodes = [],
    center = null,
    radius = 10,
    onMarkerClick = null,
    onBackgroundClick = null,
    insetRight = 0,
    onInViewChange = null,
    announceOffscreen = true,
    // Pointing at a patch previews it (docs/adr/078): the map reports what
    // the pointer is over, and renders emphasis for whatever the parent says
    // is being previewed — which may be a card the pointer is over instead.
    onPatchHover = null,
    hoveredId = null,
  } = $props();

  let mapContainer;
  let map = $state(null);
  let basemap = $state(null);
  let basemapTheme = null; // the theme the basemap is currently drawn in
  let markersLayer;
  let markerById = new Map(); // id → marker, for highlighting without rebuilding
  let hasFit = false; // the viewport is the person's once they have it

  // A quilt-colored teardrop pin: filled with the patch's identity color
  // (its palette primary — the same color the quilt tile uses), so a patch
  // reads the same on the map as on the quilt. divIcon keeps it self-hosted
  // (no external marker sprites) and themeable.
  function patchMarkerIcon(node) {
    const fill = identityColorForPatch(node);
    const glyph = textOnColor(fill); // the motif reads against the fill
    // The motif is the one thing a marker says beyond where it is
    // (docs/adr/078): a 26px teardrop carries one glyph, and unclaimed and
    // role live on the card. Same mark the tile wears in its top-left
    // corner, so a patch reads the same on both surfaces.
    const motif = createMotifGroup(node, 12, glyph).outerHTML;
    const html =
      `<svg width="26" height="34" viewBox="0 0 24 32" xmlns="http://www.w3.org/2000/svg">` +
      `<path d="M12 0C5.4 0 0 5.4 0 12c0 9 12 20 12 20s12-11 12-20C24 5.4 18.6 0 12 0z" ` +
      `fill="${fill}" stroke="rgba(0,0,0,0.35)" stroke-width="1"/>` +
      `<g transform="translate(6,6)">${motif}</g>` +
      `</svg>`;
    return L.divIcon({
      html,
      className: 'patch-marker',
      iconSize: [26, 34],
      iconAnchor: [13, 34],
      popupAnchor: [0, -32],
    });
  }

  $effect(() => {
    if (!mapContainer) return;

    const initial = untrack(() => center);
    const defaultCenter = initial?.lat && initial?.lng
      ? [initial.lat, initial.lng]
      : [40.0379, -76.3055]; // Lancaster, PA fallback

    // Held locally as well as in state: reading the `map` rune back inside
    // the effect that wrote it would make this effect depend on itself and
    // tear the map down mid-setup.
    const instance = L.map(mapContainer, { fadeAnimation: false, maxZoom: BASEMAP_MAX_ZOOM })
      .setView(defaultCenter, 12);
    map = instance;

    // The basemap loads its renderer on demand, so it lands a tick or two
    // after the map; the theme effect below picks it up when it does.
    const startTheme = untrack(() => getResolvedTheme());
    addBasemap(instance, startTheme).then((b) => {
      basemap = b;
      basemapTheme = b ? startTheme : null;
    });

    // Leaflet caches the container size at init; recompute whenever it
    // changes — a viewport resize across the mobile/desktop breakpoint, or
    // the pane being shown after starting hidden (mobile List → Map).
    const ro = new ResizeObserver(() => {
      if (map) map.invalidateSize();
    });
    ro.observe(mapContainer);

    // The in-view lens follows the map wherever it settles. fitBounds emits
    // moveend too, so the first report comes from the initial fit.
    instance.on('moveend', reportInView);
    instance.on('zoomend', reportInView);

    // A pinch on the map is a map zoom, never a page zoom (iOS Safari scales
    // the document on gesture events regardless of Leaflet's touch-action).
    const unblockZoom = blockPageZoom(mapContainer);

    // A press on the map itself, rather than on a marker, dismisses whatever
    // the last press summoned.
    instance.on('click', () => onBackgroundClick && onBackgroundClick());

    // Pixel separation changes with zoom, so the grouping is recomputed
    // there. Panning never changes it.
    const onZoom = () => { if (map) updateMarkers(); };
    instance.on('zoomend', onZoom);

    return () => {
      ro.disconnect();
      unblockZoom();
      instance.off('zoomend', onZoom);
      if (map) {
        instance.off('moveend', reportInView);
        instance.off('zoomend', reportInView);
        instance.remove();
        map = null;
        basemap = null;
        basemapTheme = null;
        markersLayer = null;
      }
    };
  });

  // Emphasis for the patch being previewed. A class toggle on the marker
  // already on screen, never a rebuild: the pointer moves between cards far
  // faster than a marker layer can be torn down and stitched again.
  let emphasised = null;
  $effect(() => {
    const id = hoveredId;
    void markersLayer; // re-apply after a rebuild drops the classes
    if (emphasised && emphasised !== id) {
      markerById.get(emphasised)?.getElement()?.classList.remove('is-previewing');
      emphasised = null;
    }
    const el = id ? markerById.get(id)?.getElement() : null;
    if (el) {
      el.classList.add('is-previewing');
      emphasised = id;
    }
  });

  // Tiles follow the app theme. Reruns when the basemap lands too, so a
  // theme toggled while its renderer was still loading still takes.
  $effect(() => {
    const theme = getResolvedTheme();
    if (!basemap || theme === basemapTheme) return;
    basemapTheme = theme;
    basemap.setTheme(theme);
  });

  $effect(() => {
    void nodes;
    void insetRight;
    // A motif resolves through the instance's tag vocabulary, which arrives
    // on its own fetch. Markers are stamped imperatively and never re-render
    // themselves, so a marker built before the vocabulary lands would wear
    // the fallback quilt mark forever — the cards beside it, being ordinary
    // components, would show the real motif and disagree.
    void getTagVocabulary().length;
    if (map) updateMarkers();
  });

  // A cluster is not a patch: neutral dark disc, a count, no identity color
  // and no motif (it has as many of each as it has members). Same rule the
  // quilt uses — identity wears the patch's color, status wears neutral.
  function clusterIcon(count) {
    const size = count > 20 ? 44 : count > 9 ? 38 : 32;
    const html =
      `<svg width="${size}" height="${size}" viewBox="0 0 ${size} ${size}" xmlns="http://www.w3.org/2000/svg">` +
      `<circle cx="${size / 2}" cy="${size / 2}" r="${size / 2 - 2}" fill="#1f2226" ` +
      `stroke="rgba(255,255,255,0.85)" stroke-width="2"/>` +
      `<text x="${size / 2}" y="${size / 2}" fill="#fff" font-size="${size * 0.4}" ` +
      `font-weight="600" text-anchor="middle" dominant-baseline="central" ` +
      `font-family="system-ui, sans-serif">${count}</text>` +
      `</svg>`;
    return L.divIcon({
      html,
      className: 'patch-cluster',
      iconSize: [size, size],
      iconAnchor: [size / 2, size / 2],
    });
  }

  // The map a reader can actually see, in layer-point space — which is what
  // the placement pass projects into; container coordinates would be off by
  // the map pane's own offset. On desktop the cards pane floats over the
  // right of the canvas, so that strip is not visible map: a name under the
  // pane is a name nobody reads, and a marker under it is not on screen.
  function visibleRect() {
    const size = map.getSize();
    const padRight = Math.round(size.x * insetRight);
    const topLeft = map.containerPointToLayerPoint([4, 4]);
    const bottomRight = map.containerPointToLayerPoint([size.x - padRight - 4, size.y - 4]);
    return {
      left: topLeft.x,
      top: topLeft.y,
      right: bottomRight.x,
      bottom: bottomRight.y,
    };
  }

  // Bring them back: the affordance names the fact and this does the moving,
  // which is the whole reason narrowing never moves the viewport by itself.
  //
  // Animated when a person asked for it — the motion is what tells them where
  // they were taken. Never animated for the opening fit: nobody asked to be
  // moved off a default view they never saw, and animating it means the map's
  // first painted state is every marker piled into one disc at a zoom that
  // was never intended, which reads as a map failing to load.
  function showAll(animate = true) {
    const placed = nodes.filter((n) => n.latitude != null && n.longitude != null);
    if (!placed.length) return;
    const padRight = Math.round((mapContainer?.clientWidth || 0) * insetRight);
    map.fitBounds(L.latLngBounds(placed.map((n) => [n.latitude, n.longitude])), {
      paddingTopLeft: [24, 24],
      paddingBottomRight: [24 + padRight, 24],
      animate,
    });
  }

  // The font the names are drawn in, read once from the rendered label so
  // measurement matches what the browser will paint.
  let labelFont = '600 12px system-ui, sans-serif';
  let labelled = new Map(); // id → the position its name took, kept across passes

  // Patches that pass the lenses but sit outside what the reader can see.
  // Only ever announced when *none* are visible: a map that has gone empty
  // is the case a person cannot explain to themselves, and the same
  // discipline as an empty result naming its lenses (docs/adr/022,
  // docs/adr/078). Panning to an empty stretch of river deserves the same
  // sentence as filtering to a tag whose patches are all uptown.
  let offscreen = $state(0);

  function markerFor(node, position) {
    const marker = L.marker([node.latitude, node.longitude], {
      icon: patchMarkerIcon(node),
      // Heavier patches sit on top, so the marker you can click is the one
      // whose name you can read (docs/adr/078).
      zIndexOffset: Math.min(patchActivity(node), 500),
    });

    // A name is a tooltip either way; what the collision pass decides is
    // whether it stands open. The rest reveal on hover — and on touch,
    // where Leaflet opens a tooltip on tap, which is the same promise.
    // A name is a tooltip either way; the placement pass decides only whether
    // it stands open, and on which side it sits when it does.
    marker.bindTooltip(node.name, {
      permanent: !!position,
      direction: position ? position.dir : 'right',
      offset: position ? position.offset : [3, -22],
      className: 'map-name',
      opacity: 1,
    });

    marker.on('click', () => onMarkerClick && onMarkerClick(node));
    if (onPatchHover) {
      marker.on('mouseover', () => onPatchHover(node));
      marker.on('mouseout', () => onPatchHover(null));
    }
    markerById.set(node.id, marker);
    return marker;
  }

  function updateMarkers() {
    const placed = nodes.filter((n) => n.latitude != null && n.longitude != null);

    // Frame first, then group. Grouping is a function of zoom, so grouping at
    // the default zoom and fitting afterwards paints one 33-marker disc that
    // exists for no reason and is gone a moment later.
    if (!hasFit && placed.length > 0) {
      hasFit = true;
      showAll(false);
    }

    if (markersLayer) map.removeLayer(markersLayer);
    markerById = new Map();
    const groups = clusterNodes(map, placed, CLUSTER_RADIUS_PX);

    // Names are placed by separation, never by zoom: zoom is only what moves
    // markers apart, and two patches at one address never separate at all.
    const visible = visibleRect();

    labelled = placeLabels(
      groups,
      (latlng) => map.latLngToLayerPoint(latlng),
      labelFont,
      labelled,
      visible,
    );

    const layers = [];
    for (const group of groups) {
      if (group.members.length === 1) {
        layers.push(markerFor(group.lead, labelled.get(group.lead.id)));
        continue;
      }
      const disc = L.marker(group.latlng, { icon: clusterIcon(group.members.length) });
      disc.on('click', () => {
        map.fitBounds(
          L.latLngBounds(group.members.map((n) => [n.latitude, n.longitude])),
          { padding: [60, 60], maxZoom: BASEMAP_MAX_ZOOM },
        );
      });
      layers.push(disc);
    }

    markersLayer = L.layerGroup(layers).addTo(map);

    // Nothing placed yet: sit on the instance's own centre until something
    // arrives. The fit above claims `hasFit` only when it had markers to
    // frame, so the first real set still gets its one fit.
    if (!hasFit && placed.length === 0 && center?.lat && center?.lng) {
      const zoom = Math.round(14 - Math.log2(radius || 10));
      map.setView([center.lat, center.lng], Math.max(zoom, 3));
    }

    // A marker set can change without the map moving (a filter toggle that
    // does not shift the fit), and moveend would never fire.
    reportInView();
  }

  // --- The in-view lens (docs/adr/074), the map's half ---
  // Container points rather than getBounds(), so the cards pane's strip is
  // excluded exactly as it is on the quilt: a marker behind the floating
  // cards is on the map but not in view. A patch with no coordinates is on
  // neither, and drops out — which is the honest answer on this surface.
  //
  // The out-of-view affordance (docs/adr/078) asks the same question this
  // does — which markers are on screen — so it is answered once here rather
  // than by a second pass with its own idea of where the edge is. The lens
  // wants the ids; the affordance wants only whether the answer was none.
  let lastInView = '';
  function reportInView() {
    if (!map) return;
    const w = mapContainer?.clientWidth || 0;
    const h = mapContainer?.clientHeight || 0;
    if (!w || !h) return;
    const rightEdge = w - w * insetRight;
    const ids = [];
    let placed = 0;
    for (const node of nodes) {
      if (node.latitude == null || node.longitude == null) continue;
      placed += 1;
      const p = map.latLngToContainerPoint([node.latitude, node.longitude]);
      if (p.x < 0 || p.x > rightEdge || p.y < 0 || p.y > h) continue;
      ids.push(node.id);
    }

    // Announced only when the map has gone completely empty — the one state
    // a person cannot explain to themselves. "Some are off screen" is true
    // at almost every street zoom and would be permanent furniture.
    offscreen = ids.length === 0 ? placed : 0;

    if (!onInViewChange) return;
    const key = ids.join(',');
    if (key === lastInView) return;
    lastInView = key;
    onInViewChange(ids);
  }

</script>

<div class="map-wrapper">
  <div bind:this={mapContainer} class="map-container"></div>

  <!-- Narrowing the set never moves the viewport (docs/adr/078), which
       leaves one case needing an explanation: everything that matches is
       somewhere else, and the map has gone blank. Naming it beats a jump. -->
  {#if offscreen > 0 && announceOffscreen}
    <!-- Centred on the map a reader can see, not on the element: the cards
         pane covers the right of the canvas on desktop. -->
    <button class="map-offscreen" style="left: {50 - insetRight * 50}%" onclick={showAll}>
      <!-- Written as markup rather than an interpolated string so the words
           are visible to the copy ledger. Text inside a {…} expression is
           stripped as markup noise, which would quietly exempt a sentence a
           visitor reads from ever being reviewed. -->
      {#if offscreen === 1}
        <b>1</b> patch is outside this view
      {:else}
        <b>{offscreen}</b> patches are outside this view
      {/if}
    </button>
  {/if}
</div>

<style>
  .map-wrapper {
    width: 100%;
    height: 100%;
    overflow: hidden;
  }

  .map-container {
    width: 100%;
    height: 100%;
    min-height: 400px;
    /* Paper-toned base so the map reads as raw cotton behind the app, not a
       stark white rectangle. */
    background: var(--color-bg);
  }

  /* Tint only the tile pane (markers/controls live in other panes, so they
     stay true-color): warm the near-white basemap toward the cream paper
     bg. Applies to the MapLibre canvas the same as to raster tiles — the GL
     layer sits in this pane. */
  .map-wrapper :global(.leaflet-tile-pane) {
    filter: sepia(0.28) saturate(0.72) brightness(0.99) contrast(0.96);
  }

  /* Dark: sepia+hue-rotate injects a cool cast into the neutral-grey dark
     tiles so they lean toward the denim bg. */
  :global([data-theme="dark"]) .map-wrapper :global(.leaflet-tile-pane) {
    filter: brightness(0.82) sepia(0.4) hue-rotate(178deg) saturate(0.7);
  }

  /* Both rules above assume tiles that already carry the target look, which
     is true of the vector styles and false of the raster fallback: that one
     builds dark by inverting light OSM tiles (basemap.js), and darkening an
     inverted tile with a tint meant for an already-dark one composes to a
     near-black rectangle. The fallback carries its own complete filter, so
     when it is in play this pane contributes nothing. Four classes outranks
     the dark rule's three, so this wins wherever it sits in the file. */
  .map-wrapper :global(.leaflet-container.basemap-raster .leaflet-tile-pane) {
    filter: none;
  }

  /* The map sits full-bleed behind the fixed global bar (56px) — keep the
     zoom controls clear of it. */
  .map-wrapper :global(.leaflet-top) {
    top: 64px;
  }

  /* On mobile the bottom nav bar overlaps the map's lower edge — lift the
     attribution above it. */
  @media (max-width: 768px) {
    .map-wrapper :global(.leaflet-bottom) {
      bottom: 60px;
    }
  }

  /* Quilt-colored teardrop markers get a soft ground shadow for depth. */
  .map-wrapper :global(.patch-marker) {
    filter: drop-shadow(0 2px 2px rgba(0, 0, 0, 0.3));
  }

  /* Textile popups: surface card, hairline border, app radius + font. */
  /* Sits over the map it is describing, clear of the zoom controls and of
     the cards pane. Not an Interruption (CONTEXT.md) — nothing is wrong,
     the reader has simply moved away from everything. */
  .map-offscreen {
    position: absolute;
    top: 12px;
    transform: translateX(-50%);
    z-index: 600;
    padding: 0.35rem 0.7rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-surface);
    color: var(--color-text);
    font-family: var(--font);
    font-size: 0.8rem;
    box-shadow: 0 2px 10px var(--color-shadow);
    cursor: pointer;
  }

  .map-offscreen b {
    font-variant-numeric: tabular-nums;
  }

  .map-offscreen:hover {
    border-color: var(--color-primary);
  }

  /* A name on the map is halo'd text, never a pill. The quilt's name badge
     is a pill because it floats over fabric it must stay readable against;
     a basemap is not fabric, and a field of pills reads as furniture on a
     surface whose whole job is location (docs/adr/078). */
  .map-wrapper :global(.map-name) {
    background: none;
    border: none;
    box-shadow: none;
    padding: 0;
    margin: 0;
    font-family: var(--font);
    font-size: 12px;
    font-weight: 600;
    color: var(--color-text);
    white-space: nowrap;
    /* Legible over any tile the basemap draws underneath. */
    paint-order: stroke fill;
    -webkit-text-stroke: 3px var(--color-bg);
    text-shadow: 0 1px 2px var(--color-bg);
  }

  .map-wrapper :global(.map-name::before) {
    display: none; /* Leaflet's tooltip arrow */
  }

  /* The previewed patch's pin lifts toward the reader — the map's half of
     the same gesture that highlights its card.
     The scale goes on the SVG *inside* the marker, never on the marker
     itself. Leaflet positions a marker with `transform: translate3d(...)`,
     and the standalone `scale` property does not sit beside a transform —
     CSS composes them as translate → rotate → scale → transform, so scaling
     the marker multiplies Leaflet's translation by the same factor and
     throws the pin a quarter of its own offset across the map. Pins near
     the pane's origin barely move; distant ones fly. The child carries no
     Leaflet transform, so scaling it is just scaling it. */
  .map-wrapper :global(.patch-marker svg) {
    transition: scale 120ms ease;
    transform-origin: 50% 100%; /* the pin's point stays on its coordinate */
  }

  .map-wrapper :global(.patch-marker.is-previewing) {
    z-index: 650 !important;
  }

  .map-wrapper :global(.patch-marker.is-previewing svg) {
    scale: 1.25;
    filter: drop-shadow(0 3px 5px rgba(0, 0, 0, 0.5));
  }

  /* A cluster is not a patch, so it wears no identity color and no motif —
     the neutral dark disc the quilt gives to status (docs/adr/078). */
  .map-wrapper :global(.patch-cluster) {
    filter: drop-shadow(0 2px 3px rgba(0, 0, 0, 0.4));
    cursor: pointer;
  }

  /* Theme the attribution + zoom controls so they read as app chrome. */
  .map-wrapper :global(.leaflet-control-attribution) {
    background: var(--color-glass);
    color: var(--color-text-muted);
    font-family: var(--font);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }

  .map-wrapper :global(.leaflet-control-attribution a) {
    color: var(--color-primary);
  }

  /* Zoom control: textile card with a hairline border, no default chrome. */
  .map-wrapper :global(.leaflet-control-zoom) {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    box-shadow: 0 2px 10px var(--color-shadow);
    overflow: hidden;
  }

  .map-wrapper :global(.leaflet-control-zoom a) {
    background: var(--color-surface);
    color: var(--color-text);
    border-color: var(--color-border);
    font-weight: 600;
  }

  .map-wrapper :global(.leaflet-control-zoom a:hover) {
    background: var(--color-overlay);
    color: var(--color-primary);
  }
</style>
