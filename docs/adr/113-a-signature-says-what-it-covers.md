# 113. A signature says what it covers

**Status:** accepted, 2026-09-16

## Context

An inbound HTTP Signature carries its own list of which headers it signed.
`VerifySignature` read that list, rebuilt the signing string from exactly it,
and checked the arithmetic. When the list was absent it fell back to `date`
alone, which is what the draft specification says to do.

So the sender chose how much of the request its signature stood behind, and
three checks downstream were resting on values nobody had signed:

* `checkDateSkew` reads the `Date` header and treats it as authentic, on the
  stated ground that "the signature is valid, so the signed Date header is
  authentic". That is only true when the signature covers `date`. A signature
  that did not would leave the replay window being enforced against a number
  the replayer writes.
* `verifyInbound` compares `Digest` against the body only when a `Digest`
  header is present. A signature that does not cover `digest`, on a request
  that simply omits the header, leaves the body unauthenticated: a captured
  POST can be re-bodied inside the skew window and still verify.
* Without `(request-target)`, a POST captured for one inbox verifies just as
  well when replayed into another.

None of this was exploitable as it stood. The compensating checks are real,
and every implementation Patchwork actually talks to signs the full set. The
defect is that the guarantees were being supplied by the sender's good manners
and by checks that sit downstream of the verifier, rather than by the verifier
itself. That is the kind of thing that survives a refactor as a hole.

Found in an outside source-level review on 2026-09-16, alongside docs/adr/110.

## Decision

**The verifier names the set it requires, and rejects a signature that covers
less.** `requiredSignedHeaders` returns `(request-target)`, `host` and `date`
for any method, plus `digest` for the methods that carry a body (POST, PUT,
PATCH). Coverage is checked before the signing string is rebuilt, and the
refusal names the missing header.

The rule is a floor and not a shape: a sender that covers more is accepted
unchanged, and the order of the list is the sender's business.

**The set is what we already send.** `SignRequest` covers exactly these, and
`delivery.go` sets a `Digest` on every outbound activity, so quilt to quilt
delivery is unaffected. Mastodon and the other mainstream implementations sign
the same set. The spec default of `date` alone is now refused, which is the
point: it is a default, not a signature anyone meaningfully produces.

**The downstream checks stay.** Digest against body, actor binding, and the
five minute skew window are all still there. This ADR does not replace them,
it makes them true by construction instead of by assumption.

## Consequences

A remote server that signs less than the set can no longer deliver to this
instance, and the error in the log names what its signature failed to cover.
That is a deliberate interop refusal, and it is the reason this is an ADR
rather than a quiet patch: it is the first thing to look at if an inbox stops
accepting from one peer after this ships.

`TestVerifyRejectsASignatureThatCoversTooLittle` holds each missing header
separately, signing each case properly so the refusal has to be about coverage
and not about arithmetic. `TestVerifyAcceptsTheRequiredSetAndMore` holds the
floor open at the top.

One existing test had to change. `TestSignIncludesHostAndDetectsMismatch`
signed a POST with a body and no `Digest` header, so under the new rule it
would have been refused for the missing digest before reaching the host
comparison it exists to make. It now sets a `Digest` first, the way
`delivery.go` does. A test that passes for a reason other than the one in its
name is worth more care than the change that exposes it.

## The other note from the same review, and why it is a comment

The review also flagged the ignored `rand.Read` error in
`internal/auth/uuid.go`, suggesting a guard so a hypothetical failure could
not silently yield a low-entropy UUID.

There is no such failure to guard. On the toolchain this module targets,
`crypto/rand.Read` never returns an error: it always fills the buffer, and
crashes the program irrecoverably if the operating system source fails. A
guard would be a branch that cannot be taken, and code that cannot run is
worse than none, because the next reader has to work out what it was for.

The answer is a comment at the call site saying so, which is now there. The
finding was correct for an older Go and is worth knowing was considered, which
is why it is recorded here rather than dismissed in a review thread nobody
will find again.
