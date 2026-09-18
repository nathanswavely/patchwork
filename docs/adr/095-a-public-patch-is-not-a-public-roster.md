# ADR 095: A public patch is not a public roster

Date: 2026-09-14. Status: **accepted**; extends docs/adr/006, which gave
the member the only switch there was.

## Context

Until now, `nodes.visibility` decided whether a patch could be found and
`memberships.visible` decided whether a person appeared on its list. There
was nothing in between, and the gap has a shape: a patch that wants to be
found, wants its events in the calendar, wants its tile on the quilt — and
does not want to hand a stranger a list of everyone in it.

That is not an exotic want. A tenants' union, a mutual aid group, a queer
collective, a reproductive health clinic's volunteer circle: all of them
are public in the sense that matters (come to our events, read what we
say, know we exist) and none of them can afford to be enumerable. The
existing answer — "each member can hide themselves" — fails them three
ways. It asks every member individually to make a decision the group has
already made together. It defaults to exposed, so the protection arrives
only after somebody thinks of it. And it produces a half-list, which is
worse than either a full one or none: the people who did not think of it
are named, and the fact that some hid is itself legible.

What the codebase already said, and what bound the design:

- **`memberships.visible` is the member's** (docs/adr/006). One switch,
  both surfaces, owned by the person. Nothing here may take it away, and
  nothing here may override it in the revealing direction.
- **The quilt is drawn from member counts.** `GET /nodes/tree` returns
  them and the treemap sizes tiles by them
  (`internal/handler/tree.go`). A control that claimed to hide "how many"
  would be contradicted by the front page.
- **There is deliberately no people search.** People are discovered
  through patches (docs/adr/006). The patch listing *is* the enumeration
  surface, which is why gating it is worth something rather than being
  security theatre.
- **`follower_permissions.members` is not this.** It hides the Members
  *tab* from signed-in followers, over a read that stays public —
  `patchWorkspace.js` says so in as many words, and `ListMembers` has
  never consulted it. It is a workspace tidiness key, not a privacy
  control, and the two must not be merged.
- **Admins are already named by their acts.** `GET /nodes/{slug}/proposals`
  is a public read carrying `author_name`, and leadership attestations
  (docs/adr/052) are public by design. Whoever runs a patch is legible
  from the outside whether or not a list says so.

## Decisions

**1. One field, three states, ordered by exposure.**
`nodes.public_member_list`, one of `everyone` (the default and today's
behaviour), `admins`, or `nobody`. It answers one question — who appears
in this patch's public member list — and the answers are a ladder, each
showing a strict subset of the one above.

**2. It only ever subtracts.** The gate the public listing runs is the
member's switch **and** the patch's setting. A patch may hide a member
who chose to be visible; no setting can reveal a member who chose to be
hidden. ADR 006's rule that the member owns their own switch survives
intact, because this field cannot write to it and cannot outvote it in
the one direction that would matter.

**3. It hides who, never how many.** `member_count` and `follower_count`
stay public at every setting. The quilt sizes tiles by member count, so a
control promising to hide the number would be broken by the front page
before the reader finished clicking. Saying this out loud is better than
a setting that quietly means less than it says: the thing being withheld
is the identities, and a patch whose *size* is sensitive wants
`visibility: private`, not this.

**4. It governs this patch's surfaces, and stops at the person's own
page.** The listing, the profile's members glimpse, and the Members tab.
It does not reach `/users/{username}`: what a person's profile says about
their memberships is theirs under docs/adr/006, and a patch-level
override there would let an admin closet a member who had chosen to be
counted. The two surfaces answer different questions — "who is in this
patch" is enumeration and belongs to the patch; "what am I in" is
disclosure and belongs to the person — so the one-switch-two-surfaces
rule of ADR 006 is amended to name its scope rather than repealed.

**5. `admins` is the middle rung, and there is no rung the other way.**
The public seeing who runs a patch is the fact an outsider most often has
a legitimate claim on: to make contact, to hold somebody accountable, to
know who they would be dealing with. The mirrored state — members shown,
admins hidden — was considered and is refused, because the product would
break the promise immediately: a public proposal names its author, a
public attestation names who attested, an approved event names who
approved it. A setting that says "our organisers are not named here"
while four other surfaces name them is worse than not offering it.

**6. It is a Patch Settings control, not a governance rule.** It sits
beside membership management, not in `governance-rules.json`, and an
admin applies it immediately. The deciding argument is that this is a
safety lever: a patch that needs its roster down needs it down now, and a
control that takes a 72-hour vote to pull is not a safety lever. The cost
is real and accepted — on a voting patch this is a thing an admin can do
without asking — and it is bounded by decision 2, since the worst an
admin can do with it is hide people, never expose them.

**7. Nothing federates.** Memberships have never travelled on an AP actor
(docs/adr/006), so there is no `publicMembers` field to add and no
remote surface to gate. The column travels in a seamrip like every other
patch setting: a fork that lost it would publish a roster the original
had taken down.

> **Corrected, 2026-09-18.** True of the actor document and false in
> effect. Memberships were never a *field* on the wire, and this decision
> stopped reading there — but a governance activity carrying a person's
> actor and a patch's is a membership assertion in all but name, because
> nobody outside a patch can vote in it or author its charters. The
> `gv:Vote` broadcast announced by actor the ballot the REST roster on the
> same request had just withheld. "No remote surface to gate" was the
> wrong question; the right one is whether a membership can be *inferred*
> from what travels, and it could. Fixed, with the reasoning, in
> docs/adr/2026-09-18-a-vote-is-a-membership-said-out-loud.md. The rest of
> this decision stands: `public_member_list` itself still federates
> nothing, and it is a separate control from the member's own switch.

## Consequences

An outsider on a `nobody` patch gets an empty listing, a hidden Members
tab, no members glimpse on the profile, and counts that still say how
many. On `admins`, the listing and the glimpse are the admins, and both
say "Admins" rather than "Members" — a section headed Members showing
three of forty people misreports the patch. The patch's own members and
admins always see the full room, as they do today, and an instance admin
sees it too (they already do, everywhere).

A follower is an outsider for this purpose. That is the one place this
lands somewhere surprising — a follower can see less than they could
before — so the empty state says the list is not public rather than
letting `No members yet` tell a follower something false about a patch
with forty people in it.

**One number, on every surface.** Decision 3 says the count stays public.
Shipping it showed that the count was not one number to begin with:
`ListMembers` had always counted under the same `m.visible` gate its
listing ran, while `GetNode` and the tree endpoint counted every active
row. A patch with three hidden memberships therefore told the same
signed-out visitor "40 Members" in its profile head and "37 members" on
the page directly below it. That predates this ADR — it is docs/adr/006's
gate, not this one's — and it is fixed the way decision 3 points: every
surface states the ungated number. A count names nobody, so the gates here
decide who is *listed*, never how many there are. Gating every surface
instead was the other coherent answer and is refused for decision 3's own
reason: the quilt would then size a patch's tile differently for each
viewer, and one member flipping their own switch would shrink their
patch's published size. The cost is the small-patch inference — five
counted over two listed says two people are not listed — which the profile
head already published and which `nobody` is the answer to.

**Known limit, recorded so it is not mistaken for a bug.** This hides the
list. It does not un-name a member whom some *other* public surface
names: a proposal carries `author_name` on a public read, an event
carries who posted it. A patch using this for safety is buying
non-enumerability, not anonymity, and the settings copy says so in one
line rather than letting the control over-promise. Extending the gate to
authorship on those surfaces is a real design question — it trades
against the public legibility of governance, which is most of why
proposals are a public read at all — and it is deliberately not answered
here.
