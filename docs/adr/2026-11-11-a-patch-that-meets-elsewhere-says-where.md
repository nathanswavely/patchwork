# A patch that meets elsewhere says where

**Status:** accepted, 2026-11-11. Extends docs/adr/052 and docs/adr/053.

## Context

A patch whose venue is `elsewhere` told every visitor, on its Governance
tab, that its proposals and its admins are decided "outside Patchwork", and
never where. The meeting existed only in the founder's own description of
the patch (F-034). The venue fields say whether a decision happens here; no
field said where it happens instead, so this needed a field before it
needed copy.

## Decision

**A patch that decides anything elsewhere has one short "where we decide"
text, `venue_note`, kept in its governance rules.** "First Tuesday of the
month, 7pm, the church hall."

- **In the rules, beside the venues.** It is a fact about how the patch
  governs, so it lives in `governance-rules.json` and the
  `governance_config` cache with the two venue fields, is edited in the
  rules editor directly under them, and appears in the readable rules and
  the rules diff. On an elsewhere patch a rules change is a direct change
  an admin applies (docs/adr/053), so that is how it is set.
- **Only where a venue is elsewhere.** The editor shows the field only then
  and clears it when both venues return to Patchwork. The server refuses a
  note on a patch that decides everything here, the field-nobody-reads
  failure of docs/adr/049.
- **Short.** 200 characters: a time and a place. Minutes and longer
  accounts are documents.
- **On the Governance tab, in place of the bare phrase.** "Proposals are
  decided where this patch meets: First Tuesday of the month, 7pm, the
  church hall." Where admins are chosen elsewhere too, the leadership line
  says "at the same meetings" rather than repeating the place.

## Consequences

- No migration: the rules file and its cache are JSON, and an absent key
  reads as no note.
- `GET /api/v1/nodes/{slug}/governance/rules` sends it, because the editor
  spreads what it loaded back into its submission.
- It is checked on both paths that write a rules file's text, proposal
  creation and revision.
