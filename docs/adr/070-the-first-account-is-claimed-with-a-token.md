# ADR 070: The first account is claimed with a token, not a race

Date: 2026-08-18. Status: accepted; implementation pending.

## Context

The first account created on a fresh instance becomes the instance admin
(`internal/auth/bootstrap.go`) — the bootstrap path for self-hosted
deploys, where no admin exists yet to invite anyone. But magic-link
signup is open registration: anyone can enter an email at /login and end
up with an account (docs/adr/013's two-phase flow). Put together, an
instance that is publicly reachable before its operator signs up is
claimable by whoever finds it first, and the prize is instance admin.

Self-hosters mostly dodge this by accident (no SMTP yet, so the magic
link prints to a log only they can read). A hosting arm provisioning
instances (docs/adr/025) loses that accident: hosted instances come up
with SMTP working, on addresses a stranger can guess, minutes or days
before the paying operator arrives. The gap is the project's either way
— an accident is not a lock.

## Decision

A **bootstrap token** gates the first account. Supplied via
`instance.bootstrap_token` in patchwork.yaml or the
`PATCHWORK_BOOTSTRAP_TOKEN` environment variable; while no real accounts
exist, completing signup requires presenting it, and once the first
account exists the token is dead — it gates bootstrap, not registration.
When no token is configured, the server generates one at startup and
prints it in the existing first-run notice — the log is already the
bootstrap channel (it is where magic links go without SMTP), so the
operator is the one reader guaranteed to have it.

A provisioning layer sets the token itself and embeds it in the
operator's first sign-in link; a self-hoster reads it off the log they
are already watching.

## Rejected

Gating at the edge instead (holding the route dark until the operator
flips it live). It protects only deployments whose proxy cooperates,
adds custom edge logic per host, and fixes nothing for self-hosters —
the vulnerability is in the bootstrap rule, so the fix belongs there.
