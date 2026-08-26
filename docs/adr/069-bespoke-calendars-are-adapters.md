# Bespoke calendars are adapters, not source types

West Art Lancaster publishes a full events calendar and none of the three
inbound source types can read it. The site is a Next.js front end over
Prismic: no ICS anywhere, no Squarespace JSON view, and the only JSON-LD
on the page is an `Organization` block — injected through Next's script
wrapper (`self.__next_s.push`) rather than a literal
`<script type="application/ld+json">` tag, so `ldScriptRe` would not
match it even if it carried Events. All three probes in `loadItems` miss,
and the admin gets the ICS parse error.

The obvious move is a fourth type: `prismic`, sitting beside
`squarespace` in `internal/eventsource`. We are not doing that, and the
reason generalizes.

The decisions:

- **A vendor source type is only allowed when the vendor fixes the
  schema.** Squarespace earned `squarespace` because every events
  collection serves the same shape at `?format=json` — `collection.
  typeName`, `upcoming`/`items`, `startDate` in epoch milliseconds. One
  parser reads every Squarespace venue that will ever exist. Prismic
  fixes only the transport: `/api/v2` for the master ref, then
  `/documents/search?q=[[at(document.type,"…")]]`, paginated, wrapping an
  opaque `data` object. Everything inside `data` is modeled per
  repository by whoever built the site. West Art's type is called `event`
  with `date`, `time`, `location`; the next Prismic venue might call it
  `show` with a Timestamp field and a `start`/`end` pair. A `prismic`
  parser would be a West Art parser wearing a vendor's name. The same
  test disqualifies Contentful, Sanity, and every other headless CMS, and
  it is the test to apply to the next proposed type.
- **The three parsers stay format-generic.** ICS, schema.org JSON-LD, and
  the one fixed-schema vendor shape are things a thousand sites emit
  identically. Nothing that requires knowing which site produced it
  belongs in the binary that every instance runs.
- **The extension point is the URL.** `event_sources` accepts any address
  that returns ICS. A site with neither feed nor markup gets an adapter
  that lives outside Patchwork, reads whatever it must, and publishes ICS
  at a stable address, which is then attached as an ordinary source. The
  one-off moves; it does not disappear. Patchwork never learns the word
  Prismic.
- **The instance takes on no new dependency.** A self-hoster attaching an
  adapter's feed is attaching a URL, exactly like a venue's Google
  Calendar link. No scraper runs on the Pi, no service must be up for
  Patchwork to start, and `internal/seamrip` already carries both
  `event_sources` and `event_source_skips`, so a fork keeps pulling from
  the same address with its admin skips intact.
- **Adapters honor the reconciler's contract or they corrupt the
  patch.** Identity is `(source_uid, source_occurrence)`. An adapter that
  mints unstable UIDs makes every sync a delete-and-reinsert: duplicate
  notifications on every poll, and `event_source_skips` rows orphaned so
  events an admin hid come back. Adapters therefore key on the source
  system's immutable id — never an editable slug — pre-expand
  occurrences into distinct UIDs rather than inventing RRULEs for
  hand-kept date lists, and emit UTC instants rather than `VALUE=DATE`
  (a date-only DTSTART is midnight UTC, which lands an all-day event on
  the previous day in every US timezone).
- **Adapters publish static files, not a live service.** Patchwork's
  conditional-GET headers cost a file host nothing, and the failure modes
  differ where it matters: a stale `.ics` keeps serving the last good
  calendar, while a down adapter service errors the fetch and parks the
  source in `status = 'error'`.
- **The adapter is the fallback, not the goal.** The first move on a
  bespoke site is asking whoever maintains it to publish ICS or mark up
  their event pages — a small change on their end, useful to every
  aggregator and not just to us, and the thing that lets the adapter be
  deleted. ADR 026's submission ladder remains the answer for sites where
  nobody is home.

## Rejected alternatives

- **A `prismic` source type.** Covered above: no fixed content schema, so
  the parser can only be site-specific. Shipping it would also put vendor
  code in every instance's binary for the benefit of one patch on one
  quilt.
- **Adapters in this repository** (a `cmd/adapters/` tree, or an
  adapters directory shipped alongside the migrations). Keeps the
  maintenance burden while adding the cost it was meant to avoid: every
  self-hoster compiles every community's one-off, and the boundary this
  ADR draws stops being visible in the layout.
- **A headless browser in the puller.** Already rejected by ADR 031 and
  still rejected — the fetcher is a plain guarded GET with a 2 MB cap
  against a Pi-class target.
- **A mapping-configurable JSON source type** — per-source field paths
  (`title` ← `data.title`, `start` ← `data.date`) stored on
  `event_sources`, which would cover Prismic and every other headless CMS
  generically — is **deferred, not rejected**. It needs a schema change,
  a UI for authoring mappings, and an answer for what a wrong mapping
  does when imported events publish without review. If several
  communities hit this wall, it is the design to revisit; one venue does
  not justify it.
