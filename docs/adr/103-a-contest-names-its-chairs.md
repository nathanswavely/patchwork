# 103. A contest names its chairs

**Status:** accepted, 2026-09-15

## Context

An election carried a number. `proposals.seats_contested` said *how many*
chairs were being decided and nothing said *which*, so resolution had to
guess, and the guess it made was "the council".

`seatWinners` refilled the council's seats in created order, emptied every
seat past the winner count, and stepped down every admin it had not seated.
All three of those are correct for a contest covering the whole council, and
wrong for every other kind.

A council with one chair ending in March and two running to the following
year is the other kind. Its March contest seated the winner in the *oldest*
chair — which might be one of the two nobody voted on — emptied the other
two, and removed two admins the electorate was never asked about. The two
people demoted had been elected, their terms had not run out, and no notice
anywhere said what had happened beyond "the election returned a different
council."

This is not an exotic configuration. docs/adr/051 put `term_ends_at` on the
seat rather than the patch *precisely* so that a council could stagger, and
called spreading a first cohort "just setting shorter initial dates on some of
them". docs/adr/100 then shipped the box that sets those dates. The
calendar already handled staggering correctly — `scheduleFor` counts only the
seats whose terms have run out, and `nextTermEnd` reports the earliest — so a
patch could open a partial contest by design and then have it resolve as
though it were a general election.

It was found by the governance simulation (docs/adr/096) while verifying the
succession fix, and is the reason docs/adr/102 scoped its calendar advance to
"only when the whole council is empty": that branch could reach a partial
contest, so it was written to avoid one rather than to rely on this being
right.

## Decision

**A contest records which chairs it is for, when it opens.**
`seats.contested_in` (migration 073, nullable) points at the election
deciding that chair, is set by `openElectionFor`, and is cleared when the
contest settles or fails.

Frozen at opening, for docs/adr/047's reason: a contest is judged by the
terms it opened with, and *which seats are up* is the first of those terms.
Deriving the set again at resolution would let the answer change under the
electorate — a term end edited mid-ballot, a chair added — between the
question people were asked and the question the tally was applied to.

Storing it on the chair rather than in a join table keeps one fact in one
place: a chair is either in a contest or it is not, and there is no second
list that can disagree with the seats. It also makes the governance page's
per-seat answer (docs/adr/100) true on a staggered council, where the page
previously told all three chairs that an election was deciding them.

**Resolution touches those chairs and nobody else.** Winners fill the
contested chairs in order. A contested chair nobody won is emptied and stays
for the next contest. The only people who step down are those who held a
contested chair and were not returned — and even then, not if they hold
another chair the contest was not for, because the role follows the chairs.
An admin whose term has not run out is not on this ballot and keeps both.

**A chair in a running contest is not available by any other route.** It
cannot be removed (docs/adr/100's furniture control), its term end cannot be
moved, and `vacantSeat` skips it, so a ratified nomination or an interim
appointment cannot drop somebody into the chair people are voting on. Each of
those would settle the contest before it closed. The refusals are per chair:
a founder setting next year's dates on a seat nobody is voting about should
not have to wait out a contest she is not part of, which is the same
narrowing this whole decision is.

**A contest that opened before this column reconstructs its chairs**: the
seats whose terms end soonest, as many as the contest counts. That is not a
guess at intent — it is the set `scheduleFor` picked, in the order it picked
it — and an election in flight when a server upgrades has to resolve somehow.
No backfill migration, because the reconstruction is the honest statement of
what is known and a backfilled column would look like a recorded fact.

**The marking travels in a seamrip**, which moves `seats` after `proposals`
in the boundary (docs/adr/002): Import retries only within a table, so a chair
landing before the contest it points at would fail its foreign key on every
pass and be dropped. A fork taken mid-ballot has to empty exactly the chairs
the electorate was asked about, and on a staggered council that is not
derivable from the term ends alone.

## Consequences

- Staggering works end to end for the first time. It was a documented
  policy with a control and a calendar, and the last step — resolving a
  partial contest — undid it.
- docs/adr/102's "only when the whole council is empty" guard on advancing
  the calendar is no longer load-bearing. It stays, because that branch runs
  only on an empty council and moving every chair is right there, but the
  hazard it was avoiding is gone.
- A patch can now be in a state where one chair is being voted on and
  another is fillable by nomination today. Both sentences are on the
  governance page, each on its own row, which is what docs/adr/100's per-seat
  routes were built for. The one-line summary above the council still names
  the contest first, because that is the thing with a deadline.
- `seatWinners` no longer removes an admin who holds no chair at all. On an
  elected patch that state is already an anomaly docs/adr/100 prevents the
  role dropdown from creating; an election is not the right place to clean it
  up silently, and doing so was how a whole-council contest quietly doubled
  as a purge.
