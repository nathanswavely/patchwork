# ADR 077: A dependency every fork inherits cannot require a key

Date: 2026-08-31. Status: **accepted** — built. Applies ADR 002's boundary
(what travels, what stays behind) to a dependency rather than to data.

## Context

CARTO began stamping "API KEY REQUIRED" across anonymous basemap tiles.
Nothing expired: Patchwork never had a CARTO key. It requested
`light_all`/`dark_all` from `basemaps.cartocdn.com` the way a hobby project
does, and CARTO changed the terms under it. The tiles still return 200, so
the failure is silent to the server and loud to every visitor — the
watermark is baked into the image.

The tile URLs lived in two components, `MapView.svelte` and
`MapLocationPicker.svelte`, each with its own copy of the URL, the
subdomains, and the attribution string.

CLAUDE.md said "Maps: Leaflet (no API key)". That line was a constraint,
not a description. Patchwork is meant to be seamripped: the reference
instance is one deployment among however many forks follow, and each one
inherits every dependency written into the source. ADR 002 draws that line
for data — keys, sessions, and AP identity stay behind. A basemap that
needs an account draws the same line through a *dependency*, and lands on
the wrong side of it: the fork inherits the code and cannot inherit the
credential.

## Decision

**1. Keyless, or it isn't a default.** A tile provider that requires
registration can be something an instance opts into; it cannot be what the
repository ships. The reference instance would look correct and every fork
would wear the watermark until someone noticed and signed up — the failure
lands on the person least equipped to diagnose it, on a screen they did not
build.

**2. OpenFreeMap vector tiles, `positron` and `dark`.** No key, no
metering, and self-hostable by anyone who outgrows the public server, which
is the same escape hatch the rest of the project offers. The styles descend
from the same cartography as CARTO's Positron and Dark Matter, so the map
reads as it did the day before and MapView's tile-pane tint — tuned to warm
the basemap toward the cream paper background — still lands on the colours
it was tuned against.

**3. The renderer loads on first draw, not on first page.** MapLibre is
about a quarter of the whole bundle, and only two surfaces need it. It sits
behind a dynamic import in `basemap.js`, so the quilt, a patch page, and
the feed cost nothing for it. The main bundle is unchanged at ~346 kB gz;
the renderer is a 274 kB gz chunk that arrives when a map does.

**4. One module owns the basemap.** `web/src/lib/basemap.js` holds the
style URLs, the attribution, the zoom ceiling, and the fallback. Two
components asking the same question two different ways is how one of them
gets fixed and the other doesn't — the picker was already a theme behind
the map view before this change.

**5. A blank map is worse than a plain one.** Vector tiles need WebGL, and
a browser grants a limited number of contexts. Where one can't be had, or
the context is lost, or no tile is parsed inside 12 seconds, the layer is
replaced with plain OSM raster filtered per theme. The timeout does not run
against a hidden document: a backgrounded tab throttles the frames MapLibre
draws in, and a map nobody is looking at has not failed.

The signal is a parsed tile because the obvious signals both lie. `loaded()`
waits on every tile in view and stays false for a long time on a slow link.
`load` looks right and is worse: it means the style parsed and a frame was
drawn, and *an empty frame counts* — so a map rendering nothing at all
reports success. A tile reaching the map is the one thing that cannot be
true unless the whole pipeline, worker included, is working.

OSM raster is a fallback and deliberately not the default. Their tile
usage policy asks distributed applications not to point at donated
infrastructure by default, and a self-hostable platform is exactly the case
that policy is written about. As a rare fallback it is a bounded load; as
the shipped default it would be every fork's traffic forever.

**6. A theme change restyles, it does not rebuild.** `setStyle` on the
existing map, rather than tearing down a GL context and building another on
every light/dark toggle.

## Considered and rejected

**Register a CARTO key and add a config field.** Keeps the exact current
look and moves the problem downstream — decision 1 is the whole argument
against it. Worth noting it was the *cheapest* option, and cheap for
exactly one deployment.

**Plain OSM raster with CSS filters as the default.** Genuinely tempting:
no new dependency, no WebGL, about twenty lines. Rejected on the usage
policy above, and on the dark theme — inverting raster tiles inverts their
labels too, which is a trick that reads as a trick.

**Stadia, or another free tier with domain registration.** Same shape as
the CARTO key: works here, breaks on every fork.

**Self-hosting tiles inside the binary.** The planet is tens of gigabytes.
The binary has to run on a Raspberry Pi 4 with 2GB of RAM.

## Consequences

- Two dependencies added: `maplibre-gl` and
  `@maplibre/maplibre-gl-leaflet`. Leaflet still owns the map — markers,
  popups, `fitBounds`, the pinch handling — and the GL layer sits in
  Leaflet's tile pane, which is why the existing pane filter still applies
  to it.
- Attribution now comes from the style itself: OpenFreeMap, OpenMapTiles,
  and OpenStreetMap contributors, rendered in Leaflet's own control.
- An instance that loses reach to `tiles.openfreemap.org` degrades to
  raster rather than to nothing. A fork that wants its own tile server
  changes two constants.
- **The swap exposed a defect it did not cause.** `MapView` read its own
  `map` rune inside the effect that wrote it, so the effect depended on
  itself and re-ran, throwing `Map container is already initialized` on
  every load. Raster tiles reattached to the second map and nobody saw it;
  an asynchronously-loaded layer did not. Both components now work off a
  local handle. The same pattern is worth looking for wherever an effect
  writes a rune and then reads it back.
- Checking for WebGL support costs a real WebGL context. Probing on every
  mount starved the live maps and left them blank — the probe now answers
  once and hands its context straight back. This was found by reading the
  page, not by the suite; there is still no Svelte render library here.
- **MapLibre's tile worker has to be handed its address.** Left alone,
  MapLibre finds the worker by assembling a URL at runtime —
  `new URL('./maplibre-gl-worker.mjs', import.meta.url)`, built from a
  variable. A bundler can only emit a file it can see, and that expression
  is computed, so Vite emitted none. In production the URL resolved against
  the app's own chunk, hit the SPA's index.html fallback, and came back as
  `text/html`. The worker never started, no tile was ever parsed, and every
  map Patchwork drew was blank behind its markers. `?worker&url` plus
  `setWorkerUrl` makes it a real build input, hashed like any other asset.

  It hid for a release because it is silent from both ends. Nothing in the
  build warns: the app compiles, the chunk loads, and the missing file is a
  200 rather than a 404 precisely because the SPA fallback answers anything.
  And the raster fallback above — which exists for exactly this, a GL map
  showing nothing — did not fire, because it was watching `load`. A safety
  net is only as good as the thing it measures, which is why point 5 now
  measures a tile. Read the page: a suite asserting on source text could
  not have caught this either.
