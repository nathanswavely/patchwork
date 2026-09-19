# A vote is a membership said out loud

Date: 2026-09-18. Status: **accepted**; corrects docs/adr/095 decision 7
and closes a hole in docs/adr/006.

## Context

ADR 006 gave each member one switch over their own membership, and
docs/adr/095 added the patch's. Both are carefully honoured on the surfaces
a person reads. `GET /api/v1/proposals/{id}` gates its voter roster on
`viewerIsInPatchRoom` and `COALESCE(m.visible, 1)`, substitutes
`HiddenMemberName`, and blanks the user id too, with a comment explaining
that keeping the id would relink the anonymous ballots across a patch's
proposals and reassemble the very list the switch removed the person from.

The same request then broadcast the ballot by name.

```go
// Broadcast vote (non-blocking)
go func() {
    voteActivity := ap.VoteToActivity(vote, pAPID, ap.UserAPID(domain, user.ID))
    ap.BroadcastToFollowers(db, "node", nodeID, voteActivity)
}()
```

No gate of any kind. A `gv:Vote` carries an actor and an object, the object
resolves to a proposal, the proposal names its patch — and only a member of
that patch may vote on its proposals. The activity never says "membership"
and asserts one anyway.

**docs/adr/095 decision 7 is why nobody looked.** It reads: "Nothing
federates. Memberships have never travelled on an AP actor, so there is no
`publicMembers` field to add and no remote surface to gate." That is a true
sentence about the actor document and a false one about the wire. It asked
whether a membership travels as a *field*, and having answered no, stopped.
The question it should have asked is whether a membership can be *inferred*
from what travels.

Two more places answered yes to that question. `ap.ProposalToObject` and
`ap.GovernanceDocToObject` both set `attributedTo` to the author's actor,
and writing either takes a membership just as voting does. Both objects go
out on a broadcast *and* are served to an anonymous fetch at
`/ap/proposals/{id}` and `/ap/governance/{id}`, so the pull side leaked what
the push side leaked. `ProposalResolvedActivity` does not: its actor is the
patch and its payload is three integers, which is the shape the rest of this
decision aims at.

## Decision

**1. A membership must not travel as a fact or as an inference.** This is
the rule docs/adr/006 always meant, stated so that the next surface has
something to check itself against. "We ship no membership field" is not a
defence.

**2. A hidden member's vote is not broadcast at all.** Suppression, not
anonymization. An activity with the actor stripped still says *somebody in
this patch voted approve at 14:03*; a remote reader holding the patch's own
followers collection, on a patch with eleven members, does not need the
actor. And the anonymized activity buys nothing, because the outcome
federates anyway when the proposal resolves — as counts, naming nobody.
There is no cost here worth paying for.

**3. A proposal or a charter still federates; its author is not named.**
The opposite call from decision 2, and the difference is what the payload
*is*. A vote minus its voter is only the leak; a proposal minus its
attribution is the patch's own public text, which is the thing the object
exists to publish. Suppressing it would put one member's private switch in
charge of what the patch may say, which is not a power ADR 006 hands
anybody. So the object goes out whole and unattributed.

**4. The builders refuse to guess.** `ProposalToObject` and
`GovernanceDocToObject` take an explicit `attributeAuthor bool` rather than
deriving the attribution from the model they are handed. A new call site
cannot inherit the leak by omission; it has to answer the question, and the
answer is a database read the `ap` package has no business doing. The
handler-side question is one function, `membershipHidden` — a sibling of
`viewerIsInPatchRoom` in the same file, for the reader who has no session to
check. Federation *is* that reader: there is no room for a remote follower
to be inside, so the viewer half of ADR 006's test falls away and only the
state of the switch is left.

**5. The pull side asks the same question as the push side.**
`/ap/proposals/{id}` and `/ap/governance/{id}` serve the same objects to an
anonymous fetch. Gating only the broadcast would have left the leak behind a
`curl`.

**6. A missing membership row reads as not hidden**, matching the voter
roster's `COALESCE(m.visible, 1)` exactly. Somebody who left has no
membership to hide, and ADR 006 governs a switch on a row that exists.

**7. The gated broadcasts are synchronous.** `BroadcastToFollowers` only
writes rows to the outbox queue; the delivery worker does the network. The
goroutine bought nothing and cost the gate its testability — against a
racing queue write a test cannot tell "withheld" from "hasn't run yet", so a
suppression is a suppression that can be deleted without anything noticing.
This is the argument `broadcastDocUpdate` already made after its own gate's
first test passed with the gate removed.

## Consequences

A hidden member votes, their ballot counts, the tally moves, the roster says
"Hidden member" to outsiders and their real name inside the room, and
nothing at all leaves for the fediverse. A remote follower of a patch where
several members are hidden sees a slightly quieter stream of votes; there is
no honest way to give them more.

This is deliberately *not* patch-level governance-record visibility, which
is a separate question with its own answer in flight: whether a patch's
proposals and votes should federate at all is the patch's call to make, and
it will gate these same three broadcasts for its own reasons. This decision
is only the ADR 006 violation, kept to its own diff so the two read apart.

What is not fixed here, and is worth writing down so it is found on purpose
rather than by accident: `/ap/nodes/{id}/followers` enumerates local
followers by actor id. A follower relationship is not a membership and ADR
006's switch does not govern it, so this is not the same violation — but it
is the same shape of question, asked of a collection nobody has audited
against docs/adr/117's "a follower is not a quieter member".
