# ADR 093: A notification follows an obligation, not an event

Date: 2026-09-07. Status: **accepted and implemented**; decisions 3 and 5
both ship with this record. Reached by grilling the question "what should
RSVP be?" until it turned out to be a question about what Patchwork is.

## Context

Patchwork sends four notifications about events — `EventCreated`,
`EventReminder`, `EventUpdated`, `EventCancelled` — and all four resolve
to `AudienceAllMembers`, whose query is `role IN ('admin','member')`
(`internal/notifications/types.go`, `notify.go`). A member of a venue that
posts weekly is pinged twice per show: once when it is created, once the
night before. The second is `PriorityHigh`, so email is on by default.

What the codebase already said, and what bound the decision:

- **The registry argues against itself.** The comment on
  `GovernanceRulesChanged` explains why that type is Normal and not High:
  "Mailing every member on every config edit is how a whole category gets
  filtered, and then the mid-vote notice below gets filtered with it."
  The four event types are built the way that comment warns against, one
  screen further down the same file.
- **Followers receive nothing at all.** The word "follower" does not
  appear anywhere in `internal/notifications`. CLAUDE.md's role table
  promises a follower "sees events in feed, gets notified." The first half
  is true through My Quilt, the map and the personal calendar feed. The
  second half has never been implemented.
- **The subscription path already exists and is the real one.** Every
  patch publishes ICS and RSS, offered from the patch overflow menu and
  from its events page (`SubscribeFeeds.svelte`); every person has a
  merged My Quilt calendar behind a secret URL (`PersonalICSFeed`). The
  mechanism for "tell me about events" is built, standard, and nearly
  invisible.
- **ADR 076** drew this line once already, for broadcasts: exactly one
  exists, it is monthly, complete, unranked, and it ships off, because
  anything aimed at people who aren't checking the site is "in mechanism,
  re-engagement mail — the other half of the machine this project declines
  to run." It settled broadcasts and left relationship-derived pings
  alone. This ADR finishes that sentence.
- **ADR 018** states email's purpose narrowly: magic links, plus
  notification delivery to people who aren't checking the site.
- **ADR 079** already routes the organizer's own RSVP or ticket form out
  to `events.event_url`, where it belongs.

## Decisions

**1. Patchwork publishes; it does not broadcast.** This is the root
decision and the rest follow from it. Patchwork is a place you go and a
source your own tools can read. It does not buy space in anyone's
attention. The quilt already refuses to rank, the bulletin already ships
off, no algorithm decides what is put in front of anyone — this states the
principle those were each an instance of.

**2. A notification follows an obligation the person took on, never a
fact about the world.** A proposal you do not vote on becomes a decision
made in your name, and you accepted that when you became a member. A rule
change alters the terms of that membership. A pending request, a report
queue, a claim: duties of a role held or answers to something the person
did. All of those may ring. An event is the world, and the world is not
Patchwork's to push at anybody.

Tested against every type in the registry, the rule cuts exactly four and
leaves the other thirty-six standing. A rule that cuts that cleanly was
found rather than fitted.

**3. The four event notifications are deleted, not defaulted off.** Call
sites: `handler/events.go` (create, update, cancel),
`handler/event_submissions.go` (approve), `eventsource/sync.go` (import),
and `checkEventReminders` in `notifications/reminders.go`, which the
reminder worker stops calling.

Default-off was considered and refused. A default-off type is a setting,
and a setting invites the argument back every release, because someone can
always show that their case is the important one. Deleting the type ends
the argument instead of scheduling it.

**4. Cancellation is not an exception, and refusing it is the point.**
This is the strongest case for a carve-out — a person's evening is wasted
— and it is refused on two independent grounds.

The first is that the principle only means something at its hardest case.
If "this one is important enough to interrupt" wins once, it wins forever,
because every subsequent feature arrives with an importance argument.

The second does not depend on principle at all. **Any push Patchwork sends
is structurally incomplete.** It reaches the subset of an audience that
happens to use Patchwork, which is never the audience. A venue that
believes Patchwork told everyone is worse off than a venue that knows it
has to tell everyone itself. Announcing that the show is off is the
venue's job. Patchwork's job is that its record is right when somebody
looks.

