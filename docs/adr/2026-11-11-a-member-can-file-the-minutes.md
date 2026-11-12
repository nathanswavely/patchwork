# A member can file the minutes

**Status:** accepted, 2026-11-11. Amends the creation rule of docs/adr/036
for patches that decide elsewhere; leaves docs/adr/052, docs/adr/053 and
docs/adr/017 standing.

## Context

A patch whose decisions happen at a meeting keeps minutes, and Governance →
Documents already says that is where they go. On such a patch the minutes
are taken by whoever keeps them, and that is often not an admin. Harriet is
a coalition's secretary and a member. Its page said "the minutes are posted
here afterwards", and she looked everywhere for the "here" and was the one
person in the room who could not write them (F-057). `CreateGovernanceDoc`
accepted a patch admin and nobody else.

Two neighbouring acts are admin-only for reasons that do not apply to
filing a document. Publishing a document puts it in front of the whole
internet and is a deliberate act (docs/adr/036). Recording a leadership
decision moves who holds admin, and is step-up gated (docs/adr/017,
docs/adr/052).

## Decision

**On a patch that decides anything outside Patchwork, any active member may
add a governance document.** "Anything" is either venue: where its admins
are chosen, or where its proposals are decided. The predicate is
`meetsElsewhere`.

- **Born members-only, and stays so.** A member's document starts
  `visibility = 'members'`, and a member asking for `public` is refused
  with the reason. The editor does not offer them the choice. Publishing
  remains an admin's act at Documents.
- **Not step-up gated.** Filing a document changes nobody's role and no
  rule, which is what docs/adr/017 gates. Recording a leadership decision
  and recording an adopted text keep their admin-only, step-up-gated
  routes.
- **Nowhere else.** On a patch that decides everything in Patchwork, the
  Documents shelf stays its admins', as before.
- The Documents page tells a member of such a patch, in one sentence, that
  they can add documents such as minutes and that an admin decides whether
  to publish one.

## Consequences

- Editing stays as it was. A member who filed minutes changes them the way
  any member changes a document, by proposing an amendment; a direct edit
  is an admin's. Whether the author of a members-only document may correct
  their own typos is a follow-up, not decided here.
- Every member is told of a new document, as for an admin's, through the
  existing governance-doc notification.
- The instance-admin reach in `CreateGovernanceDoc` (docs/adr/115, issue
  #286) is unchanged and still counted once.
