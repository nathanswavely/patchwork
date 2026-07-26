# Location is one field, written name-first

An event's location is a single free-text column — `events.location`,
`TEXT NOT NULL DEFAULT ''` since migration 001 — and the obvious reading
of the data is that it should be two. Every importer that assembles one
proves the point by gluing parts together with a comma: `fillJSONLDLocation`
joins a Place's `name` and `streetAddress`, the Squarespace reader joins
`AddressTitle`, `AddressLine1` and `AddressLine2`, and the ICS path takes
`LOCATION` verbatim from a field feeds conventionally fill venue-first.
What lands in the column is "Lanc Workshop & Tool Library, 433 Ice Avenue,
Lancaster, PA, 17602" — a venue and a postal address wearing one string.

The pressure to split came from the patch profile, where that string has
to share a narrow row with the event's title and time. The tidy fix is a
`venue` and an `address`, showing the venue where room is short.

We decided to keep one field, and to write down the property that makes
that safe: **a location is name-first**. Every importer already puts the
venue ahead of the street, so a surface short on room truncates from the
tail and keeps the useful half — "Lanc Workshop & Tool Library…" — while
dropping a city and ZIP that a local audience already knows. A one-line
clamp with a trailing ellipsis is not a compromise here; it degrades in
exactly the direction the data is ordered.

This turns an accident into a contract. Name-first was true only because
three importers independently happened to agree, and nothing tested it or
said it. A fourth importer leading with a street address would break every
clamped surface at once, silently — the text would still render, it would
just stop naming the place. The rule now lives in `CONTEXT.md` beside
**Address**, which already reserved the word `location` for this field
without ever defining it.

**Considered and rejected: splitting into venue and address.** It is the
better model on paper and it is not worth its price. The migration's
backfill would have to *guess* where the venue ends in strings we glued
together ourselves — the comma is a separator we introduced, not one any
source vouched for, and "The Selvage, Prince St" and "433 Ice Avenue,
Lancaster" split the same way to opposite effect. It costs a migration,
three importer rewrites, an event-form change, and that guess, all to
serve a truncation that `text-overflow: ellipsis` already gets right.
Nothing else is waiting on the split: `events.latitude` and
`events.longitude` are already separate columns, so map placement never
needed the address parsed. The cost of reversing this grows with the data,
which is why it is recorded rather than left implicit — if a future
importer or a real address-shaped requirement forces the split, this is
the note that says the backfill guess was the blocking problem.

Consequences: the name-first contract binds new importers, and the two
that were violating a related half of it are fixed in the same change —
the ICS and Squarespace location paths did not run `html.UnescapeString`,
so an escaped `&amp;` reached the column and would have sat in the visible
head of a clamped string while the ellipsis ate the harmless tail. Only
the JSON-LD path unescaped. Existing rows heal without a backfill: the
reconciler's `changed()` compares stored text against a fresh parse, so a
corrected value differs and updates on the next hourly sync. Three cases
do not heal — soft-removed rows (moderation outranks the feed by design),
events detached from their source, and past events the feed no longer
carries. All three are outside what the profile's upcoming-events glimpse
shows, so no backfill is scheduled.
