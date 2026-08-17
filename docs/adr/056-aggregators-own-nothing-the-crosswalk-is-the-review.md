# Aggregators own nothing; the crosswalk is the review

Every community has a calendar that lists other people's events — a city
tourism bureau, a chamber of commerce, an alt-weekly, a county parks
department. Lancaster's is the motivating case: 196 events, published as
ICS, that ADR 031's machinery could pull today. It must not.

ADR 031's premise fails here. Attaching a source is vouching for the feed
once, and only owners may attach — but nobody can vouch on behalf of
forty organizations they do not run. The feed makes this concrete: it
carries no `ORGANIZER` on any event. 196 events across 55 `LOCATION`
strings resolving to roughly 38 real places, and the host appears nowhere
a machine can read it. `Binns Park` arrives four ways (two street
numbers, a misspelt city, and once merged with the plaza next door);
`Gardner Theatre at Lancaster Country Day School` also arrives as `Day
Scool`; `PA`, `Lancaster City` and `3rd floor atrium` arrive as
locations. `CATEGORIES` names a sponsor.

Two obvious shapes were available and both are wrong. Making the
aggregator a patch would hand a tourism bureau the largest tile on the
quilt — ADR 015 sizes tiles by activity — for events it does not run,
while every venue's own calendar stayed empty. Making it an event source
would put forty organizations' calendars under one owner.

The decisions:

- **An aggregator is ADR 026's trusted contributor, made non-human.** An
  instance-level feed that lists events it does not own: it owns nothing,
  has no tile, and creates no event until a crosswalk entry addresses
  one. Its standing is the grant ADR 026 already defines — it may place
  events on unclaimed patches, where the instance admin holds the
  calendars in trust. Instance authority never *publishes* onto an
  autonomous patch's calendar, which is the line ADR 026 drew.
  A consequence worth stating: the patches most likely to already carry
  their own events are exactly the ones with no reason to opt in, so
  duplicates mostly evaporate by construction rather than by detection.
- **Writing is not the same as offering, and a claimed patch's own
  switch decides which it gets.** `nodes.accept_event_suggestions`
  already exists: a patch that turns it on has said "suggest to me",
  which is how an owner who will not maintain Patchwork lets others keep
  their calendar alive. So an aggregator may reach a claimed patch that
  has switched it on, and what arrives is **suggestions** — routed as
  `pending_review`, publishing nothing, announcing nothing, federating
  nothing, until that patch's own admins approve them. A patch with the
  switch off can be mapped by nobody but its own admins. The switch is
  not permission to adopt a feed — ADR 031 keeps "review individual
  events" and "accept a source that produces events indefinitely"
  apart — so a patch's own admins mapping themselves still publishes
  directly, and that remains the only standing consent.
- **The patch's protection is sight and exit, not a handshake.** Every
  crosswalk entry pointing at a patch appears in its own settings, says
  who set it up and whether it suggests or publishes, and can be stopped
  there. A propose/confirm ceremony was considered and rejected as a
  second consent mechanism for something the review queue already gates:
  nothing a suggesting entry brings is visible to anyone until the patch
  acts, so there is nothing to consent to in advance. Stopping one keeps
  what the patch approved — those are its events now — and drops what
  was still waiting, which was never theirs.
- **Rejecting a suggestion skip-lists its feed item.** Otherwise the next
  sync re-inserts it and the same rejection is owed every hour. This was
  a live defect in the review path the moment a feed could reach it.
- **The crosswalk is the review, performed once per name.** Mapping
  `Binns Park` to Binns Park is a standing consent covering every event
  that venue will ever host — not an approval of one event. Matching is
  exact on the normalized first field of the location (ADR 046) and
  never fuzzy: a wrong match puts a stranger's event on your calendar,
  which is the failure ADR 031 rejected prose-guessing over. Several
  names may point at one patch, because venues are misspelt and
  re-addressed constantly. A name with no entry is **unrouted**, and a
  name judged to mean no organization can be **ignored**: `PA` and `3rd
  floor atrium` are set aside rather than left to sit in the list
  forever. Ignoring hides a name from the instance admin's own screen
  and nowhere else — a patch's picker still offers every unmapped name,
  because whether a patch answers to one is that patch's judgement, and
  a shared ignore list would be a quiet route around that. It is
  reversible and touches no listings.
- **A routed event follows the feed, and detach means "this is mine
  now".** ADR 031's read-only import and its detach escape hatch are
  reused whole, so an upstream cancellation still propagates — most of
  why a community would take a city feed at all. Detach carries more
  weight here than in ADR 031, because the venue cannot go fix the
  city's typo upstream. **Unmapping detaches everything it routed rather
  than deleting it**, departing from ADR 031's source-removal rule: the
  patch consented individually and must not lose its calendar because a
  name was unmapped.
