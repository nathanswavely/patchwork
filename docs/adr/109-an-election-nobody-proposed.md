# 109. An election nobody proposed

**Status:** accepted, 2026-09-15

## Context

An election is the one proposal no person raises: the calendar opens it
(docs/adr/051), on its own schedule, at whatever hour the sweep runs. The
`proposals` row still needs an author, and `systemAuthorFor` supplied the
patch's longest-standing admin — with a comment saying the record needed a
name and that inventing a synthetic user would have been worse.

Both halves of that were wrong.

**The name is not a stand-in; every surface reads it as authorship.** The
proposal page printed "Council election — Proposed by Priya Natarajan —
9/14/2026" for a contest a timer opened while Priya had been away for nine
months. She had not been near the site. Worse than the byline, the
participants audience includes a proposal's author, so the two comments
members left on "her" proposal rang *her* bell — a ghost authorship turning
into somebody's obligation. Priya:

> I did not call this election. Something did, and it put my name on it. If a
> member asked me "why did you call another one, Priya", I would have no
> answer.

Nell, on another patch the same day: *"I was asleep on the 13th of September
and I have never in my life proposed a council election... Tam is going to
read that page and think I made a move."*

**And the synthetic user was never invented.** `_system` — display name
"Community" — has been in the schema since migration 015. It owns unclaimed
patches, the bootstrap rule excludes it, the seamrip excludes it, and a wipe
re-seeds it.

The same sitting turned up three more things about what an election tells
people, all in the same family: the product knowing something and not saying
it.

**A contest that seated a council told one person.** At the resolution
instant the database held exactly one result notification — `You were elected`
to the winner. The other candidate was told nothing, not even that the
contest had closed; her next line was "Nominations are open" for the
follow-on, in the same second. The four members who voted were told nothing.
A contest that settles *nothing* notifies every member; the one outcome a
year of elections is for notified one.

**A settled election's banner described an amendment.** It resolves to
`status='approved', state='in_effect'` and fell through the generic branch to
"The community approved this change… This change is in effect" — a community's
leadership called "this change", with no winner, no chair and no term. Its
`unsettled` sibling has had a sentence of its own since docs/adr/097.

**And the candidate names on the ballot were inside the checkbox's label.**
Rosa, on a phone, tapped a name to find out who the man was:

> Tapping the name ticked the box. I hadn't decided anything yet and the site
> had recorded me as approving him... Same names, two pages, opposite
> behaviour.

The same name on the Members tab is a link to a profile.

## Decision

**A contest the calendar opened is signed by the calendar.**
`systemAuthorFor` returns `model.SystemUserID`, and the proposal page says
"Opened by this patch's election calendar" where a proposer's name used to
be. The page keys that on the election itself rather than on the author id,
so contests raised before this change read correctly too.

**The participants audience stops including the sentinel**, so a notification
row is never written for a user nobody reads as. It also gains
`election_ballots`: an election's votes live in their own table, so somebody
who voted in a contest was not a participant in it and heard nothing about the
discussion underneath. Two lines of one query; both were the same oversight
about elections being proposals with different furniture.

**A contest that settles something tells the patch.** One notice —
"The election in X has closed" — with how many of its seats were filled and
what happens to the rest. The winner's personal notice and the outgoing
admin's are untouched; they were always right. Losing is news too, and it
should not be learned from the silence after somebody else's good news.

**A settled election gets its own banner**, before the amendment branch:
"This election has closed and the council below is seated." It deliberately
does not name who — the panel directly beneath it lists every candidate with
their approvals and tags the seated ones, and a banner repeating that is a
second place for it to go wrong.

**The name on a ballot is a link to the person, and the checkbox is the
control.** The label wraps the box alone, with an accessible name of its own;
the name is an `<a>` to the profile, styled quietly so a slate does not read
as a list of links. A name should never be the thing that casts a vote.

## Consequences

- Elections already in flight change byline immediately, which is the point:
  the old one was wrong about them too.
- `_system` now authors rows in `proposals` as well as `nodes`. It is a real
  user row, so every foreign key and join holds; what it must never do is
  receive a notification, which is the one place this decision had to say so
  out loud.
- The result notice uses `ProposalApplied`, an existing type in the proposals
  category, rather than a new one. A council being seated *is* a proposal
  taking effect, and a new type would be one more switch for a member to find
  in settings to say the same thing.
- Nothing here gives a candidate anywhere to say why they are standing
  (F-095). Four voters this epoch decided on the strength of a comment behind
  a Discussion tab, and the ballot is still a list of bare names with a link
  each. That wants a column and is its own piece of work.
