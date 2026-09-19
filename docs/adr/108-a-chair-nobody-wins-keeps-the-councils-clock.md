# 108. A chair nobody wins keeps the council's clock

**Status:** accepted, 2026-09-15

## Context

docs/adr/051 put `term_ends_at` on the seat and derived dueness from it, so
that one fact lives in one place and staggering is free. Nothing, anywhere,
ever moved that date on a chair nobody was sitting in.

So an empty chair keeps the term end of whoever sat in it last, which is a
date in the past, which means it is overdue on every pass of the calendar
for ever. The only brake was the breather — one contest length after a
contest that settled nothing — and a brake is not a calendar. Bobbin Hall
Co-op ran six contests on 2025-11-10, 2026-01-05, 03-02, 04-27, 06-22 and
08-17: **exactly 56 days apart, five times running**, which is 28 days of
contest plus a 28-day breather. Its `admin_term_months` is 12, its overview
said "Each term runs 12 months", and its public description says "elected by
the members every year".

Kofi did the arithmetic himself rather than being told it:

> Fifty-six days between every single one of them, five times in a row. Eight
> weeks, exactly, like clockwork... A term cannot be a year if the chair is
> contested every eight weeks.

He drew the conclusion that matters: *"When your pages contradict each other
this plainly, the rest of what they say stops being worth reading, and I am
the sort who stops reading."*

Devon, trying to work out when he could next stand, could not:

> My honest best guess is "about eight weeks after this one ends, probably
> late November", and I worked that out by counting gaps in a list, which is
> not how I should have to find out.

There is a second, quieter half. `contestOpensFor` derives its date from the
term end alone and knows nothing about the breather, so a patch inside one was
told "the next contest is due now" on each of twenty-eight consecutive days
while nothing happened. The date a member was given was never the day
something would occur.

## Decision

**A chair a contest does not fill takes the council's own term end**, at the
moment the contest closes — settled or unsettled. It then comes up with the
rest of the council instead of every eight weeks under its own stale clock.

The date is borrowed from a chair somebody actually holds, so it is a term the
electorate set rather than a number this code invented.

**In the meantime the chair is a vacancy, and a vacancy is filled by
nomination on any day** (docs/adr/100). That route was always there; the
permanently-overdue calendar had been standing in for it, badly. Nothing here
narrows what a contest contests: docs/adr/100's rule that the council's size
is its seats and "the next contest contests the seats that exist" holds, and
an admin who adds five chairs still adds five contests. What changes is the
*date* a vacant chair carries, not whether it is contested.

**Except where nobody holds a chair at all.** There is no admin to raise a
nomination and no held term to borrow, and the contest is the patch's only way
back — which is precisely what docs/adr/102 leaves behind and relies on being
retried. Those chairs stay overdue and the calendar keeps offering the
contest. An earlier draft of this decision dropped every vacant chair out of
the calendar and had to be thrown away: it broke docs/adr/051's deliberate
rule that the earliest term end is the date a member is asking about, whoever
is sitting there, and the suite said so in three places.

**The breather becomes a date the pages can print.** `breatherUntil` returns
the day the calendar may next open a contest here, `scheduleFor` reads it
instead of returning early on its own arithmetic, and `contestOpensFor` takes
it as a floor — the later of "the term runs out" and "the breather ends" is
the day something happens. One definition, read by the sweep and by every
surface that prints a date.

**And a vacant chair on a council with no admins says so.** The seat row used
to read "Filled by nomination: an admin puts a member forward" on a patch with
no admins, which describes a person who does not exist there; Devon said as
much. That chair now carries its own fill route (`contest_fills`) and the
council's next contest date, and the page above it stops telling members to
ask an admin.

## Consequences

- A patch can now have a vacant chair for as long as a term. That is the
  honest position: the council is quorate, somebody is sitting, and the chair
  is fillable today by the people who are there. A contest every eight weeks
  was not urgency, it was a clock nobody had wound.
- The exception means an admin-less elected patch still generates a contest
  roughly every six weeks (docs/adr/106 shortened an unstood contest to the
  nomination window alone). On a patch with nobody in charge that is the
  product trying to fix the situation rather than crying wolf, and each
  attempt now says when the next one opens.
- `nextTermEnd` and `scheduleFor` read one helper, `calendarSeats`, so the
  contest that opens and the date a member is told are answers about the same
  chairs. They were two queries with the same intent and one of them was
  wrong.
- Nothing migrates. Chairs stranded on old dates are repaired the next time a
  contest closes over them, which for an overdue council is within one cycle.
  A migration guessing at term ends would be inventing mandates, and the
  calendar fixes itself in the ordinary course of doing its job.
