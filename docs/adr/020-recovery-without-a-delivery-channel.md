# ADR 020: Account recovery without a delivery channel

Date: 2026-07-20. Status: **accepted**. Recovery codes and the second-passkey
nudge shipped 2026-07-20 (migration 023, `/api/v1/auth/recovery`,
`/api/v1/auth/recovery-codes`); admin-mediated recovery links remain a
follow-up. Companion to ADR 018 (email as one
relay plus recipes) and ADR 019 (self-hosted mail spike): those two are about
doing email well; this one is about needing it less.

## Context

Patchwork's auth is passkeys plus two link mechanisms: invite links
(out-of-band bootstrap) and magic links (email login, and — the load-bearing
case — recovery when a passkey is lost). So email's real job in the auth
story is recovery, and recovery is exactly where a delivery dependency hurts
most: the person who lost their passkey is the person a spam-foldered or
undelivered message locks out.

ADR 019 explores making that delivery channel ours. This ADR asks the prior
question: which alternatives avoid needing a delivery channel at all, and
which merely swap one oligopoly for another? The framing matters because the
obvious candidates — SMS, messaging apps — all fail the same test the email
giants do: someone else's infrastructure, someone else's terms, sitting
between a member and their own account.

One reframe up front: Patchwork has no passwords, so "2FA" is the wrong
lens. Passkeys are already a strong single factor. The problem to solve is
not adding factors; it is **recovering access when the factor is lost**,
without renting the recovery path from a third party.

## Decision

Adopt three channel-free recovery mechanisms, in priority order, and demote
email to a convenience rather than the recovery path:

1. **Recovery codes.** At passkey enrollment, generate ten single-use codes
   (stored hashed, like magic-link tokens). Tell people to write them down —
   paper is the one storage medium with no vendor. This is the workhorse: it
   turns "lost my passkey" from a delivery problem into a no-network
   problem, with UX every password manager user already knows.

2. **Admin-mediated recovery links.** A locked-out member asks a human who
   knows them; an instance admin generates a single-use recovery link for
   that account and hands it over out-of-band — Signal thread, in person at
   the venue, however the community already talks. This is deliberately the
   invite-link pattern pointed at an existing account, and it reuses that
   machinery: single-use, expiring, hash-stored, plus an audit-log entry
   naming which admin issued it and when. The trust anchor becomes a person
   who can vouch for you, which is not a workaround — for a grassroots
   community it is the *correct* trust model, and it matches how the
   platform already onboards people. The social-engineering risk (an admin
   talked into issuing a link for someone else's account) is real and is why
   the audit trail and expiry are requirements, not niceties; it is the same
   risk every "contact support to recover" flow has, with a smaller, more
   accountable support desk.

3. **Nudge toward a second passkey.** Most recovery never needs to happen if
   enrollment encourages redundancy — phone plus laptop. A settings prompt
   ("one passkey is one lost phone away from a locked account") is nearly
   free and shrinks the recovery population at the source.

Email magic links remain as a login convenience and a fourth recovery path
where SMTP is configured — nothing is removed. What changes is the story:
an instance with zero email service now has a *complete* recovery answer,
which ADR 018's "email is optional" claim quietly lacked.

## Considered and rejected channels

- **SMS.** Rejected outright, and worth recording why: it is a *tighter*
  oligopoly than email. Application-to-person SMS in the US requires 10DLC
  campaign registration through carrier-blessed aggregators, costs per
  message, offers no delivery visibility, and SIM-swapping makes a phone
  number the weakest common identity anchor. Every axis this project cares
  about — cost, self-hostability, member safety — SMS loses.
- **Signal.** No sanctioned API; the community tooling (signal-cli)
  registers via a phone number (SMS sneaks back in) and unofficial
  automation risks rate-limiting or bans. And Signal, however aligned its
  politics, is one organization's centralized servers. Communities should
  absolutely *share recovery links over* Signal — that is the out-of-band
  channel working as intended. The platform should not *depend* on it.
- **Telegram.** Free bot API, genuinely easy — and a single company's
  servers with none of Signal's politics. Same verdict, less sympathy.
- **TOTP (authenticator apps).** Not rejected — genuinely channel-free, open
  standard, offline. Deferred as redundant: next to passkeys plus recovery
  codes it adds enrollment friction and one more thing to lose, without
  covering a case the codes don't. Revisit if a community asks for it.
- **Web Push.** Blind relays (payloads are encrypted) but the relays belong
  to browser vendors, and push requires an already-authenticated browser —
  useless for recovery by construction. Noted as a future *notification*
  channel, where it may beat email; out of scope for auth.
- **ActivityPub DM.** Deliver a login code to a linked fediverse account.
  Ideologically the best fit in this list — the member picks their own
  instance, which can be self-hosted, and the AP plumbing already exists in
  `internal/ap`. Practically: best-effort delivery, and only helps members
  who have and link a fediverse account. Recorded as an optional flourish
  for later, not a foundation.
- **Matrix.** Honest and self-hostable, but running a homeserver to send
  login codes is a bigger operational commitment than ADR 019's entire mail
  spike, for a channel most members don't have. No.

## Consequences

- Implementation is deliberately small and reuses existing patterns: a
  `recovery_codes` table (hashed, per-user, single-use — the magic-link
  token discipline), an admin endpoint + audit entry for recovery links
  (the invite-link discipline), and enrollment/settings UI for codes and
  the second-passkey nudge. No new dependencies.
- The SMTP-less instance story upgrades from "possible" to "complete":
  bootstrap (invite links), daily auth (passkeys), and recovery (codes,
  admin links) all work with zero delivery infrastructure. That claim
  should land in DEPLOYMENT.md's email section once this ships.
- ADR 019's stakes drop. If self-hosted outbound turns out fragile, the
  blast radius is notifications and login convenience — not access to
  accounts. The two ADRs compose: 018 makes the channel ours, 019 makes
  the channel unnecessary for the part that matters most.
- UI vocabulary needs a pass (CONTEXT.md): "recovery codes" is probably
  right as-is, but the admin-issued link wants a name that isn't "invite
  link" — it re-admits someone who already belongs.
- Ordering: recovery codes and the second-passkey nudge are unblocked and
  worth doing soon; admin-mediated links next; AP DM someday-maybe. None
  of it waits on the mail spike.
