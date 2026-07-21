# ADR 025: The global patchwork is an instance, not a service

Date: 2026-07-20. Status: **accepted** as a shape constraint; nothing here
is scheduled. Builds on ADR 024 (cross-quilt following).

## Context

The idea: a "global patchwork" website where anyone can view, manage, and
link out to their followed patches across all quilts in one place — and,
downstream, a paid hosting arm that runs instances for communities that
don't want to self-host (the WordPress.com relationship to WordPress.org).

The aggregation half collides with ADR 024, which made a person's *home
instance* the one place their cross-quilt follows live and render (My
Quilt regions, server-side remote follows, AP notifications). A separate
aggregator app with its own accounts would be a second source of truth
for the same relationship — the localStorage problem reborn one level up.
The real gap the idea points at is narrower: people with no home instance
have nowhere to stand.

## Decision

**If a global patchwork exists, it is a normal Patchwork instance** — one
with few or no patches of its own; a quilt that is mostly sashing. Its
users are ordinary accounts using the multi-quilt features as shipped:
connected quilts, remote follows, My Quilt, federated notifications. Its
front door is a public **registry** (already in the glossary: a discovery
flyer, session-only overlay) grown into a browsable directory of quilts.
No new service type, no new accounts system, no aggregator API.

**The hosting arm is a provisioning layer, not a product change.** A
tenant is one container + subdomain + Caddy route; the single-binary,
SQLite, 2GB-Pi footprint makes tenants cheap. Seamrip is what makes paid
hosting consistent with the project's politics: a hosted community can
always rip out and self-host, so hosting is sold convenience, never
built lock-in.

**The flagship's gates are openly editorial.** The registry publishes its
inclusion criteria with the anti-discrimination baseline as the floor;
listing is a choice the directory makes, not a right a quilt holds. The
hosting arm applies the same baseline as a contract term. This is not
neutrality-washing in reverse: the open registry format is where
neutrality lives — the excluded can self-host and publish rival
directories — so the flagship never has to pretend its own list is
value-free. "Antifascist by design" would be a slogan if it stopped at
the surface with the most reach.

**The global instance gets no special powers.** It speaks the same
protocol as every other quilt, and the registry format stays open so
anyone can publish a rival directory. This is the structural answer to
the centralization tension — a flagship instance is a center of gravity,
and the WordPress.com history shows how a convenience center quietly
becomes the default home. The protocol must never know which instance is
"the" one.

## Rejected

A standalone aggregator web app (own accounts, reads public APIs,
manages follows independently). Rejected because it forks the follow
source of truth, re-implements My Quilt outside the codebase, and cannot
join the AP notification loop without becoming an instance anyway.
Expect this to be re-proposed; this ADR is the reply.
