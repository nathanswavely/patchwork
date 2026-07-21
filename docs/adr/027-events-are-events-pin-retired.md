# 027. Events are events — "pin" retired

Date: 2026-07-20

## Status

Accepted

## Context

The founding naming table gave every core entity a textile UI word, and
events got "pin". On paper it fit the register — you pin fabric before
you sew. In the product it collided on every side:

- **Map pins.** Events appear as Leaflet markers on the map. The one
  place events are most visually present is the one place "pin" already
  means something else — `MapView.svelte` draws a literal "quilt-colored
  teardrop pin" for each patch. "Pins on the map" is unparseable when
  both readings are true at once.
- **Pinned posts.** Every social tool people arrive from uses "pin" for
  sticking an item to the top of a feed or channel. A "pin feed" reads
  as a feed of stickied posts, not upcoming events.
- **The build had already voted.** The UI grew Events pages, `/events/*`
  routes, "Upcoming events" headings, and notification labels ("New
  event", category "Events") — while "pin" survived almost nowhere a
  user looks. Its one visible string was a single "Upcoming pins"
  heading. Meanwhile the AP layer redirected browsers to `/pins/{id}`,
  a route the SPA never had — federated event links 404'd because the
  frontend authors, unprompted, spoke "events". A term nobody reaches
  for while building the thing isn't the thing's name.

PR #71 then spread the term into new surfaces ("pin feed", the
`remote.pin` notification type), which forced the decision before it
calcified further. `remote.pin` also broke the backend-speaks-generic
convention: every registered notification type uses backend terms
(`event.created`, `event.updated`) — the textile word had leaked into a
stored wire identifier.

## Decision

Retire "pin". The UI word for an event is **event** — the first core
entity whose UI and backend words are deliberately the same, joining
"person" as proof the textile vocabulary is a register, not a quota
(CONTEXT.md: people are not artifacts; and a clever word that needs a
glossary of its own loses to the plain one).

Concretely:

- CONTEXT.md gains an **Event** entry; "pin" is an _Avoid_ word.
- The `remote.pin` notification type becomes `remote.event`, with a
  data migration rewriting existing rows (including their `?pin=` dedup
  link tails) so redelivery dedup keeps working across the rename.
- AP object URLs and browser redirects use `/events/{id}` — the route
  that actually exists. `/pins/{id}` remains as a permanent redirect,
  because pin-era URLs were already federated into other instances'
  timelines.
- User-facing strings ("Upcoming pins", "New pin:") say "event".

## Consequences

- Migration 029 rewrites `remote.pin` notification rows in place.
- Textile words remain for made things and governance acts (patch,
  quilt, baste request, lining, seamrip). "Pin" joins stitches, edges,
  and moderator in the removed-concepts list; the word is now free to
  mean only map pins.
- Old federated `/pins/{id}` links resolve via redirect indefinitely;
  new AP objects never emit them.
- Docs were swept (README, STATUS, ADR 024 prose) so a reader never
  meets "pin" as an event term; only this ADR and history remember it.
