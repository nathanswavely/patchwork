# ADR 061: What this quilt runs on includes whether it federates

Date: 2026-08-21. Status: **accepted** — built, unlike ADR 058–060.
Discharges ADR 060's fourth decision ("until then, say it"), which named
the obligation without naming the surface. Extends ADR 023 (the Label).

## Context

ADR 023 built the Label to state how a quilt is run: who stewards it, what
it depends on and why, what that costs, how to support it, and where the
exit is.

Federation was stated nowhere, in either direction. `federation.enabled`
is a boolean in `patchwork.yaml` whose effects are entirely invisible in
the product: with it on, every public patch has an address other sites can
follow; with it off, no patch does. Neither fact appeared on any screen —
the SPA contained the word twice before this branch, both incidental.

ADR 049 says state only what you enforce. Its mirror had gone unwritten:
**don't leave an enforced thing unstated when somebody's decision turns on
it.** A person choosing between quilts, or a venue asking whether putting
its calendar here will reach anybody, is asking a question Patchwork
already knows the answer to and never volunteered.

## Decision

**1. Federation is a material, so it goes in "What this runs on."** Not a
new section. That section already ends with the running version, which is
derived rather than stored and is there because ADR 023 called a version
"materials, and the Label is where a quilt says what it is made of." A
patch's reachability is the same class of fact, and now sits on the next
line.

**2. Derived, never stored.** Read from `cfg.Federation.Enabled` on every
request, exactly like `Version`. No column, no migration, no admin toggle,
nothing to keep in step — and nothing for the ADR 002 boundary to decide.
A stored copy could disagree with the routes actually mounted, which is
the one way this statement could become a lie.

**3. Both directions, in the same slot.** "Not federating. Patches here
can't be followed from other sites" is as much a disclosure as its
opposite. A line that renders only when the answer is flattering is
marketing, not a label.

**4. The door says what leaving costs.** ADR 060 established that
`ap_followers` and `ap_id` stay behind on a seamrip, so a fork keeps every
member and loses every remote follower. The Label is where the exit is
described, so it is where the exit's price belongs: what travels is the
community, what doesn't is this quilt's addresses. Rendered only when the
quilt federates — a quilt with no remote followers has none to lose, and
the sentence would be noise.

The same clause goes on the admin export panel, which listed what stays
behind ("credentials, sessions, and federation keys") without mentioning
the followers. Naming the keys and not the audience is the more misleading
half: a reader reasonably concludes keys are regenerated and nothing is
lost.

## Considered and rejected

**A "Connections" section of its own.** Cleaner-sounding, wrong twice. ADR
023 settled that money is structured and everything else is prose, and a
section holding one derived boolean is a heading in search of content. The
materials line already existed and already did this job for the version.

**An admin dashboard warning** — "federation is off, your patches have
addresses nobody can reach" — which ADR 059 floated and deferred here.
Rejected for now, on a fact rather than a preference: there is no warning
surface in `AdminDashboard.svelte` to join. SMTP, which CLAUDE.md
describes as warning in the dashboard, warns only in the startup log and
appears nowhere in the SPA. Building one notice system for one message is
the wrong order. The Label states the fact to every reader including the
admin, and an admin who has not read their own Label has a different
problem. If a warning surface is ever built for other reasons, this is a
good first tenant.

**Stating multi-quilt beside it.** `multi_quilt` is the other cross-quilt
capability, equally enforced and equally unstated, and it was tempting to
sweep it in. Left out because it answers a different question — whether
*other* quilts may read this one, which is reciprocity rather than
materials — and because a disclosure stays readable one line at a time.
Worth revisiting on its own.

## Consequences

- `GetLabel` takes `*config.Config` now; its only other caller was the
  test.
- The unpublished branch of `/api/v1/label` carries the flag too, mirroring
  `version`, so a consumer learns this even from a quilt whose stewards
  have written no Label.
- ADR 060's fourth decision is discharged in the two places seamrip is
  described to a person: the Label's door and the admin export panel. It
  is **not** audited anywhere else — the export CLI's own output, and any
  future copy about forking, can still imply the audience travels.
- Reading the page caught a spacing defect no source-text test could:
  `.costs` had no bottom margin, so its last materials line sat on the
  dashed rule the door draws above itself. One line stacked on another
  made it obvious. Fixed at 24px, matching the sections above.
