# ADR 058: atproto is a source and an identity, not a second federation

Date: 2026-08-21. Status: **proposed** as a boundary. Nothing here is
scheduled; the constraint is the decision, and the four steps below are an
order, not a plan. Builds on ADR 024 (cross-quilt following), ADR 031
(event sources), ADR 049 (state only what you enforce), ADR 054
(federation needs a receiver).

## Context

The AT Protocol comes up as an integration target, and the question is
worth answering at the boundary before anyone builds toward it, because
the appealing version of the answer is the one this project has already
refused twice on other grounds.

Three parts of atproto are what Patchwork would touch:

- **Identity is a DID**, permanent, with the human-readable handle as a
  pointer proved by a DNS TXT record at `_atproto.<domain>` or a
  `/.well-known/atproto-did` file.
- **Data is a signed repository** of typed JSON records held on a PDS.
  Record types are named by Lexicon NSIDs (`app.bsky.feed.post`,
  `community.lexicon.calendar.event`). A record is verifiable no matter
  who hands it to you.
- **Distribution is publish-and-crawl.** PDSes emit a firehose, relays
  fan many PDSes into one stream, and *AppViews* consume that stream and
  build their own indexes. Clients write to a PDS; nobody pushes to
  anybody.

That last part is the whole difference. ActivityPub is servers pushing
signed envelopes into each other's inboxes — which is what `internal/ap`
does today, Mastodon-verified 2026-07-13. atproto is everyone publishing
verifiable data and anyone building a view over it. **Participating in
ActivityPub means running a server. Participating in atproto fully means
running an index.**

## Decision

**Four constraints, and then a sequence that follows from them.**

**1. Patchwork never runs atproto infrastructure.** No relay, no AppView,
no PDS. A firehose consumer with an index is background workers, sustained
network, and real storage — the exact negation of "single-process, no
queues, no workers" and "must run on a Raspberry Pi 4 with 2GB." Patchwork
is a client: targeted XRPC reads against somebody else's AppView, and at
most a filtered Jetstream subscription to one collection. Any atproto
client goes through `internal/safehttp` like `ap` and `eventsource`
already do.

**2. ActivityPub remains the federation. atproto is not a second one.**
ADR 054 established the rule the hard way: `gv:Vote` and
`gv:ResolveProposal` are emitted into a void because no inbound side
exists for them. We have one write-only vocabulary already. A second
federation surface — announced as such, before anything reads it — is ADR
049's failure mode with a protocol attached.

**3. The atproto surface can never exceed the public slice ActivityPub
already gets.** Repo records are world-readable, crawled, and archived by
strangers, permanently. Private patches, member-only charters (ADR 036),
private follows (ADR 024), and hidden memberships (ADR 006) are
categorically excluded — not as a policy that could be relaxed, but
because publishing to a repo *is* publishing to everyone forever.

**4. Where atproto earns its place, it arrives as a concept that already
exists** — a source type, a verification method, an auth path — never as a
new noun. Every slot below is one Patchwork already has.

### The order, and why it is this order

**A. An atproto event source type.** `event_sources.type` is already a
column defaulting to `'ics'` (migration 033). The reason this one is first
is ADR 056: the Lancaster aggregator feed carried **no `ORGANIZER` on any
of 196 events** — the host appeared nowhere a machine could read, which is
why the crosswalk had to be a human review of 55 `LOCATION` strings. An
atproto event record is authored by a DID *by construction*. Provenance is
not an optional field somebody forgot to fill in; it is the substrate.

Stated honestly, because it would be easy to overclaim: this does not
dissolve the crosswalk. A DID tells you who wrote the record, not that
they are the venue. What it changes is what the human is reviewing —
from "who is this?" to "is this DID that patch?", which is (B). It is
nonetheless the first feed shape where ADR 031's "attaching a source is
vouching for the feed" could be *checked* rather than asserted.

