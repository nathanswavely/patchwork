# ADR 010: One seed dataset — the music profile is grafted in, not maintained in parallel

Date: 2026-07-13. Status: accepted.

## Context

The seeder carried two alternative profiles: the default `arts` dataset
(what E2E and `make seed` load) and a parallel `music` dataset behind
`-profile music` (originally imagined as a
second themed instance for multi-quilt demos). In practice nothing
automated ever loaded the music profile — its only consumers were one
README line and occasional manual runs — while every seed-wide decision
(most recently the ADR 009 fictionalization) had to be applied twice.
Worse, the structural coverage that justified it lived in the wrong
place: the music profile had the only invite-only patches, so the E2E
test "join invite-only patch returns forbidden" was a no-op asserting
`201 or 409` against an open patch.

## Decision

One seed dataset. The music profile's *structure* was grafted into the
default seed — four invite-only, all-admin bands (Mill 72, Static Season,
Chestnut Hollow, Conestoga Drift) rostered from existing users, plus
curated `extraMemberships` so the quilt's threads connect bands to the
studios and co-ops their members already belong to — and the
`-profile` flag, the second dataset, and the profile indirection were
deleted. The E2E invite-only test now asserts a real 403 against
`static-season`.

Two persona-driven additions ride along:

- The role-ladder dev cookies (`dev-admin-token` … `dev-new-token`) are
  documented in the README, not just seeder stdout, so an evaluating
  organizer can sit in every chair without reading Go.
- The seeder refuses to run against a database that has users but no
  seed marker (`admin@localhost`), even with `-force` — enforcing
  ADR 009's "never production content" instead of merely documenting it.

## Considered options

- **Keep both profiles**: rejected — full double maintenance for a
  dataset with near-zero consumers, and the coverage it held didn't
  protect anything from where it lived.
- **Wholesale merge** (all 11 bands + 7 music institutions into one
  dataset): rejected — bloats the demo and duplicates concepts without
  adding coverage beyond what the graft achieves.
- **Second instance for multi-quilt demos** (the original rationale for
  a parallel profile): deferred, not preserved. If that demo ever
  materializes, seamrip export/import of the seeded instance under a
  different config is a better second instance than a hand-maintained
  parallel fiction.

## Consequences

- Every future seed-wide change is a one-file change.
- The deleted music dataset (fictional bands, venues, festivals, a
  governance-heavy booking collective) survives in git history at this
  ADR's commit if anyone needs to mine it.
- `go run ./cmd/seed -profile music` no longer exists.
