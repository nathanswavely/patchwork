# ADR 008: The ledger and the commons — earmarked contributions, not stored value

Date: 2026-07-13. Status: proposed.

> Amended by ADR 023: **steward** has now been grilled and entered
> CONTEXT.md as "a person publicly accountable for how a quilt is run."
> The commons stewards this ADR describes are those same people doing the
> money-specific part of the job, never a second role. *Batting* and *the
> Bee* remain provisional and ungrilled. ADR 023's Label is this ADR's
> transparency promise at phase 0 — self-reported and dated rather than
> audited; when phase 2's ledger lands, real entries render in the
> Label's cost slot and the staleness machinery switches off.

## Context

ADR 007 gives patches metered, usage-based infrastructure costs. Someone
has to pay them, and the paying should be a feature, not a chore: a
platform where every patch's real costs are a public document is the
transparency social media never had. But the audience is non-technical
grassroots organizers — nobody in a Lancaster arts collective wants to
become a payments merchant — and "we keep your unused balance" is, if
framed as stored value, gift-card law with refund obligations attached.

## Decision

Money, accounting, and metering are three packages that never touch:

- **Metering** (`internal/media.Usage`): knows bytes per patch, no
  concept of money. Ships with ADR 007 and works forever with payments
  off.
- **Ledger** (`internal/ledger`): an append-only double-entry table in
  SQLite, integer cents. Entry kinds: `contribution` (person → patch
  earmark), `usage` (patch → commons, priced from metering),
  `sweep` (unused earmark → commons at month end), `disbursement`
  (commons → an expense). The ledger does not know payment providers
  exist; it records entries. A public read endpoint makes the instance's
  full economics auditable by anyone — this endpoint is the product.
- **Payments** (`internal/payments`, gated by `payments.enabled`): a
  provider interface whose entire job is converting an external money
  event into one ledger entry. A **manual entry** (cash handed over at a
  show, recorded by an instance admin) is a first-class provider, not a
  hack. With `payments.enabled: false` everything still works and the
  host eats the cost — today's status quo, preserved as a feature.

**Contributions are donations, not deposits.** A monthly fill-up is a
contribution to the patchwork earmarked for one patch's usage. The
earmark balance is accounting, not owned stored value: no refunds, no
cash-out, and the month-end sweep into the commons is the stated design,
not fine print. This framing is what keeps instances out of
stored-value/gift-card territory, and it is more honest, not less: "what
your patch doesn't use, the quilt keeps."

**The commons is governed like a patch.** Disbursements — the hosting
bill, a solidarity grant covering a broke patch's usage, funding a zine —
are proposals (baste requests) in a stewards patch, decided with the
governance machinery that already exists (roles, voting, linings for the
rules of allowable spending). No new governance mechanism is invented;
fiscal governance gets the same contributor ladder as everything else.
A steward executes the approved expense on the fiscal host; the link
between vote and payout is social, not API-enforced, and that is
acceptable because the ledger makes divergence publicly visible.

**Open Collective is the reference fiscal provider.** Instance =
collective, patch = project (OC projects carry their own sub-budgets and
contribution pages); webhooks become ledger entries; the GraphQL API
reconciles. Patchwork never holds money — the fiscal host does, along
with receipts, taxes, and legal existence for unincorporated groups. The
docs must name both postures: find a fiscal host (mission-aligned
sponsors on the OC platform; a local arts council), or be an independent
collective if the community already has an entity and a bank account.
The dissolution of Open Collective Foundation (2024, 600+ projects,
$18M displaced) is the cautionary tale: the fiscal layer can vanish with
a year's notice, which is why the ledger is owned by Patchwork and the
provider is an interface. Using a fiscal host is encouraged, never
required — payments off, manual entries, and OC are all first-class.

**Seamrip boundary (extends ADR 002):** ledger history travels — the
books are community memory and the fork keeps its transparency record.
Balances do not: every earmark and the commons recompute to zero on
import, because money is held by a legal entity, not by the database,
and the entity stays behind. The export README says so.

**Phasing.** Each stage ships independently and is useful if the next
never lands: (1) metering, with ADR 007; (2) ledger + public endpoint,
manual entries only; (3) OC provider, monthly earmark + sweep; (4)
commons governance wired to proposals, solidarity grants.

## Vocabulary (provisional, pending CONTEXT.md grill)

Backend terms: ledger, contribution, earmark, usage, sweep, commons,
disbursement. Proposed UI coinages — **Batting** for the commons (the
hidden middle layer that keeps the whole quilt warm), **the Bee** for the
stewards patch (a quilting bee is communal work on a shared quilt).
"Contribution" and "usage" stay plain English; not everything needs
coinage. None of these enter CONTEXT.md until grilled — "trust,"
"credits," and "fill up" are rejected now (banker clothes; "credits"
also implies owned stored value, the exact framing this ADR avoids).

## Considered options

- Stripe-first with an instance bank account: rejected for v1 — makes
  every instance admin a merchant (PCI scope via checkout is avoidable,
  but taxes, refunds, and liability are not), and duplicates what a
  fiscal host provides. Stripe remains a future provider behind the same
  interface for instances that outgrow OC.
- Stored-value credits that expire: rejected — legally murky (gift-card
  breakage), philosophically wrong (the platform holding user balances),
  and unnecessary once contributions are donations with earmarks.
- Keeping leftover funds as instance revenue: rejected — "your leftover
  money becomes our money" is a dark pattern; the commons framing makes
  the same flow consensual, stated, and governed by the people who
  contributed it.
- Hard-coupling to Open Collective (no ledger of our own): rejected —
  OCF's dissolution proves the failure mode; the books must survive a
  host migration.
- A new governance surface for the commons: rejected — baste requests,
  roles, and linings already exist; a second decision mechanism would
  fork governance itself.
