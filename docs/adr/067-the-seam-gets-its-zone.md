# ADR 067: The seam gets its zone

Date: 2026-08-26. Status: **accepted**. Implements ADR 045, which has sat
in "adopted as design boundaries — implementation is backlog" since it was
written. Supersedes ADR 065's config key and narrows its ingest rule.
Nothing in ADR 045's reasoning is revisited here; this records what
building it actually took and the two places reality argued back.

## Context

ADR 045 decided that an event's time belongs to the place it happens
rather than to its reader, and named `web/src/lib/datetime.js` as "the
seam that change lands on." The module said so about itself, in its own
header, for as long as the decision went unbuilt.

Three of the four bugs ADR 045 catalogued had been fixed independently in
the meantime — the edit form's double conversion became
`toLocalInputValue`/`fromLocalInputValue`, the emptied single-day filter
became `dayStart`/`dayEnd` emitting full instants, the week presets'
disagreement became `isoWeekday`. The fourth, floating and all-day feed
times read as UTC, was fixed by ADR 065 in the only way available at the
time: one zone for the whole instance, because there was nowhere else to
put one.

So what was left of ADR 045 was the thing itself. The zone.

## Decision

**1. ADR 045 as specified, with no amendments to its shape.** Migration
057 adds `events.timezone` and `nodes.timezone`, both nullable, NULL
meaning inherit. Resolution runs event → patch → instance → UTC at read
time, in SQL, so the API never emits a null zone and no client
reimplements the fallback. `starts_at` stays a UTC instant and stays the
sort key, the index, and the wire format — the alternative ADR 045
rejected on blast radius is still rejected, and for the same reason.

**2. The config key moves to where ADR 045 put it, and the old spelling
keeps working.** ADR 065 landed `instance.timezone`; ADR 045 specifies
`geographic.timezone`, beside the coordinates making the same claim, plus
an `instance_settings` override editable from the admin panel. The
geographic key now wins, the instance key is still read, and setting both
warns. The deprecation matters more than it looks: ADR 065 shipped days
ago and an operator may already have deployed it, so removing the key
outright would have moved every event on a live quilt back by its offset
at the exact moment they upgraded to the fix for that.

**3. Ingest asks the patch, not the process.** ADR 065 read zoneless feed
times in a single instance-wide zone. That was the right answer for an
instance-wide setting and the wrong one for this model: a venue's own
calendar publishing floating times is publishing them in *its* zone, and
an instance can hold a Lancaster venue and a Portland one. The zone is
now resolved per source, from the patch that attached the feed, and
carried on the `Source` rather than read from a package global — so the
same `DTSTART:20260904T190000` in two feeds becomes two different
instants in one process. There is a test that fails if it does not.

An aggregator is the exception and belongs to no patch: a city calendar
feeding many venues is read in the instance's zone, once, and the
crosswalk entries inherit that reading rather than each reinterpreting it.

**4. Annotation is conditional, and "same zone" means the same clock.**
ADR 045 says show the zone abbreviation only when the event's zone differs
from the viewer's, because annotating everything teaches the reader to
ignore the annotation. Implementing that surfaced a case the ADR did not
name: `America/New_York` and `America/Detroit` are different zone names
that never disagree, so comparing names alone would stamp "EDT" on every
Lancaster event for a reader in Detroit — noise in exactly the place it
claims to be signal. So the comparison falls back to asking whether the
two zones render this instant identically.

**5. The form is written in the event's zone.** An organizer editing a
Lancaster show sees 8:00 PM in the box whether they are in Lancaster or on
tour, because 8pm is the fact being edited. `toZonedInputValue` and
`fromZonedInputValue` are the zoned twins of the existing local pair — the
pairing rule ADR 045's first bug was about is unchanged and now has a zone
in it. The conversion resolves the offset twice: the first pass is wrong
by at most an hour, the second settles it, and one pass alone picks the
wrong side of a DST boundary for evening events on exactly the two nights
a year when that is most likely to matter.

## Consequences

A quilt that configures nothing keeps UTC and keeps today's rendering,
with a startup warning. Setting `geographic.timezone` renders the whole
existing corpus correctly without rewriting a row, because the stored
instants were always right — it was the reconstruction that was guessing.

Both zone columns travel in a seamrip. The boundary test caught them
before a human did, which is the check that exists because migration 050's
`seats` silently didn't travel; a fork that lost its zones would keep the
encoding and lose the fact.

The zone control on an event is opt-in behind a link rather than a field
on the form. ADR 045's rule — ask only where it differs from the patch —
needs the patch's own zone to compare against, and an event payload's zone
is already resolved, so an inheriting event and one pinning its patch's
zone are indistinguishable in it. The form fetches the patch to tell them
apart. Getting that wrong the first time would have made every ordinary
save freeze a copy of a zone the event had been inheriting, which is the
failure mode ADR 045's "inherited, never asked for" exists to prevent.

Two things ADR 045 named as out of scope stay out of scope, unchanged.
**All-day events are a distinct kind, not a time**: a `VALUE=DATE` feed
entry has no clock, and midnight-in-a-zone is a second lie layered on the
first. It still wants an `all_day` flag and its own ADR. And `starts_at`
still holds **two incompatible precisions**, `.000Z` from the browser
against zero-fraction from feed ingest, which still breaks the exact-match
dedupe key in `event_upload.go`. Neither is a timezone decision and
neither got quietly bundled in here.

ActivityPub is unchanged: `startTime` stays a spec-correct UTC instant,
because AP has no standard property for an event's zone. The
Patchwork-to-Patchwork JSON read path carries `timezone` directly, so the
cross-quilt case that motivated ADR 045's rejection of a single
instance-wide zone — My Quilt's merged feed — is served exactly.
