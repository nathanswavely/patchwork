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

## Known gap

Git-backed governance repos (linings in `internal/governance`) do not
travel yet; the `governance_docs` table does. Repo transfer belongs to the
Phase C governance-federation work.
