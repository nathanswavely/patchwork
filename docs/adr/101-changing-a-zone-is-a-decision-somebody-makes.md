# 101. Changing a zone is a decision somebody makes

**Status:** accepted, 2026-09-15

## Context

June runs a choir and books every gig it plays. In one sitting in the
governance simulation she entered four rehearsals at 7pm, then set her
patch's timezone, and the page quietly began saying 2pm. Her words:
*"Changing the timezone moved every rehearsal five hours earlier and
didn't say a word. I typed 7pm four times and the page quietly started
saying 2pm. Warn me, or don't move them."*

Nothing had moved. docs/adr/067 separates an event's stored instant from
its wall-clock reading, and a patch's zone is what turns one into the
other, so changing the zone changed every reading and no row. That is
correct machinery and it is not what a person means. Which of the two
she wanted is genuinely ambiguous:

- A community that set its zone *late*, having entered local times all
  along, means "these were always 7pm, you were reading them wrong" —
  keep the clock, move the instants.
- A community that *moves*, or corrects a zone it had right for the
  events it entered, means "those moments were the moments" — keep the
  instants, let the clock read differently.

Both are ordinary. Neither is safely a default: guessing wrong sends
twenty people to a hall at the wrong hour, and the product has no way to
know which kind of change this is.

The same sitting turned up a second, quieter version of the problem.
Both zone validators accepted `EST`, because `time.LoadLocation` resolves
tzdata's fixed-offset compatibility entries without complaint, and the
browser accepted it too. `EST` is not a place. It is an hour wrong from
March to November, which is when a choir rehearses.

## Decision

**A zone change that would alter any reading is refused until somebody
says which they meant.** `PATCH /api/v1/nodes/{slug}` answers 409 with
the number of events affected, both zone names, and the two choices; the
caller re-sends naming one, and the success response reports what was
done and to how many. The change is audited on its own.

Refusing is the point. A confirmation dialog that defaults to one answer
teaches people to click through it, and the two answers here are equally
legitimate, so the product has no business preferring one. Where there is
nothing to decide — two zones that agree on the clock, or a patch with no
events reading in its zone — no question is asked at all.

**Two kinds of event are never re-anchored**: an event that pins its own
zone, because it already said where it is, and an imported one, because
a feed's instant is the feed's fact (docs/adr/031) and the next sync
would overwrite the guess anyway. Neither counts toward the question.

**A timezone must name a place.** One rule, in `internal/config`, which
every write path and the startup config check delegate to: area/location
names plus `UTC`. `EST`, `CET`, `GMT`, `Local` and the `Etc/GMT±N` family
are refused with a message that says what to type instead, and the
browser mirrors it rather than trusting `Intl`. docs/adr/045 said an
event's time belongs to its place; a fixed offset is not a place, it is
half of one place's year.

**And the product stops offering recurrence it does not run.** "Repeats
weekly" was stored, displayed, and expanded nowhere — not into
occurrences, not into the ICS feed. docs/adr/049 says Patchwork states
only what it enforces, so the control is withdrawn rather than faked.
Expansion is not a feature but a data model: occurrences or a virtual
expander, an RRULE in the feed, an exception model, a diff in the ADR 031
reconciler that keys imported occurrences by start instant, and an answer
for every list, count and reminder about which Tuesday it means. Four
events is four events, and a community that keeps a series elsewhere
attaches it as an event source, where an RRULE *is* expanded, in the
calendar's own zone. Rows that already carry the word keep it and render
it as the organizer's claim, attributed, with the caveat that only this
date is on the calendar.

## Consequences

- A zone change is now two requests. Every caller must handle the 409,
  and a client that ignores it silently stops changing zones — which is
  the failure we want, rather than the one we had.
- **The instance-wide zone still has this defect at a wider radius.**
  `PATCH /api/v1/admin/settings` changes the reading of every inheriting
  event on every patch with no consent step and no count. It refuses
  non-place names now, but the consent flow is not wired to it, because
  that needs a per-patch fan-out and an admin-panel flow. This is the
  clearest follow-up and it is written down here so it is not forgotten.
- Withdrawing recurrence is a visible loss for whoever wanted it, and the
  honest answer — "add each date, or attach the calendar" — is worse for
  a weekly rehearsal than the word they were offered. If that proves too
  thin, expansion is its own ADR and its own piece of work, with the
  reconciler in front of it.
- `Etc/UTC` and `UTC` remain legal, because an instance that genuinely
  keeps no local time is a real deployment, not a mistake.

## Amendment, 2026-09-15: the follow-up is done

The second consequence above named the instance-wide zone as the clearest
follow-up and said the consent flow was not wired to it. It is now:
docs/adr/105 gives `PATCH /api/v1/admin/settings` the same 409, the same two
answers, and a count that names the patches as well as the events — and only
what actually inherits, so a patch that named its own zone is untouched. The
admin panel's own zone box also stopped trusting `Intl`, which this ADR asked
for and only the patch form had done.

The consequence text above stays as it was written. What was known when is the
point of keeping these.
