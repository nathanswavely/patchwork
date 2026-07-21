# ADR 009: Seed profiles are fiction

Date: 2026-07-13. Status: accepted.

## Context

The seed profiles (`cmd/seed -profile arts|music`) mixed fictional entities
with real Lancaster organizations. Some real orgs appeared as *unclaimed*
directory entries (factual, defensible), but others — Music For Everyone,
the Lancaster Symphony Orchestra, Tellus360, Zoetropolis — were seeded as
*claimed* patches with fictional owners, fabricated staff, invented
governance documents, and approved-or-rejected proposals attributed to
them. In a public repo whose demos get deployed, screenshotted, indexed,
and (with federation live) potentially delivered to Mastodon, that puts
words in real organizations' mouths. The reference deployment is *for* the
Lancaster scene; burning goodwill with it is the worst failure mode.

This forced the question the seed had never answered: what is it for?

## Decision

The seed is a **fixture**, serving exactly two purposes:

1. **Dev/E2E fixture** — deterministic data the Playwright suite and dev
   tokens depend on.
2. **Evaluation demo** — a dense, living instance so a prospective
   self-hoster can feel the product, including from the admin chair.

It is **never production content**. It must not bootstrap a real instance.

Rules that follow:

- **Real places, fictional actors** (see CONTEXT.md). Real geography —
  streets, parks, neighborhoods, coordinates, the city itself — is
  setting and stays. Every *actor* in the fiction (any patch, any venue
  hosting a seeded event, any employer or partner named in a bio) must be
  invented. The test: if a reference puts a real organization into a
  fabricated relationship, it's out.
- **Density stays.** The quilt's signature feature — threads inferred from
  member overlap — is an emergent property of ~30 users with curated
  cross-membership. A minimal "Test Patch 1" seed would demo everything
  except the point. The existing fictional structure (rosters, overlap,
  proposal pipelines) is preserved; only real-entity names are replaced.
- **No live-resolvable third-party URLs or handles**, on fictional
  entities included. Websites use `.example` (RFC 2606, unregistrable);
  social links are dropped or point at `.example` paths. A fictional org
  linking a real Instagram handle is the participation problem one hop
  removed — someone can own that handle tomorrow.
- **Fictional names must be distinctive, not generic-civic.** "Conestoga
  Drift" and "Static Season" are the model: unmistakably invented, fully
  plausible. Names shaped like *Lancaster + civic noun* ("Lancaster
  Community Chorus") collide with real organizations almost by
  construction. If a name would look at home in a Google Maps result for
  Lancaster, invent harder — and check each name against a search before
  it ships.

## Considered options

- **Point evaluators at a real Patchwork instead of seeding.** Rejected:
  no real instance exists yet, an evaluator on someone else's instance can
  never experience the admin/governance surfaces, and the E2E suite needs
  the fixture regardless. A hosted showcase instance is a future
  *complement*, not a replacement.
- **Tiny seed with obvious mock names.** Rejected: kills the quilt demo
  (no overlap, no threads) and trades verisimilitude for nothing a README
  disclaimer doesn't already provide.
- **Real venues as unclaimed patches only.** The most defensible use of
  real names (factual directory entries mirroring the claim flow), and the
  music profile already drew this line. Still rejected for the fixture: a
  fictional venue exercises the same code path, and a real-venue starter
  directory belongs to a future factual-only dataset built for the actual
  Lancaster launch, where every entry is verifiable public fact and no
  fictional users exist alongside it.

## Consequences

- The ~10 real organizations currently in the profiles get renamed in
  place, preserving membership edges, events, and proposals so the
  affinity story survives; remaining generic-civic fictional names get a
  collision check.
- CLAUDE.md's example prose and one frontend unit-test mock reference
  Tellus360 and follow the rename. E2E specs pin only fictional slugs and
  are untouched.
- If the real Lancaster deployment wants launch content, that is a new,
  separate dataset — unclaimed factual entries only — not a seed profile.
