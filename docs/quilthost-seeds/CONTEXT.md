# Quilthost

Glossary for Quilthost, the managed hosting service for Patchwork
instances. Quilthost is a control plane and nothing else: hosted quilts run
the unmodified public Patchwork binary. Where a term exists in
Patchwork's own CONTEXT.md, that definition wins; this file adds only
what hosting introduces.

## Language

**Host**:
Quilthost's role toward a quilt: it runs the machine. Exactly the sense
Patchwork's glossary reserves the word for ("the fiscal host or the
machine") — never a synonym for steward, and never a governance role.
_Avoid_: provider (vague), platform (that is Patchwork itself), operator

**Account**:
The mechanical, billable thing on Quilthost's side: a subscription, its
holder, and the quilts it pays for. Backend term only — the console UI
speaks of stewards and quilts, never accounts or customers.
_Avoid_: customer, tenant (both banned from UI copy), user (that is a
person on a quilt)

**Steward**:
Borrowed unchanged from Patchwork: a person publicly accountable for how
a quilt is run, named on its Label. Quilthost's console addresses its
account holders as stewards and says out loud that running a hosted
quilt makes you one — but who the Label names belongs to the community,
never the host.
_Avoid_: owner, admin (that is a role inside the quilt), account holder
(backend register)

**Seamrip guarantee**:
Quilthost's published, versioned promise of what leaving looks like: same
public binary always, self-serve export unconditional for ordinary
endings, discretionary-by-default for eviction for cause with the
unlawful-content carve-out — every limit printed on the guarantee page
itself, never only in the terms.
_Avoid_: data portability policy (register), export feature (the
guarantee is a promise, the export is a button)

**Exit runbook**:
The published "Leaving Quilthost" document that turns an export into
`docker compose up` on the community's own hardware, exercised in CI
against every release so the door is known to open. Reachable in the
console as the standing "Self-host this quilt" page.
_Avoid_: migration guide (nothing is migrated for you), offboarding
(corporate register)

**Claim link**:
The one-time link the console hands a new steward: it opens their fresh
instance's signup with the bootstrap token attached (patchwork
docs/adr/057), making them the first account — the instance admin — no
matter who found the subdomain first.
_Avoid_: setup link, activation (nothing is inactive), invite (that is a
Patchwork admin's instrument, and no admin exists yet)

**Channel**:
The steward's only upgrade control: stable (default — releases land
after soaking on the canary cohort) or latest (the canary cohort).
Within a channel, timing belongs to Quilthost. There is no version
pinning.
_Avoid_: version (nobody picks one), track, ring (Microsoft register)

**Resting**:
The public face of a suspended quilt: the edge serves a plain "this
quilt is resting" page naming the steward path back. Nothing is deleted
while a quilt rests, and the export stays self-serve — suspension is
never a hostage state.
_Avoid_: suspended (backend state name, not the visitor-facing word),
disabled, offline (says broken, not paused)

**Retention window**:
The period after termination — 90 days — during which a quilt's data
still exists and its export stays self-serve, ending in actual deletion
with notice. Applies to every ending; only the eviction-for-cause tier
makes the export discretionary (ADR 0002).
_Avoid_: grace period (that is the past-due phase, a different clock),
backup retention (an ops setting, not this promise)
