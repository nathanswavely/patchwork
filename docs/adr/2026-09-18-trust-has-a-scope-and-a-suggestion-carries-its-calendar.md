# ADR: Trust has a scope, and a suggestion carries its calendar

Date: 2026-09-18. Status: accepted. Amends ADR 026 (the trusted-contributor
grant gains a per-patch scope and waives listing review at quilt-wide scope),
ADR 031 (trusted contributors may attach event sources on the patches their
grant reaches) and ADR 039 (whose claim that trusted contributors create
unclaimed patches directly was never true in code, and is now). Decided while
grilling a user-testing finding.

## Context

A tester wanted to put an organization's events on the quilt. He does not run
the organization. The right path was a patch suggestion, and he was told so.
He opened "New patch", read past one muted sentence at the top of the form,
created the patch himself, and became admin of an organization that has not
admitted him: the fabricated relationship the glossary's unclaimed-patch entry
exists to forbid, with a lining adopted in that organization's name.

The sentence was not the only problem. The path he was told to take was
worse than anyone had said:

- A suggestion queues for the instance admin. Once approved, every event the
  suggester adds queues again, for the same admin, because trusted contributor
  is the only thing that waives it and the grant is quilt-wide or nothing.
- The suggester hears nothing. Approval and rejection notify site admins
  only; there was no notification type for either outcome, so he would have
  had no signal to come back and add events at all.
- A suggestion could not carry a feed, and no one below instance admin could
  attach one to a listing, because ADR 031 excludes trusted contributors from
  event sources on purpose.

Two of the three fixes cut against written decisions. ADR 026 rejected
automatic promotion outright, and the glossary said the grant is "never
earned automatically". ADR 031 said the grant "delegates the review queue,
not standing feeds".

## Decision

**1. Creation opens on a fork.** `/patches/new` asks one question before any
field: "Is this your patch to run?" Two equal cards: *I run this patch*, which
leads to the form and says you become its admin, and *Someone else runs it*,
which leads to the suggest form and says the patch joins the quilt unclaimed
for its owner to claim. Claim setup skips the step, since a claimant has
already answered. Every surface that led to creation is relabelled to the
neutral "Add a patch" so nothing promises creation before the question is
asked. A separate interstitial route was considered and rejected: it would
have left three entry points to re-point and a button still reading "Create".

**2. The trusted-contributor grant has a scope: the whole quilt, or one
patch.** Both are given explicitly by an instance admin and revoked by one.
The per-patch grant is the same standing on one unclaimed patch and nothing
else, and it ends when the patch is claimed, in the same transaction that
flips the patch active, because the calendar it opened now has an owner. It
is not a membership row: an unclaimed patch admits nobody. It reaches exactly
the gates that are about one patch: posting an event directly, editing your
own event, standing in the link handshake (ADR 057), CSV event upload, and
now event sources. It does not reach the one gate that is about no patch:
deriving a verification domain from a new suggestion's website.

**3. The per-patch grant is offered at suggestion approval, checked, and the
admin may uncheck it.** This is the line between what ADR 026 refused and
what a suggester needs. Approving a listing is a judgement about the place.
Letting its suggester keep its calendar unreviewed is a second judgement, and
an approval click that carried it silently would be the automatic promotion
ADR 026 rejected. A checked line the admin reads before clicking keeps "given
explicitly" true while costing a good-faith community nothing. A fully
automatic grant was the first instinct and was rejected for that reason; a
grant of nothing was the status quo that produced the shortcut.

**4. The quilt-wide grant waives listing review too.** ADR 026's own
reasoning is that the grant "is the instance admin delegating their own
queue", and the listing queue is that same admin's queue; nothing about a
listing is owed to patch admins, since there are none yet. A quilt-wide
trusted contributor's suggestion lands as an unclaimed patch at once, audited
as a submission the way `submissions.auto_approve` already is. The per-patch
grant does not reach this: a new suggestion is not the patch it names.

**5. Trusted contributors may attach, sync and detach event sources on the
unclaimed patches their grant reaches.** ADR 031's exclusion rested on a
distinction the code had already undercut: a trusted contributor could
CSV-upload forty events silently onto a listing, and a feed is that batch
kept current. The grant is revocable and an instance admin can detach any
source, so the standing consent is never out of anyone's hands. Revoking the
grant does not detach the sources that person attached, as ADR 057 leaves
their pending links standing: they are ordinary rows the admin sees and can
stop.

**6. A suggestion may carry a feed.** An optional field on the suggest form,
fetched through the existing detection on submit so a URL Patchwork cannot
read is refused now rather than never. It attaches when the listing
publishes: at submission for a quilt-wide trusted suggester, at approval for
everyone else, where the admin sees the feed and its upcoming count and can
uncheck it. `added_by` is whoever held standing at that moment: the suggester
if the admin granted trust, otherwise the admin. The vouch is theirs.

**7. A trust request is answered, never merely seen.** The ask lives in the
one place the review cost is being paid: the event form, when the event is
about to queue on an unclaimed patch, with that patch preselected. It names a
scope (that patch, more unclaimed patches, or the whole quilt) and may carry
a short message. One open per person. The instance admin answers it from
Admin → Users, where the toggle already lives, at whatever scope they judge
right, wider or narrower than asked, with requested and granted scope kept
as two facts in the audit line. Declined is not spent forever the way a tag
is, because a person is not a word; they may ask again after a cooldown, and
an admin may grant at any time regardless. A named patch claimed before the
answer drops off the request; a request with nothing left resolves itself as
moot. Active patches are never askable. There is no entry point in navigation
or account settings: nobody should go looking for a rank.

**8. The suggester is told.** Two notification types, to the suggester only.
Approval is high priority, links to the patch and states what they now hold.
Decline carries the reviewing admin's note, which the review form already
collected and only wrote to the audit log.

## Not decided here

**Turning an active patch back into a listing.** The tester's patch stays as
it is; he knows the owner and will hand it over if they want it. A "release
to unclaimed" verb would have to delete a governance repo and decide what
happens to proposals, notices, seats and contact shares the patch accumulated
while it lived. If a wrongly created patch with a history worth keeping ever
arises, that is its own decision, and the fork in decision 1 is what should
keep it from arising.

## Consequences

- Schema: a per-patch grant table and a trust-request table, each with a
  boundary decision for the seamrip (ADR 002) and a member-view rule (ADR
  089). Neither travels: ADR 026 already holds that trust is granted
  per-instance and a fork's steward re-grants it.
- The frontend's "direct or suggest" decision, which reads the flag off the
  signed-in user, needs the patch payload to answer "is the viewer trusted
  here" instead.
- The suggest page's heading becomes "Suggest a patch", matching every link
  into it, and the second card on the fork says you can add its events once
  it is approved, which is now true.
