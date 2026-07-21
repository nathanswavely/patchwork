# ADR 018: Email is one relay plus recipes, not a platform

Date: 2026-07-20. Status: **accepted** (docs change; one small code follow-up
proposed at the end).

## Context

The reference instance (lancasterpatchwork.org) got working email in July 2026
by stitching together three free services: Namecheap BasicDNS (registrar DNS,
whose email settings lock the MX section to either-or modes), ImprovMX
(inbound forwarding, which owns the root MX and an SPF record, aliasing
`hello@` to the admin's mailbox), and Resend (outbound SMTP relay on
`smtp.resend.com:587`, with its records on a `send.` subdomain plus a
`resend._domainkey` DKIM record).

It works, but it took three accounts and hand-edited DNS records spread across
three dashboards, with one genuine footgun found along the way (the registrar
MX lockout). Patchwork is white-label by design — any community can seamrip
and deploy on its own domain — so "repeat what Lancaster did" is a poor answer
for the next admin. The question is what to tell them instead.

Two facts reframe the problem before any provider comparison.

**The app's only email dependency is outbound.** Both senders —
`internal/auth/magic.go` (magic links) and `internal/notifications/email.go`
(notification emails) — do exactly one thing: submit a message to a configured
SMTP relay. Nothing in Patchwork receives mail. The inbound half of Lancaster's
stack (ImprovMX, the root MX record, `hello@`) exists for *humans*: so the
community has a contact address, so replies to the From address land somewhere,
and so providers can send verification and DMARC reports. That matters, but it
is not an app dependency, and bundling it into "what Patchwork needs" is how a
one-account problem grew into a three-account one.

**Email is already optional.** Invite links plus passkeys run a full instance
with no SMTP at all; magic links print to the server log for local dev. This
was a deliberate design choice and it holds. Email buys two things: magic-link
login for people who lost or never enrolled a passkey, and notification
delivery to people who aren't checking the site. Worth having on a real
instance, not worth blocking a launch on.

