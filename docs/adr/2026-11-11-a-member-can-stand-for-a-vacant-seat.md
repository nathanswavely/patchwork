# A member can stand for a vacant seat

**Status:** accepted, 2026-11-11. Extends docs/adr/100 and docs/adr/051;
leaves docs/adr/108 standing.

## Context

On an `elected` patch between contests, nothing offered a member a way onto
the council. The hub's Leadership block listed the chairs and said how each
was filled; `ElectionPanel` exists only on an election proposal, which
between contests there is none of; and the proposal form offered the
Membership type only to an admin, because a membership proposal is a
nomination and `validateNomination` required its author to hold admin.

Sam, a member of three years, used the only door he could find and made a
consensus vote on his own name that would have decided nothing either way
(F-042). docs/adr/100 closed that half: a membership proposal now has to
name its subject. It did not open a door, and Sam walked into the wall a
second time, on a council with two vacant chairs and a sentence telling him
an admin fills them: "So the only way onto the board is for somebody
already on it to remember me."

An elected council's seats are the electorate's to fill. A contest already
lets any member stand. Between contests the only route was the
meritocratic one docs/adr/051 borrowed for mid-term vacancies, and it kept
the meritocratic rule that only an admin raises a nomination.

## Decision

**On an elected patch, a member may stand for a vacant seat themselves.**
Standing is a membership proposal whose subject is its own author, and the
members ratify it exactly as they ratify a nomination today: the same vote,
the same `ratifyNomination`, the same chair with its remaining term. Standing
grants nothing by itself.

- **Which chairs.** The ones `seatsOf` already routes to `nomination`:
  vacant, not in the contest running now (docs/adr/103), on a council that
  still has an admin. A chair in a running contest is stood for in that
  contest, and the refusal says so.
- **Not on an empty council.** Where nobody holds admin, docs/adr/108 makes
  the contest the way back and keeps the calendar offering it. A standing
  proposal racing that contest for the same chair would seat somebody the
  ballot could then unseat, so standing there is refused with the date's
  sentence: the next election fills it, and any member may stand.
- **Only for yourself.** Putting somebody else forward is still a
  nomination, and still an admin's act. A follower is not on the ladder and
  may do neither.
- **The doors.** A vacant chair whose route is nomination carries "Stand for
  this seat" for a member, linking to the proposal form with
  `?type=stand`, where a "Stand for a seat" type sends the membership
  proposal with the member's own id. The chair's sentence reads "a member
  stands, or an admin puts one forward", and the hub's what-you-can-do line
  tells a member they can stand rather than to ask an admin.
- **The proposal says who it is about.** A membership proposal's page now
  opens with "Priya Natarajan is standing for admin" (or "Sam Pryor
  nominated Priya Natarajan for admin") and what passing it does. Before,
  a nomination was a title and an Approve button, the shape that put a
  Reject button under a volunteer's name.

## Consequences

- Recusal applies as it does to any nomination: where `subject_recusal` is
  on, the candidate does not vote on their own candidacy.
- An admin-decides elected patch sends a member's candidacy to the
  maintainer, like any member's proposal (docs/adr/092), and the
  maintainer's approval seats them (docs/adr/2026-10-18-a-nomination-takes-effect-where-the-decision-lands.md).
- The hub does not count a candidate's own proposal in "needs your vote",
  with recusal on or off. That landed separately (F-042): the subject may
  still vote where recusal is off, but the count does not ask them to.
- No schema change: `proposals.target_user_id` already carries the subject.
