# An instance vouches for an app

Date: 2026-09-20. Status: **accepted**. Extends docs/adr/017 (the step-up
gate), docs/adr/013 (usernames are chosen) and docs/adr/025 (the flagship
gets no special powers); applies docs/adr/087's shape, that a domain proves
a thing and a person does not.

## Context

A native Patchwork client is being built. It browses any quilt whose
address a person types, which is the only honest shape for a client of a
self-hostable platform, and it needs a way to sign in. Sign-in on the web
is passwordless (docs/adr/017, docs/adr/020, docs/adr/071): an emailed
magic link, a passkey, an invite link, or a recovery code. Two of those
do not survive the move to a native client on their own, and both fail for
the same reason: they are bound to a domain.

**The magic link lands in the wrong place.** The email carries a URL. Tapped
from a mail app, it opens the system browser, the server sets the session
cookie there, and the client that asked for it is still signed out. The link
was designed for the case where the asker and the opener are the same
browser, and a native app is not one.

**A passkey is bound to the domain that made it.** The relying party is
`instance.domain`, and a platform passkey API will only answer for a domain
that has said, at a well-known address, which apps may ask. On Apple's
platforms that is `/.well-known/apple-app-site-association` naming a Team ID
and bundle identifier, and the app must carry the matching entitlement; on
Android it is `/.well-known/assetlinks.json` naming a package and its
signing certificate, and the assertion arrives with an `android:apk-key-hash`
origin the server must recognise. Neither side can be skipped and neither is
the project's to decide alone: the file is served by the instance, and the
entitlement is compiled into the app.

The obvious shortcut is to hardcode the project's own app in the served
file. That would make every instance vouch for one publisher's app without
being asked, which is precisely the centre of gravity docs/adr/025 refuses
to build: the protocol must never know which instance, or which app, is
"the" one.

The other shortcut is to route sign-in through a web view and harvest the
cookie. It does not work. The system authentication sheet hands back a
callback URL and never the cookies; an embedded web view can hold cookies
but is not permitted to use the domain's passkeys unless the app is
associated with the domain, which is the constraint being avoided. There
is no web detour that gets a native client a session without the server
knowing about native clients.

## Decision

**A sign-in code proves the same thing a link does.** Every magic-link
email now carries, beside the link, a six-digit code minted from the same
row. `POST /api/v1/auth/magic-link/verify` takes the address and the code
and answers exactly as the link's JSON branch does: a session cookie and the
user, or a signup token when the address is new (docs/adr/013 still holds;
no account is created until a username is chosen). A code is six digits, so
the row stops answering after five wrong guesses, and the link burns with
it, because the two are one proof of one mailbox and a burnt half is not a
half. Every failure is the same sentence, so the endpoint cannot be used to
learn whether an address was ever asked for. Nothing in this names a
platform: it is for any client that cannot receive the link in its own
session, and the web's own "check your email" panel accepts it too.

**An instance vouches for an app, and only an instance can.** A new table,
`native_apps`, holds the apps this instance's admins have listed, by
platform and identifier. It is empty on every instance until an admin adds
a row, and while it is empty the server serves neither association file:
an instance that has not listed an app has nothing platform-shaped at any
well-known address. Listing is an act at Administration → Settings → Apps,
and it is **step-up gated** (docs/adr/017), because vouching for an app lets
that app invoke the domain's passkeys, which is the same class of act as
pointing an account at a mailbox (docs/adr/072). Removing an app is not
gated: withdrawing trust is the safe direction. Both are audited.

**The files say what the table says, and nothing else.** The Apple file
carries `webcredentials` for the listed apps. It carries no `applinks`
block yet: routing the emailed link into an app is a separate decision
about which paths an app may claim, and it waits until an app handles
them. The Android file carries `get_login_creds` for each listed package
and its fingerprints, and the server adds each package's
`android:apk-key-hash` origin to the relying party's accepted origins, so a
listed Android app's assertion verifies rather than failing on origin.
Apple assertions already arrive with the `https://` origin the relying
party accepts.

**The table stays behind in a seamrip.** A fork has a different domain and
vouches for its own apps; carrying the list would make the fork's domain
assert something its admins never said.

**What the app lists is the app's business, and it is published.** An app's
entitlement is a closed list of domains, so native passkeys are a
convenience only listed instances get; that is a platform constraint, and
no server change lifts it. Three things keep it honest. Email sign-in, by
link or by code, is at full parity on every instance, so the gap is a
faster path and never a capability. The reference app is under the same
licence as the server, so anyone can ship it with their own domain list,
the way docs/adr/025 says anyone can publish a rival registry. And the
reference app's criteria for adding a domain are published with the app,
the way docs/adr/025 makes the flagship registry's inclusion criteria
openly editorial rather than quietly applied.

## Consequences

- A native client on any instance signs in by email with no server
  operator doing anything. A client on a listed instance also signs in by
  passkey, with the same passkey the person uses on the web.
- Step-up on an instance the app is not associated with falls back to
  recovery codes, which docs/adr/099 already made a proof of presence. The
  client should say so rather than offer a passkey button that cannot work
  there.
- The reference instance will list the reference app. That is one row an
  admin adds, recorded in the audit log, not a line in the source.
- The link-in-email UX for associated apps (an `applinks` block, and the
  path components it names) is deferred until a client handles the route.
  When it comes it is a second decision on this table, not a new one.
- An instance without SMTP prints the code beside the link in the server
  log, since there the log is the delivery channel (#222).

## Rejected

- **Hardcoding the reference app's identifiers in the served file.** Makes
  every instance vouch for one publisher without asking. See above.
- **Serving the files from patchwork.yaml.** The list is a trust decision
  about who may hold this quilt's passkeys, not a deployment fact like a
  port; it belongs beside neighbours and the label, editable by an admin
  with a step-up, and audited. Deployment concerns never lived in
  `instance_settings` (docs/adr/014) and this is not one.
- **A web view that harvests the cookie.** Cannot use passkeys on an
  unassociated domain and cannot read the cookie from the system sheet.
  It only looks like it avoids the server change.
- **An app-specific link scheme in the email.** Would put the app's name in
  every instance's mail, and would still open the wrong app for anyone
  with two clients installed. The code is client-neutral and works from a
  desktop mail reader to a phone.
- **Making the code longer or alphabetic.** Six digits is what every phone
  keyboard and one-time-code autofill expects. The attempt cap, the
  fifteen-minute life, and the per-address throttle carry the security,
  not the alphabet.
