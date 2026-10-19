# A listed app may open the quilt's links

Date: 2026-10-18. Status: **accepted**; implemented. Extends
docs/adr/2026-09-20-an-instance-vouches-for-an-app.md, which deferred this
decision; keeps docs/adr/017 (the step-up gate) where that record put it.

## Context

The 2026-09-20 record let an instance vouch for a native app: an admin
lists it, the server publishes its identifier, and the app may hold the
domain's passkeys. It published `webcredentials` and deliberately no
`applinks`, because telling every iPhone to open the quilt's URLs in an app
is a different grant from "this app may hold our passkeys", and no client
could yet render the pages it would be claiming.

One can now. The iOS client renders the quilt's own pages natively: home,
the map, events, discovery, the workspace, notifications, the about and
legal pages, a patch and its events, members, governance hub, proposals,
documents, noticeboard, claim and setup, a person's profile, an invite, and
a patch on another quilt. A link to any of these tapped in Mail or Messages
still opens Safari, and the person is left reading the quilt in a browser
beside the app made for it.

Apple's mechanism is the same file. An `applinks` block names app IDs and
a list of path patterns; an iPhone with a named app installed opens a
matching link in it instead of the browser. The patterns are the whole
question: claim too little and links stay in Safari for no reason, claim
too much and a person tapping a settings link lands in an app that has
never seen that page.

## Decision

**Opening links is a second switch, per listed app, off by default.**
`native_apps.open_links` is a new column, `0` on every existing row and on
every new one. An admin turns it on for one listed Apple app at
Administration → Settings → Apps ("Open this quilt's links in the app"), by
`PATCH /api/v1/admin/native-apps/{id}` with `{"open_links": true}`. The list
response carries the field on every row. While no app has it on, the Apple
file is exactly what it was: `webcredentials` alone. Once one does, the file
gains an `applinks` block whose `appIDs` are only the apps that were turned
on. Listing an app still does not claim a single URL.

**No step-up; audited.** The step-up gate guards grants outward, and the
grant outward here already happened: listing the app, under step-up, is
what let that app hold this domain's passkeys. Opening links names no new
party, hands over no credential, and can only route a link to an app the
quilt already vouches for. Turning it off is always the safe direction. So
the PATCH is admin-only like removal, and every change (not a no-op) is
audited as `admin.native_app_open_links` with the app and the direction.
One thing it does change is worth naming: an invite link
(`/invite/{token}`) carries its token in the path, and with links on, that
token is delivered to the listed app instead of the browser. That is the
app the quilt already trusts with its passkeys, which is a stronger thing
to trust it with.

**What is claimed mirrors what the client renders.** The paths are one Go
value, `appleLinkComponents` in `internal/handler/native_apps.go`, in
Apple's components format (iOS 13 and later). Apple walks the list in
order and the first match decides; `*` is zero or more characters, `?` is
exactly one, `?*` is one or more, and both wildcards cross `/`. The list is
built in two parts:

1. **What stays the browser's whatever follows**, as exclusions: the API,
   `/.well-known/`, the admin panel, account settings, sign-in and signup,
   `/welcome`, `/submit`, `/activity`, claim verification, creating and
   editing an event, creating a patch, patch settings and the old manage
   paths, and the governance writes (new proposal, rules and rule
   proposals, the record, new document, a document's propose and history
   pages), and a new notice.
2. **The claim.** The quilt's own top-level pages are matched exactly.
   Every include that ends in a parameter (`/events/?*`, `/users/?*`,
   `/patches/?*`, a proposal, a notice) is preceded by an exclusion of
   anything deeper, because a wildcard that crosses `/` would otherwise
   claim every sub-page the web has under it, including ones the client
   has never seen: the governance document and proposal lists, the legacy
   redirects under a patch, a patch-scoped event path. Those stay the
   browser's.

`TestApplinksClaimMatchesTheClientsRoutes` reads the published list the way
an iPhone does and checks both sides: every route the client renders opens
in the app, and every route named above, plus the web-only routes under
claimed prefixes, does not. When a client learns a route, or the web grows
one under a claimed prefix, the list and that test change together.

**Sign-in links stay the browser's.** `/login/link/{token}` is excluded
with the rest of `/login`. The link already works for a native client
through its code (docs/adr/2026-09-28-a-link-knows-where-it-was-asked-for.md),
and routing the link itself into an app is a separate decision about which
client is "the one that asked", not a path to add to this list.

**Android is out of scope.** The Android file still publishes
`get_login_creds` and nothing else. The PATCH refuses to turn the switch on
for an Android row, and the refusal lives in the handler rather than a
`CHECK` on the column, so that Android's `handle_all_urls` (with its own
intent filters and verification rules) can be a later decision on this same
column without a table rebuild.

## Consequences

- Every existing instance is unchanged after the migration: the column
  defaults to off, so no file changes until an admin acts.
- Apple's CDN caches the file for up to a day, so turning the switch on or
  off reaches phones on Apple's schedule, not ours. The admin page says so.
- The claim is a list of the client's routes kept on the server. A client
  that renders a new page has its links opened in Safari until this list
  learns it, which is the safe failure. A web route added under a claimed
  prefix stays the browser's by default, because of the guarding
  exclusions, which is also the safe failure.
- One claim serves every opted-in app. Two apps rendering different route
  sets would need one `details` entry each; nothing lists two today.

## Rejected

- **Claiming everything (`/*`) and letting the client open what it cannot
  render in an in-app browser.** It works, and it is worse: a person tapping
  a settings or sign-in link gets a browser inside an app, without their
  Safari session, instead of the browser they were already signed in to.
- **Turning links on with the listing.** Folds two grants into one checkbox
  and changes the published file of every instance that listed an app for
  passkeys alone.
- **A quilt-wide switch rather than per app.** The grant is "this app may
  open our links", and an instance listing two apps should be able to say
  it of one.
- **Step-up on the switch.** It would gate a routing preference among
  already-trusted apps behind the same ceremony as vouching for a new one,
  and put a gate in front of turning it off.
- **Keeping the path list in `patchwork.yaml`.** The list describes what a
  client renders, which is code, not a deployment fact; and an operator
  editing it would be guessing at the client's routes.
