# ADR 023: The Label — a quilt states how it is run, who stewards it, and what it costs

Date: 2026-07-20. Status: accepted.

## Context

A Patchwork instance is a machine somebody pays for and a set of keys
somebody holds. Nothing in the product says so. A person arriving at
the reference instance sees patches, events, and a treemap, and has no way
to learn who is behind it, what it depends on, or what it costs to keep
running — the same opacity as any commercial platform, which is the thing
this project exists to refuse.

Three existing decisions leave a shaped hole:

- ADR 008 promises the instance's economics as a public, auditable ledger
  and calls that endpoint "the product." It is `proposed`, phased behind
  ADR 007's metering, and unbuilt. Its stated posture today is "payments
  off and the host eats the cost — today's status quo, preserved as a
  feature." Nothing renders that status quo to anyone.
- ADR 014 split configuration by who changes it: deployment concerns stay
  in `patchwork.yaml` and belong to whoever operates the machine, while
  community identity became database state edited in the UI. Stewardship
  is neither — it is a *statement about* the deployment made to the
  community, and it had no home on either side of that line.
- ADR 012 makes leaving a member right. But a right nobody is told about
  at the moment it matters is decorative, and onboarding currently
  introduces the quilt without ever mentioning the door.

## Decision

**The Label is a disclosure about the deployment, not a biography of its
admin.** It states how this quilt is run and paid for: who stewards it,
what outside services it depends on and why, what those cost, how to
support the work, how to send feedback, and where the exit is. Stewards
are named *inside* it and link to their existing profiles, so the page
survives a handoff without a rewrite — the subject is the quilt, and the
people are current answers to one of its questions. Named for the label
sewn to the back of a real quilt: maker, date, materials.

The layout leads with whoever is smallest in number. One steward and the
page opens with their face, their handle, and their own words —
indistinguishable from "hi, I'm Nathan, I run this." Five stewards and it
opens with the roster and the same prose reads as "we." One schema, one
data model, no `if solo` branch anywhere but the layout.

**Stewardship is a stated responsibility, never a permission.** The
stewards list is its own thing, not a view of the instance-admin role.
Holding the admin bit never publishes a person, and a person without it
can still be a steward — the one paying the bill and the one with root
are not reliably the same human. Deriving the roster from the role table
was the obvious implementation and is rejected outright: auto-publishing
the names and faces of everyone with root on an antifascist organizing
platform builds a targeting list out of a trust feature. Each steward
owns their own listing, in the spirit of ADR 006's one switch, and says
in their own words what they look after.

**A Label cannot publish with zero stewards, and a handle is enough.**
Accountability and safety genuinely conflict here and the floor is where
we resolved them. If everyone may hide, the Label says this quilt is run
by nobody — the unaccountable posture the feature refuses, and one that
guts the seamrip argument, since you cannot decide you dislike stewards
who do not exist. If nobody may hide, the feature doxxes organizers. At
least one named accountable party, no one conscripted into being the
face, and no legal name ever required: the username is already public and
already the federated identity, so a handle discloses nothing new.

