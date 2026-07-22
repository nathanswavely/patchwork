# Event sources: vouch for the feed, not each event

Venue calendars live in other systems — Google Calendar, Squarespace,
Eventbrite, WordPress event plugins — and nearly all of them publish ICS.
We integrate by pulling ICS feeds, not by vendor APIs: one standard covers
almost every tool a community actually uses, with no API keys and no
external service dependency (the Leaflet move, applied to calendars). A
per-patch `event_sources` table is polled by an in-process worker (the
reminder-worker pattern), reusing the ActivityPub outbound-fetch hardening.

The decisions:

- **Attaching a source is vouching for the feed once.** Imported events
  publish directly (`active`), skipping per-event review. This does not
  contradict ADR 026's "no auto-approve for events, ever" — that rule
  targets strangers submitting to someone else's calendar; here the
  calendar's owner wired up their own feed. Accordingly, only owners may
  attach: patch admins on their own patch, the instance admin on
  unclaimed patches (who holds those calendars in trust). Trusted
  contributors may not attach sources — their ADR 026 grant delegates the
  admin's review queue for individual events, not the admin's judgment
  about a feed that produces events indefinitely.
- **The source is authoritative.** Imported events are read-only in the
  UI ("edit it in the source calendar"); re-polls update matched events
  by feed UID. The escape hatch is **detach**: an explicit act that
  converts one imported event to an ordinary local event and puts its UID
  on the source's skip list. No field-level merging — that is a sync
  engine, and this is a poller.
- **An unreachable feed never deletes anything.** Only a successful fetch
  that no longer contains an event's UID cancels it. This is the
  remote-follows rule (deletion and going-private are indistinguishable
  from outside) applied to feeds: an expired SSL cert must not wipe a
  venue's calendar.
- **Removing a source keeps past events, removes future ones.** History
  belongs to the patch; future events were promises the feed was making
  and nobody maintains them anymore — a stale upcoming show that never
  gets cancelled is worse than a gap. Detached events are unaffected.
- **Recurring feed events are expanded into occurrence rows** within a
  rolling horizon (~90 days), keyed by UID + occurrence date so EXDATEs
  and per-occurrence overrides just work. Mapping RRULEs onto the native
  recurrence enum was rejected: the native model never advances an
  event's date, so an imported weekly open mic would drift into the past
  while claiming to repeat — a dead-looking venue on a discovery surface.
- **Provenance is workspace-only on active patches.** Publicly the event
  is simply the patch's event — the admin attached the source, so it is
  the organization announcing. A public "synced" chip would read as a
  caveat, the way ADR 026's glossary work rejected "unverified". On
  unclaimed patches the community-submitted label applies as always
  (derived from patch status, never stored per event).
- **Outbound feeds ship alongside:** a public ICS feed and RSS feed per
  patch (anonymous, public events only — also how one Patchwork becomes
  another's event source), and a **personal feed** — one ICS of
  everything on a person's My Quilt, via a secret regenerable URL,
  because calendar apps cannot send session cookies. The feed secret is
  the instance's first non-cookie credential; it is read-only and
  per-person, which bounds a leak to "someone sees which events I
  follow", and regeneration is the recovery path.

## Rejected alternatives

- **RSS as an inbound event source.** RSS items carry no structured start
  time; guessing dates out of prose produces wrong events, which are
  worse than none. Everything that publishes an events RSS feed also
  publishes ICS. RSS earns its keep outbound.
- **Generic crawling/scraping.** Brittle, maintenance-heavy, and
  dependency-hungry against the single-binary/Pi principle. The narrow
  future version — parsing schema.org Event JSON-LD out of a venue page —
  is anticipated as a second `event_sources` type, not built now. For
  sites with neither feed nor markup, ADR 026's submission ladder is the
  crawler: humans, on-mission.

**Amended 2026-07-22:** two non-ICS types shipped. `squarespace` —
Squarespace exposes no whole-calendar ICS, but every events collection
serves structured JSON at `?format=json` (one fetch, stable ids), and
small venues live there. `jsonld` — the anticipated generic type: any
page embedding schema.org Event markup (Humanitix host pages were the
motivating case). Detection is automatic and ordered by cost: a pasted
address that isn't ICS is probed for Event JSON-LD in the body already
fetched (free), then for a Squarespace JSON view (one extra fetch);
a successful detection is persisted.
- **Vendor APIs (Google Calendar API, Eventbrite API).** OAuth apps, API
  keys, per-vendor adapters, and terms-of-service exposure — for data the
  same vendors already publish as ICS.
- **Feeding imports through the review queue.** Would make an admin
  re-review their own calendar forever; review exists for strangers, not
  for the owner's own feed.
- **Push API tokens** (scoped tokens for `POST /api/v1/events`) are
  deferred, not rejected — a write-capable credential class deserves its
  own design pass.
