# ADR 090: The old home says where the new one is

Date: 2026-09-07. Status: **accepted**; implemented. Completes
docs/adr/012's third egress affordance. Issue #228, split out of #195.

## Context

ADR 012 named three affordances that together make "leaving is a member
right" structurally true rather than aspirational. The personal export
shipped (affordance 1). The member seamrip is in flight (affordance 2).
This is the third, and it is the one the other two are useless without.

A fork is only a safety valve if the people who need it can find it. After
a seamrip the old instance is still where every link points: the flyer with
the URL on it, the search result, the calendar subscription, the bookmark,
the Mastodon account that followed the patch two years ago. A community can
carry its whole record to a new server and still lose most of its audience,
because the audience is holding an address that now resolves to a room
nobody is in.

The obvious fix is a redirect, and the obvious redirect is the instance
admin's. That is exactly the person ADR 012 assumes may be the problem. The
whole point of the affordance is a pointer the *community* can set, needing
nothing from whoever runs the server beyond the server staying up.

The field is also the thing federated `Move` is layered on. Mastodon-style
clients already know how to render `movedTo` on an actor, so the pointer is
worth publishing whether or not Patchwork ever emits the activity.

## Decisions

**1. Two columns, `nodes.moved_to` and `users.moved_to`.** Nullable TEXT,
one migration (066). Not one instance-level pointer, because the two moves
are different moves: a patch can move without the quilt around it, and a
person can move without their patches. NULL is "has not moved" and is the
only representation of it — clearing the field writes NULL rather than the
empty string.

**2. On a patch it is an admin act, not a proposal.** Set and cleared at
Patch Settings by that patch's admins, the way every other settings field
is.

The alternative was live: a hostile admin pointing a community at a fake
successor is a real failure, and it is uncomfortably close to the one this
feature exists to route around. It was rejected for now on two grounds.
First, it adds no power. A patch admin already edits the name, the
description, every link, the visibility and the membership policy, and can
delete the patch's content outright — an admin who wants to mislead a
community has a dozen easier levers, and a pointer is the most reversible
of them. The adversary ADR 012 actually guards against is the *instance*
admin, and this field is precisely the thing that routes around them.
Second, a proposal is not one mechanism here but two: a patch with
`proposal_venue: elsewhere` (docs/adr/052) conducts no ballot at all, so
making the pointer a proposal would immediately owe that patch an
attestation path. That is a decision worth making deliberately, on its own,
after the field has been used. Revisit this if a real instance reports a
pointer nobody agreed to.

An instance admin can set it too, because `UpdateNode` accepts one on every
field and always has. This does not weaken the affordance: the pointer's
value to a community is that a patch admin *can* set it, not that nobody
else can.

**3. On a profile it is the person's own.** Set and cleared at account
settings, rendered on the public profile, and deliberately independent of
any patch. Nobody else can write it, including an instance admin.

**4. Validated as an outbound href at every write path**, exactly the rule
`events.event_url` follows (docs/adr/079): http and https only, bounded at
2048 characters, because this value ends up as an `href` on a page other
people read. http is allowed for the same reason it is allowed there — a
community that has just stood a server up somewhere cheap should not be
told its new home is not a link.

One clause is added: **the pointer may not name this instance's own
domain.** A patch saying it moved to the quilt it is already on is a typo
at best and a loop for anything that follows the field. Host comparison is
case-insensitive and port-sensitive, so a dev quilt on `:8443` is a
different host.

**5. A moved patch is read-mostly.** Joining and following are declined
with the pointer in the error body, and so are event submissions from
people who are not members. Everything a member or admin could already do
still works, and existing followers keep their relationship. The old home
stays a record, and a record can be corrected — what stops is starting a
new relationship with a room the community has left, and asking strangers
to keep feeding a calendar nobody reads. Nothing is deleted and nothing is
hidden.

**6. Federation carries the field and not the activity.** A patch or person
with a pointer gets `movedTo` on their actor document, together with the
JSON-LD term definition that gives it meaning (the plain AS2 context does
not define it, so an actor carrying it bare would be publishing a property
no consumer is obliged to understand). An actor that has not moved is
byte-for-byte the document it was.

