# 105. Moving the quilt's clock asks the same question

**Status:** accepted, 2026-09-15

## Context

docs/adr/101 made a patch's timezone change ask which of two things the
person meant, because moving a zone leaves every instant where it is and
changes every reading — four rehearsals typed as 7pm start saying 2pm, in
silence. It then wrote down, in its own consequences, the thing it had not
done:

> **The instance-wide zone still has this defect at a wider radius.**
> `PATCH /api/v1/admin/settings` changes the reading of every inheriting
> event on every patch with no consent step and no count. It refuses
> non-place names now, but the consent flow is not wired to it, because
> that needs a per-patch fan-out and an admin-panel flow. This is the
> clearest follow-up and it is written down here so it is not forgotten.

The quilt's zone is the last rung of docs/adr/045's chain: an event falls
through to its patch, and a patch falls through to the quilt. So the reading
of every event that never named a zone rests on this one setting, and a
single text box in the admin panel could move a whole instance's calendars —
across communities whose admins are not in the room and are not asked.

It is not a rare edit. Getting the zone wrong at deploy shows up as every
event being hours off, which is exactly when an instance admin opens that box,
after events already exist.

## Decision

**The same question, one rung up.** `PATCH /api/v1/admin/settings` answers
409 `timezone_events_undecided` when the change would alter any reading, and
the caller re-sends with `timezone_events` set to `keep_clock` or
`keep_instant`. The same two answers, the same words for them, the same
refusal to prefer one, and the same silence where there is nothing to decide:
two zones that agree on every affected calendar, or nothing inheriting, is not
a question, and asking one with no consequences is how people learn to click
through the ones that have them.

**The count names patches as well as events.** "134 events across 9 patches"
is a sentence an instance admin can weigh; "134 events" is not. The radius is
the whole difference between this decision and docs/adr/101, so it is the
thing the refusal says.

**Only what inherits, at both rungs.** A patch that named its own zone is not
in the count and is not touched, and neither is an event that named its own,
nor one from a calendar feed (docs/adr/031 — the feed's instant is the feed's
fact, and the next sync would overwrite the rewrite anyway). That is what
makes this safe to offer at all: an instance setting cannot move the calendar
of a community that has said where it keeps time. The panel says so, in the
same box, because an admin weighing the choice needs to know whose calendars
are *not* in it.

**Archived patches count.** Their events keep their rows and can come back,
and re-anchoring is about a clock reading staying true rather than about who
can currently see it. Leaving them out would mean a restored patch's calendar
had silently shifted while it was away.

**The zone is written first, then the instants**, exactly as the patch path
does it and for the same reason: of the two half-states, a zone saved with its
events unmoved is one somebody can still finish, and events re-anchored to a
zone the quilt does not keep is a calendar nobody can reason about. The whole
change is audited as `admin.instance_timezone_changed` with both zones, the
mode, and all three counts.

**`settings.EffectiveTimezoneWith`** answers what the effective zone *would*
be for a given override, because the count of affected events is the
difference between the zone now and the zone after, and the second is not
readable anywhere until it is too late to refuse.

**And the admin box stops trusting `Intl`.** docs/adr/101 said the browser
should mirror the place rule rather than trust `Intl.DateTimeFormat`, which
resolves `EST` happily; the patch settings form was changed and the admin
panel was not. It uses the shared `isPlaceZone` now, so the two forms refuse
the same names for the same reason the server does.

## Consequences

- Changing the quilt's zone is now two requests, like a patch's. A client
  that ignores the 409 silently stops changing zones, which is the failure
  we want.
- An instance admin can still move every inheriting calendar on the quilt.
  This does not take that power away — it makes them see the size of it and
  say which of two things they meant. The people whose events move are still
  not asked; the honest fix for that is beyond a settings box, and a patch
  that does not want to be moved can name its own zone.
- docs/adr/101's follow-up is closed. Its consequence text stays as written,
  with an amendment pointing here, because the record of what was known when
  is the point of keeping these.
- The two consent flows are now two implementations of one idea, sharing the
  planner's helpers and the zone vocabulary but not their copy. If a third
  rung ever appears, they should be one flow rather than three.
