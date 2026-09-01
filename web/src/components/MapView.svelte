<script>
  import { untrack } from 'svelte';
  import L from 'leaflet';
  import 'leaflet/dist/leaflet.css';
  import { getResolvedTheme } from '../stores/theme.svelte.js';
  import { identityColorForPatch, textOnColor } from '../lib/quiltTheme.js';
  import { blockPageZoom } from '../lib/pageZoom.js';
  import { addBasemap, BASEMAP_MAX_ZOOM } from '../lib/basemap.js';

  // insetRight (0–1): fraction of width covered by the floating cards panel
  // on desktop, so markers fit into the visible left portion instead of
  // hiding behind the cards. 0 on mobile (cards are a separate pane).
  let { nodes = [], center = null, radius = 10, onMarkerClick = null, insetRight = 0 } = $props();

  let mapContainer;
  let map = $state(null);
  let basemap = $state(null);
  let basemapTheme = null; // the theme the basemap is currently drawn in
  let markersLayer;

  // A quilt-colored teardrop pin: filled with the patch's identity color
  // (its palette primary — the same color the quilt tile uses), so a patch
  // reads the same on the map as on the quilt. divIcon keeps it self-hosted
  // (no external marker sprites) and themeable.
  function patchMarkerIcon(node) {
    const fill = identityColorForPatch(node);
    const dot = textOnColor(fill); // readable center dot on the fill
    const html =
      `<svg width="26" height="34" viewBox="0 0 24 32" xmlns="http://www.w3.org/2000/svg">` +
      `<path d="M12 0C5.4 0 0 5.4 0 12c0 9 12 20 12 20s12-11 12-20C24 5.4 18.6 0 12 0z" ` +
      `fill="${fill}" stroke="rgba(0,0,0,0.35)" stroke-width="1"/>` +
      `<circle cx="12" cy="12" r="4.5" fill="${dot}"/>` +
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

    // A pinch on the map is a map zoom, never a page zoom (iOS Safari scales
    // the document on gesture events regardless of Leaflet's touch-action).
    const unblockZoom = blockPageZoom(mapContainer);

    return () => {
      ro.disconnect();
      unblockZoom();
      if (map) {
        map.remove();
        map = null;
        basemap = null;
        basemapTheme = null;
        markersLayer = null;
      }
    };
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
    if (map) updateMarkers();
  });

  function updateMarkers() {
    if (markersLayer) {
      map.removeLayer(markersLayer);
    }

    const markers = [];
    for (const node of nodes) {
      if (node.latitude == null || node.longitude == null) continue;

      const marker = L.marker([node.latitude, node.longitude], { icon: patchMarkerIcon(node) });
      marker.bindPopup(popupContent(node));
      markers.push(marker);
    }

    markersLayer = L.layerGroup(markers).addTo(map);

    // Keep markers clear of the floating cards panel on desktop.
    const padRight = Math.round((mapContainer?.clientWidth || 0) * insetRight);
    const fitOpts = {
      paddingTopLeft: [24, 24],
      paddingBottomRight: [24 + padRight, 24],
    };

    if (markers.length > 0) {
      const group = L.featureGroup(markers);
      map.fitBounds(group.getBounds(), fitOpts);
    } else if (center?.lat && center?.lng) {
      // Estimate zoom from radius (km)
      const zoom = Math.round(14 - Math.log2(radius || 10));
      map.setView([center.lat, center.lng], Math.max(zoom, 3));
    }
  }

  function popupContent(node) {
    const div = document.createElement('div');
    div.className = 'map-popup';

    const title = document.createElement('strong');
    title.textContent = node.name;
    div.appendChild(title);

    if (node.description) {
      const snippet = node.description.slice(0, 100) + (node.description.length > 100 ? '…' : '');
      const p = document.createElement('p');
      p.className = 'map-popup-desc';
      p.textContent = snippet;
      div.appendChild(p);
    }

    const link = document.createElement('a');
    link.className = 'map-popup-link';
    link.href = `/patches/${encodeURIComponent(node.slug)}`;
    link.textContent = 'View patch';
    link.addEventListener('click', (e) => {
      if (onMarkerClick) {
        e.preventDefault();
        onMarkerClick(node);
      }
    });
    div.appendChild(link);

    return div;
  }
</script>

<div class="map-wrapper">
  <div bind:this={mapContainer} class="map-container"></div>
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
  .map-wrapper :global(.leaflet-popup-content-wrapper),
  .map-wrapper :global(.leaflet-popup-tip) {
    background: var(--color-surface);
    color: var(--color-text);
    border: 1px solid var(--color-border);
    box-shadow: 0 2px 12px var(--color-shadow);
  }

  .map-wrapper :global(.leaflet-popup-content-wrapper) {
    border-radius: var(--radius);
  }

  .map-wrapper :global(.leaflet-popup-content) {
    font-family: var(--font);
    margin: 0.7rem 0.85rem;
  }

  .map-wrapper :global(.map-popup strong) {
    color: var(--color-text);
    font-size: 0.95rem;
  }

  .map-wrapper :global(.map-popup-desc) {
    margin: 0.25rem 0 0.5rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .map-wrapper :global(.map-popup-link) {
    display: inline-block;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--color-primary);
  }

  .map-wrapper :global(.leaflet-container a.leaflet-popup-close-button) {
    color: var(--color-text-muted);
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
