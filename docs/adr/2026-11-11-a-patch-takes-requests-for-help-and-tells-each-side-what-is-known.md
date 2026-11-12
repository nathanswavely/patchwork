# A patch takes requests for help, and tells each side what is known

Date: 2026-11-11. Status: **proposed**, not built.
Applies docs/adr/081 (a room is checked in the handler, and its reports go to
the patch), docs/adr/093 (a notification follows an obligation the person took
on), docs/adr/095 (small numbers name people),
docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md
(a grant lives in its own table and is never a membership row),
docs/adr/2026-09-18-the-default-should-match-the-assumption.md (a surface is
born closed and opens by an act), docs/adr/083 (nothing is shared by a rule),
docs/adr/086 (erase the person, keep the acts) and docs/adr/077 (a dependency
every fork inherits cannot require a key).

## Context

A person who needs help from a church calls the office or walks in while it is
open. The person at the desk texts a member of the right committee. That
member calls back to gather the details, then texts volunteers one at a time
until somebody says yes. The person who asked hears nothing until someone
calls, and the record of it is one person's text messages.

The people in that chain are the valuable part. The waste is in the handoffs:
a request can only arrive in office hours, each handoff asks again what the
last one heard, the search for a volunteer is serial, and nobody can see
where a request stands.

The commercial tools that solve this for rides and deliveries earn money per
task. A community has no reason to route its neighbors through one and no way
to own it. Patchwork already holds most of what a community-owned version
needs: passkey identity, patches with a contributor ladder, a quilt of many
patches on one instance, an audit log, a per-patch report queue, and a review
queue for things that arrive from outside.

The maintainer's constraints, from the session this record came out of:

- **Built on trust, the way a community already works.** People will sometimes
  take advantage. The answer is a record and a way to respond, not a wall in
  front of everyone. A first draft that made coordinator review mandatory for
  a stranger's first request was overruled.
- **Informed consent over gatekeeping.** Each side is told what is and is not
  known about the other and decides for itself.
- **Honest about capacity.** It is for small communities and says so.
- **No money.** No fees, pay, tips or payments, and no goods bought through it.
- **The core runs on small hardware with nothing rented.**

What the codebase already said, and what bound the design:

- **"Requester" is taken.** CONTEXT.md gives it to a person who asked to join
  an approval-required patch. "Request" alone is also ambiguous: there are
  trust requests and membership requests. This feature could not use either
  word bare.
- **"Offer" is taken** by event sources (docs/adr/063), and **"broadcast"**
  names the one thing Patchwork refuses to do outside the bulletin
  (docs/adr/076, docs/adr/093).
- **There are no patches inside patches.** A church's committees cannot be
  sub-patches.
- **No anonymous caller writes content today.** Every route in `openToAnyone`
  is a sign-in step, a token redemption or a federation inbox. A public
  request form would be the first.
- **Nothing sends a push notification.** The bell and email are the two
  channels, and email is optional.

## Decision

**1. A help request is asked of a patch, and it is a room of its own.** The
feature is per patch and off until a patch's admins turn it on. A help request
is read by the coordinators of its kind, the patch's admins, and the volunteer
who accepted it. Nobody else: not the patch's other members, not followers,
not an instance admin holding no role there, who gets a 404 as on the
noticeboard (docs/adr/081). Requests never federate and never appear on a
public surface.

**2. A patch defines its kinds of help, and each kind carries three
settings.** A kind of help is a named thing the patch offers: rides, yard
work, meals.

| Setting | Column | Values |
|---|---|---|
| Clearance | `help_kind_clearances` | zero or more of the patch's clearances |
| Review | `help_kinds.review` | `every`, `first`, `none` |
| Dispatch | `help_kinds.dispatch` | `assign`, `eligible`, `on_call` |

- **Review starts at `every`.** A coordinator reads each request before any
  volunteer sees it. `first` reviews only a recipient's first request to this
  patch. `none` puts the judgement with the volunteer, who sees everything the
  recipient wrote, may ask a question first, and may decline with no mark
  against them.
- **Dispatch starts at `assign`.** A coordinator picks a volunteer. `eligible`
  alerts every eligible volunteer and the first to accept takes it. `on_call`
  alerts only those on a shift.
