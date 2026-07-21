# Claims are assertions, not reservations

The first claim flow treated a claim as a lock: one pending claim on a
patch blocked every other claimant (and the original claimant, who had no
way to withdraw or even see their claim after a page reload). Combined
with no expiry, one stuck claim — including one opened via the email
method, which was a stub that could never complete — froze a patch's
claimability forever, and a bad actor could freeze every unclaimed patch
on the instance with one request each. It also anchored DNS and meta-tag
verification on the node's `website` field, which any authenticated user
can supply through community submissions: submit a fake listing for a
real venue with your own URL, then "prove" you control the website you
yourself wrote down.

We decided a claim is **an assertion of ownership pending proof, never a
reservation**, and that proof anchors on a domain the platform vetted,
never on user-supplied display data:

- **Claims run concurrently.** Anyone may open a claim regardless of
  others' pending claims; the only uniqueness rule is one open claim per
  user per patch. First proof (or admin approval) wins;
  `transferOwnership` auto-rejects the siblings, as it already did. For
  competing admin-review claims, the admin seeing both side by side beats
  first-come-first-served. This removes the lockup abuse vector entirely
  rather than rate-limiting it.
- **`withdrawn` is a first-class status.** Claimants can withdraw their
  own pending claim from the claim page. `rejected` keeps meaning "the
  system or an admin said no" (including sibling auto-rejection);
  the audit trail never records a claimant as their own reviewer.
- **`verification_domain` on the node is the sole trust anchor** for all
  self-verification (DNS TXT, meta tag, email). It is set only through
  admin and trusted paths: auto-derived from the website when an instance
  admin or trusted contributor supplies one, suggested to the admin at
  submission-approval time, editable by admins — never writable by
  ordinary submitters. A shared-platform blocklist (gmail.com,
  facebook.com, linktr.ee, …) refuses auto-derivation, because a small
  org's "website" is often a Facebook page and a shared platform must
  never anchor ownership proof. `website` itself is cosmetic and carries
  zero trust. No vetted domain → the only method is admin review.
- **Email verification is real and domain-locked.** The claimant supplies
  a full address whose domain must exactly match the verification domain
  (no subdomains until someone real needs them). A single-use token link
  is mailed; the SPA link page requires an explicit confirm click (mail
  scanners prefetch GETs), and completion transfers to the claimant's
  account regardless of who clicks — possessing the link is the proof.
  Tokens expire after 24 hours and can be re-sent, rate-limited. The
  method is offered only when SMTP is configured; without it the link
  prints to the server log like magic links do.
- **Pending claims expire after 30 days** (the schema's until-now-unused
  `expired` status), swept on the existing reminder worker tick. Expiry
  is hygiene, not security: it keeps the admin queue meaning "needs you
  now," and an expired claim costs the claimant nothing but re-opening.

## Rejected alternatives

- **Keep the lock, add expiry + withdraw.** Still abusable within the
  window (re-open a fresh claim every N days), and a reservation semantic
  buys nothing: an unproven assertion shouldn't exclude provable ones.
- **Anchor verification on `website` gated by provenance.** Less schema,
  but it turns a display field into a security property and makes every
  future edit path to `website` a verification-bypass surface.
- **Reuse `rejected` for withdrawal.** No migration, but it muddies the
  claim history and fakes a reviewer.
