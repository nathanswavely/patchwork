# ADR 081: A noticeboard with replies, not a feed

Date: 2026-09-04. Status: **accepted**; not yet built. Grilled in session
against CONTEXT.md and the ADRs below, one branch of the design tree at a
time; every decision here was put as a question with a recommendation and
answered, and the two that went the other way (replies, and who gets
told) are recorded as the first recommendation being overruled.

## Context

A community organizer, in a feedback session, asked for "a bulletin board
or feed internal to the patch that allows approved roles and members."
The ask is real: a patch has events, proposals, charters, and a member
list, and no place to say "the PA is broken, who has one" to the people
in it. Today that sentence goes to a Signal group, and the patch on
Patchwork is a directory entry for a community whose life happens
elsewhere.

The maintainer's constraint was equally plain: "I don't want to build
something that turns into Facebook fights." Every decision below is a
consequence of taking both seriously.

What the codebase already said, and what bound the design:

- **"Bulletin" is taken.** ADR 076 defines the bulletin as the *one*
  broadcast Patchwork sends — monthly, complete, unranked, opt-in — and
  makes a point of it being the single exception to "every notification
  is a consequence of a relationship the recipient already holds." The
  organizer's word could not be this feature's word.
- **A discussion mechanism exists, bound to proposals.** Migration 014's
  `proposal_comments` (threaded, with emoji reactions) is the project's
  one prior answer to "where do members talk," and ADR 048 retired the
  separate discussion stage so talk happens beside a decision. The
  comment-level endpoints (edit, delete, react) are already generic; only
  list/create and the `CommentThread` component are keyed to a proposal.
- **Follower permissions gate taking part, not reading** (ADR 050).
  Everything behind a workspace tab except charters is a public read.
  Anything members-only on a new tab needs a real check, not a hidden tab.
- **Moderation today is thin on purpose.** A comment can be deleted by
  its author or a patch admin (hard delete, audited) and edited by its
  author. Reports exist only for nodes, events, and users, and go to the
  instance admin panel. A person can be banned from a patch
  (`memberships.status = 'banned'`). There is no lock, mute, filter, or
  tombstone.
- **The front door promises no algorithm runs it** (ADR 040), and ADR 076
  reads that promise as being about *who decides what is put in front of
  someone*. A feed is the thing that promise rules out.

## Decision

**1. It is a noticeboard with replies, not a feed, and not a forum.** The
first recommendation was a noticeboard with *no* replies — a cork board,
where nobody writes on your flyer — on the argument that fights need a
reply box. It was rejected: people will demand replies, and a board that
refuses them will be routed around. The shape that survived is a
noticeboard where **each notice decides whether it takes replies**.

**2. The moderation kit is closed, and it is four tools.**

1. *Replies can be switched off after the fact*, by the notice's author or
   a patch admin. Existing replies stay readable; the box goes. This is
   the per-notice switch made flippable at any time, and it is the lock.
2. *A reply can be removed* by its author or a patch admin — hard delete
   plus audit, the same as proposal comments. This is sufficient because
   **replies are flat**: no reply to a reply, so a removal orphans nothing
   and there are no tombstones; and **no reactions**, because a flat list
   without reactions cannot grow the pile-on that threads and thumbs
   exist for.
3. *A notice or reply can be reported, to the patch's admins.* New: every
   report today lands in the instance panel, and an instance admin cannot
   read a members-only room. The room's stewards get its queue. The
   existing user report remains the backstop to the instance.
4. *The ban that exists is the ban.* A persistent fighter is banned from
   the patch through the door already there, and is gone from the
   noticeboard with everything else. No board-only ban, no mute.

Two patch settings: who may put up a notice (admins only, or members
too), and whether new notices take replies by default.

Everything else — mute, word filters, edit history, a "restricted" state
for a person, tombstones — is a tool for running a forum, and is refused
here so that the next person proposing one has to argue for it.

**3. Members-only, always.** Read by the patch's active admins and
members; never followers, never the public, never another quilt, no
follower-permissions key. Three reasons. It is the line that makes the
kit above sufficient: the room's door is the ban, and following is
frictionless by design, so a banned reader who could follow would be back
in a click. Followers already have a broadcast surface — events, the ICS
and RSS feeds, the AP outbox; what they lack is a *conversation* with the
patch, and that is precisely what is not being built for five hundred
strangers. And it keeps the promise one sentence long in the privacy
policy, the same sentence the contact card uses (ADR 080).

