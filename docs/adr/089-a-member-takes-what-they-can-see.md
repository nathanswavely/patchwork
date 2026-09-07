# ADR 089: A member takes what they can see, and people travel as stubs

Date: 2026-09-07. Status: **accepted**; implemented. Implements docs/adr/012
affordance 2 and extends docs/adr/002. Issue #227.

## Context

docs/adr/012 named three egress affordances and shipped the first one. This
is the second: an authenticated export of the requesting member's *view* of
the quilt, in the format `cmd/import` already reads, so a community can
stand itself up elsewhere without anybody's permission.

The affordance exists because the only whole-instance export is
`GET /api/v1/admin/export`, and it is admin-gated on purpose: it carries
every user's email address so a fork can re-admit people by magic link, and
it carries hidden memberships, which are the one thing ADR 006 promises are
the member's own. That is a custody transfer, and it should require trust.
The consequence is that the exact scenario the seamrip exists for —
leadership gone sideways — was protected by nothing but the deployment rule
of never running exactly one admin. A hostile sole admin could refuse the
export and the community had no valve.

The naive way to build this is a second export: a file of queries that
re-derive, for each kind of thing, which rows this person may read. It is
also the way this codebase has already been burned. ADR 002 exists because
three copies of the export logic had drifted apart, and its two amendments
exist because the boundary was a sentence in a document that nothing
checked. A second export would be a fourth copy, and a fourth copy of a
rule that has to be right about privacy rather than about counts.

There is a second, quieter problem. A bundle that carries a membership must
carry the person the membership names, or the import mints an id for
somebody who is not in the archive and SQLite refuses the row. The failure
is silent in the direction that matters: rows go missing from the thing
whose whole job is not to lose rows. So "which people travel" is not a
policy question that can be answered independently of the other tables. It
is their closure.

## Decision

**1. One boundary, two axes.** `internal/seamrip` stays the single
definition of portability and gains a second question asked of every
travelling table. `Tables()` says what *travels*. `memberViews()`, beside
it, says for each of those tables which rows a given viewer may *carry
out*, as prose plus a SQL predicate. `TestEveryTableHasAMemberViewRule`
fails the build when a table answers the first question and not the second,
exactly as `TestEveryTableHasABoundaryDecision` fails when it answers
neither. Adding a table is now two decisions in one edit.

**2. The export runs the same writer.** A member seamrip is the admin
seamrip's own queries, wrapped:

```sql
SELECT <the table's columns, some replaced by an expression>
FROM ( <the table's own query> ) WHERE <the member-view predicate>
```

There is no second column list to keep in step. A column added to
`Tables()` reaches both bundles or neither, which is the property ADR 002's
amendments were written to buy and the one a parallel implementation would
have spent.

**3. Every predicate is a read check that already exists.** Private
patches only where the caller holds an active membership (the set
`ListNodes` serves under `scope=my`). Memberships by ADR 006's public
member-list rule, widened to everything inside a patch the caller is a
member or admin of, because a hidden membership is visible in the
workspace. Charters by ADR 036. Members-only events only inside the event's
own patch, the way `ListEvents` has it, so a confirmed link never widens
visibility. Proposals, votes, ballots, comments and revisions wherever the
proposal travels. Where the export and the API could drift, the export
takes the narrower answer.

**4. Never, for four kinds of thing.** The noticeboard, because the room is
the members' (docs/adr/081) and a person carries their own notices out
through the personal export instead. Contact cards, because sharing is
per-patch and per-person (docs/adr/080) and nobody consented to a copy on a
server they have not heard of. Claims, because a claim is evidence of who
somebody is, handled by this instance's review (docs/adr/030). Aggregators
and their crosswalk, because that is instance-level curation read only in
the admin panel, and a feed URL can carry a token. Calendar feeds attached
to a patch travel only for an admin of that patch, for the same reason the
URL is treated as a quasi-secret in ADR 002.

