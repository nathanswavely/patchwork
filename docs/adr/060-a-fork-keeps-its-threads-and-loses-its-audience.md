# ADR 060: A fork keeps its threads and loses its audience

Date: 2026-08-21. Status: **proposed**. Names a gap in ADR 002 that
predates the atproto work and was found while reading against it. ADR 058
and ADR 059 both point here rather than carrying the argument themselves.

## Context

Seamrip is the governance safety valve: if a community's leadership goes
sideways, members fork the data to a new instance. ADR 002 drew the
boundary, and the community half of it holds up well — memberships
travel, so the fork keeps its inferred threads. The people arrive
together, still connected to each other. Abramov's "Open Social" calls
collective export-and-import "a near-impossible feat of coordination";
Patchwork made it `make export` and `make import`.

The federated half does not hold up, and `internal/seamrip/seamrip_test.go`
says so in its own words:

- `ap_followers` — *"remote followers belong to the old instance's identity"*
- `nodes.ap_id`, `users.ap_id` — *"names an actor on the old domain"*
- `private_key` / `public_key` — *"a fork mints its own on first boot"*

**So a patch that seamrips arrives with all of its people and none of its
audience.** Every remote follower is left behind. The Selvage keeps its
members, its events, its charters and its threads, and loses every person
who followed it from anywhere else.

What makes this hard is that each of those reasons is individually
correct. An `ap_id` naming an actor on a domain the fork does not control
is genuinely unusable. A follower row is genuinely not ours to move: the
Follow lives in the *remote* instance's database, and no amount of moving
our own rows tells that server anything. Only the remote side can
repoint a follow, and only if somebody asks it to.

## The reason this cannot be patched from inside ActivityPub

ActivityPub has an answer to account migration — the `Move` activity, as
Mastodon implements it — and it requires the **old** actor to publish the
move, with the new actor acknowledging the old one. Cooperation from the
instance being left.

That is exactly the cooperation seamrip cannot assume. The scenario the
valve exists for is leadership going sideways: the old instance is the
party that has just become hostile, or negligent, or unreachable. A
migration mechanism that needs its signature is a valve that works in
every case except the one it was built for.

This is not a Patchwork bug; it is a property of instance-scoped identity
everywhere in the fediverse. It bites harder here only because Patchwork
*promises* the fork. Everyone else leaves the exit unmentioned.

## Decision

**Name the gap, decline to paper over it, and state the criterion that
would close it.**

**1. The ADR 002 boundary does not move.** Every stays-behind entry above
keeps its reason. Nothing about this gap is fixed by exporting rows that
name a domain the fork does not own.

**2. No `Move` flow.** Rejected on the reasoning above: it would ship a
migration path that works whenever nobody needs it. Worse, it would read
as a solved problem on the tin — a fork button that quietly does nothing
in the hostile case is more dangerous than no button, because a community
would plan around it.

**3. What would close it is an identity anchored on something the
community owns.** That means a domain. The instance's domain belongs to
whoever runs the instance, which is the wrong party by construction; a
patch's own domain belongs to the patch. Patchwork already vets one:
`nodes.verification_domain`, established through the claim flow (ADR 030)
and already proved by DNS or a well-known file.

This is what promotes ADR 058's step B from a convenience to the load-
bearing piece. A `did:web` handle on a patch's own domain is an identity
the instance cannot hold hostage, which is the only version of a handle
that survives the act seamrip exists to make possible. That is a
different and much better argument for atproto than "a nicer event
source," and it is the argument this project should be making.

Note what this does *not* license: nothing here weakens ADR 058's
constraints. No relay, no AppView, and no expansion past the public
slice. The claim is narrow — the durable-identity property is the reason
step B matters, not a reason to run infrastructure.

**4. Until then, say it.** ADR 049 is the house rule: state only what you
enforce, and by implication do not imply what you don't. Seamrip's own
copy must not suggest the audience travels, and ADR 059's handle row must
say the address is the quilt's rather than the patch's.

## Considered and rejected

**Export the follower list as data, so the fork can ask them again.** A
list of remote actor URLs is portable and nearly useless: the fork would
have to solicit each one to re-follow, from a new address, with no way to
prove it is the same community. That is a mailing list built out of
somebody else's social graph, and it inverts ADR 024's care that nobody
be enumerable in a followers collection. It also fails the hostile case
differently — the old instance keeps serving the original patch, so both
addresses look live and followers have no way to tell which is the
community.

**Treat this as acceptable and stop mentioning it.** Defensible — losing
followers is a small price against losing the community — and rejected on
ADR 049 grounds. The cost is only small if the community knows it is
paying it, and today nothing anywhere says so.

## Consequences

- ADR 059 gains a consequence: its handle is instance-scoped and does not
  survive a seamrip. That is a reason to publish it *with the caveat*, not
  a reason to withhold it — an address that might move still beats an
  address nobody knows exists.
- The gap is now the strongest single argument for ADR 058's step B, and
  step B should be re-read in that light rather than as the small
  mechanical win it was written as.
- **The `Move` mechanics above want checking against the live Mastodon
  setup** that verified interop on 2026-07-13. The decision does not turn
  on the details — cooperation from the old instance is the part that
  matters, and that much is structural — but the specifics were written
  from prior knowledge rather than from the spec.