- **Visibility follows dispatch.** Under `assign` an open request is seen by
  coordinators only, which is the pilot's default. Choosing `eligible` or
  `on_call` is what shows an open request to volunteers, and it shows them the
  description and an area line, never the address or contact.

**3. A coordinator is a grant on one kind of help, not a role.** Each kind
names its coordinators. A coordinator must be an active member or admin of the
patch and need not be an admin: a care team coordinates without holding the
patch's settings. The grant lives in its own table, as the per-patch trust
grant does, so nothing that reads memberships mistakes it for a rung. This is
also how a request reaches the right committee with no sub-patches: a
committee, for this feature, is the answer to "who handles this kind of help".

**4. A volunteer is a member who signed up for a kind of help.** Volunteering
is a member act. A follower who wants to help becomes a member, which is the
ladder working as designed. Signing up is what makes an alert an obligation
the person took on (docs/adr/093): nobody is alerted about a kind of help they
did not put their name to. A volunteer can pause without leaving.

**5. A clearance is a statement a patch stands behind.** It says who verified
what, and until when. A patch defines its own: a background check, a license
and insurance check, an orientation. An admin records that a member holds one,
with an expiry date. Patchwork runs no check and stores no document.

- An expired clearance is not held. The volunteer stops being eligible for
  kinds that need it.
- A clearance always carries its patch's name. There is no bare "vetted".
- Where a volunteer holds none, the recipient is shown "No organization has
  vetted this volunteer" in the place a clearance would appear.
- A recipient may ask for vetted volunteers only (`help_requests.vetted_only`).

A clearance is not an **attestation**, which is governance's word
(docs/adr/052), and not the **trusted contributor** grant, which is the
instance admin's and is about calendars.

**6. A recipient need not have an account.** The recipient is the person the
help is for. The patch holds a record of them: a name, one way to reach them,
and optionally the member who vouches for them. Three doors create a request:

- a signed-in member asks for themselves,
- a coordinator enters one for somebody who phoned or walked in,
- anyone fills in the patch's public help page, where the patch has opened it.

`nodes.help_open_to` is `members` or `anyone`, and a patch is born at
`members`. Opening the door to the street is an act
(docs/adr/2026-09-18-the-default-should-match-the-assumption.md).

**7. The public form is the first anonymous write, and is fenced
accordingly.** It requires a way to reach the recipient. It is rate limited
per source and per contact. What it accepts is stored and shown as plain text,
never rendered as markdown and never linked. It notifies coordinators through
the bell only. It is refused outright while a patch's review for that kind is
`none` and its dispatch is not `assign`, so an anonymous form can never page
volunteers with nobody having read it.

**8. The recipient follows the request on a private link.** Submitting returns
a status link whose token is stored hashed, like the personal feed secret. The
page shows where the request stands, the name and clearances of the volunteer
who accepted, and the decline message if there is one. It never shows the
address back. Reading it spends nothing.

**9. The address is released on acceptance.** Before accepting, a volunteer
sees the kind, the time, the description and an area line. The address and the
contact are returned only to coordinators and to the volunteer who accepted.
The concept document said "close to the time"; this narrows it, because a
volunteer who cannot see where they are going cannot plan to go.

**10. Both sides confirm, and either may say how it went.** The volunteer
marks the help given and the recipient confirms. Each may leave a private note
that only the kind's coordinators read. Either may raise a concern, which is a
`content_reports` row with `entity_type = 'help_request'` routed by `node_id`
to the patch's own queue (docs/adr/081). A concern goes to the patch's admins,
not to the coordinator, who may be its subject.

**11. No scores.** There is no rating of a volunteer or a recipient, public or
private. A volunteer's record is a count of requests answered and a date since
when, shown to coordinators.

**12. A patch may decline, and may stop serving somebody.** A declined request
carries the patch's own message and referral, written from a shipped template.
A patch may mark a recipient as no longer served. That mark stays in the patch
and is never shared with another patch or another quilt: a shared list would
be a blacklist of the people least able to answer it.

**13. A request never goes quiet.** Every request ends confirmed, declined,
withdrawn, or back with a coordinator. An alert nobody accepts returns the
request to the coordinators after the kind's wait, and a request nobody has
reviewed is on the coordinators' page until it is.

