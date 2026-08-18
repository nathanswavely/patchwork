# Two-store backup; the database wins on restore

Instance state is one directory holding two stores: the SQLite database
and the per-patch governance git repos. Litestream replicates the
database continuously (point-in-time restore); a nightly restic snapshot
of all of /data catches the repos. On restore the two can disagree —
database newer than repos — and that is acceptable by Patchwork's own
canon rule (patchwork docs/adr/011: the DB row is canonical, the git
file is its history mirror): a stale mirror is a cosmetic gap, a stale
database is data loss.

Verified limitation, not assumed: a missing repo does not self-heal —
governance writes fail at openBare, and ForkForNode runs only at patch
creation — so a database-only restore is not complete today. Upstream
candidate: a rebuild-governance-repos-from-DB repair command, which
would also close patchwork ADR 002's known gap (repos don't travel in
seamrip). Until it exists, restic is load-bearing, not optional.
