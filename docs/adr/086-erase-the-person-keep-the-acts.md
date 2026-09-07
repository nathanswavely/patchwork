# ADR 086: Erase the person, keep the acts

Date: 2026-09-07. Status: **accepted**; implemented. Extends docs/adr/017
(step-up), docs/adr/012 (leaving is a member right) and docs/adr/002
(the seamrip boundary). Issue #127.

## Context

Patchwork had no account deletion at all — not self-serve, and not
admin-side either. The only lever anyone had was **suspend**, which sets
`users.suspended_at` and revokes sessions. The row, the address, the
handle, the posts and the votes all stayed, and the shipped privacy policy
said so: *"There is no self-serve deletion button yet."* It also said the
stewards could "remove it with the administrative tools", which was
generous — those tools can suspend.

The button is the easy part. The hard part is that a `DELETE FROM users`
does not work here, and would be wrong even if it did.

**It does not work.** Most foreign keys into `users(id)` carry no
`ON DELETE` clause, so they are RESTRICT and the statement simply fails:
`proposals.author_id` and `applied_by`, `votes.user_id`,
`governance_docs.created_by`, `proposal_comments.author_id`,
`proposal_revisions.author_id`, `notices.author_id`,
`notice_replies.author_id`, `attestations.recorded_by`,
`amendment_attestations.recorded_by`, `nodes.owner_id` and `submitted_by`,
`events.created_by`, `event_sources.added_by`, `event_links.requested_by`,
`claim_requests.user_id`, `content_reports.reporter_id`,
`invite_links.created_by`, and the aggregator tables.

**And it would be wrong.** Those constraints are not an accident of
schema-writing. This is a platform whose stated point is that a community's
decisions are a durable, auditable record — the whole of docs/adr/055 is
about the record being assembled rather than written. A tally that quietly
loses a voter is not a record anybody can check, and a proposal with no
proposer is not a decision anybody can trace. Loosening those FKs so a
delete could succeed would trade the feature Patchwork exists for against a
statement about privacy that a tombstone can make just as well.

## Decision

**1. Deletion erases the person and keeps the acts.** The `users` row
survives as a **tombstone**: `deleted_at` set (migration 064), every
identity column emptied — email, display name, bio, avatar, links, contact
card, keypair, feed secret, and the preference flags that reveal how
somebody browses. Every RESTRICT foreign key is satisfied because nothing
is deleted, and every act the person took is still attached to a row that
still exists.

**2. Rows that are the person and nothing else are deleted.** Sessions,
credentials, recovery codes, notifications and notification preferences,
`user_quilts`, `remote_follows`, the `label_stewards` entry, their
`ap_followers` rows, **all memberships** in every patch and every role, and
claims still pending review.

`election_candidates` is on that list and `votes` and `election_ballots`
are not, which reads as inconsistent until you ask what each row is. A
ballot is a decision somebody made, and it stays. A candidacy is a standing
offer to serve, and an account that no longer exists cannot serve — leaving
it would let a tombstone win a seat, since `seatWinners` seats whoever the
tally returns. A pending claim goes for the same reason: it asks an admin to
hand a patch to somebody who will not be there to receive it. Settled claims
stay as the record of a review, minus the address they carried for it.

**3. Pointers the schema already wanted nulled, get nulled.**
`seats.holder_id`, `proposals.target_user_id`, and
`nodes.designated_successor_id` are all declared `ON DELETE SET NULL`, which
never fires because nothing is deleted; the handler carries out the schema's
intent by hand. `attestation_names.user_id` too — the name *text* stays,
because docs/adr/052 says the record may name anyone and it is the
community's sentence, not the person's row. A vacated seat is the holdover
case docs/adr/051 already knows how to sit out.

**4. The username is retired, not freed.** It stays on the tombstone, so
the UNIQUE constraint blocks re-registration forever and
`CheckUsernameAvailable` needs no change. Nothing renders it: the public
profile route 404s, the AP actor answers 410, WebFinger stops resolving it,
and every API surface that joins `users` for a name substitutes at the SQL
layer (`internal/handler/deleted_accounts.go`) so a deleted author arrives
at the frontend as "Deleted account" with a blank handle. Substituting in
SQL rather than in Svelte is deliberate: there are roughly twenty query
sites and many more render sites, and a component can only leak a name the
API sent it.

**5. Refused while somebody depends on you.** Deletion is refused when the
person is the **only active admin of any patch** — the response names those
patches by slug and name, and Settings lists them. It is refused when they
are the **last instance admin**, the invariant `internal/auth/bootstrap.go`
establishes when it makes the first account an admin.

The patch rule is exactly the floor `LeaveNode` already enforces, applied to
every patch at once, because deleting an account *is* leaving all of them
(docs/adr/012). That includes its one exception: on a `maintainer` patch
with a valid designated successor, leaving is the handover (docs/adr/051),
so deletion promotes the successor and proceeds. Making account deletion
stricter than the leave button would have been a rule nobody could explain.
A suspended account is not an admin present — the count asks whether
somebody can run this patch tomorrow.

**6. Immediate, step-up gated, typed confirmation.**
`DELETE /api/v1/users/me` sits behind `SudoRequired` (docs/adr/017) and
requires the username typed exactly. The step-up assertion is the safeguard
against a stolen cookie; sessions are revoked as part of the erase and the
client is sent home.

