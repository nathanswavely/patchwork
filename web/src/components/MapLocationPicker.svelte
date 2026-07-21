<script>
  import { untrack } from 'svelte';
  import L from 'leaflet';
  import 'leaflet/dist/leaflet.css';
  import { blockPageZoom } from '../lib/pageZoom.js';
  import { formatCoord, roundCoord, hasMapLocation } from '../lib/mapLocation.js';

  // A deliberate placement surface (issue #4): the admin drags or clicks a
  // single marker, sees the chosen coordinates, and saves explicitly. Nothing
  // is written on drag — the parent owns the save call.
  let {
    lat = null,
    lng = null,
    center = null,
    saving = false,
    onSave = null,
    onCancel = null,
  } = $props();

  let mapContainer;
  let map = $state(null);
  let marker = null;

  // The draft position, starting from any saved location. null until the
  // admin places a marker — Save stays disabled while it is null.
  let draftLat = $state(hasMapLocation(lat, lng) ? lat : null);
  let draftLng = $state(hasMapLocation(lat, lng) ? lng : null);

  let hasDraft = $derived(draftLat != null && draftLng != null);
  let readout = $derived(hasDraft ? formatCoord(draftLat, draftLng) : '');

  function markerIcon() {
    // Self-hosted teardrop in the app's primary color — no external sprite.
    const primary =
      getComputedStyle(document.documentElement)
        .getPropertyValue('--color-primary')
        .trim() || '#7c3aed';
    const html =
      `<svg width="28" height="38" viewBox="0 0 24 32" xmlns="http://www.w3.org/2000/svg">` +
      `<path d="M12 0C5.4 0 0 5.4 0 12c0 9 12 20 12 20s12-11 12-20C24 5.4 18.6 0 12 0z" ` +
      `fill="${primary}" stroke="rgba(0,0,0,0.4)" stroke-width="1"/>` +
      `<circle cx="12" cy="12" r="4.5" fill="#fff"/>` +
      `</svg>`;
    return L.divIcon({
      html,
      className: 'place-marker',
      iconSize: [28, 38],
      iconAnchor: [14, 38],
    });
  }

  function placeAt(la, ln) {
    draftLat = roundCoord(la);
    draftLng = roundCoord(ln);
    if (!map) return;
    if (marker) {
      marker.setLatLng([draftLat, draftLng]);
    } else {
      marker = L.marker([draftLat, draftLng], {
        icon: markerIcon(),
        draggable: true,
      }).addTo(map);
      marker.on('dragend', () => {
        const p = marker.getLatLng();
        draftLat = roundCoord(p.lat);
        draftLng = roundCoord(p.lng);
      });
    }
  }

  $effect(() => {
    if (!mapContainer) return;

    const start = untrack(() => {
      if (hasMapLocation(lat, lng)) return [lat, lng];
      if (center?.lat != null && center?.lng != null) return [center.lat, center.lng];
      return [40.0379, -76.3055]; // Lancaster, PA fallback (matches MapView)
    });

    map = L.map(mapContainer, { fadeAnimation: false }).setView(start, 13);

    L.tileLayer('https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png', {
      attribution:
        '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>',
      subdomains: 'abcd',
      maxZoom: 20,
    }).addTo(map);

    // Seed the marker if there is already a saved location.
    if (hasMapLocation(lat, lng)) placeAt(lat, lng);

    // Click anywhere to place or move the marker.
    map.on('click', (e) => placeAt(e.latlng.lat, e.latlng.lng));

    const ro = new ResizeObserver(() => {
      if (map) map.invalidateSize();
    });
    ro.observe(mapContainer);
    const unblockZoom = blockPageZoom(mapContainer);

    return () => {
      ro.disconnect();
      unblockZoom();
      if (map) {
        map.remove();
        map = null;
        marker = null;
      }
    };
  });

  function save() {
    if (!hasDraft || saving || !onSave) return;
    onSave(draftLat, draftLng);
  }
</script>

<div class="picker">
  <div bind:this={mapContainer} class="picker-map"></div>

  <p class="picker-hint">
    Click the map to drop the marker, then drag it to adjust. Place it as
    precisely or as approximately as you like — nothing is saved until you
    save.
  </p>

  <div class="picker-readout">
    {#if hasDraft}
      <span class="picker-coords">{readout}</span>
    {:else}
      <span class="muted">No marker placed yet.</span>
    {/if}
  </div>

  <div class="picker-actions">
    <button class="btn btn-primary btn-sm" onclick={save} disabled={!hasDraft || saving}>
      {saving ? 'Saving...' : 'Save location'}
    </button>
    <button class="btn btn-secondary btn-sm" onclick={() => onCancel && onCancel()} disabled={saving}>
      Cancel
    </button>
  </div>
</div>

<style>
  .picker {
    margin-top: 0.5rem;
  }

  .picker-map {
    width: 100%;
    height: 320px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .picker-map :global(.leaflet-container) {
    font-family: var(--font);
    background: var(--color-bg);
  }

  .picker-map :global(.place-marker) {
    filter: drop-shadow(0 2px 2px rgba(0, 0, 0, 0.3));
  }

  .picker-hint {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    margin: 0.5rem 0 0.35rem;
  }

  .picker-readout {
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
    min-height: 1.2em;
  }

  .picker-coords {
    font-variant-numeric: tabular-nums;
    color: var(--color-text);
  }

  .picker-actions {
    display: flex;
    gap: 0.4rem;
  }

  .btn-sm {
    padding: 0.25rem 0.6rem;
    font-size: 0.78rem;
  }
</style>