**5. People travel as stubs, and the set is a closure.** A person in a
member seamrip is four fields: id, username, display name, avatar. No
email, ever — that is the line between this bundle and the admin one, and
the reason the fork re-invites people out of band. No bio, no links, no
contact card, no instance role: a stub is enough to keep the memberships
whole, and nobody asked this person whether their profile should be copied
to a new server. A tombstone travels as the tombstone it is (docs/adr/086),
`deleted_at` included, or it would arrive on the fork as a live account
with its handle free to sign in under.

*Which* people travel is derived rather than declared: `MemberExport` reads
the schema's own foreign keys into `users` and carries exactly the people
the other travelling rows name. That set is, in practice, "everyone visible
through a membership the caller can read, an authorship, a seat, or an
attestation" — but stating it that way and maintaining a list by hand is
how a column added next year quietly falls out of it. The FK is the list.

**6. The bundle says which kind it is.** `instance.json` gains a `kind`,
and a member seamrip writes a `manifest.json` beside it naming the kind,
the requesting username, the time, and the instance domain, so a fork can
show where it came from. `cmd/import` reads it and says out loud that a
member seamrip carries no addresses and that its people must be invited
back. The two bundles import through the same code and arrive at very
different places, and that difference should not be discovered when nobody
can sign in.

## Consequences

- "The community can take its data and walk" becomes structurally true for
  members. The valve no longer depends on an admin's cooperation.
- Re-consent is the shape of the fork, not a defect of it. People join the
  new quilt by choice and set their own visibility there, which is what
  joining the old one was.
- A member seamrip of a quilt whose patches are mostly private is a small
  bundle, and that is correct. What a person can carry is bounded by what
  they were let in to.
- The mirrored charter text in an amendment is withheld more coarsely than
  the API withholds it. The API joins `target_doc` to a filename derived in
  Go (`governance.Filename`); the export asks the cheaper question, "is the
  viewer inside this patch, and does this patch keep any members-only
  charter at all". A non-member of such a patch loses the mirrored text of
  amendments to its *public* charters too. The error is in the direction of
  withholding, and the alternative was a second copy of a filename rule.
- The audit log gains `user.seamrip` beside `user.export`. An instance can
  see that somebody took a copy, which is the honest state of affairs: this
  is a right, not a secret.
- Rate limited to two per account per day, against five per fifty minutes
  for the personal export. The bundle is the whole quilt rather than one
  person's rows, and the binary has to run on a Raspberry Pi.

## Considered options

- **A parallel export implementation** — a `member_seamrip.go` holding its
  own queries per entity. Rejected: it is a fourth copy of the boundary,
  and ADR 002 is a document about what happens to copies of the boundary.
  The version that reuses the writer cannot drift, because there is nothing
  to drift from.
- **Carrying emails for people who consented** — a per-person "let my
  address travel in a member seamrip" switch. Rejected. It is a consent
  question nobody can answer well in advance ("to which fork, taken by
  whom, when?"), and the default would decide it for almost everybody
  either way. Worse, it converts the bundle into a partial contact list,
  which is the harvest vector ADR 012 rejected when it refused to make the
  full seamrip member-accessible. Out-of-band re-invitation is not a
  hardship in the communities this is for. They already have a group chat.
- **Letting the caller pick tables** — `?include=events,proposals`, or a
  UI of checkboxes. Rejected as a surface that looks like control and is
  not: every combination is a different bundle to reason about, no
  combination is more private than the whole one (the boundary already
  removed everything they may not see), and a fork missing a table it did
  not know it needed is a support burden the person cannot diagnose. One
  bundle, one rule.
- **Blanking the person's own hidden memberships too**, for symmetry with
  other people's. Rejected: the switch is theirs (docs/adr/006), the
  personal export already hands them those rows, and a fork that lost the
  exporter's own membership would place them outside the community they
  are standing back up.
- **Deriving nothing, and listing the people by hand** — a stated union of
  "authors, members, seat holders, attestation names". Rejected for the
  closure reason above. The hand-written list is right the day it is
  written and silently wrong after the next migration, and the symptom is
  rows quietly dropped at import.

## Status of ADR 012

Affordance 2 is shipped. Affordance 3, the moved-to pointer, remains
unbuilt. The user agreement's "any member can export what they can already
see", left standing as an outstanding claim when affordance 1 landed, is
now a description of `GET /api/v1/users/me/seamrip`.
