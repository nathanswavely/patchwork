# ADR 019: Self-hosted mail sidecar — spike before deciding

Date: 2026-07-20. Status: **proposed** — this records a spike plan and the
architecture it would validate, not a decision to ship. ADR 018's recipes
remain the documented default throughout.

## Context

ADR 018 settled what to tell the next admin: one outbound relay, blessed
recipes, email optional. This ADR is about a different appetite — running
real email on hardware we control and decoupling from third parties as far
as physics allows.

The constraint is not compute. Stalwart (SMTP + IMAP + JMAP + spam filtering,
one Rust binary) runs in ~100MB RAM, which fits the 2GB reference box beside
the app and Caddy. The constraint is **outbound reputation**: Gmail and
Microsoft receive most members' mail and score senders by IP and domain
history. Cloud IPs arrive with baggage, fresh ones have no history, and the
feedback when you lose is silence — mail folders or bounces and nothing tells
you. Inbound has no such gatekeeper: receiving mail is a listener on port 25
and an MX record you already own.

Two facts make a spike cheap enough to just run:

- **Hetzner's port-25 block is a form, not a wall.** Outgoing 25/465 is
  blocked by default; accounts past their first paid invoice can file a limit
  request, decided case-by-case, usually within a day. The production VPS qualifies.
- **The two outbound modes are one config line apart.** A Stalwart sidecar
  owns inbound, mailboxes, and the app's SMTP submission either way; outbound
  is either *direct* (port 25 to the world) or *via smarthost* (any 587
  relay). Trying direct and retreating to smarthost is a config edit, not a
  re-architecture. So the only genuinely open question is empirical: **where
  does direct-sent mail from our IP actually land?**

There is also a brand argument, and it deserves to be in the record because
it changes the success criteria. Patchwork's stance is self-reliance;
"we send our own mail and the giants punish independents for it — check your
spam folder" is honest copy, on brand, and asks users to work a little
harder as part of the deal. But the mitigation is asymmetric:

- **Magic links: copy works.** The person is at the login screen actively
  waiting for an email. A "check spam" line beside the "link sent" message is
  actionable in the exact moment it's needed.
- **Notifications: copy can't help.** Nobody is waiting for them. A
  spam-foldered notification is silently lost engagement; there is no moment
  to show the warning. Acceptable only because notifications are best-effort
  by design.
- **Hard rejection: nothing helps.** A 550 never reaches a spam folder to be
  found. If a major receiver rejects outright, direct mode is dead for that
  receiver regardless of framing.

So the punk copy widens what counts as "good enough" for the auth path
specifically. It does not make deliverability irrelevant.

## The spike

Time-boxed to ~3 weeks of mostly-idle measurement. Nothing touches
the reference instance's mail or DNS.

1. **File the Hetzner limit request** for outgoing 25 on the production VPS (account
   action — Nathan). State the use case plainly: transactional mail for a
   community platform, SPF/DKIM/DMARC configured, no bulk sending.
2. **Buy a throwaway domain** (~$3–10). Not a subdomain of
   the production domain: Gmail's reputation model weighs the organizational
   domain, so a test that goes badly must not splash the real one.
3. **Stand up Stalwart** as a compose sidecar on the production VPS (or a scratch
   server), listening on the test domain. Full authentication from day one:
   SPF, DKIM, DMARC (`p=none` with reports to a mailbox we read), correct
   rDNS/PTR via the Hetzner console, MTA-STS if time allows. Register the
   test domain with Google Postmaster Tools — it is the only window Gmail
   offers into how it scores you.
4. **Send the real shape of mail.** Magic-link-shaped messages (short,
   plain-text, one URL) and notification-shaped messages (the HTML template),
   a few per day for two weeks, to fresh accounts at Gmail, Outlook, and
   Yahoo, plus one Fastmail/Proton account as a control. No warm-up
   theatrics — the production instance won't do them either, so the spike
   shouldn't.
5. **Score placement**, per receiver, per week: inbox / spam folder /
   rejected. Also confirm the inbound leg while the box is up: mail *to* the
   test domain arrives, survives Stalwart's spam filter, and is readable
   over IMAP/JMAP.

**Reading the result:**

- **Mostly inbox at Gmail and Outlook by week two** → direct mode is real
  for this IP. Proceed toward the sidecar as an optional recipe D, punk copy
  on the magic-link screen as honest insurance.
- **Spam-foldered but accepted** → direct mode is viable *for auth only*,
  leaning on the copy; notifications should route via smarthost. (Stalwart
  can route per-destination or the app can stay simple and everything uses
  the smarthost — decide then.)
- **Rejections from any major receiver** → smarthost is the resting state.
  Still a win worth shipping: the sidecar owns inbound, mailboxes, and
  identity; the third party is reduced to final delivery of outbound
  messages, holding nothing.

## What would ship if the spike succeeds (sketch, not commitment)

- An optional compose profile adding a Stalwart container: MX for the
  instance domain, mailboxes for admins, SMTP submission on the internal
  network (the app's `smtp:` block points at the sidecar), outbound
  direct-or-smarthost by config.
- Login-screen copy after "link sent," in the project voice (defer wording
  to design review): the honest version of "we send our own mail;
  the big providers make that hard; check spam and mark us Not Spam — it
  actually teaches your inbox."
- DEPLOYMENT.md recipe D with the full cost list stated up front: mail
  spools join the backup set, PTR/MX/MTA-STS join the DNS records, blocklist
  monitoring joins the ops routine, and being down now means other people's
  servers are queuing against you. ADR 018's recipes stay the default for
  admins who don't want any of that — self-hosting mail is a choice a
  community makes, not a hazing ritual the platform imposes.

## Risks worth naming now

- **Stalwart is young.** One binary and modern defaults are why it fits;
  "less battle-tested than Postfix" is the price. The spike is also a
  soak test.
- **Reputation is rented, never owned.** A clean result in August can sour in
  November because a neighboring /24 got noisy. Direct mode is an ongoing
  relationship, and the dashboard-checking falls on whoever runs the
  instance. The recipe D docs must say so.
- **The spike measures one IP.** The production VPS's result doesn't transfer to the
  next fork's VPS. What transfers is the sidecar architecture, the
  measurement protocol, and the honest copy — recipe D would teach forks to
  run this same test, not promise them its outcome.
