# A patch is asked its timezone when it is made

**Status:** accepted, 2026-11-11

## Context

An event's time belongs to its place (docs/adr/045), and a patch's place
keeps a clock: `nodes.timezone`, falling back to the quilt's zone, falling
back to UTC. Patch Settings has asked for that zone since F-074, and asks
what should happen to existing events when it changes (docs/adr/101).

Creation never asked. A patch was born following the quilt, and on a fresh
instance the quilt has no zone of its own, so the first event a founder
posted landed in UTC. In the governance simulation a 2pm workshop read
"2:00 PM UTC" to everyone who opened it, and the person who posted it said
the form had picked the one zone nobody lives in (F-024). The fix afterwards
is real, but it is a fix to something the form could have asked once.

## Decision

**The create form asks for the timezone, preselected to this device's.**
`Intl.DateTimeFormat().resolvedOptions().timeZone` is where the person
making the patch is standing, which is usually where the patch is. The
quilt's own zone is offered beside it when it differs, so a founder making a
patch for somewhere else picks the quilt's in one choice, and "Another
timezone" takes any place name. A device zone that is not a place (a fixed
offset like EST, refused everywhere since F-074) is not offered.

**The server takes `timezone` on `POST /api/v1/nodes`** with the same check
and the same correction Patch Settings gives. Empty or absent stores NULL,
which still means "follow the quilt", so an API client that sends nothing
gets exactly what it got before.

**A chosen zone is stored, even when it equals the quilt's.** Picking a zone
is saying where the patch is. If the quilt later moves its clock
(docs/adr/105), a patch that said it is in New York should stay in New York.

**Creation only, not claim setup.** A listing can already carry events,
imported or suggested, and naming its zone at setup would change what time
they read as. Patch Settings is where that question is asked properly, with
the count and the choice docs/adr/101 requires, and a claimant reaches it
one step after setup.

## Consequences

- A patch made in a browser starts in a real place. The UTC fallback remains
  the last rung for API clients and imports, as docs/adr/045 intends.
- A founder whose device is set to a zone other than the patch's has to
  change one select. That is the cost of preselecting anything, and the
  alternative, starting blank, is the failure this replaces.
