# 102. To lose a seat you stop governing; to be handed one you are here

**Status:** accepted, 2026-09-15

## Context

The governance simulation (docs/adr/096) let a five-patch world sit for
320 days with nobody acting — a community that has a quiet year, which is
ordinary. It produced **1,348 `membership.succession` rows and 1,347
`membership.seat_vacated` rows**. Every founder had been stripped of
admin and replaced by whoever joined earliest. The chair of the co-op was
a plain member of the patch she created, and the `seats` table still
recorded her holding a chair.

Four faults, compounding.

**The absence clock never restarted.** The sweep measured an admin's
absence as `MAX(last governance act, joined_at)`. Somebody promoted to
fill another person's absence carries a `joined_at` that is already older
than the vacate threshold, so they were inactive the instant they were
appointed, vacated on the next pass, and the role walked through the
whole membership for ever, ringing a bell at every step.

**Promotion read "active" as a status.** The shipped succession plan says
the seats pass to "the three longest-tenured **active** members".
`status = 'active'` is a membership state, not presence, so the patch was
handed to people last seen a year ago — who then lapsed on the same
clock. Fixing only the first fault slows the loop to sixty-day rounds; it
does not stop it.

**Vacating an admin left their chair held.** The role changed and the
`seats` row did not, so the council and the admin roll disagreed — the
divergence docs/adr/100 exists to prevent, reached through another door.

**Succession ignored the leadership model.** On an `elected` patch the
sweep promoted the longest-tenured member directly: the one act
docs/adr/100 forbids the role dropdown, and the thing docs/adr/051 says
must be nomination plus ratification. The election the patch's own page
promises was silently outranked by a tenure rule.

None of this needs an unusual patch. Activity counts votes, proposals and
proposal comments; posting events is deliberately not participation
(a seat on the council goes quiet, not an account). A venue that runs
events every week and holds no votes reads as entirely inactive, and the
Formal template vacates after sixty days of that.

## Decision

**An absence is measured from the later of three facts**: the person's
last governance act, when they joined, and **when they took the role**.
`memberships.role_since` (migration 072, nullable, no backfill — NULL
reads as `joined_at`, which is the floor those rows already had) is
stamped by every write that moves somebody into or out of a role. The
audit log was the obvious alternative and is the wrong one: its entries
key on different entities, they are append-only narrative rather than a
column anything joins, and they are prunable. Whose seat is safe must not
change after housekeeping.

**To be handed a seat you must still be here.** Interim promotion now
reads "active" as presence — a governance act, an event posted, a notice
or reply written, or joining — inside the same window that costs an admin
their seat, ordered by tenure among those present. Every interim admin
is, on the day they are appointed, somebody the sweep would not vacate,
so the loop terminates by construction rather than by a counter.

The asymmetry is the decision: **to lose a seat you must stop governing;
to be handed one you must still be here.** Governing is a narrower act
than being present, and that is right in both directions. A council
member who stops governing has stopped doing the job the seat is for. A
member who turns up, posts, and answers is somebody the patch can lean on
in an emergency, whether or not they have ever voted.

**Emptying a chair empties the chair.** The seat stays and its holder
leaves, which is docs/adr/051's rule that a seat outlives its holder.

**Filling follows the model**, because the model is what the patch's page
promises:

| Model | What happens |
|---|---|
| venue `elsewhere` | Vacate, record, appoint nobody — Patchwork conducts nothing (docs/adr/052) |
| `elected` here | Chairs empty and stay; the council's term ends move to today so the calendar opens the contest on its next pass. Nobody is promoted |
| `meritocratic` | Nobody promoted — it ratifies. The instance admins are told, because a nomination needs an admin to raise it |
| `maintainer` / none | The designated successor first, else the configured `succession_policy` |

Advancing the calendar happens **only when the whole council is empty**,
because a contest for part of a staggered council would unseat
colleagues the electorate never voted on (F-086, filed and not yet
fixed).

**A patch with no admins is not sealed.** Three layers. An elected patch
elects its way back, with no sitting admin required to start it. The
product says out loud, in a notification and on the governance page, that
nobody holds the role and what refills it — worded per model. And
`UpdateMember`'s three model gates lift while a patch has zero active
admins: each of those gates exists to protect a mechanism that is
*started by an admin*, so with none left the gate stops protecting
anything and starts sealing the door. Only an instance admin can reach it
in that state, the restored admin is seated in a vacant chair keeping its
term, the audit entry says `"reason":"council_empty"`, and the gate closes
again the moment somebody holds the role.

**Interim promotion does not ask for consent, and invitation does.**
docs/adr/098's consent rule is about the community's *record*: adding
someone puts them into it without their say. An interim admin is already
in that record — they joined. What changes is a permission, not a
relationship, and the `invited` status means "not a membership yet", so
reusing it on an active row would destroy a membership in order to ask
about a role. The deciding argument is cost: this is a bus-factor rule
for a patch that has already lost its council, it now fires only at
people established to be present, and requiring an affirmative act means
that if nobody answers, nothing happens — with no calendar to retry. The
exit is real, since demotion is never gated by model and an interim admin
can step down through the ordinary control.

## Consequences

- **This is a live-instance risk, not a theoretical one.** Any patch
  whose admins do not vote, propose or comment for `2 × inactivity_days`
  is affected — sixty days on Formal, and migration 041 backfilled
  `inactivity_days: 90` onto nearly every pre-existing patch. An
  instance that has been up for two months should be checked for
  `membership.succession` rows before this is assumed to be caught in
  time.
- `role_since` travels in the seamrip boundary. Without it every admin on
  a fork would arrive with their clock reset to joining, which is the
  original bug wearing a different hat.
- A patch can now legitimately have no admins for a while, and the
  product has to keep saying so rather than looking broken. That is the
  honest state: docs/adr/051 accepted that inactivity may empty a patch,
  and this is what that looks like when it happens.
- Reading "active" as presence makes a member who posts events eligible
  to be an interim admin though the same activity would not have saved
  their seat. Stated plainly above because it looks like an
  inconsistency and is a deliberate asymmetry.