No `Move` activity is emitted, and that is a decision rather than an
omission. `Move` asks every remote server to unfollow the old actor and
follow the new one. It is a write on somebody else's database, it is
irreversible from here, and it requires the new actor to name the old one
in `alsoKnownAs` before anybody will honour it. A field a community can set
and clear in ten seconds is a very different commitment from an activity
that redistributes their audience permanently. `Move` deserves its own
decision, made once the field exists and somebody has used it.

**7. Both columns travel** (docs/adr/002). A fork of a fork still has to
know where things went: a chain of moves that forgets its own last hop
strands anyone reading it from the far end. Nothing on the import side sets
a pointer — see the rejected alternatives.

**8. A tombstoned account's pointer is cleared** with the other identity
columns (docs/adr/086). It is a sentence about where to find this person,
and somebody who deleted their account asked for the opposite of that.

## Consequences

The banner sits at the top of the patch page, above the description,
because a reader who has landed on a patch the community has left needs the
forwarding address before they need the blurb. The discovery card wears a
"Moved" chip; the address itself is one tap in, on the patch's own page.

Where the pointer matches the `https://host/patches/slug` shape the
discovery search and Connected Quilts already recognise (docs/adr/024), the
notice also offers the read-only remote patch card at
`/quilts/{host}/patches/{slug}`, so a move to another Patchwork lands
somewhere a reader can act rather than merely arrive. Anything else is a
plain external link with `rel="noopener"`. That shape was written out at
three call sites before this change and is now in `web/src/lib/patchLink.js`
once — three copies of a rule is a rule that disagrees with itself the first
time somebody widens it.

The relationship row drops Follow and Become a member on a moved patch, and
the event door stops offering "Suggest an event" to anybody who is not a
member — docs/adr/042's rule applied again, since an absent door beats a 403
at the end of a ceremony and the banner above the row has already explained
why. The server is still the authority in both cases and still refuses. The
standing control itself is untouched: somebody already in the patch keeps the
control that lets them leave.

**Nothing enforces that the pointer is true.** It is a claim the patch's
admins or the person made, rendered as a claim. This is the same standing
every other self-declared field on the instance has, and docs/adr/049's
line applies: Patchwork states only what it enforces, so the banner says
this patch has moved to a host and does not say the host is legitimate.

**The legal defaults were checked and not changed.** The privacy policy
describes the pointer already, without naming it: it is part of "the profile
you choose to write", and the deletion section's "your account row survives
with nothing in it" stays true because the column is cleared. The user
agreement's claims about leaving are about the export and the seamrip, which
this does not touch. Per the standing rule in `legal_defaults.go`, a
sentence is edited when a change falsifies it, and none did.

## Rejected alternatives

**A proposal instead of an admin act.** Rejected for now, with the
reasoning in decision 2. It is the closest call in this set, and the
`proposal_venue: elsewhere` gap is the concrete reason to defer rather than
guess.

**Emitting `Move`.** Rejected for now, with the reasoning in decision 6.
The field is the prerequisite, not the whole feature, and ADR 012 already
called the federated emission future work.

**A single instance-level pointer.** Rejected: it is the instance admin's
field by construction, which makes it useless in the scenario the affordance
exists for. It also cannot express the common case — one patch leaving a
quilt that is otherwise fine — and would say nothing at all about a person
who moved without their community.

**Auto-setting the old side's pointer on import.** The issue raised this and
answered it, and the answer holds: the instance being left is the last one
that should get to write where a community went. An import that reached back
and stamped the origin would be a fork asserting succession on the old
server's behalf, and in the motivating scenario the old server is hostile —
it would simply be undone, or, worse, aimed somewhere else. The pointer is a
statement its owner makes, at the moment they choose, on the instance they
still control.

**Reusing `validateEventURL`.** Rejected on a small point that matters: the
same-origin clause is about this quilt rather than about the link, and
folding it in behind a flag would have made an event's ticket link one
boolean away from refusing a venue that happens to be hosted here.
