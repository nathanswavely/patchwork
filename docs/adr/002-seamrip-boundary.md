# ADR 002: The seamrip boundary — community data travels, instance identity does not

Date: 2026-07-13. Status: accepted.

## Context

Seamrip (export/import) is the governance safety valve: if leadership goes
sideways, members fork the data to a new instance. The original export
covered 5 tables and the import dropped memberships entirely, so a fork lost
the shared-member overlap that threads and the quilt are inferred from —
the mechanism contradicted the mission it existed to protect. Three copies
of the export logic (admin zip, export CLI, import CLI) had drifted apart.

## Decision

One package, `internal/seamrip`, defines the portability boundary and all
three consumers use it.

**Travels (community data):** user profiles including **email and instance
role**, patches (all profile/governance columns), memberships, tags,
events, proposals with **raw votes**, governance docs, proposal comments /
reactions / revisions, claim requests (minus verification tokens),
notification preferences.

**Stays behind (instance identity & secrets):** credentials, sessions,
magic/invite links, ActivityPub keypairs and `ap_id`s, remote followers,
delivery queue, audit log, content reports, in-app notification rows,
reminder-dedup state. A fork mints its own federation identity on first
boot (`PopulateAPIds` / `BackfillKeypairs`).

Emails travel deliberately: without them, nobody can re-authenticate on the
fork and every imported account is orphaned. The export is admin-only, and
the instance operator already holds the SQLite file — the zip is a
convenience, not a privacy boundary. The README inside the archive says so.

Import rewrites every ID (old→new map saved to `id_map.json`), maps the
sentinel unclaimed-patch owner to itself, and preserves all relationships.

## Amendment, 2026-07-31: the boundary is now enforced, not just written

Three tables added by migration 050 (`seats`, `election_candidates`,
`election_ballots`) never reached this list, and the omission was silent
in the worst way. Election dueness is derived from `seats.term_ends_at`
rather than stored (docs/adr/051), so a fork arrived with no seats and
**never scheduled another election** — the safety valve stripping the
machinery that rotates leadership. The election proposals travelled, so
the record said a contest had happened and could not say what it decided.

Two more columns were missing for the same reason: `seats_contested`,
without which a proposal carrying candidates is not read as an election
by anything, and `voting_terms`, without which a fork's in-flight votes
finish under the *new* instance's rules rather than the ones people cast
ballots under (docs/adr/047). Event links and cross-quilt mentions
(docs/adr/032) were absent too; both are community data by the same
argument memberships are, and both now travel — confirmed links only,
since a fork cannot carry a handshake nobody finished.

The fix that matters is not the entries, it is
`TestEveryTableHasABoundaryDecision`: every table in the schema must be
either in `Tables()` or in an explicit stays-behind list with a reason.
A new table now fails the build until someone decides. "Community data
travels" was a sentence in this ADR that nothing checked, which is how it
went three releases without holding.

## Amendment, 2026-08-01: the boundary is checked per column, not per table

The table-level check added above catches a whole table nobody decided
about. It does not catch a *column*, and a column is how this keeps going
wrong: `seats_contested` reached the schema without reaching `Tables()`,
and a careless edit later dropped `website` and `links` out of the nodes
column list with nothing noticing, because the table was still listed.

`TestEveryColumnHasABoundaryDecision` now requires every column of every
exported table to be either in that table's `Columns` or in an explicit
stays-behind list with a reason. Writing that list found three columns
that had silently not been travelling:

- **`memberships.visible`** — the member's own visibility switch
  (docs/adr/006), defaulting to 1. A fork **re-exposed every membership
  somebody had chosen to hide**, on their profile and in the patch's
  public member list at once, since one switch drives both. This is the
  one that matters: a seamrip is what a community does when its
  leadership goes sideways, which is exactly when a member's choice to
  stay unlisted is most likely to be the thing they care about. The
  mechanism meant to protect people was quietly undoing one of their
  protections.
- **`nodes.accept_event_suggestions`** — an admin's choice about their
  own door (docs/adr/026). A patch that had closed it found it open on
  the fork.
- **`users.links`** — profile links, the same shape as a patch's, which
  did travel. A profile arrived with its bio and no way to reach anybody.

All three now travel, with `def()` fallbacks so archives written before
this import unchanged.

## Known gap

Git-backed governance repos (linings in `internal/governance`) do not
travel yet; the `governance_docs` table does. Repo transfer belongs to the
Phase C governance-federation work.
