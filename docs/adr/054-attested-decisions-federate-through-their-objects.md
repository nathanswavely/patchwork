# Attested decisions federate through their objects, or not at all

docs/adr/052 and docs/adr/053 each left the same question open, and each
said it should be answered together with the other: whether an
attestation federates as its own activity, or only through the objects it
changes.

We decided **only through the objects, and for the leadership half that
means not at all.** No new activity type, no new field, no code.

## What we found first

The `gv:` governance vocabulary is very nearly write-only. Patchwork
broadcasts `gv:Proposal`, `gv:Vote`, `gv:ResolveProposal`, and `Update` of
`gv:GovernanceDocument`. Nothing anywhere consumes a `gv:` type:

- Node inboxes handle `Follow` and `Undo`. Everything else logs
  "unhandled activity type" and answers 202.
- The instance inbox handles `Accept`, `Reject`, `Create`, `Update`.

That last one is why the picture is not simply "governance does not
federate". Under docs/adr/024 the instance service actor holds the remote
follow, so a node's broadcasts land in the *instance* inbox, where
`handleInstanceCreate` has a generic fallback. A charter update already
produces a remote notification today. It reads "Updated from a followed
patch: Bylaws". Meanwhile `gv:Vote` and `gv:ResolveProposal` are bare
activity types rather than `Create`/`Update`, so they reach the default
branch and are discarded.

So the honest state is: objects wrapped in `Create`/`Update` produce a
vague notification with no governance meaning, and governance activities
proper are emitted into a void. Any decision here had to be made against
that, not against the assumption that a receiver exists.

## The audience is people, not instances

Three audiences were possible: other Patchwork instances reading a
machine-readable record, people on other quilts reading a notification,
or the wider fediverse. We chose **people on other quilts**.

The instance-to-instance audience is real and worth having eventually,
but it needs an inbound side that does not exist for *any* governance
type, plus a trust decision nobody has made — whether this instance
believes another instance's claim about who its admins are. Adding
`gv:Attestation` to the outbound vocabulary before then would ship a
field nothing reads, one federation hop out from exactly the failure
docs/adr/049 was written about. We would be able to say "attestations
federate" and it would be true only in the sense that packets leave the
building.

## The question splits, unevenly

It was posed as one question. It is two, with different answers.

**An amendment attestation already federates,** through the charter it
rewrites. `broadcastDocUpdate` sends an `Update` of the
`gv:GovernanceDocument` when that charter is public. It rides an object
that was federating for its own reasons before any of this existed.
Nothing to build.

**A leadership attestation has no object to ride.** What it changes is a
membership, and docs/adr/006 bars memberships categorically — "memberships
never enter actor docs or activities". There is no vehicle, and this is
not an attestation quirk: `seatWinners` and `succeedOnDeparture` are
equally silent, so *no leadership change of any kind federates today, in
any model, at any venue.*

We are keeping it that way, and the reason is not privacy. A patch's
admin list is public on its own page. The reason is that **federating
attested councils but not elected ones would shape the federation surface
by venue.** A remote instance would learn who runs a patch when that patch
decides at meetings and not when it votes here. The venue is a fact about
how a community works internally; it must not decide what the community
publishes. If leadership changes should federate, they should federate
for all three models — a larger decision that docs/adr/006 currently
forecloses, and one that has nothing to do with attestation.

## Provenance does not cross the wire

The premise of docs/adr/052 is that "the membership decided this,
elsewhere" is a much larger claim than "an admin decided this", and
CONTEXT.md says the two are never worded alike. Remotely they are worded
identically: a charter changed by attestation and one changed by a passed
amendment both arrive as `Update` of a `gv:GovernanceDocument`, and the
*receiving* instance composes the sentence from the object's type and
name. The sender cannot influence it.

We accept that, because docs/adr/024 already drew this line. **Objects
blend, places don't.** What crosses is the object — the charter's text,
which is the same text however it was adopted. The account of *how* it was
adopted is a property of the place: it lives on the patch's own page, in
the "Adopted elsewhere" record with its date and its recorder, one doorway
click away. Carrying provenance in the object would put a fact about a
place into the thing that blends, which is the boundary 024 exists to
hold.

## Consequences

- **The visibility gate is the load-bearing part, and it now has a test.**
  `broadcastDocUpdate` returns early unless the charter is public, and a
  document created by a first attestation is born `members`
  (docs/adr/036), so a community correcting Patchwork's stale copy of a
  private charter publishes nothing. That is the property worth checking —
  not the federation, which is one line of pre-existing code.

  Writing that test cost the one line of implementation this decision has:
  the broadcast was wrapped in a goroutine, so the queue write raced the
  assertion and the first version of the test **passed with the gate
  deleted**. `BroadcastToFollowers` only writes rows — the delivery worker
  does the network — so the goroutine bought nothing and cost the gate its
  testability. It is now synchronous. The older broadcast sites still wrap
  theirs; they were left alone, and are equally untestable for it.

- **The remote view of an attested amendment is indistinguishable from a
  voted one.** Recorded as accepted rather than left implicit, so a later
  reader does not mistake the silence for an oversight and fix it by
  inventing a field.

- **A person following a patch from another quilt sees its charter change
  and never sees its council change.** A real hole, named rather than
  quietly filled for one model.

## Considered and rejected

- **A `gv:Attestation` activity.** The obvious shape, and write-only:
  no Patchwork reads any `gv:` type. It would also have to carry either
  the names — publishing memberships, against docs/adr/006 — or no names,
  and "the council changed, we won't say to whom" is a worse notification
  than none.

- **A provenance field on the governance-document object.** Cheaper, and
  still write-only, since the receiver composes the sentence and would
  have to be taught to read it. Puts a place-fact into a blending object.

- **Building inbound governance ingestion now.** The right eventual
  answer for the instance-to-instance audience. Deferred because nobody
  has asked for it, and because it opens the trust question — whether to
  believe a remote instance's account of its own governance — which
  deserves its own decision rather than arriving as a side effect of
  closing this one.

## Open

- Whether governance activities should be *received* at all, and if so
  what a receiving instance does with a claim it cannot verify. This is
  the audience deferred above.
- Whether leadership changes should federate for every model, which would
  require revisiting docs/adr/006. Not a question about attestation.

**Status: adopted.** This closes the federation question left open by
docs/adr/052 and docs/adr/053. The behaviour it describes already
shipped; what this adds is a test of the gate and, to make that test
mean anything, one broadcast made synchronous.
