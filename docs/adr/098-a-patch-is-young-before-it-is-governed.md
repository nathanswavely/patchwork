# 098. A patch is young before it is governed

**Status:** accepted, 2026-09-14

## Context

Epoch 0 of the governance simulation (docs/adr/096) had five founders
create their organizations through the site, with no script and no
documentation, and keep journals. Three of the day's findings are about
the same thing: the governance model treats a patch that is one hour old
as if it were a community, and a community it is not yet.

**The birth election.** docs/adr/097 applied 051's "adopting elected
leadership starts an election" to creation under an elected template, so
that a founder would hold a seat and the calendar would run. Both
founders who chose the Formal template found what that means on day one:
one seat, contested the moment the page exists, with nobody on the patch
but them, attributed "Proposed by *their name*", unstoppable, and closed
to their own vote. Worse, because seats are created only by a *resolved*
contest, a birth election that settles nothing — and with an electorate
of nobody it always settles nothing — leaves no seats behind, so
`ScheduleDueElections` finds nothing due and the calendar never runs
again. The same dead end waits for an existing patch that adopts elected
leadership by a rules edit and whose first contest fails.

**The locked rules.** The Formal template sets thirty days' tenure
before a member may vote (docs/adr/044). The founder's own membership is
one day old. So the founder who wants to fix a rule the template got
wrong opens a vote whose electorate is zero, reads "0 of 0 needed", and
watches it lapse two weeks later (docs/adr/097) with no ballots. The
template picker's "you can change this later" was, for a month, untrue.

**The door with no key.** A founder who chose invite-only found no way
to invite anyone. Instance invite links (docs/adr/001) create accounts;
nothing admits a person to a patch that does not let them ask. The band
— the archetype the model was designed around — could not admit its own
members.

## Decisions

**Creation seats the founder.** A patch born under an elected template
seats its founding admin at once, with a term that starts today and runs
`admin_term_months` (no end when the template sets no term). No election
opens at birth: a founder alone has nobody to elect from, and 051's
"adoption starts an election" describes a community that already exists
deciding to govern itself differently, not a page that has existed for a
minute. The calendar then does what it already does — opens the first
real contest a lead time before the founding term ends, by which time
there are members with tenure to hold it. That founding term is not a
mandate stolen from anybody; it is the term every founder of every
organization serves before its first election.

**Adoption seats the sitting council before it contests it.** Where an
existing patch adopts elected leadership, the admins it already has take
seats whose term ended today, and *then* the contest opens. Holdover
(051) now has something to hold: an unsettled contest leaves overdue
seats behind, the breather in `scheduleFor` runs, and the calendar tries
again one contest length later instead of falling silent.

**A patch younger than its tenure bar has no tenure bar.** Nobody can be
required to have been a member longer than the patch has existed, and
until the patch is as old as its own rule there is nobody on it who is
not a newcomer, so there is nobody the rule protects anyone from. While
the patch's age in days is below `min_voting_tenure_days` the required
tenure is zero; from that day on it is the configured number. One
helper computes it and both the vote gate and the quorum denominator
call it, so docs/adr/044's one set stays one set. A day-old Formal
patch lets its day-old members vote; the thirty-day bar arrives on the
thirtieth day, by which time it means something. The tempting
alternative, "the smaller of the bar and the age", was tried first and
rejected: it admits only people who joined on the first day, because
everyone after has less tenure than the patch has age and waits the
full number regardless — the co-op's board, joining on day two, would
have sat out the first month, which is the finding this exists to fix.
There is no founder exception, because the founder is not special.

**A patch may state its age.** `nodes.founded_at`, a date, settable by a
patch admin at Settings → Info, NULL meaning "when the row was created".
The cap reads the founding date, not the row's. This exists for the
organization that predates its patch: a co-op founded in 2015 that moves
its bylaws onto Patchwork keeps its thirty-day rule from the first day,
because its rule was never about the website's age. The default is the
cap; the override is the strictness, and choosing it is the
organization's act.

**An admin invites; the person accepts.** A membership status `invited`:
an active admin names a person by username, the person is told, and the
row becomes a membership only when they accept. Declining deletes the
row, because an invitation nobody took is not a record of anything.
Joining while invited accepts; withdrawing (docs/adr/088) still takes
only a pending row, because an invitation was never the person's
request. Invited is not membership: it counts nowhere, threads nothing,
receives no room's notices, and appears only to the patch's admins.
Consent stays with the person — the alternative, an admin adding people
directly, puts someone into a community's record without their say, and
a community's record is exactly the thing this platform exists to keep
honest.

## Consequences

- docs/adr/097's third decision ("born elected means adopted elected")
  is superseded in its mechanism and kept in its intent: the founder is
  seated rather than contested, and the calendar runs either way.
- The `seats` table gains rows that no election wrote. The governance
  record must narrate a founding seat as a founding, not as a result;
  the audit log carries `seat.founded` and `seat.holdover` so it can.
- A patch whose founding date is set far in the past and whose members
  all joined this week has, deliberately, no electorate for a month.
  That is the organization's rule applied by the organization's choice,
  and Settings → Info says so in one sentence beside the field.
- `invited` joins `pending`, `active`, `left` and `banned` in the
  membership CHECK, and every membership query that filtered on anything
  looser than `status = 'active'` was found and tightened. A new query
  that forgets will count invitations as members; the electorate helper
  and the member-count queries are the places to copy from.
- The create form now asks who can join before it asks anything about
  governance (F-008, the finding every founder hit), because that is the
  first question a person making a group page asks, and the API had
  been answering it silently.
