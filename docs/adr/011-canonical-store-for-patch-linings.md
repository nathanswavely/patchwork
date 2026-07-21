# ADR 011: The canonical store for a patch's lining — DB, with git as a derived history mirror

Date: 2026-07-13. Status: accepted (implemented same day).

## Context

A patch's lining (governance doc) exists in two stores that were built at
different times and never fully reconciled, and it is currently undefined
which one is authoritative. On `CreateNode` the backend does two
independent things:

1. `CreateDefaultLining` inserts a row into the `governance_docs` DB table,
   title **"Community Lining"**, body = the short `handler.DefaultLiningBody`
   paragraph. The governance hub (`ListGovernanceDocs`, the only lining read
   path the UI has) reads exclusively from this table.
2. The governance repo fork writes **`community-standards.md`**, body = the
   long `governance.communityStandards` markdown. This git file is what
   amendment proposals target (`proposals.target_doc = "community-standards.md"`)
   and what version-history/diff views read.

These are not two copies of one document — they have different titles,
different bodies, and, critically, different filenames, so nothing links
them:

- A **direct edit** (`PUT /governance/{id}`) writes the DB row and mirrors
  to git under a filename *derived from the title*
  (`governanceFilename("Community Lining")` → `community-lining.md`) — a
  third name, never `community-standards.md`.
- An **amendment** (`ApplyProposal` → `MergeBranch`) writes only git
  `community-standards.md`. It syncs back to the DB solely for
  `governance-rules.json` (via `SyncRulesToDB`); a lining amendment never
  updates `governance_docs`, so it is invisible in the hub.

Net effect for the default lining: the hub shows a short "Community Lining"
that amendments can't reach, while the amendable "Community Standards"
lives only in git and never appears in the hub. Neither reads as
authoritative, and the drift is silent.

The dev log previously recorded this as "two drifting copies… still says
Moderators." The "Moderators" text is long gone (both sources say
"admins"); the real, unresolved issue is *which store is canonical*, which
is what this ADR settles.

## Decision

**The `governance_docs` DB table is the canonical store for a patch's
lining. Git is a derived, append-only mirror kept for version history and
diffs — never a second source of truth.**

Concretely, this decision entails:

- **Amendment apply writes back to the DB.** Symmetric to the existing
  DB→git mirror on direct edit, `ApplyProposal` for a lining target must
  update the `governance_docs` row (and bump its version) from the merged
  content, so the hub reflects what the community voted in. Today only
  rules changes sync back; linings must too.
- **One default lining, one identity.** `CreateDefaultLining` and the git
  fork must produce a single logical document — same title, same
  slugified filename, same body — so the DB row and its git file are two
  representations of one thing, not two documents. (Pick one: either the
  short paragraph or the long markdown, in one constant, used by both.)
- **Reads never depend on git.** The hub, profiles, and seamrip continue
  to read the DB. Git is consulted only for history/diff views.

## Considered options

- **DB canonical, git as derived mirror (chosen).** Aligns with ADR 002's
  portability boundary — `governance_docs` travels in a seamrip, git repos
  deliberately do not — so a forked community keeps a coherent, current
  lining. Matches every existing read path and the seamrip export. Cost:
  the amendment-apply flow must gain a git→DB write-back for linings.
- **Git canonical, DB as a read cache.** Makes branches/diffs/history
  first-class and fits the amendment flow, which is already git-native.
  Rejected: it contradicts ADR 002 (git doesn't travel, so a fork would
  lose its canonical lining and all history), forces the hub and profiles
  to read from git (slower, requires a live repo per patch, awkward for
  federation/seamrip), and makes the DB — which every other feature reads
  — the untrustworthy copy.
- **Keep both, add a two-way bridge.** Unify the filename/title mapping and
  have every writer update both stores. Rejected: two authoritative
  writers is exactly the drift this ADR exists to end; the bug above is
  what that architecture produces in practice.

## Consequences

- One code change with teeth: `ApplyProposal` must sync a merged lining
  back into `governance_docs`. Until it does, applied lining amendments
  keep not showing in the hub.
- The default-lining constants must be unified into one; the current
  short-DB / long-git split goes away. Decide which text survives (the long
  markdown is richer and is what amendments already target).
- Existing patches seeded or created before the fix may have the split
  (short DB "Community Lining" + orphan git "Community Standards"); a small
  migration or backfill may be needed, or accept it for pre-launch data
  since the seed is fiction (ADR 009) and re-seeds cleanly.
- ADR 002's boundary is reaffirmed, not changed: memberships and
  `governance_docs` travel; git identity and history stay.