**B. A DID as a fourth claim verification method.** `claims.method` is
already `'dns' | 'meta_tag' | 'email' | 'admin'`, anchored on the vetted
`nodes.verification_domain` (migration 031). atproto handle verification
is DNS TXT or a well-known file against that same domain — the two
mechanisms already implemented, pointed at the same anchor. A new method
value, no new anchor, no new trust root. It composes with (A): once a DID
is bound to a patch, that patch's own atproto events are self-attributed
and need no crosswalk at all.

**C. atproto sign-in as a fourth auth path.** This is the one with a
mission argument rather than a mechanical one. Seamrip is a safety valve
for *the community* — members fork the data out when leadership goes
sideways — but the person's identity stays behind: ADR 002 keeps keys,
sessions, and AP identity on the instance. atproto account portability is
the same politics one level down, for the member rather than the patch.
It is passwordless like the other three paths.

Two things are unsettled and this ADR does not settle them: what a
Patchwork session means when the identity it rests on is external and
revocable elsewhere, and what happens on DID rotation or handle change.

**D. Publishing public events as records — last, and only against a
reader.** ADR 054's rule applies verbatim. It applies more easily here
than it did to governance, because the calendar and RSVP lexicons are
owned by a third party (`lexicon.community`, the schemas Smoke Signal
runs on), so "a reader exists" is a fact somebody can check rather than a
hope.

## Considered and rejected

**Replace ActivityPub with atproto.** Trading a working, interop-verified
surface for an unbuilt one, to gain reach nobody has measured. If atproto
reading proves valuable, that is an argument for (A) through (C), not
against `internal/ap`.

**Run an AppView so cross-quilt affinity can be real.** The seductive one,
and the reason this ADR exists. The pitch: memberships become records in
each person's own repo, Patchwork indexes them, and the quilt's affinity
layout finally works across instances — the thing ADR 024 could not do.
It fails twice independently. Constraint 1 kills the AppView. ADR 006
kills the records: "memberships never enter actor docs or activities," and
a repo record is a stronger publication than an activity, not a weaker
one. ADR 024 already found the destination itself dishonest — the quilt
treemap's layout *is* member-overlap affinity, and a merged treemap
computed from public-only data "would be a lie about connection, the one
thing the quilt is honest about." A better protocol does not make it true.

**Adopt labelers for moderation.** The vocabulary collision — atproto's
"labeler" against **the Label** (ADR 023), which is the stewardship
disclosure — is the smallest problem. The real one is constraint 1: a
labeler is a service somebody runs. And Patchwork already chose this
shape at its own scale: `users.hide_amended_linings` plus the instance
`hide_amended_linings` policy, strictest wins, is a stackable subscribed
filter with no service behind it.

**Governance lexicons.** ADR 054 already ruled on the general case: no
inbound side exists for any governance type, and federating another
instance's claim about who its admins are needs a trust decision nobody
has made. Publishing proposals or attestations as repo records would
additionally cross constraint 3 for any patch whose charter is
members-only.

## Consequences

- **Nothing here creates a table**, so the ADR 002 boundary has nothing to
  decide yet. (A) is rows in `event_sources`, which already travels. (B)
  is a value in an existing column. **(C) is a genuine seamrip question**
  and should be answered when it is built, not now: a binding between a
  user and an external DID is arguably instance identity (stays behind,
  like AP identity) or arguably the person's own (travels, like
  membership). `TestEveryTableHasABoundaryDecision` will force the answer
  either way.
- **CONTEXT.md needs a ruling before any of these words enter the UI**:
  "labeler" against the Label, and whether a person's atproto identity is
  spoken of as a handle or a DID. One term, chosen once.
- **The ordering is load-bearing, not a preference.** (A) without (B)
  imports events whose authorship nobody can tie to a patch; (D) before a
  reader exists is ADR 054's void again.

## Note on sources

The protocol details above were drafted from secondary sources and prior
knowledge: `atproto.com` was unreachable from the environment this was
written in. Before any of (A) through (D) is built, the identity
resolution mechanics and the current state of the community calendar
lexicons should be re-read from the specification itself.
