# ADR 072: An admin can attach an address, and the old one is told

Date: 2026-08-28. Status: accepted.

## Context

docs/adr/071 closed the door that strands an account: no signup completes
without a credential that outlives its session. It does nothing for the
accounts already through it. `hswavely` on lancasterpatchwork.org has no
passkey, no recovery codes, and no email address, and 071 leaves it exactly
where it was — unreachable by every path the product offers.

071 considered this endpoint and rejected it, for two reasons. The first is
right and stands: it repairs the lockout instead of preventing it, so it is
not the fix. The second is the one worth answering — *it hands an admin a way
to point any account at an address they control.*

That objection is real and it is not a reason to withhold the endpoint,
because the alternative is not "no repair". It is the repair that already
happened: SSH to production, open the SQLite file, hand-write
`UPDATE users SET email = ...` next to a UNIQUE column, with no record that
anyone did it and nobody told. Every hazard 071 named is present in the SSH
version, plus the ones a text editor adds. The choice is not whether an admin
can point an account at a mailbox. It is whether doing so is gated, recorded,
and visible to the person it happens to.

The same shape already has an answer here. Promotion to instance admin is a
privilege escalation an admin can perform on any account, and docs/adr/017
does not forbid it — it requires presence, a fresh passkey assertion rather
than whatever cookie is lying around. This is that, plus the part promotion
does not need: the account holder finding out.

There is a second, duller reason to have it. An address typed with a typo is
the same lockout arriving by accident, and it has no repair either.

## Decision

**`PUT /api/v1/admin/users/{id}/email`, step-up gated, audited under its own
action, and announced to the address being replaced as well as the one
replacing it.**

**Its own route, not a field on `PATCH /api/v1/admin/users/{id}`.** That PATCH
carries `role`, `suspended_at`, and `trusted_contributor`, one of which is
dangerous — so its step-up check is conditional, written inside the handler,
reading the target's current role to tell promotion from demotion. That is the
right shape when a dangerous field sits beside harmless ones, and the wrong
shape to reach for twice. A route whose only field is the dangerous one takes
`middleware.SudoRequired` whole, in the router, where it can be read. It also
means the gate cannot be quietly widened later: the way past a gate is a field
added beside it, and this route has no room for one.

**Normalized through `auth.NormalizeEmail`** (docs/adr/071), the same function
every other entry point uses. Account lookup is `WHERE email = ?`, an exact
match, so an address stored as the admin happened to type it is an address the
person cannot sign in with — a repair that appears to succeed and fails weeks
later is worse than one that fails now. The response returns the stored form,
so the admin can tell the person what to type rather than reciting what they
typed.

**An address another account holds is refused, checked case-insensitively.**
`users.email` is UNIQUE, so the write would fail regardless — but it would
fail as `UNIQUE constraint failed: users.email`, which is not an answer. The
case-insensitive part is not defensive tidiness: rows written before 071
normalized anything can hold a mixed-case address, and the UNIQUE index is
case-*sensitive*, so it would happily seat a lowercase twin beside one. That
is the collision this check exists to prevent, and the index cannot see it.

**Audited as `admin.user_email_set`, with both addresses.** Not
`admin.user_update`. The question someone brings to the audit log is whether
an admin pointed an account at a mailbox they control, and an entry reading
"user updated" cannot answer it. Both addresses are recorded because the
answer is which mailbox — admins can already read every address in the users
list, so the entry withholds nothing they did not have.

**The old address is told, not just the new one.** The new address reaches
whoever now holds that mailbox, which in the case worth worrying about is the
admin. The old address reaches the person who can say this was wrong. It goes
out through `mail.Send` directly rather than through the notification
channels, for two reasons: those resolve their recipient by user ID, so they
would only ever deliver to the address just written, and they are
preference-gated, which is the wrong disposition for a notice that your
account changed hands. An in-app notification is written as well, so the
change is visible from inside the product and not only from whatever mail did
or did not do.

**Setting the address a user already has is a no-op** — no audit entry
claiming a change, no notices announcing one.

## What this does not do

It does not make the address verified. Patchwork has never verified an
address; delivery of a magic link is the proof, and that is unchanged. An
admin asserting an address is exactly as trustworthy as that admin, which is
what the gate, the log, and the two notices are for.

## Rejected

**A `email` field on the existing PATCH.** The smaller diff, and it puts the
route's most dangerous capability in the one handler that already has to
reason about which of its fields need presence. docs/adr/053 turned on a
related lesson: `governance-rules.json` is excluded from attestation because
machine configuration riding along beside adopted text is a two-step route
around the leadership gate. A general-purpose PATCH is where a quiet field
goes to hide.

**Let an admin clear an address.** `users.email` is nullable, so the column
allows it, and the endpoint does not. Removing an account's only remaining
credential is the lockout being *created* by the tool built to repair it.

**Revoke the account's sessions on change.** It looks like the cautious
option and buys nothing: an address takeover does not need the old sessions,
because the new address can request a magic link. What it does buy is signing
out a person who may be legitimately signed in at that moment — the ordinary
case punished for the benefit of no case at all.

**Require the account holder to confirm at the new address.** Correct for a
change the *user* initiates, and exactly backwards here. The account this
exists for is the one nobody can sign into; a confirmation link the stranded
person cannot click is the lockout with an extra step.

**Leave it to SSH.** The status quo. It requires production database access,
puts a hand-written `UPDATE` beside a UNIQUE column with no collision check,
tells the account holder nothing, and leaves no record — the least witnessed
version of the act 071 was right to be wary of.