- **Announcement is anchored on the crosswalk entry, not the
  aggregator.** An entry's first routing pass is silent back-fill, and
  new listings for that name announce normally afterward. Anchoring on
  the aggregator's last success — ADR 031's rule — would fire fifty
  notifications on the day a venue opts in, which is event upload's "a
  season arriving is not forty notifications" violated by a different
  door.
- **A routed event takes second patches the ordinary way.** A crosswalk
  entry addresses a location, and a location is a venue: some of what a
  venue hosts belongs to somebody else, and only unevenly — `LIP At
  Large: Improv Comedy at West Art` is Lancaster Improv Players' night,
  `Latin Dance Night at West Art` is not. Routing does not try to tell
  them apart. A routed event is an ordinary event in every respect but
  its authority, so ADR 032 applies unchanged: either side's admins
  propose the link, the other confirms, and the confirmer may absorb
  their own duplicate. Links survive re-syncs, because the reconciler
  only ever updates an event's own columns. A standing rule keyed on
  title — "West Art events mentioning LIP also link to LIP" — is the
  prose-parsing rules engine ADR 031 declined, arrived at from a second
  direction.
- **A duplicate is held, never guessed.** Same patch and same start
  instant is a collision candidate; the listing is not created and waits
  in that patch's workspace until an admin says same event (a permanent
  skip, the absorb pattern of ADR 032) or different (it is created).
  Titles are never compared — the city writes `Music Friday hosted by
  Music For Everyone` where the venue writes `Music Friday`. The
  patch's own event wins by default, because its admins wrote it.
- **A name is opened before it is judged.** The listing count on each
  unrouted name is a door to the listings themselves — title, time,
  description, and a link to the publisher's own page, carried from the
  feed's `URL`. Whether `West Art` is an organization or a room is not
  answerable from a name and a number, and an admin who cannot check
  will either guess or stall.
- **Both travel in a seamrip, arriving unvouched.** A crosswalk is
  dozens of names mapped by hand — community labor, and seamrip exists
  so a community leaving bad leadership takes its work with it. The
  standing to write onto patches is one instance's judgement, which ADR
  026 keeps per-instance. So both export, and the aggregator imports
  paused: no sync until the fork's steward attaches it themselves.
- **Aggregators live in the database with an admin panel**, like
  neighbor quilts (ADR 014), never in `patchwork.yaml`. An aggregator is
  instance data with a curated table hanging off it, and no fork should
  have to hand-edit a config file to inherit one.

## Rejected alternatives

- **An aggregator patch owning every event.** 196 events on a tourism
  bureau's tile puts it at the centre of a quilt for events it does not
  run, and leaves each venue to claim its own calendar off someone
  else's page.
- **Per-event review.** ADR 026's queue is right for a person submitting
  one event and wrong for a feed producing hundreds a quarter; the
  reviewer would be rubber-stamping inside a week, which is worse than
  no review because it looks like review.
- **Auto-creating patches for unmatched names.** Would mint `PA`,
  `Lancaster City`, `Downtown Lancaster` and `3rd floor atrium` as
  patches. Minting a patch is a thing a human does at the moment they
  decide a name deserves one.
- **Fuzzy name matching.** Would catch `Scool` for free, and would
  silently misroute the rest.
- **Title-keyed crosswalk rules** ("anything containing Music Friday
  belongs to Music For Everyone"). This is the one case location cannot
  route — the host appears only in the summary. A rules engine over
  event prose is the crawler ADR 031 declined; that host can attach its
  own event source, or a person can submit the event.
- **Patch-level aliases** — a patch declaring the names it answers to,
  matched by every aggregator at once. Less repetitive, and a broader
  consent than the one being given: opting in to a feed should be a
  decision about that feed.
- **Instance admin mapping to active patches.** A routing rule that
  writes onto an autonomous patch's calendar is ADR 026's forbidden
  reach wearing a different hat.
- **Vendor APIs or scraping the calendar page.** Tockify publishes ICS,
  as does every aggregator worth taking. ADR 031 settled this.

## Known gap

A patch cannot discover that a venue's routed calendar names it. ADR
032's handshake is available to both sides, but neither has a reason to
go looking, so the link gets made only where a steward happens to notice.
Deferred rather than rejected: the fix would be a candidate list — events
elsewhere whose text names this patch, offered to its admins to propose
from — which would draw a line this ADR does not yet draw, that text
matching may *suggest to a human* even though it may never *route*. The
failure modes differ: a bad route puts a stranger's event on your
calendar silently, a bad suggestion is dismissed. Worth building when an
instance is large enough to feel the gap, not before.