**14. Capacity is stated, in ranges, and derived.** The public help page says
roughly how many volunteers the patch has for a kind, whether a person
reviews requests, how long an answer has usually taken, and whether anybody is
on call now. Counts are shown in ranges ("fewer than 10", "10 to 25") because
"1 person on call" names that person (docs/adr/095). The usual wait is
calculated from the patch's own closed requests and is not shown until there
are enough of them; until then the patch states its own hours. The page says
it is not an emergency service.

**15. Alerts reach a phone through Web Push.** It is the one channel fast
enough for a ride and it needs no account, key or fee
(docs/adr/077): the server holds a VAPID key pair it generates itself and
posts to the endpoint the browser handed it. It is a new way for the server to
reach another machine, so it joins `aiPageOutbound` and the privacy policy.
Text messages and a native-app push relay are optional providers behind the
same small interface and are not in the core.

**16. No live location.** Patchwork never learns where a volunteer is. A large
patch may narrow an alert by a volunteer's stated home area.

**17. Personal details are erased after a request closes.** Thirty days after
a request reaches a terminal state, a sweep empties its address, contact,
area line and description and the notes left on it. The record keeps the kind,
the dates, who coordinated, who went and how it ended. A recipient with no
open request and no request newer than a year is erased the same way.

**18. The requester's page wears the patch, not the quilt.** The public help
page is `/patches/{slug}/help` on the instance's own address. It shows the
patch's name and identity color and carries no global bar and no textile
vocabulary. The person asking is asking the patch. No subdomain: passkeys and
Web Push are both bound to the origin, and a second origin is DNS and
certificate work for every self-hoster.

**19. The room has no name; its two doors are "Ask for a hand" and "Lend a
hand".** The two phrases are what a person says to a neighbor, and each names
its side without explanation. "Ask for a hand" is lighter than "request help",
and that is deliberate, since the people this is for are often reluctant to
ask. The public help page is headed "Ask {patch} for a hand". The workspace
tab is "Lend a hand": a member is on the lending side, and "Requests" would
read as the membership requests one tab over. The coordinators' page sits
under that tab, which is accepted. The thing itself
is a "request" wherever the surface already says which kind. Code keeps
`help_request`, as the backend keeps `node` for a patch. The phrases are an
English idiom, so a translation must re-say them and cannot convert them.

A coined name for the room was considered and refused while this is a part of
Patchwork: the patch is who a person is asking, and a second name beside it is
one more thing to learn. "Handwork", the quilter's word for stitching done by
hand, is the name on record should this ever be branded as a product of its
own. "Many Hands" was refused as a proverb. "Bee" stays refused for the reason
docs/adr/081 gave: a bee is an event.

## Data model

Backend names are generic, as everywhere. All ids are UUIDv7 and all
timestamps ISO 8601 text.

```
help_kinds               id, node_id, name, description,
                         review, dispatch, wait_minutes, active
help_kind_coordinators   kind_id, user_id, granted_by, granted_at
help_kind_volunteers     kind_id, user_id, since, paused_at
clearances               id, node_id, name, description
clearance_grants         id, clearance_id, user_id, verified_by,
                         verified_at, expires_at, revoked_at
help_kind_clearances     kind_id, clearance_id
help_recipients          id, node_id, user_id?, name, contact_kind,
                         contact_value, vouched_by?, first_reviewed_at?,
                         unserved_at?, unserved_by?, erased_at?
help_requests            id, node_id, kind_id, recipient_id, entered_by?,
                         description, area, address, wanted_at?,
                         vetted_only, state, status_token_hash,
                         reviewed_by?, volunteer_id?, decline_note,
                         created_at, reviewed_at?, sent_at?, accepted_at?,
                         given_at?, confirmed_at?, closed_at?, erased_at?
help_request_notes       request_id, side, body, created_at
help_shifts              id, node_id, kind_id?, user_id, starts_at, ends_at
push_subscriptions       id, user_id, endpoint, p256dh, auth, created_at
nodes                    + help_enabled, help_open_to, help_decline_message
```

`help_requests.state` is `received`, `open`, `accepted`, `given`, `confirmed`,
`declined` or `withdrawn`. "Back with a coordinator" is `received` again with
`sent_at` kept, so the page can say it was tried.

A returning recipient with no account is recognized by their normalized
contact within the patch. That is weak identity and is meant to be: it decides
only whether `first` review applies, and a coordinator can correct it.

