# ADR 071: Every account leaves signup with a way back in

Date: 2026-08-27. Status: accepted.

## Context

An account on the Lancaster instance could not be signed into. Its holder
tapped "Sign in with passkey" on Android and got the browser's own words for
a failed ceremony — *"The operation either timed out or was not allowed"* —
which reads like an outage. It was not. The account had no passkey, and had
never had one.

It was made through an invite link, which asks for a username and nothing
else. Redemption hands back a session, and the next screen offers "Create a
passkey" beside **"Skip for now"**. Skipping is the ordinary thing to do on
a phone you are borrowing, in a hurry, or when the prompt is confusing. The
session then carried the person for as long as it lasted, and everything
worked. When it expired there was nothing left: no passkey, no recovery
codes, and — because the invite path never asked — no email address to send
a link to.

Nothing in the product could fix it. `PATCH /api/v1/admin/users/{id}` takes
`role`, `suspended_at`, and `trusted_contributor`; there is no field for an
address, so not even the instance admin could open a door. The repair was a
hand-written `UPDATE users SET email` against the production database.

Three separate things had to be true for this to happen, and all three were:

1. Signup could complete with **zero** authentication factors.
2. The failure was **silent** — an account with no way in looks exactly like
   a working one until the session runs out, which can be weeks later.
3. The error at the end named none of it.

## Decision

**An account may not leave signup without at least one credential that
outlives its session.** Which credential depends on what the instance can
actually do:

| SMTP | The floor | The fast path |
|---|---|---|
| configured | an email address, required at invite signup | passkey, optional |
| not configured | recovery codes, generated and acknowledged | passkey, optional |

`GET /api/v1/instance` gains `email_enabled` so the signup page knows which
of the two it is offering. Where mail works, "Skip for now" stays and is
finally harmless — there is a door behind it. Where mail does not, the
escape hatch from the passkey step is "Use recovery codes instead", and the
button out of signup stays disabled until the codes are acknowledged.

Two supporting fixes fell out of the same investigation:

**Registration now asks for a discoverable credential.** Sign-in is
`BeginDiscoverableLogin` — an empty `allowCredentials`, which only a
client-side discoverable credential can answer. But `webauthn.Config` set no
`AuthenticatorSelection`, so registration sent an empty one, and an absent
`residentKey` means *discouraged*. An authenticator was free to hand back a
credential sign-in could never find again. Syncing passkey providers store
everything discoverably and hid this for the entire life of the feature; a
security key would have enrolled happily and then been useless. The config
now names `residentKey: required`.

**The browser's error text no longer reaches the page.** Every ordinary
outcome — no passkey on this device, prompt dismissed, prompt timed out —
arrives as one indistinguishable `NotAllowedError`. `passkeyErrorMessage()`
in `web/src/lib/webauthn.js` maps it per ceremony: on sign-in it says no
passkey was found *and points at the email link*, which is the door that is
open. `passkeysSupported()` hides the button outright where WebAuthn is
absent, rather than offering one that cannot work.

Email is also normalized — trimmed and lowercased — at every entry point.
Account lookup is `WHERE email = ?`, an exact match, so an address stored
with a capital letter misses its own row on the next sign-in, and the magic-
link path reads that miss as a new visitor and offers to build a *second*
account. That is the same lockout by a different route.

## Rejected

**Make magic links the only way in.** It is the tempting simplification —
one door, always present, nothing to skip. It makes SMTP mandatory, and
Patchwork's claim that "invite links + passkeys still work without it" is
load-bearing: the invite link exists for the community organizing off a
flyer and a QR code, with no mail server anywhere. It also makes the weakest
credential the only credential — an email compromise becomes a full account
takeover, where a passkey is phishing-resistant by construction — and puts
the slowest path (leave the site, open mail, come back) on every sign-in.
The problem was never that there were several doors. It was that there could
be none.

**Make the passkey step mandatory instead.** Enrollment fails for reasons
the person cannot fix in the moment — a borrowed device, a browser with no
WebAuthn, a passkey provider mid-setup. A required step that can fail with
no way around it is a locked front door rather than a locked back one.

**Auto-issue recovery codes to everyone.** Codes nobody was asked to save
are codes nobody saved, and they would let the real floor — an address that
can be mailed — go on being skipped while looking covered.

**Let an admin attach an address after the fact.** Worth having, and does
not solve this: it repairs the lockout instead of preventing it, and it
hands an admin a way to point any account at an address they control.
