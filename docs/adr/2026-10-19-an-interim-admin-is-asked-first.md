# An interim admin is asked first

**Status:** accepted, 2026-10-19

## Context

When inactivity empties a patch's council and its `succession_policy` is
`longest_tenure`, the sweep in `internal/notifications/inactivity.go` hands the
patch to up to three of its longest-standing members who have been around
recently. Until now it did that by making them admins on the spot and telling
them afterwards: "You are now an interim admin of …".

The governance simulation filed this as F-085: somebody is made an admin by a
notification, with no way to decline. Half of the original finding has since
been fixed. The sweep no longer reaches a stranger, because
`promoteLongestTenured` counts only people present in that patch inside the
window. The other half was untouched: nobody is asked.

The project has already answered this question twice, one relationship over
each time.

- An invitation (docs/adr/098) is an admin asking and the person answering. The
  row is `invited` until they accept or decline.
- A follower is never promoted by the role control (docs/adr/117), because "the
  person has not said yes, and a role set here is what they would become
  without anyone having asked them". The same record states the asymmetry this
  rests on: "Ending a relationship needs nobody's consent the way starting one
  does."

An interim admin is a role starting. The sweep was the one remaining path that
started a role without asking the person.

## Decision

**The sweep offers; the person accepts.** Where it used to promote, it now
records an offer for each person it picks and notifies them with a question:
"Will you step in as an interim admin of …?". Nobody becomes an admin until
they accept.

**An offer is answered at the patch's Governance page**, where the notification
links, by the person it was made to and nobody else:
`POST /api/v1/nodes/{slug}/interim-offer/accept` and `.../decline`, with
`GET /api/v1/users/me/interim-offers` listing the caller's own open ones. The
question sits above every other banner on that page and renders for nobody
else. Anyone without an open offer gets the same 404 an unknown patch gets, so
the route says nothing about who was asked.

**Accepting makes them an admin** through the same `makeAdmin` every other
path uses, so their inactivity clock starts from the day they said yes
(migration 072). It is audited as `membership.succession`, as the old
promotion was, now with the person as the actor rather than nobody.

**Declining passes the offer on while the patch still has no admins.** The
next present member, by the same ordering the sweep uses, is asked, so the
patch still looks for up to three people. Once somebody has accepted, the patch
is governed again and a later decline recruits nobody; the questions already
out stay answerable, as the old rule would have made all three admins at once.
Someone who declined is not asked again until the council has been filled and
emptied again. If everybody who has been around says no, the patch falls into
the path that already exists for a patch nobody here can run: an instance
admin is told it needs one. Somebody who leaves before answering is treated as
having said no, since a person who has gone cannot step in.

**No clock answers for anybody.** An offer stands until it is answered. It is
not withdrawn after a period, and an unanswered offer is not taken as a yes or
a no. This is the same rule as holdover: no clock removes anybody, and no
clock appoints anybody either.

**The members are told the council is empty when it is.** Before, a
successful promotion meant nobody was ever told the patch had been without
admins, because by the time the sweep finished it was not. Now it is, until
somebody accepts, so the existing one-time "has no admins" notice goes out
with a sentence saying its longest-standing active members have been asked. It
does not name them: an offer is between the patch and the person.

The Governance page says the same thing. Its line for a patch with no admins
used to say only an instance admin could put anybody back, which was false the
moment a question was out and sat directly under the question's Accept. While
any question is still waiting on a member, the overview reports
`interim_offers_open` and the line says the longest-standing active members
have been asked. It never says who.

## Consequences

**A patch can now sit without admins while offers are open.** That state
already existed (a `freeze` policy, an `instance_admin` policy, a patch with
nobody present), it is already announced, and an instance admin can already
restore an admin there (`UpdateMember`'s empty-council route). This decision
puts more patches into it, for longer. That is the cost of asking, and it is
the cost the project already accepted for invitations.

**A new table, `interim_admin_offers`.** One row per offer, with
`answered_at` and `answer` once it is answered. A unique index allows one open
offer per person per patch. Answered offers are cleared when the sweep next
finds the council non-empty, which is what lets a future emptying ask the same
people again; the audit log keeps the record of every answer.

- **It stays behind in a seamrip.** An open offer is one instance's sweep
  asking one person about one emptying. A fork runs its own sweep, on its own
  clock, and asks again if it needs to.
- **Account deletion deletes it.** An offer is a question addressed to the
  person, and an account that no longer exists cannot answer or serve (the
  same reasoning that deletes a standing candidacy).

**A later acceptance can add an interim admin to a council that has since
been filled.** With up to three offers out, the first acceptance fills the
council and the others remain open. Accepting one later adds a second or third
interim admin, which is what the old code did to all three at once. The
difference is that each of them said yes.

**A year of silence now settles with no admins and nobody asked twice.** The
quiet-year regression (`inactivity_settles_test.go`) used to expect the role
to move twice, because the old rule moved it without anyone acting. Nobody
acting now includes nobody saying yes, so the role never moves and the two
people who were around stay asked. The path where they do say yes and later
lapse is held by its own test, and still settles.

## Not decided here

**A designated successor is still installed without being asked.**
`promoteSuccessor` makes the person a maintainer named as successor and tells
them afterwards, and its notice offers a way to hand the patch on. The
maintainer chose them, which the longest-tenure rule does not have, but the
person still did not say yes. Whether naming a successor should itself ask the
named person is a separate question about docs/adr/051, and it is left open
rather than answered by analogy.

**Elected and meritocratic patches are unchanged.** Their councils refill by
election and by nomination with ratification, which already ask: a candidate
stands, and a nominee is ratified by the members.

## Alternative considered

**Install, then let them step back.** Keep promoting immediately, and offer a
"Step back" that returns the person to member and passes the role on. Smaller,
with no new table, and the sweep never leaves a patch without admins by its
own act. Rejected because the role still starts without asking, which is the
shape docs/adr/117 removed from the role control.