**5. The calendar is the reminder, and it needs one affordance.**
Subscription is built. What is missing is the single-event add: a person
who wants one night, not a whole venue. That is one endpoint returning one
event as `.ics`, and a button on the event page. No table, no preference,
nothing to purge.

Its limit is stated rather than fixed: a downloaded file is a copy, not a
live link, so a show that moves or is cancelled goes stale in that
person's calendar. A subscribed feed corrects itself on refresh; a
download never does. That is the accepted cost of decision 4, not an
oversight to be repaired later with a ping.

Shipped as `GET /api/v1/events/{id}/event.ics`. It carries the same UID
the patch feed gives that event, so downloading tonight's show and later
subscribing to the venue reconciles to one entry rather than two. It
follows `ListEvents` on visibility rather than `GetEvent`, which gates
neither: a non-public event is a file only a member or admin of its own
patch can take away, and a pending submission has none at all. The
wildcard needs its own path segment because a Go mux pattern cannot mix a
wildcard and a literal in one.

**6. There is no RSVP.** Attendance is a fact about the world, so decision
2 already excludes it, and it would put the first row in this schema
asserting that a named person intends to be at a place at a time — the row
this project would least like to be holding. Two further things are true
and worth recording: RSVP means a reply to an invitation, and Patchwork
has no invitations, because you cannot invite a person to an event or even
to a patch and there is deliberately no way to find one (ADR 006). Where a
real RSVP exists, it is the organizer's, and ADR 079 already points at it.

**7. A digest, if it is ever built, is not a broadcast.** A weekly "what's
on at your patches" derives entirely from relationships the person chose,
which is ADR 076's own test for what a broadcast is not. It would be
opt-in, complete and unranked, and `NotifyCoalesced` already exists to
build it. Not built here, and not needed for anything above.

## An argument withdrawn

This session first proposed the opposite of decision 3: a private
per-event save, reasoning that followers get nothing and members get too
much, and one primitive repairs both. Both halves were wrong about what
Patchwork is.

The follower silence is not a gap. A follower has taken on no obligation,
so there is nothing to tell them, and the code has been right where the
documentation was wrong. And repairing "too many pings" with a
finer-grained ping keeps the platform in the attention business at a
smaller scale, which is the business decision 1 declines. It is recorded
because it is the obvious first answer and will be reached for again.

## Consequences

- **CLAUDE.md's role table is corrected** in this change. A follower sees
  events in their feed, calendar and map; it no longer says "gets
  notified."
- **The Events notification category survives and changes meaning.** What
  remains — `EventSuggested`, submission approved and rejected, event link
  requested and confirmed, `ProgramOffer` — is without exception an admin
  queue duty or an answer to something the person did. Its user-facing
  label no longer implies it carries events: its description becomes
  "Submissions, links, and program offers", which is a copy-ledger
  string.
- **Stored preferences for the deleted types go inert.** Rows in
  `notification_preferences` keyed to the four types are simply never read
  again; no migration is required, and the settings page stops rendering
  them. `notification_reminders_sent` keeps its event rows and stops
  gaining new ones.
- **The default privacy policy is falsified in the shrinking direction.**
  `internal/handler/legal_defaults.go` claims the site records "the events
  you RSVP to" in three places — the collection summary, the Activity
  item, and the expectations section. No RSVP table has ever existed, and
  after decision 6 none is coming. Comments in
  `internal/eventsource/timezone.go` and `migrations/056` describing
  events that keep "their links and RSVPs" across a resync are wrong the
  same way. That file's header requires such claims to be corrected in the
  same change that falsifies them; here they were false already.
- **Nothing is added to the schema**, so the seamrip boundary and
  `TestEveryTableHasABoundaryDecision` are untouched.
- **Federation is unaffected.** Event creation still delivers `Create` to
  a patch's ActivityPub followers. That is publication, which decision 1
  endorses, and it is a remote server's business what it does with it.
