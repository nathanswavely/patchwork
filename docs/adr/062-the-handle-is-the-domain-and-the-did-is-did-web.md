# ADR 062: The handle is the domain, and the DID must be did:web

Date: 2026-08-21. Status: **accepted**. Builds ADR 058's step B. Corrects
an overstatement in ADR 060 that surfaced while designing it.

## The correction, first

ADR 060 decision 3 says a `did:web` handle on a patch's own domain "is the
only version of a handle that survives the act seamrip exists to make
possible." Designing the thing showed that sentence claims more than it
can deliver, and the gap is worth naming before anything is built on it.

**A DID makes the identifier durable. It does not make the audience
durable.** Today's followers are ActivityPub followers, and a remote
server keys its Follow to our actor's `id` URL —
`https://<instance>/ap/nodes/<uuid>`. Nothing in ActivityPub resolves a
DID; Mastodon has never heard of one. So a forking patch that carries its
`did:web` handle to a new instance carries an identifier that no existing
follower is watching, and arrives with the same empty followers
collection ADR 060 described.

The audience becomes portable only when the follows are *to the DID* —
atproto-native follows, which need ADR 058's step D and a repository
somewhere, and constraint 1 says that somewhere is not us.

So step B is a **precondition** for closing ADR 060's gap, not the
closure. That is still worth building: an identity anchored on the
community's own domain is the part that cannot be retrofitted later, and
every other step needs it. But the honest claim is "the name survives,"
not "the audience does."

## Decision

**1. The handle is the verification domain.** atproto handles are
domains, and ADR 030 already vets one per patch on a trust anchor set
through admin or self-service verification. So the method asks nothing
new: it proves control of the atproto account whose handle *is*
`nodes.verification_domain`. No second field to keep in step, no second
anchor to reason about, and a patch with no vetted domain simply is not
offered the method — the same gate `dns` and `meta_tag` already sit
behind in `claimMethodsFor`.

**2. `did:web` only. `did:plc` is refused, deliberately.** This follows
from ADR 060's own reasoning rather than from taste. A `did:plc`
identifier resolves through `plc.directory`: a registry the community
does not run, cannot leave without cooperation, and does not own. That is
the hostage relationship ADR 060 objected to, moved one level out and
made harder to see. A `did:web` resolves at
`https://<domain>/.well-known/did.json` — served by the same domain the
claim was already anchored on, under the community's own control.

A second reason surfaced while wiring the seamrip boundary, and it is the
stronger one. `verification_domain` **stays behind** on a fork —
"vetted by the old instance's claim review" — while `did` travels. That
asymmetry only works because a `did:web` value *names its own document's
location*: `did:web:tellus.example` tells the fork exactly where to
re-prove itself, against the community's own domain, trusting the old
instance for nothing. A `did:plc` says nothing without asking
plc.directory what it means, and a bare `verification_domain` is a
judgement rather than a fact. **Being self-locating is what makes an
identity portable**, which is the property ADR 060 was reaching for.

This rejects the common case on purpose: most atproto accounts in the
world are `did:plc`. A patch whose handle resolves to one is told plainly
why it is not accepted, rather than being quietly failed. If that proves
too strict in practice it is a decision to revisit with evidence, not a
default to soften pre-emptively.

**3. Verification is bidirectional, because atproto's is.** Domain
control alone is not the claim being made. Both directions are checked:

- **Handle → DID.** A TXT record at `_atproto.<domain>`, or
  `https://<domain>/.well-known/atproto-did`. Whichever answers first;
  a patch needs only one.
- **DID → handle.** Fetch the `did:web` document and require
  `alsoKnownAs` to contain `at://<domain>`.

One direction alone is forgeable: anyone who can publish a DNS record can
point a handle at somebody else's DID, and any DID document can name a
handle it does not hold. Requiring both is what makes the binding mean
anything, and it is the rule atproto itself applies.

This makes the method **stronger than the three it joins**, which prove
domain control against a token Patchwork itself issued. Worth stating
because it inverts the usual expectation that the newest method is the
weakest.

**4. A verified DID is recorded and does nothing else yet.**
`nodes.did`, set when the claim verifies. No new surface, no change to
any actor document, no effect on federation. ADR 049 cuts both ways: a
stored fact that makes no claim to anybody needs no page, and inventing
one before steps A or D exist would be the void ADR 054 warned about.

## Considered and rejected

**A separate handle field, so a patch can use any atproto identity.**
More flexible and strictly worse. It would create a second identity
anchor beside `verification_domain`, with no rule for what happens when
they disagree, and it would let a patch bind an identity on a domain
nobody vetted — dissolving the property that makes the claim flow
trustworthy in the first place.

**Accepting `did:plc` with a warning.** Rejected because the warning
would be the only thing standing between the project and exactly the
dependency ADR 060 exists to refuse, and warnings lose to convenience
every time. The strictness *is* the feature.

**Replacing `dns` or `meta_tag` with this.** They prove a different,
weaker thing and remain the right answer for a venue with a website and
no atproto presence at all, which is nearly all of them today.

## Consequences

- One migration for `nodes.did`. `claim_requests.method` has no CHECK
  constraint, so the new value needs no schema change — a fact worth
  recording, since it is not obvious from reading migration 031.
- **`nodes.did` travels while `verification_domain` beside it does not.**
  Discovered rather than planned: the boundary already refuses the domain
  as the old instance's vetting judgement. The DID travels because it is
  re-checkable without us, which is decision 2's second reason. The two
  sitting in one table with opposite answers is correct, and the `def()`
  entry says why so nobody "fixes" the inconsistency later.
- The column list in `Tables()` must mirror its `SELECT` **in order** —
  positional, not by name. Getting that wrong skips every node row and
  cascades to every table that references one, which the round-trip test
  reports as "skipped 2 rows" rather than as a mismatch.
- ADR 060's decision 3 keeps its number and its reasoning; the
  overstatement is corrected here rather than edited there, so the record
  shows what was believed when.
- Resolution runs through `internal/safehttp`, like `ap` and
  `eventsource`. A DID document is fetched from a domain a stranger
  chose, which is the SSRF shape that package exists for.
