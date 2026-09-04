# ADR 080: A gazetteer suggests; a person places

Date: 2026-09-04. Status: **accepted**; implemented. Applies docs/adr/077
(a dependency every fork inherits cannot require a key) to a dataset rather
than to a tile server, and amends the **Address** and **Map location**
entries in CONTEXT.md.

## Context

A patch's map location can only be set from Patch Settings. Someone creating
a patch types an address, finishes the form, and lands on a patch that is
nowhere on the map — the two acts are separated by a navigation and a menu,
and the second one mostly does not happen. Most of the quilt is unplaced,
and the map is emptier than the community is.

The obvious fix is to geocode the address the person just typed. The obvious
fix collides with two things this repository has already written down.

**The glossary forbade it, twice and on purpose.** **Address** was "prose
meant for people to read, never parsed and never geocoded." **Map location**
was a marker "never geocoded from any text," independent of the address, and
"naming one never sets the other." That was not an implementation note. It
was a claim about what a coordinate on this map *means*: somebody looked at
a map and said *there*. One of the glossary's own three examples of a valid
address is "above the record shop on Prince St" — text no geocoder resolves,
and text nobody should be nagged about.

**And a hosted geocoder is the CARTO problem again.** ADR 077 removed a
basemap that started demanding a key, and drew the rule: a fork inherits the
code and cannot inherit a credential. It also rejected raster OpenStreetMap
tiles as a *default* — not on quality, but because their usage policy asks
distributed applications not to point at donated infrastructure. Nominatim
is that same donated infrastructure under a stricter policy. Every argument
ADR 077 made against shipping OSM raster by default applies to shipping
Nominatim by default, only harder.

What makes a third answer available is that this software is hyperlocal by
configuration. `geographic.latitude`, `longitude` and `radius` are already
declared in `patchwork.yaml`. An instance does not need the planet. It needs
one radius, and one radius is small.

## Decision

**1. The gazetteer is a file, built offline, copied in.** A CLI crops an
OpenStreetMap extract to the instance's configured radius and writes a
SQLite file. The admin puts it beside `patchwork.db`. The server opens it
read-only and answers lookups from it. Nothing is fetched at request time,
by the server or the browser, so the deployment stays as self-contained as
it was.

The crop happens on whatever machine has the disk, not on the instance. A
state-sized extract runs to hundreds of megabytes and the binary has to run
on a Raspberry Pi 4 with 2GB of RAM. The Pi answers queries; it never parses
a continent.

**2. OpenStreetMap, because the attribution is already being paid.** ADR 077
already renders "OpenStreetMap contributors" on every map, so the licence
surface does not grow. It has global coverage, so a fork outside the United
States gets the same experience — which TIGER, the obvious American answer,
cannot offer. And it carries venue names as well as housenumbers, which
matters because ADR 046 made locations name-first: "The Selvage" is what
somebody types, not a street number.

**3. A suggestion is not a placement.** The lookup produces a **suggested
placement** — a provisional marker, rendered distinctly, saved only when a
person confirms it. Until confirmed the patch has no map location at all.

This is the decision the rest hangs off. The create form submits every field
at once, so an auto-filled marker would be accepted by silence: nobody would
have to look at it, and a wrong point would ship because the form was
submitted rather than because anyone agreed. The confirm step is what keeps
the glossary's claim true. A coordinate on this map is still somebody
saying *there*; the gazetteer only shortens the walk to the right street.

**4. A miss is silent and normal.** No match produces no marker, no error,
and no prompt. The picker still opens, centered on the instance geography,
exactly as it does today. "Above the record shop on Prince St" is a good
address that resolves to nothing, and the interface must agree.

**5. No gazetteer, no suggestion, no difference.** An instance without the
file gets today's behavior: a picker centered on the configured geography
and a marker placed by hand. This is the ADR 077 failure test — the fork
that never installs anything must not end up with a broken screen it did not
build and cannot diagnose. Here it ends up with the status quo.

**6. In settings, only for an unplaced patch.** The same affordance reaches
every patch that already exists, which is where most of the quilt is. It is
withheld from a patch that already has a marker: a one-click path to
overwrite a deliberate human placement with a machine's guess is the exact
inversion of decision 3. Editing the address of a placed patch changes
nothing about its marker.

## Considered and rejected

**Call Nominatim, or any hosted keyless geocoder.** Cheapest by a wide
margin and works immediately. Rejected on ADR 077's reasoning: it makes
every fork's traffic somebody's donated infrastructure forever, under a
usage policy written about exactly this case. The cheap option was cheap for
one deployment last time too.

**A keyed provider behind a config field.** Honest about the cost and
opt-in, which satisfies the letter of ADR 077. Rejected because it is the
CARTO shape again: correct on the reference instance, absent on every fork,
and the feature silently does nothing for whoever did not sign up.

**Import on the server from an admin-supplied URL.** Matches how the idea
was first described — a setup step in the admin panel. Rejected on the
hardware: streaming and cropping a large extract on a Pi is a long,
memory-tight job that can fail halfway, and it needs a progress UI, disk
headroom, and a guarded outbound fetch to do something that only ever
happens once. A file copy has none of those failure modes. Worth
reconsidering if the CLI's output ever becomes something a project publishes
per region, since then the download is small and the crop is already done.