**Money is structured; everything else is prose.** Costs are line items —
service, purpose, amount, currency, period, and a free-text *why*, which
is where the values live ("Hetzner because EU-based and cheap"; "no
Cloudflare, we won't put the quilt behind a CDN that can be leaned on").
Amounts are integer minor units, as in ADR 008's ledger, never floats.
Periods normalize to monthly for the total and display as entered, since
annual registrations are the common case and an un-normalized total is
simply wrong. Currency is one instance-wide choice.

A **prose-only Label is legal** and marked unstructured: no total, no
staleness detection, and it never upgrades to ledger-backed. An admin who
wants to write "costs me about fifteen bucks a month, mostly the server"
may, and sees plainly what that costs them.

**The Label discloses its own reliability.** Every line item carries a
`stated_on` date, and past a threshold the page itself — to every reader,
not as an admin nag — says the figures have not been reviewed in N
months. Self-reported numbers decay silently, and a stale money claim is
worse than none, because it wears the costume of an audit. The copy says
"stated by the stewards, not audited" and never implies otherwise. When
ADR 008 phase 2 lands, real ledger entries render in the same slot with
the staleness machinery switched off: the section goes from claimed to
audited without the Label changing shape.

**Cost sources follow ADR 008's provider pattern, with `manual` first-class.**
A `cost_source` per line item is either `manual` — shipped, permanent,
never second-class — or a bound provider that fetches the figure. Three
rules make automation safe:

- **A source binds to a resource, never an account.** Server ID, bucket,
  project. Billing APIs are usually account-scoped, so "fetch my spend"
  can return a total spanning an admin's unrelated client work and
  publish it. If a provider cannot scope below the account, it stays
  manual. The failure is silent and irreversible once rendered.
- **A provider that cannot issue read-only, billing-scoped credentials
  stays manual.** Credentials are encrypted, never in a seamrip, never in
  `GET /api/v1/instance`.
- **The Label renders from cache when a provider is down**, showing the
  last known figure with its own `fetched_at` stamp. A billing outage or
  an expired token can never blank the page. This bends "no external
  service dependencies" no further than federation already does.

First fetch on a new binding lands as a proposed value a steward
confirms, so the account-total mistake is caught while still private;
afterwards refreshes publish automatically, and a figure that jumps past
a margin reverts to proposed rather than publishing, because a 10× jump
means a broken binding far more often than 10× hosting. Confirm-first is
configurable but is the default: options protect the admin who reads
settings, defaults protect the one who does not.

**Placement: canonical at `/label`, reachable logged out.** The Label's
most important reader has no account — someone deciding whether to join,
or comparing quilts. Anything behind auth fails its main job. It is
linked from a slim attribution strip in the shell, overlaid on the quilt
and map views in the register of a map's terms-and-attribution line
(quilt name · stewarded by @handle · the Label · seamrip) and rendered as
an ordinary footer elsewhere — **one component with two densities, not
two footers**, since ADR 005 rejected its alternatives partly for forking
one bar into two implementations. The strip obeys the takeover rule: like
the discovery rail, it does not exist inside workspaces or the admin
panel, because the Label is an instance concept and a workspace is a
patch context. Also linked from the user menu and from each entry in
Connected Quilts, which is decision support at the deciding moment.

**On mobile the strip collapses to an info button; it never stacks.** At
≤768px the discovery rail already *is* the bottom edge — `SocialShell`
turns `.sidebar-rail` into a full-width glass bottom bar with its own
safe-area inset. A second bottom strip would stack above it, spending
~28px of permanent viewport on the smallest screen, between the nav and
the content. The reference pattern does not do this either: a map's
attribution collapses to a tapped affordance on mobile rather than
rendering the strip.

So: ordinary pages get the ordinary footer at scroll end on every screen
size, free of chrome cost. The quilt and map views, which have no scroll
end, get a single info button pinned bottom-left, opening a sheet with
the quilt name, stewards, monthly total, and the seamrip link. It is
added as a **leading sibling of `.rail-center`, not an item inside it**,
so the real nav items keep their `space-around` distribution undiluted
while the button takes the edge — a position that reads as subordinate,
which it is. This reuses the rail's existing glass, z-order, and
`env(safe-area-inset-bottom)`, adds no fixed layer, and cannot collide
with the mobile filter card anchored at `top: 56px`. The button is
mobile-only and hidden above 768px: the desktop rail is a column, where a
leading sibling would land at the top of the sidebar, and desktop already
has the strip.

**Onboarding gets a panel, not a step.** Step 1 already asks people to
agree to something before they begin; the Label's summary — steward
names, monthly total, a line of prose, "read the Label →" — sits beside
it. A dedicated step people click past teaches them the Label is a
formality, and this feature dies the moment it reads as boilerplate. The
panel simply does not render when no Label has been written, so an
instance that has not written one leaves no conspicuous hole in the flow.

**Federation exposes the summary, not the roster.** `GET /api/v1/instance`
gains steward *count*, monthly total, currency, staleness date, and the
Label URL — never handles, never prose, never line items. A quilt-picker
needs "is anyone accountable, and what does it cost," not names; names
are one click away on the Label, where a reader arrives in context. Every
steward consented to appearing on a page, which is not the same as
consenting to a machine-readable directory: that endpoint is CORS-open
and registry-aggregated, so including handles would make "enumerate every
Patchwork admin" a one-line scrape across every connected quilt. A second
per-steward switch for federated visibility was rejected as reopening
exactly what ADR 006 settled.

**Seamrip boundary (extends ADR 002 and ADR 014):** the Label does not
travel. It is instance identity, and worse, it is *false* in a fork —
different stewards, different server, different bill — so inheriting it
would make a fork's Label a lie on its first day, the precise failure
this feature exists to prevent. The fork lands with a blank Label and a
written prompt, since a seamrip is the best moment anyone will ever have
to say who they are and what this costs. One line is prefilled: a
**seamripped from** provenance link to the origin quilt, removable
without ceremony — lineage is worth recording, but a community fleeing
its stewards must never be forced to keep a link to them.

## Considered options

- **A person-shaped "about the admin" page**: rejected — "who I am" and
  "what this server costs" have different half-lives, and the page breaks
  on handoff or a second admin. Warmth was the real argument for it, and
  it is preserved by authored prose and the solo-first layout rather than
  by making a person the subject.
- **Calling it "About"**: rejected — no collision and instantly legible,
  but About pages are where boilerplate goes to die. A label states
  materials and maker, and that promise is the feature.
- **Calling it "Backing"**: rejected — ADR 008 provisionally claims
  *Batting* for the commons; two adjacent layer-metaphors in the same
  money-shaped neighborhood is worse than one sharp word.
- **Colophon**: rejected — the true ancestor of this page and precise,
  but the wrong register for grassroots organizers.
- **Deriving stewards from the instance-admin role**: rejected on safety;
  see above.
- **Allowing a fully anonymous Label**: rejected — see the floor of one.
- **Verified costs via provider integrations as the primary mechanism**:
  rejected as primary, allowed as an opt-in source. A `stated_on` date
  buys most of the trust for almost none of the cost, on a platform that
  must run on a Pi.
- **A dedicated onboarding step**: rejected — buys attention that a step
  people are motivated to dismiss does not actually deliver.
- **A footer on every surface including workspaces**: rejected — ADR 005's
  takeover principle; discovery furniture does not follow people into a
  patch context.
- **Stacking the attribution strip above the mobile rail**: rejected —
  permanent viewport cost on the most cramped screen, in the layout's
  worst slot.
- **The mobile info button as a `.rail-center` item**: rejected — the
  rail is primary navigation, and `space-around` means every added item
  dilutes the real destinations on a 360px screen. As a leading sibling
  it gets the edge without entering that flex group.
- **A floating chip above the rail**: rejected — one more fixed element
  to keep clear of the filter card, duplicating glass and safe-area
  handling the rail already does.
- **Steward handles in `GET /api/v1/instance`**: rejected — best UX,
  worst aggregation risk; the count carries the comparison.
- **Letting the Label travel in a seamrip**: rejected — it would be
  wrong on arrival.
