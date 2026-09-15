# 100. A seat is a chair you can count

**Status:** accepted, 2026-09-15

## Context

Epoch 1 of the governance simulation put members into the five patches and
let their founders run them for a day. The audit's first-ranked finding is
a contradiction two members worked out unaided.

Nell's collective and Priya's co-op both declare `leadership_model:
elected`. Their governance pages say so: "The community elects admins for
fixed terms." Both founders then made a colleague an admin from the
dropdown on Settings → Members, and the site let them. The database shows
three `membership.role_change` rows, zero new seats, no election, and a
governance record that still reads "Nothing settled yet". The contest the
calendar will open in 2027 is for **one** seat, the founder's, while three
people hold admin power. Nell put it plainly: "Either the election means
something or the dropdown does."

Two further things came from the same root. Priya wanted a board of three
and could find nowhere to say so — the patch has exactly one seat because
exactly one person founded it, and Devon, who joined expecting his own
seat to come up in November, could not find where that gets put right.
And Sam, who wants a seat and found no way to ask for one, used the only
door available: he raised a `membership` proposal about himself, which
carries a NULL target, changes nothing whichever way it closes, and is
now a three-day consensus vote on a person, published under a Reject
button.

docs/adr/051 already settled the mechanism. Under `elected`, "the council
may appoint a replacement from active members. The appointment must be
ratified by the community within 14 days" — nomination plus ratification,
the same ordinary proposal meritocratic runs on — and "someone appointed
to a mid-term vacancy serves out the remainder, not a fresh term",
because otherwise "a seat opens, an ally is appointed, and that person
holds a full term without ever having faced the electorate." The dropdown
is that ally being appointed, with the ratification skipped and the seat
never involved at all. This is a decided thing that was never built, not
an open question.

What 051 does not say is how many chairs there are.

## Decision

**The council's size is its seats, and a seat is added and removed
explicitly.** An admin of an elected patch may add a vacant seat, or
remove a seat that nobody holds. Neither act puts anybody in power or
takes anyone out of it — the filling is governed, below, and removal can
only ever touch an empty chair — so the furniture is administrative while
sitting in it is not. The seats table is the council's size; there is no
second number anywhere, and the next contest contests the seats that
exist rather than "however many admins happen to hold the role", which is
how a three-person council was heading for a one-seat election.

`max_admins` was the obvious candidate and is the wrong one.
docs/adr/049 enforced it, docs/adr/051 retracted that on the grounds that
a council's size "is a function of how it governs, not a number it
configures", and migration 041 backfilled `"max_admins": 3` into nearly
every patch — so adopting it as the seat count would quietly cap live
councils at three with no way out. That is the breakage 051 named. The
field stays what it is: displayed, unenforced, and a separate question.

**On an elected patch the role dropdown does not make an admin.** It
answers that this patch elects its council, says when nominations next
open, and — when a seat is actually vacant — points at the nomination
that fills it. A control that silently outranks the mechanism the page
advertises is worse than no control: both founders believed they were
doing the ordinary thing, and neither was told otherwise.

**A vacancy is filled by nomination and ratification, into the seat.**
The existing membership proposal, already used by meritocratic patches,
targets the person; on approval `ratifyNomination` promotes them *and*
seats them in the vacant seat, inheriting its `term_ends_at`. No vacant
seat, no nomination: the answer is the date of the next contest. This
makes the appointee's term the seat's remaining term, which is 051's
integrity argument, and it puts every admin on an elected patch into a
chair the record can name.

**A membership proposal must name somebody.** Sam's proposal is the
shape a candidacy takes when the product offers no other, and it is
malformed: no target, no effect, and a public up-or-down vote on a
person's name. The API and the form both refuse a membership proposal
with no `target_user_id`, and the governance page tells a member who
wants a seat when nominations open instead of leaving them to invent
this.

## Consequences

- Promotion on an elected patch now costs a vote, which is the point,
  and takes as long as the patch's voting window. A founder setting up
  their first council will feel this most: three founding colleagues
  means three nominations. The alternative — a founding grace period
  where the dropdown still works — was considered and rejected, because
  "the rules do not apply yet" is exactly the sentence that erodes them,
  and docs/adr/098 already gives a founder one seated term to start from.
- `maintainer` and `meritocratic` patches are untouched: the dropdown is
  how a maintainer's patch works, and meritocratic already routes
  through ratification.
- An elected patch whose council is full and whose seats all sit vacant
  after an unsettled contest can still be filled by nomination, which is
  right: holdover keeps the sitting council, and a genuinely empty
  council is the emergency the template's own vacancy rule covers.
- Adding a chair is an admin's act and filling it is the community's.
  That split is the whole design, and it is worth saying out loud that an
  admin can therefore enlarge a council without a vote — they simply
  cannot put anyone in the new chair, and the electorate decides who
  does. An admin who adds five seats has added five contests.
- **A membership proposal now needs a nominee, which retires it as a
  general-purpose type.** Combined with the existing rule that a
  *targeted* nomination is refused outside meritocratic and elected, the
  type is no longer available on a maintainer patch at all. Nothing is
  lost that `action` or `other` does not carry, and the type's own
  description always said it was about a person — but it is a behaviour
  change this decision implies rather than states, and it is stated here
  now.
- **Legacy patches can show more admins than seats**, because nothing
  backfills chairs for admins promoted before this. The overview displays
  both numbers honestly rather than papering over the gap, and the repair
  is an admin adding the chairs. Worth knowing before this reaches a live
  instance with a council already in place.
- Not decided here: whether a member can register standing interest in a
  seat between contests. Sam wanted to, and the honest answer today is
  that nominations happen in a window and the page should say when. If
  that proves too thin, it is its own ADR.
