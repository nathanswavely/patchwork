# ADR 012: Leaving is a member right — the egress boundary and its three affordances

Date: 2026-07-14. Status: accepted as design boundary; affordances 1 and 2
shipped 2026-09-07 (docs/adr/089), affordance 3 remains backlog.

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

## Status, 2026-09-07: affordance 1 shipped

`GET /api/v1/users/me/export` is live, behind the session and rate-limited
to five downloads per account per fifty minutes, audited as `user.export`,
and offered at Account settings as **Download my data**. It carries one
JSON document: the person's `users` row (contact card folded into the shape
the API uses elsewhere), every membership *including the hidden ones*, and
their rows from every author- or actor-attributed table in the schema —
proposals, votes, election candidacies and ballots, comments, reactions,
revisions, notices and replies, events, event links they requested,
attestations they recorded and ones that name them, claims, event sources
and aggregator curation, notification preferences, notifications received,
connected quilts, remote follows, reports they filed, their steward
listing, and their own audit-log rows.

Three things are deliberately absent, and the reasons are in
`internal/handler/personal_export.go` rather than only here:

- **Authentication material** — credentials, sessions, recovery codes,
  magic/invite/signup links, `feed_secret_hash`, the AP keypair. An export
  is a file that gets copied to a laptop and emailed to oneself; nothing in
  it should help anybody get in. This is ADR 002's line drawn tighter,
  since a person carrying their own record needs no keys at all.
- **Moderation they performed** — `content_reports.reviewed_by` and
  `claim_requests.reviewed_by`. A decision about somebody else's content is
  the instance's record of how it handled a third party. Reports they
  *filed* are in the export; those they wrote.
- **Other people's writing**, even where it names them. A reply under their
  notice belongs to the replier.

The privacy policy gained a paragraph saying the download exists and what
is in it, since it previously named only the admin export. The user
agreement's "any member can export what they can already see" was left
alone on purpose: it describes affordance 2, it is a sentence a person
wrote (the copy ledger marks it `human`), and this affordance is not the
one it promises. It stays an outstanding claim until the member seamrip
lands.

## Status, 2026-09-07: affordance 2 shipped

`GET /api/v1/users/me/seamrip` is live. docs/adr/089 records how, and the
decisions this ADR left open.

In short: the portability boundary in `internal/seamrip` grew a second axis
rather than a second implementation. Every travelling table now states both
what travels and which rows a given member may carry out, and the member
export runs the admin export's own queries through that filter, so a column
added to the boundary reaches both bundles or neither. `people travel as
stubs` — id, username, display name, avatar — and the set of them is the
closure of everybody the other travelling rows name, read from the schema's
own foreign keys rather than from a list somebody has to remember to
update. No emails, no hidden memberships from a patch the caller is not in,
no members-only charters or events from one either, no noticeboards, no
contact cards, no claims, no calendar feed URLs outside a patch the caller
administers. A tombstone travels as the tombstone it is (docs/adr/086).

Two downloads per account per day, audited as `user.seamrip`, offered at
Account settings as **Member seamrip**. The bundle carries a manifest
naming the kind, the person who took it, the time, and the instance, and
`cmd/import` says which of the two kinds it is reading rather than assuming
the admin one.

The user agreement's "any member can export what they can already see" is
no longer an outstanding claim.

Affordance 3 (moved-to pointer) is unbuilt.
