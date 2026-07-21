# ADR 012: Leaving is a member right — the egress boundary and its three affordances

Date: 2026-07-14. Status: accepted as design boundary; implementation is
post-launch backlog.

## Context

The mission says the community can take its data and walk, but today every
egress path is admin-shaped: the only export endpoint is
`GET /api/v1/admin/export` (AdminRequired), and it deliberately carries
every user's email so a fork can re-admit people by magic link
(`internal/seamrip`). In the exact scenario seamrip exists for — leadership
gone sideways — a hostile *sole* admin can refuse the export and members
have no valve. The domain-custody discussion surfaced the same shape:
concentration lives wherever exactly one
person holds a handle, whether that's the registrar login or the export
button.

Encrypting exports "so anyone can carry them but only admins can open
them" was considered and rejected. Patchwork is passwordless: members hold
no long-lived secrets (passkeys are signing-only and domain-bound), so
sealed exports require minting admin keypairs — volunteer key custody,
key-loss data death, and the concentration problem recursed onto whoever
holds the key. Worse, in the motivating scenario the decryptors are the
adversary: the fleeing community would carry a blob only the person they
are fleeing can open. And a running server processes plaintext regardless;
encryption can only ever protect the artifact, never the live system from
its operator.

## Decision

**The boundary rule: a member can take what they can see. Other people's
secrets — emails, hidden memberships — move only with their owner's
consent (re-joining) or the custodian's cooperation (full seamrip).**

Three affordances implement it (post-launch backlog, in this order):

1. **Personal export** — `GET /api/v1/users/me/export`: everything about
   *you*. Profile, your memberships including your hidden flags (they are
   yours), proposals you authored, your votes and comments. No admin
   involved, no one else's data included. Every member's data-rights
   baseline.
2. **Member seamrip** — an authenticated, rate-limited export of the
   requesting member's *view*: public patches, member lists they can read,
   events, governance docs, proposals and tallies, in the seamrip import
   format. No emails, no hidden memberships — nothing they couldn't read
   through the API today, but bundled so any member can seed a fork
   without anyone's permission. The fork re-invites people out-of-band
   (invite links where the community already talks); each person re-joins
   by choice and re-sets their own visibility. Re-consent is a feature,
   not a loss: joining the fork mirrors joining the original.
3. **Moved-to pointer** — a profile- and patch-level "we've moved" field
   (local `alsoKnownAs`): a banner and link from the old home to the new
   one, requiring nothing from the instance admin beyond the server
   staying up. The federated Move emission remains future work layered on
   the same field (a known limitation).

**Full seamrip stays admin-gated and is understood as custody transfer** —
it moves the contact list, so it should require trust. The residual
concentration (a hostile sole admin uniquely holds emails) is answered
socially, not cryptographically: deployment guidance says never exactly
one admin, same rule as the registrar account.

## Considered options

- **Admin-sealed encrypted member export**: rejected (keyholder regress,
  adversary-holds-keys, volunteer key custody — see Context).
- **N-of-M community escrow** (Shamir over a recovery bundle including
  emails): the only coherent cryptographic variant. Real machinery and UX
  cost for volunteer-run instances; deferred, not dismissed. Revisit if a
  real community hits the hostile-sole-admin wall.
- **Make full seamrip member-accessible**: rejected — one click hands any
  member the full contact list (harvest vector) and other members' hidden
  memberships (breaks ADR 006's promise that the visibility switch is
  owned by the member).

## Consequences

- "Leaving is easy" becomes structurally true for members, not just for
  admins: fork the visible quilt (member seamrip), carry your own record
  (personal export), find the successor (moved-to) — none require the
  authority's cooperation.
- The seamrip vocabulary splits: *seamrip* (full, admin, custody
  transfer) vs *member seamrip* (view-scoped, any member). CONTEXT.md
  carries both.
- Nothing here blocks the Lancaster launch; the affordances land in the
  post-launch backlog.
