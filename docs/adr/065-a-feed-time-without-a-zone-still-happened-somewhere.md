# ADR 065: A feed time without a zone still happened somewhere

Date: 2026-08-22. Status: **accepted, partly superseded by ADR 067**.
Its finding stands — a feed time carrying no zone is not UTC — and two of
its mechanisms do not. `instance.timezone` moved to `geographic.timezone`
with an admin override, and "instance-wide rather than per-source" was
wrong: ADR 045 already had a chain to hang it on, and ingest now reads a
feed in the zone of the patch that attached it. This ADR was written
without citing ADR 045, which had decided the shape a year of numbering
earlier; the "no instance timezone exists anywhere in the stack" line it
quotes from datetime.js was the next sentence's setup, not a settled rule. Sits inside ADR 031's model
unchanged — this is about how a fetched document is read, not about what
a source is or what attaching one means. Adds the one piece of
configuration the frontend's datetime rules deliberately never needed.

## Context

Tellus360's calendar put every show on the quilt four hours early. The
event page read "Friday, August 21, 2026 · 3:00 PM" above a description
that began "21+ | 7pm". Nobody had mistyped anything: 7pm in Lancaster is
23:00 UTC, and the row said 19:00 UTC.

The 19:00 came straight out of the feed. Three of the four ingest paths
receive an unambiguous instant — an ICS `DTSTART` ending in `Z` or
carrying a `TZID`, Squarespace's epoch milliseconds, an atproto record's
RFC 3339 `startsAt` — and were always right. The fourth shape is a
**floating** time: a wall clock with nothing attached. ICS spells it
`DTSTART:20260821T190000`; schema.org spells it
`"startDate":"2026-08-21T19:00:00"`, which is what a great many CMS event
plugins emit because the site already knows where it is and sees no
reason to say so. Both were read as UTC, which is a decision disguised as
a default.

The same slip has a second face. A date is a floating time too: an
all-day `DTSTART;VALUE=DATE:20260821` read as midnight UTC is 8pm on the
20th in Eastern, so an all-day event landed on the wrong **day**. And a
third: an `RRULE` expanded from a floating `DTSTART` was expanded in UTC,
so a weekly 7pm show became a weekly 6pm show the week the clocks
changed.

`web/src/lib/datetime.js` says, correctly, that "no instance timezone
exists anywhere in the stack." That is a **display** rule and it is a
good one: events are stored as instants and rendered in each reader's own
browser zone, so a visitor from Pittsburgh and one from Berlin each see
the right thing without anybody configuring anything. Ingest is a
different question, asked in the other direction. Display converts an
instant into a wall clock and the reader's browser answers "whose clock".
Ingest converts a wall clock into an instant and, when the document
declines to say, **nothing in the system answers at all**.

There is no way to duck this. A floating time is not missing data to be
handled gracefully; it is data that means something specific to the
person who published it, and reading it as UTC is a guess that is wrong
everywhere except Greenwich.

## Decision

**1. Ingest gets a zone; display still does not.** `instance.timezone`
(IANA, e.g. `America/New_York`) is used for exactly one thing: deciding
what a zoneless *feed* time meant. It is validated at startup — a
misspelling is otherwise invisible until somebody notices a show is
early — and never reaches a rendering path. Every surface still renders
in the reader's browser zone, and ADR 045's date-widening rule is
untouched.

Instance-wide rather than per-source, because a quilt is a geography: ADR
031 attaches feeds belonging to the community the instance is for, and
`geographic.latitude/longitude/radius` already says that community sits
in one place. A per-source override is a small migration away if a quilt
ever aggregates another city's calendar, and nothing here forecloses it.

**2. The document outranks the configuration.** An ICS `X-WR-TIMEZONE` is
the calendar telling us where it is. It is not in RFC 5545, and it is
also what Google, Apple, and most of the CMS plugins in between actually
write, and it is the only statement a feed full of floating times ever
makes about itself. It wins. `instance.timezone` is the fallback, not the
rule.

**3. A zone we can't read shifts an event; it must never delete one.**
Before this, a `TZID` that `time.LoadLocation` rejected — Outlook writes
`TZID="Eastern Standard Time"` on everything it exports — made the whole
`VEVENT` unreadable, and an unreadable event is simply absent from the
parser's output. Absence is how this reconciler spells *removal*
(ADR 031): the show did not arrive at the wrong time, it vanished, and
the feed looked like it had cancelled it.

So unresolvable `TZID`s are now resolved down a ladder — tzdata name, the
CLDR Windows-name table, an offset the publisher baked into the name
itself — and if every rung fails, the parameter is dropped and the value
is read as floating, the same treatment a feed that named no zone at all
gets. A time that is present and possibly an hour off beats a show that
disappeared with no error anywhere.

The ladder is deliberately not a `VTIMEZONE` interpreter. Building a
DST-aware `time.Location` out of a calendar's own `STANDARD`/`DAYLIGHT`
rules is real work, and the two rungs above it already cover what feeds
in the wild actually carry.

**4. Recurrence expands in the calendar's zone.** A weekly show is at the
same o'clock all year; that is what "weekly" means to the person who set
it up. Expanding in the zone rather than in UTC is what keeps it there
across a daylight-saving boundary.

**5. Repairing the fix's own blast radius is part of the fix.** A parser
correction repairs nothing already stored, because an unchanged feed
answers 304 and never reaches the reconciler. Migration 056 drops the
conditional-GET state so every source re-reads once.

It also nulls `last_success_at`. A one-off event keeps its identity
across the correction and is updated in place, but an occurrence of a
recurring event is keyed by its start instant — exactly what changes
here — so those arrive as new keys, and a sync that has succeeded before
announces new events to every follower. Nulling makes that one pass adopt
the corrected calendar the way a first sync does: silently. The cached
aggregator listings are deliberately *not* cleared, for the reason in
decision 3 — a crosswalk entry handed an empty feed would read it as the
whole calendar being withdrawn.

## Consequences

An instance that configures nothing keeps UTC and keeps the old
behaviour, with a startup warning saying so. That is the honest default:
guessing a zone from the map centre would be wrong at every timezone
border, and picking Eastern because the reference deployment is in
Lancaster would make the white-label promise a lie.

Fixed-offset fallback zones (decision 3, bottom rung) get the
standard/daylight split wrong half the year. That is accepted: the rung
exists so a feed nothing else can read still produces events, and it is
below two rungs that are exact.

`instance.timezone` is deployment configuration and does not travel in a
seamrip — a fork chooses its own, the way it chooses its own domain.
