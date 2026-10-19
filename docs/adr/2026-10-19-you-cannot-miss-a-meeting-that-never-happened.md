# You cannot miss a meeting that never happened

Date: 2026-10-19. Status: **accepted**; implemented. Amends docs/adr/102
(how an admin's absence is measured) and docs/adr/051 (the inactivity rule
that gives `succession_policy` its teeth). Keeps docs/adr/102's asymmetry:
a seat is lost by not governing and offered to somebody who is here.

## Context

The Allfornauts is a band on the reference instance. It keeps its calendar
in Patchwork and has never raised a proposal, because a band does not vote
on anything. Its admin got this in the bell:

> Your admin seat in The Allfornauts is inactive. You have not taken part
> in governance here for 90 days. Vote, propose, or comment to keep the
> seat; otherwise it is declared vacant at 180 days.

Nothing in that message was false, and acting on it was impossible: there was
nothing to vote on or comment on, and raising a proposal to save the seat is
governance theatre. At 180 days every admin the band had would have been
vacated together, and the succession policy would have found nobody "present"
to promote, because everyone present was an admin it had just removed. The
instance admins would then have been told the band had no admins left.

docs/adr/102 saw this coming in one sentence ("A venue that runs events
every week and holds no votes reads as entirely inactive") and left it. The
Minimal template, the one a band should be on, ships `inactivity_days: 0`, so
a band born on it is never swept. But migration 041 backfilled 90 onto almost
every patch that predates it, and nothing stops a band or a venue from
choosing Casual. Every patch that uses Patchwork for its calendar and not for
its decisions is on the same path.

Two fixes were proposed and turned down.

- **An "I'm still here" button.** Clicking it proves an account, not
  governance. It also makes the warning a chore for every events-only patch,
  every 90 days, for ever: the same notice with a button on it.
- **Only sweep patches above a size.** Member count stands in for the real
  question, and gets it wrong both ways. A four-person co-op that votes is
  where one admin who has disappeared hurts most. A thirty-member venue that
  never votes should never get this.

## Decision

**Absence is counted from the first proposal an admin let pass**, not from
their last act. Concretely, for each admin:

1. `since` is the latest of their last governance act, when they joined,
   and when they took the role (docs/adr/102's three facts, unchanged).
2. The **occasion** is the earliest proposal on the patch raised strictly
   after `since`, and not withdrawn.
3. No occasion means no absence. Otherwise they are warned once the occasion
   is `inactivity_days` old and vacated at twice that.

"Strictly after" carries three rules without any extra code. An admin's own
proposal is an act, so it is never one they missed. A proposal raised before
they joined or took the seat is not one they could have answered, and
counting it would put every newcomer on a clock from arrival, which is the
fault migration 072 fixed. And any act moves `since` past everything before
it, so taking part in one proposal answers for the ones raised alongside it.

**A withdrawn proposal is no occasion.** Its author took it back, and a
proposal raised by mistake and pulled a minute later must not start a clock
that runs for months.

**What counts as taking part is a little wider**, because the occasion makes
the old gaps matter more. The list is `governanceActs` in
`internal/notifications/inactivity.go`, shared by the sweep and by interim
promotion's presence test so the two cannot drift:

- votes, proposals, and proposal comments, as before;
- **election ballots**, which live in their own table (docs/adr/051) and were
  never counted, so an admin who voted in a contest and nothing else read as
  absent;
- **deciding a proposal** on an admin-decides patch (docs/adr/092), whether
  by applying it (`applied_by`, `applied_at`) or declining it (`declined_by`,
  `updated_at`). It is the act that proposal was waiting for. Without it, a
  maintainer who answers every member by deciding would be absent from
  exactly the occasions they answered.

Standing as a candidate is left out on purpose. A candidacy row can be put
there by somebody else (docs/adr/107), so it is no evidence that its subject
was here.

**The warning names the proposal and opens it.** The bell now reads "You have
not voted, proposed, or commented here since this proposal was raised N days
ago: *title*. Take part in governance here to keep the seat; otherwise it is
declared vacant in M days," and links to the proposal rather than to the
governance hub. F-115's rule holds: it states the day, worked out from the
proposal's age rather than assumed. A patch that turns the rule on over an old
proposal can find it well past the warning day, and saying "90 days" there
would be wrong.

The governance overview, the readable-rules list, and the Formal and
Collaborative charter templates say the same thing: counted from a proposal,
and a patch with no proposals warns nobody.

## Consequences

- **An events-only patch is never swept**, at any size, on any template, with
  any `inactivity_days`. The Allfornauts needs no change to its rules.
- **The stranded patch is still caught.** The case docs/adr/051 built the
  sweep for (the only admin vanished and the patch cannot move) shows up as a
  member's proposal that nobody answers. That is a better signal than elapsed
  time: somebody needed governing and nobody came.
- **A quiet patch keeps its interims.** In docs/adr/102's quiet-year
  simulation the interim admins lapsed on the same clock and the council
  emptied a second time. Now they were seated after the only proposal there
  was, nothing came up after, and they keep the seats. The role still moves a
  bounded number of times; it now also stops moving.
- **A proposal already open when somebody takes the seat starts no clock for
  them; the next one does.** That is generous, and deliberately so: the
  alternative counts proposals that closed before the person arrived.
- **The member who raised the occasion may not be eligible to inherit.**
  Presence for interim promotion still uses `2 × inactivity_days`, and the
  admin is vacated when the occasion reaches the same age. A member whose
  only act was raising that proposal is therefore just outside the window on
  the day the seat goes. They qualify if they did anything else since: posted
  an event, wrote a notice, voted. Widening the presence window to cover the
  asker is a separate decision and is not made here.
- **Notices already sent stay sent.** Admins warned under the old rule keep
  their `inactivity_warning` row until the next sweep finds no missed
  occasion for them, which clears it. A warned admin on an events-only patch
  is therefore cleared on the first pass after upgrade, and no seat is
  vacated on the strength of the old measurement.
