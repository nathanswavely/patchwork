# A council changing size is recorded, not announced

**Status:** accepted, 2026-11-11. Amends docs/adr/100's notification and
extends docs/adr/055's Record.

## Context

docs/adr/100 made a council's size its seats and gave an admin two acts,
add a vacant chair and remove one. Each press sent `governance.rules_changed`
to every member at once. Priya pressed Add twice and sent fourteen
notifications; Nell added a seat and removed it ten seconds later and sent
six, for a council that ended the day as it started. None of it carried an
obligation for anybody who received it.

And none of it reached the Record. `seat.add` and `seat.remove` were written
to the audit log, which only an instance admin can read. Ana went looking
for the council changing size and found it only in her own bell, "which is
mine alone and will scroll away" (F-066).

That is backwards: a broadcast for something nobody has to act on, and no
record of something the community may want to look up.

## Decision

**Adding or removing a seat notifies nobody, and is an entry on the
governance Record.**

- `AddSeat` and `RemoveSeat` stop calling `announceCouncil`. Their audit
  rows now carry `seats`, the council's size after the act, so the entry
  can say what the council became without recounting a table that has moved
  on since.
- `councilFurniture` reads those rows into the Record beside
  `councilChanges` (docs/adr/055's assemble-don't-store rule, and the same
  reasoning F-100 used for the audit rows behind a vacated seat): "A seat
  was added. The council went to 3 seats. By Priya." Outside the room the
  actor is withheld, as for the other council entries.
- **A term end moving still notifies.** `SetSeatTerm` keeps
  `announceCouncil`, because a seat's date is the election calendar and
  changes when a member can stand. That is an obligation in the sense the
  furniture is not.

## Consequences

- Rows written before this change carry no `seats`, so their entries say
  who added or removed a seat without a count.
- Nothing migrates and no table is added: the audit rows were already the
  authority.
