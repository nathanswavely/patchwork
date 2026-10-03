# A link knows where it was asked for

Date: 2026-09-28. Status: **accepted**. Amends
docs/adr/2026-09-20-an-instance-vouches-for-an-app.md (the code moves out
of the email and onto the page the link opens); keeps docs/adr/013
(usernames are chosen) and docs/adr/017 (the step-up gate) as they are.

## Context

Since docs/adr/2026-09-20-an-instance-vouches-for-an-app.md every sign-in
email carries two things: a link, and a six-digit code for a client that
cannot receive the link in its own session. That fixed native clients and
second devices, and it left three problems.

**The email asks the reader to choose.** "Click this, or if you asked from
somewhere else, type this" makes the person work out which case they are
in. They usually know where they asked, but the email cannot know where
they are reading it, so it has to present both and explain both.

**The link signs in whichever browser opens it.** A person who asks on a
laptop and taps the link on their phone ends up signed in on the phone and
still signed out on the laptop, having spent the link and the code with it.
The link has no idea where it was asked for, so it cannot tell the case it
was built for from the case the code was built for.

**The link is spent by a GET.** `GET /api/v1/auth/verify/{token}` marks the
row used and sets a session cookie in the response. Mail security scanners
fetch links before a person sees them. Today a scanner that follows the
link spends it, and the person's click then answers "magic link already
used" (the same symptom #222 traced to throttling). Nothing an unattended
fetch does should decide a sign-in.

Other products solve the first two the same way: the page the link opens
checks whether it is the browser that asked. If it is, it signs in. If it
is not, it shows a code to type on the device that asked. Patchwork already
has every part of that except the check.

## Decision

**Asking for a link marks the browser that asked.** `POST
/api/v1/auth/magic-link` sets a short-lived cookie holding a random nonce
(HttpOnly, Secure, SameSite=Lax, scoped to `/api/v1/auth`, fifteen minutes,
the row's own life), and the row records the nonce's hash as
`requester_hash`. A browser that asks twice while the cookie lives reuses
it, so both rows are bound to it. A client that sends no cookie back (a
native app, a CLI, `curl`) leaves the row unbound. Nothing about the
response changes: the blanket 200 still says nothing about whether the
address has an account.

**Opening the link does nothing by itself.** The emailed URL is a page in
the SPA. The GET routes that spend a token today, including the
`/auth/verify/{token}` alias for old mail, stop spending anything and
redirect a browser to that page. The page makes one POST,
`/api/v1/auth/magic-link/open` with the token, and the server answers one
of three ways:

- **This is the browser that asked** (the cookie's hash matches
  `requester_hash`): the row is spent and the person is signed in, or sent
  to choose a username when the address is new (docs/adr/013), exactly as
  the link does today. No button, no code; this is the common case and it
  stays one click.
- **This is not the browser that asked, or nobody bound the row:** nothing
  is spent. The page shows the code and says to enter it where sign-in was
  requested. The code is redeemed at
  `POST /api/v1/auth/magic-link/verify`, unchanged. Opening the link again
  shows the same code.
- **The row is spent, expired, or unknown:** one sentence, as now.

A scanner that fetches the URL gets the SPA shell and never runs the POST.
One that does run it has no cookie, so it lands in the second case, which
spends nothing and hands the code only to something that already holds the
link.

**The code page offers "Sign in on this device".** Asking on one device and
wanting to be signed in on the one you read mail on is common enough that
the code page must not make it impossible, and today's behaviour is exactly
that. A button under the code posts
`/api/v1/auth/magic-link/open` again with `here: true`, which signs this
browser in and spends the row, so the code stops working. It is a button
and never the default, for two reasons. The asking device is the one the
person was looking at when they asked, so that is where the code should
work. And a click that signs in the browser it happens in is the one thing
an unattended fetch must never be able to do; a deliberate press is the
proof that a person is there.

**The click never approves another device.** Some products let the link
itself sign in the device that asked, with that device waiting and
polling. That makes an unsolicited email dangerous: someone types your
address on their own machine, you click a link you did not ask for, and
they are in. Here the code has to be carried to the asking device by hand,
so clicking a link you did not ask for grants nobody anything. The page
says so in one line, and says not to give the code to anyone. That is the
same exposure the email has today with the code printed in it, and no
wider.

**The email carries the link alone.** One thing to press, one sentence
about expiry. The code is no longer in the email, because the page the
link opens is now where it is shown. A person who asked from an app taps
the link on the same phone, the browser opens, it is not the browser that
asked, and the code is on screen to type into the app. An instance without
SMTP still prints both the link and the code to the log, because there the
log is the delivery channel (#222) and the operator reading it may be the
client.

**The row keeps the code, not only its hash.** Showing the code again on a
later open means the server has to be able to read it. The hash was never
what protected it: a six-digit code's sha256 is recovered by trying all
million of them, which takes well under a second, so migration 20260920's
claim that a read of the database yields nothing replayable was never true
of the code (it is true of the 32-byte token, which keeps its hash). What
bounds a code is its fifteen-minute life, single use, and the five-guess
cap on the row, and all three are unchanged.

## Consequences

- Asking and clicking in one browser is one click, as now. Asking in one
  place and clicking in another shows a code instead of silently signing
  in the wrong place.
- A native client needs nothing new. It keeps asking without a cookie and
  keeps redeeming codes; the person now reads the code off the link's page
  instead of the email.
- A scanner can no longer spend a link, which removes one cause of "magic
  link already used".
- Clearing cookies, or a browser that drops them, between asking and
  clicking turns the one-click case into the code case. The code and the
  button both still work, so it costs a step and never a sign-in.
- Mail sent before the change still carries a code and a link. Rows live
  fifteen minutes, so this is a deploy-day edge and needs no handling: the
  code works, and the link lands on the new page.
- When `applinks` arrives (deferred in
  docs/adr/2026-09-20-an-instance-vouches-for-an-app.md), an app that
  opens the link itself can redeem the token through the same `open`
  endpoint with `here: true`, and the code page is never shown.
- New columns on `magic_links` (`requester_hash`, the code), a new route,
  and a new cookie. The route is open to anyone by design and is named in
  `openToAnyone` with that reason.

## Rejected

- **Keep the code in the email as well.** It puts the choice back on the
  reader, which is the first problem. The code is one tap away on the
  link's page for everyone who has the email.
- **Sign in on the clicked device by default and show the code as the
  button.** It keeps today's behaviour for the phone-reader and gets the
  laptop-asker wrong every time, which is the case the code exists for. It
  also leaves the click able to sign in by itself, which is what the
  scanner problem needs removed.
- **Let the click approve the waiting device.** Turns an email you did not
  ask for into a way in. See above.
- **Mint the code on first open instead of storing it.** Either a second
  open cannot show the code, or each open replaces it and a scanner's open
  silently invalidates what the person is looking at. Storing it is simpler
  and, per the hash argument above, no weaker.
- **Bind the row to IP or user agent instead of a cookie.** Both change
  under a person on a phone moving between networks, and both are shared
  by strangers behind one NAT. A nonce the requester holds is the only
  thing that means "the same browser".
