# ADR 073: A feed can be wrong about its own zone

Date: 2026-08-28. Status: **accepted**. Sits beside docs/adr/045 and
docs/adr/067 rather than inside them: those decide what a time with *no*
zone means, and this one is about a feed that states a zone confidently
and is wrong. Applies docs/adr/069's test to a defect rather than a
format.

## Context

Tellus360's shows read four hours early on the quilt. Three sessions of
timezone work had not fixed it, and the reason is that none of it was
ever going to.

Their page publishes schema.org markup:

```json
"startDate": "2026-08-28T15:00:00-04:00"
```

That is 19:00Z, and it is what Patchwork stored. The prose beside it, on
the same page, says `21+ | 7pm`. Every parser here read the offset
correctly; the offset is simply not the truth. Their site takes the
venue's wall clock, emits it as though the digits were UTC, and renders
that instant back through Eastern — a double conversion, at the source,
before anything of ours sees it.

The tell was Yoga on the Rooftop: described as 10am, stored as 6:00 AM.
Nobody runs rooftop yoga at six. Across nine events the structured data
was uniformly the real time minus four, and Binns Park and Mickey's — on
the same instance, through the same parsers — were exactly right, which
is what ruled out our end.

ADR 045's chain resolves event → patch → instance → UTC and answers "what
did this zoneless time mean". It cannot help here. The feed is not
leaving the question open; it is answering it, and we believe it, and we
are right to believe feeds that state offsets. Something has to let a
person say *this publisher is wrong*.

## Decision

**1. A per-source switch, not a number of hours.** `event_sources.
local_time_stamped_utc`. When set, each parsed instant's UTC wall clock
is re-read in the patch's zone: 19:00Z becomes 19:00 in
America/New_York, which is 23:00Z — 7pm, the number the publisher
started from.

The obvious alternative, an hours field, was rejected on arithmetic. The
error equals the venue's UTC offset, so it is four hours in August and
five in November. A stored `+4` would be right when it was set and
silently wrong for the four months after the clocks change — which is
precisely the bug class ADR 065 and ADR 067 exist to remove, reintroduced
as a feature. Re-deriving the shift from the zone on every sync needs no
seasonal edit and no one to remember.

A field also cannot be audited. `+4` records a magnitude and loses the
reason; six months on nobody knows whether it still applies. "This
publisher stamps local time as UTC" is a diagnosis a person can go check
against the feed, and can untick when the venue fixes their site.

**2. Declared, never detected.** A feed with this defect is internally
consistent — a valid offset, a plausible instant, no contradiction a
parser could find. The only evidence is the venue's own page saying
something else, which is outside the data. So this is set by whoever
attached the source, and defaults off: every existing feed keeps being
believed, because being believed is the correct default for a publisher
who has not been shown to be wrong.

**3. The identity key does not move.** The correction rewrites `StartsAt`
and `EndsAt` and leaves `Occurrence` alone. Occurrence is the reconciler's
key (docs/adr/031), not a time anybody reads; shifting it would make the
first sync after the switch a delete-and-reinsert of every recurring
event — duplicate notifications, and `event_source_skips` rows orphaned so
events an admin hid come back. Left alone, the correction lands as an
UPDATE in place and events keep their ids, links and RSVPs.

**4. Flipping the switch clears the conditional-GET state.** Otherwise it
would sit there doing nothing until the venue next edited its calendar: an
unchanged feed answers 304, the reconciler never runs, and the times stay
exactly as wrong as they were. This is the same trap migration 056 was
written for, and a setting whose effect waits on a stranger is not a
setting.

**5. The settings page previews against a real event.** "Its next event
would move from 3:00 PM to 7:00 PM", both rendered in the patch's zone,
before anything is saved. Those are the numbers an admin can hold against
the venue's page; the stored instants are not. Upcoming rather than any
event, because a past one is outside the sync window and would promise a
change that never lands.

## Consequences

This does not replace asking the publisher. ADR 069 is right that the
first move on a broken site is a small change on their end, useful to
every aggregator reading it and not only to us — Tellus360's markup is
wrong for Google and Apple Maps too. The switch is what a quilt does
about a publisher it does not control, and it stays useful after this one
is fixed.

It is also not the adapter ADR 069 prescribes, and the distinction is
deliberate. An adapter exists because a site is *unreadable* — no feed, no
markup, nothing generic to parse. This site is readable; one field is
wrong. Standing up a static-file pipeline to flip a sign would put
infrastructure between a self-hoster and a boolean, which cuts against
that ADR's own "the instance takes on no new dependency". No site-specific
code enters the binary: the switch is a row, set by a person, and the
parsers stay format-generic exactly as ADR 069 requires.

The flag travels in a seamrip. It is a finding about the publisher rather
than about this instance, and it stays true after a fork — one that lost
it would re-import the same events hours off, with no trace of why they
were ever right.

**Aggregators do not get this yet.** `aggregators` is a separate table
with a separate sync, and no aggregator has shown the defect. ADR 069's
"one venue does not justify it" applies to the second copy of a mechanism
as much as to a source type; when a city feed turns out to stamp local as
UTC, this is the shape to give it.

Two neighbouring defects stay unhandled, and both would want their own
answer rather than this one stretched: a publisher off by a flat hour for
an unrelated reason, and a venue whose CMS is set to the wrong zone and
correctly emits `-07:00` for a Lancaster show. If either turns up, that is
the moment to make this boolean a small enum — and not before, because
today there is exactly one shape and a boolean names it exactly.
