# A record can be struck through, and names who stood down

**Status:** accepted, 2026-11-11. Extends docs/adr/052.

## Context

A leadership attestation records who a meeting put in place, and
`applyAttestation` makes that true: everyone it names who is here holds
admin, and any sitting admin it does not name steps down. Two things were
missing (F-078).

**Who came off.** The form had one people field, "Who it put in place".
The September meeting of a coalition also took Clyde Ferris out of the
chair, and that survived only as a clause in the free-text summary. The
effect was right, since he was not named, but the record could not say so.

**A record entered by mistake.** Omar pressed Record twice. The overview
showed the latest, the Record listed both, and there was no way to remove
either. "Correct the last one" writes a superseding record, so the honest
repair for a duplicate was a third entry. Omar: "Our minutes book has a
rule about crossing a thing out and initialling it. This has no rule at
all."

## Decision

**A recorded name says which side of the decision it is on.**
`attestation_names.effect` is `seated` (the default, and what every name
recorded before meant) or `stood_down`. The form gains "Who stood down",
optional and empty by default, with the same picker and free-text fallback
as the seated names. Only seated names carry a role. A stood-down name is
the record saying who came off, which the effect already brings about by
leaving them off the council, so being named there can never be what keeps
somebody on. One person on both sides of one decision is refused.

**An admin can mark a record entered in error. Nothing is deleted.**
`POST /api/v1/nodes/{slug}/attestations/{id}/strike` sets `struck_at` and
`struck_by`, the date and the initials. The record stays listed and on the
Record, crossed out, with who marked it. From then on it counts for nothing:

- it is never the record in force;
- a correction of it is refused, since it would mend a line already
  crossed out;
- a correction it made stops correcting, so the record it corrected is in
  force again and may be corrected afresh.

Where the struck record was the one in force, the record now in force is
applied, because who holds admin on an elsewhere patch is what that record
says. A duplicate changes nobody. A wrong record is undone back to the
decision before it. Where there is no decision before it, nobody's role
moves: no record is left to say who should hold it, and the way forward is
to record what the meeting actually decided.

Striking is admin-only and step-up gated, like recording, because striking
the record in force can move the admin role. It is refused on a patch that
chooses its admins in Patchwork, where these records no longer move anybody.

## Consequences

- Migration `20261111T225617_attestation_stood_down_and_struck` adds the
  three columns. All three travel in a seamrip, `struck_by` remapped like
  `recorded_by`, `effect` defaulting to `seated` and the struck columns to
  NULL for older archives, so a fork receives a struck record struck rather
  than live.
- Personal export carries `effect` on a person's own names, `struck_at` on
  records they made, and a new `attestations_struck` section for records
  they crossed out. Account deletion keeps the row (docs/adr/086) and the
  striker renders as "Deleted account".
- Audited as `attestation.strike`, naming the record now in force.
- Amendment attestations (docs/adr/053) are not struck by this; they
  replace a charter's text, and a mistaken one is mended by recording the
  right text.