**Build the gazetteer from the quilt's own placed patches.** Free, licence-
clean, no import, and it improves as the community uses it. Rejected for
this use: a patch being created is new by definition, so its address is
precisely the one the quilt has never seen. Genuinely attractive as a
*complement* later, and it is the only option here that costs nothing.

**Let the guess auto-fill and count submission as approval.** Would place
far more patches. Rejected as decision 3 — it converts a placed point into a
derived one and hides wrong answers behind a button nobody read.

## Consequences

- The **Address** and **Map location** entries in CONTEXT.md are amended,
  and **Gazetteer** and **Suggested placement** are added. The rewrite is
  narrow on purpose: prose is still never *stored* as anything but itself,
  and a map location is still a placed point. What changed is that a lookup
  may now *propose*.
- The gazetteer never enters `migrations/`, so it never reaches
  `TestEveryTableHasABoundaryDecision`, which enumerates `sqlite_master`.
  That is the right answer rather than a dodge: it is a cache of somebody
  else's dataset, the same shape as `aggregator_listings`, and it must not
  travel in a seamrip. A fork rebuilds or re-copies it.
- `POST /api/v1/nodes` already accepts `latitude` and `longitude`, so
  creation-time placement needs no migration and no new write path.
- The lookup endpoint is authenticated and rate-limited. There is nothing
  confidential in it — the data is ODbL-derived and public — so the limit is
  about load, not secrecy.
- **The extract is read as XML, with the standard library.** This was left
  open above and resolved during the build, in a way that dissolved half the
  question. The premise that the CLI needed a build tag or its own module was
  simply wrong: `cmd/gazetteer` is its own main package and `cmd/patchwork`
  does not import it, so Go never links its dependencies into the server
  binary either way. The real cost was `go.mod`, which every fork inherits —
  a `.osm.pbf` reader would put protocol buffers in a module that has none,
  forever, to build a file that ships separately. `encoding/xml`,
  `compress/bzip2` and `compress/gzip` cost nothing and read `.osm`, `.osm.gz`
  and `.osm.bz2`. An extract held as `.pbf` converts in one command
  (`osmium cat in.osm.pbf -o out.osm.bz2`), which is a fair ask of somebody
  already running a command-line build step.
- **No FTS5, because this driver has none.** `mattn/go-sqlite3` compiles FTS5
  only under the `sqlite_fts5` build tag and this project sets no tags;
  `CREATE VIRTUAL TABLE ... USING fts5` fails at runtime with "no such module:
  fts5". Verified rather than assumed. The index is a plain token table with
  one index on it, which is enough for a bounded local corpus.
- **A way is placed at its first node, not its centroid.** Buildings carry
  the address on the way rather than on the nodes that draw it, and resolving
  a true centroid means holding geometry the crop cannot bound. The error is
  the width of a building, and somebody is about to look at the marker and
  confirm it.
- **Memory at build time is bounded by the configured radius**, not by the
  extract: pass one keeps coordinates only for nodes inside it. That is the
  reason the radius is read from the instance's own config rather than asked
  for again.
- **Scale was measured, on a generated extract rather than a downloaded one.**
  The egress policy of the environment this was built in blocks
  `download.geofabrik.de`, `overpass-api.de` and `api.openstreetmap.org`, so a
  real regional extract was never in reach. A 264 MB extract shaped like a
  Geofabrik one — 1.2M nodes, 180k ways, 5k relations, with escaped entities,
  multi-byte street names and the version/timestamp/uid attributes a real file
  carries — cropped to a 25 km radius in 18 seconds, peaked at 29 MB of RSS,
  and produced a 4.3 MB index of 22,092 places answering in single-digit
  milliseconds. What that run cannot speak to is real-world data messiness
  nobody thought to simulate, which is exactly the blind spot a synthetic
  fixture has.
- **It found a real scoring bug, which is the argument for running it.** A
  matching city earned nothing beyond the token it contributed, and that token
  is often already in the street: "Millersville Road, Millersville" tokenizes
  to one `millersville`, so the same housenumber on the same street in the
  next town scored identically, the pair cancelled out under the ambiguity
  rule, and a *unique* address returned nothing. Naming the city bought
  exactly zero at the one moment it was the only thing that could help. A city
  the query named now scores, and the round-trip over indexed addresses went
  from 288/300 to 292/300 — the remaining eight are the generator inventing
  the same address twice, 27 km apart, which the ambiguity rule is right to
  refuse.
- **The volume is in events, and this does not serve them.** The event form
  has no coordinate input at all, and `events.latitude`/`longitude` are
  written by the JSON-LD importer and the aggregator and then read by
  nothing — the map plots patches only. A gazetteer justified by patch
  creation serves a few dozen one-time acts; the same machinery pointed at
  events would serve a recurring need that currently has no interface. That
  is a separate decision, deliberately not made here, but it is the reason
  this one is worth building rather than deferring.
