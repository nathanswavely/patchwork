<script>
  import { untrack } from 'svelte';
  import L from 'leaflet';
  import 'leaflet/dist/leaflet.css';
  import { blockPageZoom } from '../lib/pageZoom.js';
  import { formatCoord, roundCoord, hasMapLocation } from '../lib/mapLocation.js';
  import { addBasemap, BASEMAP_MAX_ZOOM } from '../lib/basemap.js';

  // A deliberate placement surface (issue #4): the admin drags or clicks a
  // single marker, sees the chosen coordinates, and saves explicitly. Nothing
  // is written on drag — the parent owns the save call.
  //
  // A `suggestion` seeds that marker from the gazetteer (docs/adr/080). It is
  // drawn as a proposal, not a placement: hollow, dashed, and captioned with
  // what it thinks it found. It becomes an ordinary marker the moment the
  // person moves it, and it becomes a map location only when they confirm.
  let {
    lat = null,
    lng = null,
    center = null,
    saving = false,
    suggestion = null,
    confirmLabel = 'Save location',
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

  // True while the marker on screen is the gazetteer's proposal and nobody
  // has touched it. Confirming is what turns it into a placement, so this is
  // the flag that keeps the two apart on screen as well as in the model.
  let provisional = $state(false);

  function markerIcon(proposed = false) {
    // Self-hosted teardrop in the app's primary color — no external sprite.
    const primary =
      getComputedStyle(document.documentElement)
        .getPropertyValue('--color-primary')
        .trim() || '#7c3aed';
    // A proposal is hollow and dashed; a placement is solid. The difference
    // has to be visible, because the whole point of the confirm step is that
    // somebody looked at the marker before it became the answer.
    const html = proposed
      ? `<svg width="28" height="38" viewBox="0 0 24 32" xmlns="http://www.w3.org/2000/svg">` +
        `<path d="M12 0C5.4 0 0 5.4 0 12c0 9 12 20 12 20s12-11 12-20C24 5.4 18.6 0 12 0z" ` +
        `fill="${primary}" fill-opacity="0.25" stroke="${primary}" stroke-width="1.5" ` +
        `stroke-dasharray="3 2"/>` +
        `<circle cx="12" cy="12" r="4.5" fill="#fff" stroke="${primary}" stroke-width="1.5"/>` +
        `</svg>`
      : `<svg width="28" height="38" viewBox="0 0 24 32" xmlns="http://www.w3.org/2000/svg">` +
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

  function placeAt(la, ln, proposed = false) {
    draftLat = roundCoord(la);
    draftLng = roundCoord(ln);
    provisional = proposed;
    if (!map) return;
    if (marker) {
      marker.setLatLng([draftLat, draftLng]);
      marker.setIcon(markerIcon(proposed));
    } else {
      marker = L.marker([draftLat, draftLng], {
        icon: markerIcon(proposed),
        draggable: true,
      }).addTo(map);
      marker.on('dragend', () => {
        const p = marker.getLatLng();
        draftLat = roundCoord(p.lat);
        draftLng = roundCoord(p.lng);
        // Moving the marker makes it the person's own, not a proposal.
        if (provisional) {
          provisional = false;
          marker.setIcon(markerIcon(false));
        }
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

    // Everything below works off the local handle, never the `map` rune:
    // reading that back inside the effect that wrote it makes the effect
    // depend on itself, and the second pass hits an initialized container.
    const instance = L.map(mapContainer, { fadeAnimation: false, maxZoom: BASEMAP_MAX_ZOOM })
      .setView(start, 13);
    map = instance;

    // The picker stays light in either app theme: placement is easier to
    // read against the pale style, and the marker is high-contrast on it.
    addBasemap(instance, 'light');

    // Seed the marker if there is already a saved location.
    if (hasMapLocation(lat, lng)) untrack(() => placeAt(lat, lng));

    // Click anywhere to place or move the marker.
    instance.on('click', (e) => placeAt(e.latlng.lat, e.latlng.lng));

    const ro = new ResizeObserver(() => instance.invalidateSize());
    ro.observe(mapContainer);
    const unblockZoom = blockPageZoom(mapContainer);

    return () => {
      ro.disconnect();
      unblockZoom();
      instance.remove();
      map = null;
      marker = null;
    };
  });

  // A lookup is asynchronous, so the suggestion can arrive before or after
  // the map exists. This effect covers both, and never overrides a location
  // already saved or a marker the person has moved themselves.
  $effect(() => {
    const s = suggestion;
    if (!s || !hasMapLocation(s.latitude, s.longitude)) return;
    if (hasMapLocation(lat, lng)) return;
    untrack(() => {
      if (hasDraft && !provisional) return;
      placeAt(s.latitude, s.longitude, true);
      if (map) map.setView([s.latitude, s.longitude], 16);
    });
  });

  function save() {
    if (!hasDraft || saving || !onSave) return;
    onSave(draftLat, draftLng);
  }
</script>

<div class="picker">
  <div bind:this={mapContainer} class="picker-map"></div>

  {#if provisional && suggestion?.label}
    <p class="picker-suggestion">
      Suggested from the address: <strong>{suggestion.label}</strong>
    </p>
  {/if}

  <p class="picker-hint">
    {#if provisional}
      This is a guess, not a placement. Drag the marker or click elsewhere to
      correct it, and nothing goes on the map until you confirm.
    {:else}
      Click the map to drop the marker, then drag it to adjust. Place it as
      precisely or as loosely as you like. Nothing is saved until you hit
      {confirmLabel}.
    {/if}
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
      {saving ? 'Saving...' : provisional ? 'Use this spot' : confirmLabel}
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

  .picker-suggestion {
    font-size: 0.82rem;
    margin: 0.5rem 0 0;
    color: var(--color-text);
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
