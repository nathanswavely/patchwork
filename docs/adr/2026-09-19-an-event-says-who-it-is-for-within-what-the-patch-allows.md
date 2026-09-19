# An event says who it is for, within what the patch allows

Date: 2026-09-19. Status: accepted, implementation is backlog (issue #272).
Extends docs/adr/036 (two tiers, not three, for charters) to events with one
more tier that the follower role exists for; makes `follower_permissions.events`
(migration 012) an enforced rule rather than a hidden tab.

## Context

An event has a `visibility` column since the first migration, with the
values `public`, `private` and `unlisted`. Nothing in the app sets it. The
event form never sends it, so every event created in the product is public;
the bulk CSV upload is the one door that accepts a non-public value. Four
read paths branch on it, each as a binary: the events list admits a
non-public event only to an active member or admin of its own patch, the
per-patch ICS and RSS feeds serve public events only, the personal My Quilt
feed applies the member test, and federation refuses to broadcast anything
not public. The detail endpoint applies the list's member test too, since
2026-09-16, though until PR #330 it and the single-event ICS each carried
their own copy of the rule. The JSON write paths accept any string for the
column and let a bad one fail on the CHECK constraint as a 500; the CSV
door validates.

Separately, every patch carries `follower_permissions`, a four-switch blob in
its governance rules (`events`, `proposals`, `charters`, `members`) edited
under Follower Permissions in the rules editor. Three of the four are
enforced in Go: `proposals` gates commenting and, since PR #325, reacting;
`charters` gates the private shelf and was turned off by default in v0.29.0;
`members` gates the roster. `events` is read only by the frontend, where it
hides a tab. A patch whose rules say followers do not get its events is
still serving them to followers through every API surface.

Issue #272 wants the three tiers an event can actually be for: **public**,
**followers**, **members only**. The design question it leaves open is what
the patch-level switch means once an event can name its own tier. Three
answers were weighed.

*The per-event tier wins and the switch goes.* One source of truth, the
simplest read paths, but it removes a field every template ships and every
charter repository carries, and it makes `events` the one switch of the four
that does not mean what the other three mean.

*The switch is only a default.* Off means new events start members-only, on
means they start public, and any event can be changed afterwards. Nothing
enforces, so a governance document that says followers cannot see events
while a member publishes one to them with a dropdown, which is the
promise-nothing-enforces defect already in the code today, kept on purpose.

*The switch is a ceiling.* Follower Permissions says what a follower may have
on this patch, and an event's tier chooses within that. This is the answer
the other three switches already give.

## Decision

**An event's `visibility` is one of `public`, `followers` or `members`, and
`follower_permissions.events` is a ceiling on it.** The patch's rule says
what its followers may have; the event's tier chooses within that.

1. **Three tiers, named for the role they reach.** `public` is anyone;
   `followers` is the patch's followers, members and admins; `members` is
   members and admins. `unlisted` and `private` are retired. Existing
   `private` and `unlisted` rows both become `members`, the non-exposing
   direction; `public` rows are unchanged. The UI words are "Public",
   "Followers" and "Members only", matching the roles table and how
   charters read.

2. **With the switch off, followers get what the public gets.** On a patch
   whose `follower_permissions.events` is `false`, every read path treats a
   `followers` event as `members`. The tier is not rewritten on the row; it
   is read down. Turning the switch back on restores it, because the event's
   own statement of who it is for was never lost.

3. **The form does not offer what the patch refuses.** Where the switch is
   off, the tier control on the event form shows "Public" and "Members only"
   only, with one line beneath it: *Followers can't see this patch's
   non-public events. That is set at the patch level, under Follower
   Permissions.* It links to the rules editor for an admin and states the
   fact for a member. Nothing is greyed out and nothing explains itself
   twice.

4. **Every read path asks the same question.** The list, the detail
   endpoint, both feeds, the map and quilt counts, notifications and the
   bulletin, federation, and the member seamrip each decide with one
   predicate: the viewer's role on the event's patch, the event's tier, and
   the patch's switch. The membership subquery those paths share becomes
   role-aware rather than `role IN ('member','admin')`. That predicate is
   one function, `canReadNonPublicEvent`, extracted in PR #330 ahead of the
   rest so the paths cannot drift; the same PR turned a bad `visibility`
   value on the JSON write paths from a 500 into a 400.

5. **An event source carries a default tier.** `event_sources` gains a
   `visibility` column set when a feed is attached or edited. It applies at
   insert only; the sync's update path never touches `visibility`, so a
   later per-event change sticks across syncs. Visibility is local policy,
   not a fact the feed is authoritative about, which is why this field
   departs from the docs/adr/079 habit of letting the feed refill everything
   it owns. The source form observes the same ceiling as the event form.

6. **The switch is enforced in Go, not only drawn in Svelte.** This is the
   part that makes the rules editor honest. A patch that says followers do
   not get its events stops serving them to followers on every surface,
   including the ones a follower reaches without the tab.

## Consequences

A follower on a patch that has turned events off sees only that patch's
public events, which is what its charter already claimed. Nobody loses
anything on the reference instance: every template ships the switch on and
no patch there has turned it off.

A feed defaulted to `followers` or `members` stops broadcasting `Create` to
federation, which is correct and will surprise somebody once; the source
form should say so beside the control.

This is a table rebuild for the CHECK constraint, following
`migrations/065_membership_banned_status.sql`, and it must be tested from a
database carrying `private` and `unlisted` rows, not from a fresh migrate.
The event column and the node column stop sharing a vocabulary;
`nodes.visibility` keeps `public`, `private`, `unlisted` and its own meaning,
and CONTEXT.md says so.

There is no Svelte render library here, so the three tiers need a browser
pass as anonymous, follower and member, on a patch with the switch on and
one with it off, before this ships.
