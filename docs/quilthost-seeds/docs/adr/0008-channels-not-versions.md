# Channels, not versions: the platform owns upgrade timing

Stewards choose a release channel — stable (default; releases land after
soaking on the canary cohort) or latest (the canary cohort) — and never
a version. Within a channel, timing is Quilthost's: staged rollout of
pinned release tags (snapshot, retag, restart, healthcheck gate,
snapshot-rollback on failure), a few instances at a time. No indefinite
pinning: a mixed-version fleet is how a one-person host ends up
supporting every bug that ever shipped and running known-vulnerable
instances under its own name — and "same binary, always" means the
current public release, not a museum. Releases flagged breaking (via
upstream machine-readable release notes, when they exist) queue for
manual promotion instead of auto-advancing.

Consequence, named: a bad release promoted to stable hits many
communities at once. The canary cohort, the healthcheck gate, and
snapshot-rollback are the entire defense, so they must demonstrably work
before the fleet grows.

Rejected: a per-quilt "update now" button (the WordPress pattern). It
converts a fleet into a support matrix and outsources security posture
to whoever is busiest.
