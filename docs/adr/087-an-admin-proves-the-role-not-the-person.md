# ADR 087: An admin proves the role, not the person — a signed nonce with a fifteen-minute life

Date: 2026-09-07. Status: accepted.

## Context

Somebody who administers a Patchwork instance has no way to demonstrate it
to anyone outside the instance.

The case that raised it is mundane and awkward. A hosting provider holds a
billing contact for a server, that contact leaves the collective, and the
person who now actually runs the quilt writes in to have it changed. The
provider asks, reasonably, how they are supposed to know. Everything the
admin can offer is either unverifiable — "I'm the one who runs it" — or
disproportionate: a DNS record on the domain, which needs registrar access
the admin may not have; a file dropped at a well-known path, which needs
shell access to the box. Both prove control of infrastructure rather than
the role, and both are exactly the access a community organizer running on
somebody else's donated VPS is least likely to hold. The same shape shows
up when a directory checks a submission, when one quilt's stewards vouch
for another, and when a grantmaker wants a claim on a form to be worth
something.

What the outside party is asking is narrow: *is the person writing to me an
admin of that quilt?* Not who they are. Not how many admins there are. Not
which one.

Two existing decisions constrain the answer sharply.

**ADR 023 refused to publish the admin roster**, in as many words:
auto-publishing the names and faces of everyone with root on an antifascist
organizing platform builds a targeting list out of a trust feature. The
Label names *stewards*, who opt in one by one and are deliberately not a
view of the admin role. `GET /api/v1/instance` carries a steward count and
never a handle, because that endpoint is CORS-open and registry-aggregated
and handles there would make "enumerate every Patchwork admin" a one-line
scrape. Any proof-of-role feature that leaks who, or how many, walks
straight back into what 023 closed.

**ADR 017 established what an outward-facing admin action costs.** Three
actions take a fresh WebAuthn assertion rather than a month-old cookie:
wipe, export, and promotion — later joined by setting an account's email
address (ADR 072). The common thread is that each hands something outward
or is irreversible, and a valid session proves identity where what is
needed is presence.

The instance already has the material for a proof. ADR 024's **instance
service actor** is an Application actor with an RSA keypair, minted per
instance, whose public half is already published in its actor document. It
exists so that no person's actor is enumerable in a remote followers
collection — it is, by construction, the instance speaking as itself
without naming anybody. That is precisely the voice this needs.

## Decision

**An instance admin can have the quilt sign a nonce the verifier chose. The
statement names the domain, the claim, and the times — never a person.**

The exchange is three steps and no accounts:

1. The verifier hands the admin a nonce out of band — any printable string
   of 8 to 64 characters. It is theirs, so a reply built for them cannot be
   a reply built for somebody else and forwarded.
2. The admin, signed in and stepped up, pastes it at Administration → Prove
   Admin and gets a blob back.
3. The verifier fetches the quilt's public key from the quilt's own domain
   and checks the blob.

**The statement is exactly six fields**, and every one of them is about the
instance:

```json
{
  "claim": "instance-admin",
  "domain": "arts.lancaster.example",
  "nonce": "<theirs>",
  "issued_at": "2026-09-07T14:03:00Z",
  "expires_at": "2026-09-07T14:18:00Z",
  "key_url": "https://arts.lancaster.example/api/v1/instance/attestation-key"
}
```

No username, no display name, no email, no user id, no admin count. A test
asserts the field set is exactly these six and fails on any addition, so
the next person who reaches for "and who signed it" has to argue with ADR
023 rather than with a code review. **What it proves is that an admin of
this quilt signed this string at this time, and nothing else.** The UI says
so in those words, beside the button, because the failure mode of a proof
feature is somebody downstream reading it as identity.

**It is signed with the instance service actor's key.** One instance, one
identity, one key — the same RSA keypair that signs ActivityPub deliveries.
A second keypair minted for this would be a second thing to rotate, a
second thing to lose, and a second thing a verifier has to be told about.

**The format is detached-JWS-style compact serialization:**
`base64url(payload) "." base64url(signature)`, both unpadded, the signature
RSASSA-PKCS1-v1_5 over SHA-256 — RS256 — computed over the ASCII bytes of
the *first segment* rather than over the decoded JSON. Signing the encoded
text is what keeps verification free of any JSON canonicalization question:
the exact bytes that were signed are sitting in the blob, and a verifier
never has to reproduce our key order or our spacing. RS256 is what the AP
code already does; a verifier in any language has a library for it, and a
verifier with nothing but a shell has `openssl dgst -verify`.

**The algorithm is fixed by the format and never read out of the thing
being verified.** There is no `alg` field in the payload. A verifier that
takes its algorithm from the token it is checking is the oldest hole in
signed-token design, and the cheapest way not to have it is not to offer
the field.

**`key_url` is inside the signature and pinned to `domain`.** Verification
fails if they disagree, and the shipped verifier builds the URL from the
claimed domain rather than reading it out of the blob. This closes the
mistake the format most invites: fetch the key the blob names, verify
against it, and conclude that whoever signed runs a domain they have never
touched. The field is still carried, because telling a verifier where to
look is worth a line, but it is a convenience that cannot be steered.

