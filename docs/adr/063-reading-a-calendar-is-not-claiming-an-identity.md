# ADR 063: Reading a calendar is not claiming an identity

Date: 2026-08-21. Status: **accepted**. Builds ADR 058's step A. Sits
inside ADR 031's model unchanged, and narrows ADR 062's `did:web` rule to
the thing it was actually about.

## Context

ADR 058 put an atproto event source first in its order, on the strength of
ADR 056's finding: the Lancaster aggregator's ICS carried no `ORGANIZER`
on any of 196 events, so nothing in the feed said who was announcing. An
atproto record is authored by a DID by construction.

ADR 031 already defines what a source *is* — attaching is vouching once,
the source is authoritative, imported events are read-only until detached,
an unreachable feed never deletes. None of that changes. What follows is
only what a new source type has to decide for itself.

## Decision

**1. A source is a repository and a collection, stored as an AT-URI.**
`at://<did>/community.lexicon.calendar.event`. The owner pastes a handle,
because that is what a person knows; it is resolved once at attach time
and the **DID** is what gets stored.

That is not a detail. A handle is a rebindable pointer — a venue that
renames itself from `tellus.example` to `tellussocial.example` keeps its
DID. Storing the resolved DID means the feed survives the rename, and
storing the handle would have produced a source that silently stops
finding events with no error to show. ICS has no equivalent of this,
because an ICS URL is the only name a feed has.

**2. `did:plc` is accepted here, and ADR 062 still refuses it.** These are
not in tension, and the distinction is the point of this ADR.

ADR 062 governs a patch's **own identity**: it must not be hostage to
`plc.directory`, a registry the community does not own, because the whole
argument is that a fork can take its name with it. A **source** is a feed
somebody else publishes. It is already hostage to whoever serves it —
exactly like every ICS URL Patchwork has polled since ADR 031 — and
reading it adopts nothing.

Applying 062's rule here would refuse nearly every atproto account in
existence and make the feature theoretical, in service of a property
(portability of an identity we are not adopting) that does not apply.
`did:web` resolves against the domain; `did:plc` resolves through
`plc.directory`, which bends "no external service dependencies" no
further than ADR 023 already allowed for federation.

**3. A record with no `startsAt` is skipped, never defaulted.** The
lexicon requires only `createdAt` and `name`; `startsAt` is optional.
Patchwork events require a start. The tempting fallbacks — `createdAt`,
today, the import time — each put a **fictional date on a venue's public
calendar**, which is the failure ADR 031 spent its "unreachable feed never
deletes" rule preventing, arriving through the front door instead. A
record that does not say when it happens is not yet an event, and may
become one later when its author fills the field in.

**4. The rkey is the UID.** Stable within a repo for the life of the
record, which is what the reconciler keys on, and it is what an AT-URI
addresses. No synthesis, no hashing.

**5. No recurrence, and that is the lexicon's answer rather than ours.**
`community.lexicon.calendar.event` has no RRULE equivalent: a repeating
event is repeated records. So `Occurrence` is always empty and ADR 031's
expansion machinery is bypassed rather than reimplemented. Worth stating
because the absence looks like an omission and is not one.

## Considered and rejected

**Reading the firehose, or a relay, to discover events.** The version that
would find events nobody attached. Refused by ADR 058's first constraint,
and it would also break ADR 031's consent model: an event would appear on
a patch's calendar because a stranger published a record, with no owner
having vouched for anything.

**Deriving the source from `nodes.did` automatically**, so a patch with a
verified DID gets its own calendar with no attach step. Convenient and
wrong: ADR 031 makes attaching a deliberate act because it converts a feed
into published events forever after. Having proved you own a domain is not
the same as having said "publish whatever appears in this collection."
Offering it as a one-click default at attach time is fine; doing it on
your behalf is not.

**Trusting `status` to cancel an event.** The lexicon carries a status
ref, and honouring it would be a second cancellation path beside ADR 031's
"absent from a successful fetch." One rule, in one place; a cancelled
record that disappears from the collection cancels the event like anything
else.

## Consequences

- `internal/atproto` grows PDS resolution — the DID document's service
  entry — and a `listRecords` client. Still only a client: no relay, no
  AppView, no repository, per ADR 058 constraint 1.
- Conditional GET does not apply. ICS sources carry etag/last-modified;
  `listRecords` has neither, so an atproto source refetches in full each
  cycle. The collections in question are small, and correctness beats a
  saved round trip.
- The provenance win ADR 058 claimed is real but narrow, and worth stating
  precisely so it is not overclaimed twice: the DID says which repository
  an event came from. It does not say that repository is the venue. What
  makes it trustworthy is still an owner vouching at attach time — the
  DID makes *that* vouch checkable rather than assumed.
