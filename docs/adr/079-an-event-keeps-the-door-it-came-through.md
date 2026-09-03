# ADR 079: An event keeps the door it came through

Date: 2026-09-03. Status: **accepted**; implemented. Fills a gap in
docs/adr/031 (event sources) that docs/adr/056 (aggregators) had already
half-noticed.

## Context

Patchwork aggregates. A patch attaches a calendar feed and events arrive
hourly; an instance attaches a city-wide aggregator and a crosswalk routes
listings onto patches. Between ICS, schema.org JSON-LD, Squarespace's
collection JSON and the AT Protocol events lexicon, four ingest paths land
events on this quilt without anyone typing them.

Every one of those formats carries the event's own page — `URL` in ICS,
`url` (or `offers.url`) in schema.org, `fullUrl` in Squarespace, `uris` in
the lexicon — and every one of those values was dropped on the floor.

An imported show therefore arrived with a title, a time, a location and
nothing else. For a great many of them, the *whole point* is the link: the
ticket page, the RSVP form, the venue listing with the set times and the
door price. A reader who found a show on Patchwork and wanted to go had to
guess at a search query. That is the aggregation being worse than the feed
it aggregated.

The field was half-built already. `Item.URL` has been parsed since ADR 056,
because an instance admin deciding whether an unrouted name means an
organization needs to go and look at the listing as its publisher wrote it.
It was stored on `aggregator_listings`, shown in the crosswalk picker, and
then discarded at exactly the moment an event was created from it.

And it was missing from the manual path too. A venue admin posting their own
show had a description to paste a link into and no field to put one in, so
the link — when it survived at all — arrived as bare text in the middle of a
paragraph, unlinked, un-federated, invisible to the ICS feed.

## Decision

**1. An event has one field for its own page: `events.event_url`.**

One column, one meaning: where this event lives on the web, if it lives
anywhere else. It is filled by ingest and it is filled by the form, and
nothing downstream cares which.

Not called `url`, because `event_links` already means a different thing
(docs/adr/032 — a patch's presence on someone else's event) and a reader
who conflates them will conflate them badly. Not `source_url`, because the
`source_*` columns mean feed provenance and a hand-typed event has one of
these too. `event_url` sits beside `image_url` on the same table, and means
the same *kind* of thing: a reference to something the binary never fetches.

**2. Every parser fills it, and the scheme is checked at every parser.**

http and https only, in `ParseICS`, `ParseJSONLD`, `ParseSquarespace` and
`ParseATProtoEvents` alike. This value ends up as an `href` on a page, and a
feed is somebody else's input; `javascript:` must not survive the parse. The
web app checks the scheme a second time before rendering, because a guard
that exists in one place is a guard that gets moved.

JSON-LD additionally falls back to `offers.url`. Ticketing platforms
routinely describe the show in the Event node and put the buy link in the
offer, and the buy link is what the reader wanted.

Squarespace item links are site-relative, so the parser now takes the page
address it was fetched from and resolves against the document's own
`website.baseUrl` where it has one — a venue on a custom domain must not get
links to its `.squarespace.com` staging host.

**3. The source stays authoritative about it.**

`event_url` joins the fields the reconciler diffs. A venue moving a show to
a new ticket link propagates on the next sync exactly as a retitle does.

This is also the whole backfill story, and the reason no data migration is
needed: on the first sync after deploy every imported event differs from
its feed, so every one of them is updated in place. A migration could not
have done this — it would have had to refetch every feed — and a one-off
script would have been a second implementation of the reconciler.

Detach does not clear it. Detach severs *provenance*; the link is content,
and an admin who cuts a show loose from its feed has not stopped wanting
the ticket page.

**4. Outbound feeds carry it in the description, not in `URL`.**

ICS offers exactly one `URL` property and it is already spent on the
Patchwork permalink — the one address that always exists, always resolves,
and shows the reader everything else Patchwork knows. Spending it on the
external page instead would break the feed's own round trip (one Patchwork
subscribes to another's ICS, and ADR 031 says so explicitly).

So the event's own page is appended to `DESCRIPTION`, where every calendar
client on earth renders it as a followable link. RSS does the same, for the
same reason: `<link>` is the permalink, and a feed reader shows the
description.

ActivityPub has room for both and uses it: `url` stays the permalink, and
the external page rides as an AS2 `attachment` Link — the shape Mobilizon
and Gancio already use for a ticket URL, so a federated reader gets it for
free.

**5. It travels.**

`def("event_url", "")` in the seamrip boundary. Someone else's URL, like the
flyer beside it — the fork keeps pointing at the venue's listing, which is
still the venue's.

## Consequences

An imported event is now worth clicking. The city-wide aggregator that ADR
056 built stops being a list of names and times and starts being a way to
get to a show.

The manual form gains one optional field, which is one more field on a form
that already has several. It is placed above the image, because a link is
more often had than a flyer.

CSV upload gains a `link` column with the aliases a spreadsheet actually
uses, and the preview grows a Link column *only when the sheet had one* —
the preview exists to catch a column read wrong, and a bare host is enough
to see that it wasn't.

One thing this deliberately does not do: nothing renders the link on an
event card in a list. A list is for choosing which event to look at, and
sending a reader off the quilt from a row they have not yet read is the
aggregator's failure mode, not its feature. The link lives on the event's
own page, one tap further in.