**The public key is served outside the federation gate**, at
`GET /api/v1/instance/attestation-key` — public, unauthenticated, cacheable
for an hour, returning the PEM and the algorithm name. Today `/ap/instance`
is mounted only when `federation.enabled`, and a proof that stops working
the day an instance turns federation off is not a proof anybody can rely
on. For the same reason `EnsureInstanceActor` moves out of the federation
block at startup and runs on every boot: the cost is one RSA keypair, once,
on a database that has never had one.

**Issuance is instance-admin only and step-up gated** (ADR 017), matching
promotion, export, and setting an address. The blob leaves the building and
stands on its own for fifteen minutes; that is the shape of every other
action on that list. It is rate limited modestly — ten up front, refilling
one a minute, per admin — as a ceiling on signing work rather than as a
second gate.

**Fifteen minutes.** Long enough to paste into a support ticket while
somebody is on the phone; short enough that a blob found later in a mail
archive proves nothing. The window is the whole reason a stale attestation
is not a standing credential.

**Every issuance is audited as `admin.attestation_issued`, with the
nonce.** A dispute is always the same question — did anybody here sign this
string — and the audit log is the only thing that can answer it. The blob
itself is deliberately not stored: it is reproducible from the key, it is
worthless after fifteen minutes, and a table of live proofs is a thing to
steal.

**A verifier is given two ways to check and needs neither an account nor
our goodwill.** `patchwork -verify-attestation <blob>` fetches the key from
the claimed domain and prints a verdict; `internal/attest.Verify` is an
importable Go function for anyone building on top; and DEPLOYMENT.md
carries an `openssl` recipe for a host that has a shell and no wish to run
our software. The importable function is the point of putting the format in
its own package rather than inside the handler.

## Consequences

The instance service actor's keypair is now load-bearing for something
other than federation. Rotating it — which nothing does today — would
invalidate outstanding attestations, which is harmless given the
fifteen-minute life, and would break AP followers, which is not. That
tradeoff is unchanged by this decision, but it is now a second reason not to
rotate casually.

Every instance mints a keypair on first boot whether or not it federates.
On a Pi that is a fraction of a second, once.

The claim is instance-scoped. A *patch* admin proving they run a patch is a
different question with a different blast radius, and this format does not
answer it — `claim` is a field rather than a constant precisely so that a
later decision could add one, but adding one is a decision, not an
extension.

Nothing here tells a verifier what to do with the answer. A provider who
accepts a signed nonce as authority to re-point billing has decided that
whoever administers the quilt may direct the account, which is a policy
call about their own product. Patchwork's job is to make the fact
checkable, not to make it sufficient.

## Considered options

- **An admin-only page displaying a verifier-supplied code** (the issue's
  option 1): rejected. Without a new endpoint the only way the verifier's
  code reaches the page is the URL, so the screenshot proves possession of
  a link — which the verifier themselves sent — and not the role. Anyone
  the link was forwarded to produces the same picture. It also produces an
  image rather than something checkable by a machine.
- **An authenticated "what is my role here" endpoint** (the issue's option
  3): rejected. It answers the question correctly and to the wrong party.
  The verifier would have to hold a session on the instance, which means
  the admin either shares a cookie or the verifier gets an account — the
  first is a credential handover, the second turns every billing enquiry
  into a signup. The whole difficulty is that the party who needs the
  answer is outside.
- **A bare timestamped attestation with no nonce**: rejected as replayable.
  A blob that says only "an admin signed this at 14:03" is as good in
  anybody's hands as in the admin's, and forwarding one is indistinguishable
  from producing one. The verifier's own string is what makes the reply
  theirs.
- **Naming the signing admin in the statement**: rejected — this is ADR
  023's roster arriving through a side door, one blob at a time, and a blob
  is forwardable in a way a page visit is not. The verifier's question does
  not require it, and where a human name genuinely helps, the Label already
  publishes stewards who chose to be published.
- **Publishing an admin count** (as a hint that a proof is meaningful):
  rejected — ADR 023 allows a *steward* count on `/api/v1/instance` because
  stewards opted in; an admin count is a fact about the security posture of
  a machine and says how many keys there are to steal.
- **A long-lived or non-expiring attestation**: rejected. It is a bearer
  credential with no revocation, sitting in somebody's inbox. The fifteen
  minutes is what keeps this a proof of a moment rather than a token.
- **DNS TXT or a well-known file**: rejected as the primary mechanism. Both
  prove control of infrastructure rather than the role, and both need
  registrar or shell access that the admin who most needs this is least
  likely to have. Nothing stops a provider asking for one as well.
- **A second keypair minted for attestation only**: rejected — a second key
  to rotate, lose, back up, and explain, for no separation that matters. The
  instance actor is already the instance speaking as itself.
- **Full JWS with a header segment**: rejected. Three segments and an
  algorithm-negotiation field, to carry one algorithm that never varies.
  The header's only content would be the `alg` we deliberately refuse to
  read.
- **Serving the key only under `/ap/instance`**: rejected — it already
  exists there, and it is gated on `federation.enabled`. Whether a quilt
  federates has nothing to do with whether its admin can prove the role.
- **Storing issued attestations**: rejected. The audit row answers the
  dispute; the blob adds a table of live proofs whose only use is to
  somebody who should not have it.