**4. Quiet by default.** A notice reaches the bell only when its author
checks **Tell members**, which is off by default and is shown on the
notice afterwards as a *members told* mark, so the room can see who pages
everyone and how often. A reply notifies **participants only** — the
notice's author and those who already replied — which is the rule
proposal comments use, and the mechanism that keeps a fight between the
people in it. The notification is normal priority (in-app on, email off
by recipient default): the checkbox must not become a "send everyone an
email" button, because whether the bell reaches a mailbox belongs to the
recipient's preference, not the author. The noticeboard gets **its own
notification category**, named for the surface, so a busy patch's board
can be silenced per patch without touching its events or proposals. **No
batching, no digest, no unread badge**: the last needs per-person read
state per patch and is the engagement mechanic in miniature. A
noticeboard is a room a member walks into.

**5. No @mentions, no @patch.** `@patch` is the checkbox with a worse
failure mode — the same act, but available in any text, including a reply
mid-argument, by anyone who can reply: "@patch look what she just said"
is the pile-on siren. Restricted to authors at creation it is the checkbox
spelled harder. A mention of a *person* is a summons; the legitimate case
("Nathan, can you bring the PA") is real, but a summons is also how
someone who has not spoken is dragged into a fight, and the participants
rule already covers the case that matters: once they reply, they are in.
Mentions are **parked**, not refused (see below).

**6. The words: Noticeboard, Notice, Reply, Tell members.** *Board* alone
is a governing body (CONTEXT.md, Council, avoids it). *Post* is the event
verb — members *post* events — and the Facebook noun. *Comment* stays the
proposal's word so "replies are off" never reads as a closed deliberation.
*Notify members* was rejected as the checkbox label because *notice* and
*notify* share a root; "Tell members" is the intro card's register. The
textile route — a *bee*, the quilters' gathering where the talking happens
— was considered and refused: a bee is an event, and Events is a tab.

**7. Notices and replies travel in a seamrip; reports stay behind.** The
same two precedents the boundary already holds: proposal comments travel,
because deliberation is community data and a fork that lost it would lose
the record of why things were decided; content reports stay, because
moderation history is about the old instance's handling. The noticeboard
is where "the PA is broken" and "the landlord called" live — a fork
arriving with its charters and votes and an empty noticeboard would have
lost the community's working memory. The per-notice reply switch travels
with the notice, so a locked argument stays locked; the two patch settings
travel as governance configuration; the *members told* mark travels as a
fact about the notice and no notification is re-sent, since the fork's
bell starts empty like everything else. A reported-but-unresolved notice
arrives on the fork as an ordinary notice for the fork's admins to judge
fresh, which is the right default for a fork whose reason may be that the
old admins were the problem. A notice removed before the fork is gone from
the fork: removal is a hard delete, and this is the tombstone decision
restated.

**8. A notice body is markdown, with links and an image reference.** The
treatment a charter, a proposal, the Label, and the legal documents
already get: rendered through `MarkdownRenderer` (marked, sanitized by
DOMPurify), so the noticeboard inherits whatever that component allows
and forbids and adds no second rendering path. An image is a reference,
never bytes (ADR 007), validated by the same `validateImageRef` patches and
events use — the binary never fetches it. No upload flow rides in on this
feature; when ADR 007's upload lands, notices get it with everything else.

## Parked, not rejected

- **A public noticeboard.** A patch that wants to put notices in front of
  the street does not get it from this feature. Public notices would
  federate, be reported to the instance, and need instance-level
  moderation: a different feature, to be argued for on its own.
- **@mentions of a person.** See decision 5. Build the noticeboard, and
  let organizers ask.

## Considered options

- **A noticeboard with no replies.** The first recommendation; rejected —
  people will demand replies, and a board that refuses them is routed
  around.
- **Threaded replies with reactions, by reusing the proposal component
  as-is.** Rejected: nesting needs tombstones and reactions are the
  pile-on primitive. The table and comment-level endpoints are reused;
  the shape is not.
- **A `noticeboard` follower-permissions key.** Rejected — decision 3.
- **Every notice notifies every member.** The first recommendation;
  replaced by the author's off-by-default checkbox, which is quieter and
  is the register ADR 076 already writes in.
- **`@patch` for everyone, `@name` for one person.** Refused and parked
  respectively — decision 5.
- **An unread count on the tab.** Rejected — decision 4.
- **"Board" as the tab name.** Rejected — decision 6.

## Consequences

- A migration: a `notices` table, a target generalization on
  `proposal_comments` (or a parallel table — an implementation choice, not
  a decision here), a `report` entity type for notices and replies with a
  patch-admin queue, two patch settings, and one notification category
  with two types. The `notices` table goes in `Tables()`; reports on
  notices are covered by `content_reports` staying behind
  (`TestEveryTableHasABoundaryDecision`, decision 7).
- The workspace grows a fourth tab. `patchWorkspace.js` decides tab
  subsets; an unclaimed patch has no members and therefore no noticeboard.
- The privacy policy gains one sentence, beside the contact card's.
- CONTEXT.md gains a *Noticeboard* section: Noticeboard, Notice, Reply.
- This is the second thing in a workspace that is genuinely withheld from
  followers and the public. The check lives in the handler, not the tab.