**7. Federation says goodbye and then stops answering.** Users do have
their own AP actors (`/ap/users/{id}`, a Person, with a keypair on the
`users` row and follower rows in `ap_followers`). Deletion queues a
`Delete` of that actor to every known follower inbox, and the actor
document then answers **410 Gone** rather than 404 — the fediverse reads
Gone as "existed, finished", which is what stops remote servers retrying.
Their relayed follows of remote patches are withdrawn through the existing
`relayUnfollow`, which re-counts first, so a patch somebody else here also
follows keeps its Follow (docs/adr/024).

**8. The tombstone travels.** `users` is already inside the seamrip
boundary (docs/adr/002), and `deleted_at` is added to its exported columns.
It has to be: a tombstone that arrived on the fork without it would be a
live blank account with a free handle and a profile page back on the web.
No new table, so `TestEveryTableHasABoundaryDecision` is untouched.

**9. The privacy policy is rewritten in the same commit**, per the standing
rule in `legal_defaults.go`: what is erased, what is kept and why, both
refusals, the vote-tally consequence below, and the two limits — federated
copies and exports taken beforehand — that Patchwork cannot reach into. The
false "the administrative tools can remove it" line is gone. The user
agreement's "Changes and ending" section now names self-deletion.

## Consequences

**A deleted person's open votes stop counting.** `countedBallot` asks who
is in the electorate *now*, not who was when the ballot was cast, so
deleting the memberships takes the ballot out of any tally that has not
resolved. The ballot row stays and still renders, marked uncounted. This is
not new behaviour invented here — it is exactly what already happens when
somebody leaves a patch mid-vote — but it does mean the honest sentence is
"every vote is still in the record", not "every vote still counts".
Anything already resolved keeps its stored outcome.

**Deletion is not reversible and there is no grace window.** See the
rejected alternatives.

**Instance admins lose a tombstone from the user list.** `GET
/api/v1/admin/users` filters them out: there is nobody to suspend, promote
or write to, and every control on that screen would act on an empty row.
`PUT /api/v1/admin/users/{id}/email` refuses a tombstone outright, because
pointing one at a mailbox would let whoever holds it magic-link into a
deleted person's row — the one thing deletion promised nobody could do.

**The audit log is the one place the retired handle is still legible, and
deliberately.** `user.deleted` is written with the acting user;
`audit_log.user_id` still points at the tombstone, so the actor column
renders as "Deleted account" like everywhere else, but the entry's metadata
names the username that was retired. Without it the entry reads
`user.deleted 01a07c77-…` and nothing on the instance can turn that id back
into a name — the admin user list no longer holds the row — which makes the
record useless for the question it exists to answer. This is not a leak of
anything that was ever private: the handle was on a public profile, and
retiring it is about nobody being able to *take* it, not about erasing the
string from history. Instance admins only.

**Every future join on `users` for a name is a place this can regress.** A
new surface that writes `COALESCE(u.display_name, u.username)` by hand
leaks a retired handle and nothing will fail. `displayNameExpr` and
`usernameExpr` exist to be reached for; the tests cover the surfaces that
exist today, not the ones somebody adds next year.

**Migration numbering.** 064. Three sibling branches were in flight beside
this one (they hold ADRs 083–085) and none of them adds a migration, so
the next number after main's 063 is the right one. Re-check before merge
per CLAUDE.md; a collision is renumbered on the side with fewer citations.

## Rejected alternatives

**Hard delete, loosening the FKs to make it possible.** Rejected — this is
the argument the whole ADR is about. Cascading a person out of the record
retroactively changes decided tallies, orphans proposals, and turns an
auditable governance history into one with holes at exactly the places
somebody would go looking. A community that cannot show how it decided
something has lost the thing Patchwork is for, and no deletion feature is
worth that. The tombstone gives the person the same erasure without it.

**One shared "deleted user" sentinel row that every deleted account is
repointed to.** Rejected on two counts. It requires rewriting every
authoring column across two dozen tables, which is a large migration whose
failure mode is silent misattribution. And it *merges* people: two deleted
members of the same patch become one row, so a proposal they both voted on
would show one voter, and a UNIQUE(proposal_id, user_id) on `votes` would
reject the second repoint outright. Per-account tombstones keep acts
distinguishable — which is what an auditable record requires — while telling
a reader nothing about who either person was.

**Freeing the username for reuse.** Rejected — a username is a public
identifier that has been printed on flyers, pasted into chats, and used as
the `acct:` handle in WebFinger. Freeing it lets a stranger register it and
inherit every inbound link, which is an impersonation surface handed out for
the sake of reclaiming a string. Usernames share a namespace with node slugs
and are already permanent for live accounts (docs/adr/013); retiring them on
deletion is the consistent direction.

**A grace window with a cancel link.** Rejected *for now*, and this is the
closest call in the set. A window is friendlier and gives a real out for a
compromised session, which is a genuine argument. Against it: the cancel
link has to arrive somewhere, and the mailbox is exactly what an attacker
who reached this button may already hold — so the window's safety is
borrowed from a channel the deletion is meant to sever. Meanwhile the
account has to keep half-existing for the duration, which means every
surface grows a third state between live and erased, and the person who
deliberately deleted their account is told it is still there. Step-up is a
better answer to the compromised-session case: a passkey assertion is proof
of presence, not of mailbox access. If a real instance reports people
deleting by accident, this is the decision to revisit — and the tombstone
does not stand in its way, since a window would be a delay before the same
erase.

**Blocking deletion behind "leave every patch first".** Rejected — it is
the same rule stated as homework. Deletion removes the memberships itself;
only the sole-admin case needs a human decision, and that is the only case
refused.