## Build order

Each step ships on its own and leaves the product whole.

1. **Kinds and coordinators.** Patch Settings section, the grant, the room
   check. No requests yet.
2. **Requests from members.** A member asks, a coordinator reviews and
   assigns, the volunteer accepts, both confirm. Review is `every` and
   dispatch is `assign`. The bell and email carry it.
3. **Recipients with no account.** Entry by a coordinator, the public help
   page, the status link, rate limits, the erasure sweep, the privacy
   policy's paragraph.
4. **Clearances.** Definitions, grants with expiry, eligibility, and what the
   recipient is shown.
5. **Notes, concerns and declines.** The private note, the report route, the
   decline template, "no longer served".

That is the pilot. After it:

6. **Volunteer signup and `eligible` dispatch**, with `first` and `none`
   review.
7. **On-call shifts and Web Push.**
8. **Capacity figures** on the help page.
9. **Asking another patch.** See below.

Rides wait for step 6 at the earliest, and for a patch's own answer about
insurance.

## Not decided here

- **Asking another patch.** An unfilled request could be passed to another
  patch on the quilt, whose volunteers would appear as "cleared by" their own
  patch. It needs the recipient's consent to move their details, and a rule
  for which patch's coordinators hold the request. Not designed.
- **Proof photos.** Leaning against. They are pictures of people's homes, they
  are the only part that needs file storage, and both-sides confirmation
  covers what they would prove.
- **Whether clearance grants travel in a seamrip.** See Consequences.

## Considered options

- **A separate product beside Patchwork.** Rejected. The tenant is the patch
  and the accountability is the membership ladder, so a second service would
  rebuild both and then need a way to stay in step with them.
- **Requests as noticeboard notices.** Rejected. A notice is read by the whole
  room and has no lifecycle, and a help request is read by as few people as
  can answer it.
- **Committees as patches.** Rejected for routing. A recipient asks the
  church, not its deacons, and patches are flat. A committee that wants its
  own events and members can still be a patch; that is independent of this.
- **Mandatory review of a stranger's first request.** The first
  recommendation, overruled. It is one of three settings and the patch
  chooses.
- **Star ratings.** Rejected. In a community of 200 a score is meaningless or
  a weapon, and it prices a gift.
- **Assigning the nearest volunteer.** Rejected: it needs live location.
- **A general sidecar framework for outside compute.** Rejected as premature.
  The pattern Patchwork already uses for SMTP and the gazetteer is enough: a
  small interface, a version in the core that needs nothing outside, and an
  optional provider.
- **A subdomain or a separate brand for the public page.** Rejected for the
  first version (decision 18). A brand is a question for after the pilot.

## Consequences

- Every table above needs a seamrip decision
  (`TestEveryTableHasABoundaryDecision`). Proposed: `help_kinds`,
  `clearances` and `help_kind_clearances` travel as the patch's
  configuration. `help_recipients`, `help_requests` and `help_request_notes`
  stay behind, because they hold the addresses of people who are not members
  and never agreed to follow a fork. `help_shifts` and `push_subscriptions`
  stay behind. Coordinator and volunteer rows and `clearance_grants` are the
  open question: they are the community's working arrangement, and a
  clearance is also the old patch's word about a person, which a fork under
  new admins has not checked.
- Account deletion (`internal/handler/account_deletion.go`) gains the
  coordinator, volunteer, grant, shift and subscription rows, and erases a
  recipient row that points at the account. Requests the person answered keep
  a tombstone name (docs/adr/086). The personal export gains the same rows.
- `TestEveryMutatingRouteRefusesAStranger` gets a fixture row for each new
  path parameter, and the public form is the one new entry in `openToAnyone`.
- Every list here is paged and joins `web/src/test/paged-endpoints.json`.
- One notification category, named for the surface, so a patch's help alerts
  can be silenced without touching its events or proposals.
- The shipped privacy policy gains a paragraph: Patchwork holds the name,
  contact and address of a person with no account, who can read them, and
  when they are erased.
- A volunteer who accepts a request is named to its recipient with their
  patch's clearances. That discloses a membership its holder may have hidden
  (docs/adr/006). It is deliberate and is the person's own act, the same
  standing a proposal's author has.
- CONTEXT.md gains a *Help requests* section.