The concrete constraint on any relay choice: both senders use Go's
`net/smtp.SendMail` with `smtp.PlainAuth`. That negotiates STARTTLS on a
plaintext connection (port 587 works; PlainAuth refuses to send credentials if
the server doesn't offer TLS), but it cannot open an implicit-TLS connection,
so port 465 does not work. Nor does AUTH LOGIN, which rules out a few
enterprise relays. Every provider considered below offers 587 + STARTTLS +
AUTH PLAIN, so this constraint costs nothing today, but it narrows the
provider set at the margin.

## Decision

**Don't pick one true stack. Document the requirements contract and a small
set of blessed recipes in DEPLOYMENT.md, led by one primary recipe.**
Specifically:

1. **State the contract, not the vendor.** DEPLOYMENT.md gets an email section
   that says what any relay must offer (SMTP submission, STARTTLS on 587,
   AUTH PLAIN, a From address the relay accepts for your domain) and makes the
   outbound/inbound split explicit. Providers' free tiers churn; the contract
   doesn't.

2. **Primary recipe: Cloudflare DNS + Email Routing (inbound) + Resend
   (outbound).** Two free accounts. Moving nameservers to Cloudflare replaces
   the worst part of the Lancaster experience — registrar DNS UIs with modal
   email settings — with a plain record editor where inbound MX and outbound
   SPF/DKIM records coexist without fighting. Email Routing covers the human
   inbound need (forwarding `hello@` to a real mailbox, up to 200 addresses
   free); Resend covers the app's outbound need on 587.

3. **Secondary recipe: one paid mailbox provider** (Migadu, Fastmail,
   Purelymail — roughly $10–50/year). One account gives a real mailbox *and*
   an SMTP submission endpoint on 587, no forwarding service and no relay
   account. This is the genuinely-simplest option for small quilts and worth
   blessing, with its caveats stated: cheap tiers cap outbound (Migadu's Micro
   plan is on the order of tens of messages/day, which notification fan-out on
   an active instance can exceed), and mailbox providers' terms are written
   for human mail, not application sending.

4. **Keep the Lancaster stack documented as a working reference**, MX-lockout
   caveat included, rather than pretending it didn't happen. Someone whose
   domain is already at a registrar they won't move needs exactly this recipe.

5. **The app stays email-optional and outbound-only.** No inbound processing,
   no provider APIs, no further reduction of email dependence needed — the
   floor (zero SMTP) is already as low as it goes.

## Considered options

- **Cloudflare DNS + Email Routing + any 587 relay** — accepted as the
  primary recipe. It is not literally one account, because Cloudflare has no
  outbound SMTP: Email Routing is receive-only, and the newer Cloudflare
  Email Service sends via Workers bindings, not SMTP, with arbitrary
  recipients gated behind Workers Paid — unusable by a Go binary speaking
  `net/smtp` on someone else's hardware. But two accounts with all DNS in one
  free, lockout-free editor is the real consolidation win, and the relay slot
  stays swappable (Resend today; Postmark, Mailgun, SES tomorrow) without
  touching the recipe's shape. Cost: a nameserver move, which is a bigger ask
  than editing records at the registrar — some communities' domains are
  managed by whoever bought them years ago. That is why this is the primary
  recipe and not the only one.

- **Amazon SES as the single provider** — rejected as a recommendation, kept
  as a note for admins already on AWS. On paper it consolidates: outbound on
  587 (STARTTLS supported), inbound receiving, one account, and the cheapest
  sending anywhere at scale. In practice every step is hostile to the target
  admin: an AWS account wants a credit card; new accounts start sandboxed
  (verified recipients only) and leaving the sandbox is a human-reviewed
  support case that now expects SPF/DKIM/DMARC already in place; and inbound
  "receiving" delivers to S3 — turning it into a forwarded mailbox means
  writing a Lambda. Patchwork's operator story is a Raspberry Pi and a
  weekend, not an AWS console. Recommending SES would consolidate accounts by
  deconsolidating everything else.

- **Blessed recipes rather than one stack** — accepted, and it is the
  decision's frame. The honest reason: no single stack survives contact with
  the white-label requirement. Forks arrive with their domain at arbitrary
  registrars, different tolerance for nameserver moves, different budgets, and
  free tiers that change under us (this project has already been bitten twice
  in one month: Namecheap's MX modes, and restic's B2 backend deprecation —
  the pattern of "vendor changed the deal" is real). A requirements contract
  plus recipes degrades gracefully when a vendor changes; a single blessed
  vendor becomes a doc bug.

- **Reduce the app's email dependence further** — rejected as already done.
  SMTP-less operation is the shipped default. Going further would mean
  removing magic links, which are the recovery path when a passkey is lost
  and the SMTP-configured login path for people who never enrolled one.
  Removing capability to simplify a *documentation* problem is the wrong
  trade.

- **Support implicit-TLS 465 (and AUTH LOGIN) in the Go sender** — deferred
  as a small follow-up, not required by any blessed recipe. Every recommended
  provider speaks 587/STARTTLS, so this buys compatibility margin, not a
  recipe. It is cheap (a `tls.Dial` + `smtp.NewClient` path chosen by port,
  plus an AUTH LOGIN fallback, on the order of forty lines shared by both
  senders — worth extracting the duplicated send code into one place while
  there) and it widens the field to 465-only relays and LOGIN-only
  enterprise SMTP. It should land as its own change with its own test, not
  ride along on a docs PR.

## Consequences

- DEPLOYMENT.md gains an "Email (optional)" section (landing with this ADR)
  carrying the contract, the recipes, and a DNS records summary including a
  starter DMARC record. The config-table row for `smtp` points at it.
- The next fork's admin gets a decision tree instead of archaeology: no email
  → skip entirely; want free → Cloudflare + Resend; want one account →
  paid mailbox; domain immovable → the Lancaster-style registrar recipe.
- Nothing changes on live Lancaster. Its stack is recipe C in the docs;
  migrating it to the primary recipe is possible later but is its own
  operational decision with its own blast radius (nameserver move on a live
  domain), and nothing here forces it.
- The relay slot in every recipe is a free-tier dependency (Resend: 3,000
  emails/month, 100/day). A large instance will outgrow it; the contract
  framing means outgrowing it is a credential swap, not a re-architecture.
- Follow-up (separate change): 465/implicit-TLS + AUTH LOGIN support in a
  shared sender, per above. (Landed 2026-07-20 as `internal/mail` — the port
  now picks the TLS mode, and the "587 required" caveat is gone from the
  docs.)
