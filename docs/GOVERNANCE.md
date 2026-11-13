# Governance

This page is for people who run a patch, or are about to. It explains how a
patch decides who runs it and how it makes decisions. It assumes no
technical background. If you are deciding whether to run a whole quilt,
start with [START-A-QUILT.md](START-A-QUILT.md).

Most of this is optional. A band can ignore nearly all of it. A board or a
coalition will use most of it.

## The short version

Every patch answers three questions, and Patchwork runs whatever the patch
answers:

1. **How is someone made an admin?** This is the patch's leadership model:
   maintainer, meritocratic or elected.
2. **How is a proposal decided?** This is the decision method: the admin
   decides, a majority, a supermajority, or consensus.
3. **Where are those decisions made?** In Patchwork, or somewhere else, such
   as a meeting, and recorded here afterwards. A patch answers this one
   twice: once for choosing admins and once for proposals.

You choose a starting point when you create the patch, by picking one of four
templates. Some of what a template sets can be changed later in the patch's
governance rules, and some cannot yet (see "Changing your rules").

## Roles

A person has one of three relationships with a patch:

- **Admin.** Runs the patch: edits its profile, manages members, creates
  events and proposals.
- **Member.** Takes part. Members can make proposals and vote on them, and on
  an elected patch they can stand for the council.
- **Follower.** Follows along. Followers see the patch's events and never
  vote. On most templates they can comment on proposals, and a patch can turn
  that off.

Becoming a member is the step that gives someone a vote. Following never
does.

**Admin is a permission, not a title.** Your chair, treasurer and secretary
can all be admins, or none of them can. Patchwork does not know about your
group's titles. On a maintainer patch there can be more than one admin: any
admin can make another member an admin from Settings → Roles and requests.

## Words you will see

- **Council:** the admins of an elected patch.
- **Seat:** one place on a council. It belongs to the council, not to the
  person in it.
- **Term:** how long a seat is held before it comes up for election. The date
  it runs out is the seat's term end.
- **Election:** how a council's seats are filled. It takes nominations first,
  then votes.
- **Holdover:** the sitting council staying in place when an election
  decides nothing.
- **Charter, or governance document:** a written agreement the patch keeps at
  Governance → Documents.
- **The lining:** the shared baseline charter every patch starts with.
- **Record:** the patch's history of governance decisions, at Governance →
  Record.

## Choosing a template

When you create a patch you pick one of four templates. Each card says who
decides and how. Every card also ends with the same line: decisions can be
made at a meeting instead, and recorded here. That is true of all four.

| Template | Who decides | Leadership | Tends to suit |
|---|---|---|---|
| **Minimal** | You decide. Members can propose, and you approve or decline, or ask them to vote first. | Maintainer | Bands, solo artists, pop-up projects |
| **Casual** | Members vote for three days, and a proposal passes with more approvals than rejections. The admins run things day to day. | Maintainer | Small collectives, meetups, studios |
| **Collaborative** | Members vote for a week, and a quarter of them must take part. Admins nominate new admins, and members confirm them. | Meritocratic | Venues, co-ops, makerspaces, community radio |
| **Formal** | Members elect a council for one-year terms. A proposal needs half the members to vote, and fails if anyone rejects it. | Elected council | Boards, coalitions, arts districts, mutual aid networks |

Pick by who decides, not by how big you are. A small board that elects its
officers wants Formal. A large venue where two people make the calls may
want Minimal.

Each template also gives the patch some starting documents. Every patch gets
the lining. Casual adds an operating agreement. Collaborative adds an
operating agreement, a financial transparency policy and a conflict
resolution process. Formal adds a charter, bylaws, financial transparency,
conflict resolution and a succession plan. These are drafts for you to
amend. They describe what Patchwork does, so if you change your rules, check
that the documents still agree.

The template is only where you start. After creation the patch is described
by its rules, not by the card it was made from.

## How someone becomes an admin

### Maintainer

The patch is run by its admins, with no seats and no terms. Any admin can
make another member an admin.

An admin can **name a successor**, any member of the patch, at the
Governance tab. If every admin goes quiet for long enough (see "When admins
go quiet"), the successor becomes the admin. This only happens on a patch
that has an inactivity period, and Minimal has none.

Naming a successor is also what lets you leave a patch you run alone.
Patchwork does not let a patch's only admin leave without someone to hand it
to.

### Meritocratic

Admins earn the role. There are no seats and no terms.

When the patch could use another admin, an existing admin **nominates** a
member. The members vote on the nomination like any other proposal. If they
approve it, the person becomes an admin straight away. Nobody has to apply
it afterwards.

Only an admin can nominate someone, and the role menu will not make someone
an admin directly. An admin can withdraw a nomination they made while it is
still open.

### Elected

Members elect a council. This is the only model with seats and terms.

**Seats.** The council's size is however many seats it has. An admin can add
an empty seat or remove an empty one. A seat with someone in it cannot be
removed, so changing the council's size never unseats anyone. Adding or
removing a seat goes on the Record and does not send anyone a notification.
Adding a seat makes nobody an admin: it is filled the same way as any empty
seat, described below.

**Terms.** Each seat has a term end, and those dates are the patch's
election calendar. Patchwork opens an election early enough that it
finishes when the term ends. On Formal that is four weeks ahead: two weeks
of nominations, then two weeks of voting.

**Standing.** During the nomination weeks any member can stand, and any
member can put another member forward. Sitting admins can stand again.

**Voting.** Members vote by approval: you mark every candidate you would
accept. When voting closes, the candidates with the most approvals take the
seats that were up. A candidate needs at least one approval to be seated. If
two candidates tie for the last seat, the one who stood first takes it.

**Who can vote.** On Formal, a member has to have been in the patch for 30
days before they can vote. A patch younger than 30 days has no such wait.

**Holdover.** If an election decides nothing, the sitting council stays.
That covers nobody standing, too few people voting to reach quorum, and no
candidate getting a single approval. A term ending never removes anyone by
itself: the holder keeps serving until an election that does decide
something.

An election that seats some people but fewer than the seats it was for is a
decision. The people it seated take seats, the sitting admins it did not
return step down, and the seats nobody won are left empty.

**Empty seats between elections** can be filled without waiting for the next
one. An admin can nominate a member, or a member can stand for the seat
themselves with "Stand for this seat" on the Governance tab. Either way it
becomes a proposal the members vote on. Whoever is approved takes the seat
for the rest of its term, not a fresh term, so a council cannot reset its
own clock by appointing allies. Between elections you can only stand for
yourself: putting someone else forward is a nomination, and that is an
admin's act. This route needs at least one admin on the council; when there
are none, the next election fills the seats.

**The role menu.** On an elected patch, Settings → Roles and requests does
not make anyone an admin. It tells you how many seats are empty and points
you to the Governance tab, or tells you when the next election is.

### Recusal

A patch can turn on a rule that keeps the person a nomination is about from
voting on it. None of the templates turn it on.

## When admins go quiet

Most templates set an **inactivity period**: 90 days on Casual, 60 on
Collaborative, 30 on Formal. Minimal has none.

For this, taking part means voting on a proposal, making a proposal, or
commenting on one. Posting events does not count, and neither does voting
in an election.

- After one inactivity period without taking part, the admin is warned.
- After two, they stop being an admin. On an elected patch, their seat
  becomes empty.

This is the only thing in Patchwork that takes the admin role away from
someone without a person deciding it. It applies whatever the leadership
model, including on a patch that chooses its admins elsewhere.

If that leaves the patch with **no admins at all**, what happens next
follows the leadership model:

- **Leadership chosen elsewhere:** Patchwork appoints nobody. The members
  and an instance admin are told, and the patch records its next leadership
  decision when the community makes it.
- **Maintainer:** the named successor takes over, if there is one and they
  are still a member. If not, the patch's succession policy applies. On the
  templates that is to ask the patch's longest-standing members to step in
  (see below).
- **Meritocratic:** nobody can nominate, because nominating needs an admin.
  An instance admin is told.
- **Elected:** the next election is brought forward and opens shortly, and
  any member may stand. If the seats have no term end, no election can be
  scheduled, and an instance admin is told instead.

Except when a successor takes over, the members are told the patch has no
admins.

### The interim-admin offer

When the succession policy asks members to step in, Patchwork asks first.
It never makes someone an admin without their say. Up to three of the
patch's longest-standing members who have been around recently get a
notification asking whether they will step in as an interim admin. Here
"around" is wider than above: posting an event or a notice counts too.

They answer on the patch's Governance tab, where only they can see the
question.

- Accepting makes them an admin.
- Declining passes the question to the next person in line, as long as the
  patch still has no admins.
- An offer has no deadline. No clock makes anyone an admin, and no clock
  takes the question away.
- If everyone who has been around says no, an instance admin is told.

The other members are told that the longest-standing members have been
asked, but not who.

## Proposals

A proposal is how a patch makes a change. Any member or admin can make one.
Followers cannot.

The decision method says how a proposal is decided:

- **Majority:** more approvals than rejections.
- **Supermajority:** more than two-thirds of the approve and reject votes
  are approvals. Seven for and three against passes. Two for and one against
  does not.
- **Consensus:** at least one approval and no rejections. A single
  rejection stops it. Rejecting is allowed; it is how you block.
- **Admin decides:** see "When the admin decides" below.

**Quorum** is how many of the people who can vote must take part.
Abstaining counts as taking part. Collaborative asks for a quarter of
members, Formal for half, and Minimal and Casual ask for none.

**Lapsed is not rejected.** If voting closes without quorum, the proposal
lapses. It was not turned down; not enough people took part to decide it
either way.

**The rules are fixed when the proposal is made.** Its decision method,
quorum, threshold and who may vote are the ones in force when it was
created, even if the patch changes its rules while it is open.

**After a vote passes:**

- A nomination takes effect at once.
- An amendment to a document applies itself, unless the patch has turned
  automatic application off. Formal has it off, so there an admin applies
  it.
- Any other proposal is marked approved. An admin then marks it in effect
  once it has been done, because Patchwork cannot carry out most decisions
  itself.

### When the admin decides

On a patch whose decision method is "admin decides" (Minimal's), the admin
decides every proposal.

- **A member's proposal** goes to the admin. There is no vote and no
  deadline. The admin approves or declines it.
- **An admin's own proposal** is a direct change, unless they choose to ask
  the members first.
- **Asking the members first** opens a vote, but the vote is advisory. When
  it closes, the proposal goes back to the admin with the result, and the
  admin still decides. The admin can approve or decline at any point, even
  while the vote is running.

## Documents and amendments

A patch's governance documents live at Governance → Documents. A new
document is visible to members only unless the admin creating it chooses
otherwise. Making a document public is an admin's decision.

To change a document, a member proposes an **amendment**: the new text,
which the patch then decides like any proposal. A patch can set a different
threshold for amendments than for other proposals. Collaborative uses a
supermajority and Formal uses consensus. On an "admin decides" patch, an
admin's amendment takes effect directly.

### Changing your rules

A patch's governance rules are at Governance → Rules. You can read them
there without proposing anything. Changing them is a proposal like any
other, and the editor shows exactly what would change. On an "admin
decides" patch, and on a patch that decides its proposals elsewhere, an
admin's rules change takes effect directly.

What the rules editor can change: the decision method, where admins are
chosen, where proposals are decided, the "where we decide" note, the
succession policy, who can join, what followers can see and do, and, on a
patch that votes, the quorum, the voting window, the amendment threshold,
whether amendments apply themselves, how long a member waits before voting,
and recusal.

What it cannot change yet: the leadership model, the length of a term, the
inactivity period, and the length of an election's nomination window. These
stay as the template set them.

## The lining

Every patch starts with the **lining**, a shared community-standards
baseline that ships with Patchwork and includes an anti-discrimination
floor. It is always public and its title cannot be changed.

A patch can amend its lining the same way it amends anything else. A patch
whose lining no longer matches any version Patchwork has shipped wears a
public "Amended lining" badge. People can choose to hide patches with
amended linings from what they browse, and a quilt can choose to as well.

When Patchwork ships a new version of the lining, patches still on an older,
unamended version are updated, and their members are told.

The lining can never be recorded as adopted elsewhere. On a patch that
decides its proposals elsewhere, that means the lining cannot be changed at
all.

## Deciding elsewhere

Many groups already decide things at a meeting: a board meeting, a general
assembly, a monthly circle. Patchwork does not ask you to move that online.
Every patch has two settings in its governance rules:

- **Where admins are chosen:** in Patchwork, or somewhere else, recorded
  here.
- **Where proposals are decided:** in Patchwork, or somewhere else,
  recorded here.

They are separate. A patch can elect its board at an annual meeting and
still vote on small things here, or the other way round.

When something is decided elsewhere, Patchwork holds no vote and appoints
nobody. Instead an admin **records** what was decided, as an attestation:
the community's statement that it decided this somewhere Patchwork was not.
Two kinds of decision can be recorded: who holds admin, and the adopted text
of a document. Anything else the meeting decided belongs in its minutes.

The inactivity period still applies to admins on such a patch.

### Where we decide

A patch that decides anything elsewhere can say where, in a short note in
its governance rules directly under the two venue settings: up to 200
characters, such as "First Tuesday of the month, 7pm, the church hall." It
appears on the Governance tab, so visitors see where decisions are made
instead of only "outside Patchwork". The note is only offered while at least
one of the two settings is elsewhere.

### Recording a leadership decision

When admins are chosen elsewhere, an admin records each leadership decision
on the Governance tab, with "Record a leadership decision". It asks when it
was decided, what happened, and who it put in place. On an elected patch it
also asks when this council's term ends.

- **Who it put in place.** Pick members of the patch, or type the name of
  someone who is not a member here yet. Everyone listed is recorded as an
  admin. Once a typed person joins as a member, an admin can link their
  name to them on the record in force.
- **Who stood down.** Optional. Name anyone the meeting took off. This is
  for the record: a sitting admin who is not listed under "who it put in
  place" steps down whether or not they are named here. The same member
  cannot be on both sides of one decision.

Recording a decision makes it so. Every listed person who is a member here
becomes an admin, and every current admin who is not listed stops being one.
If nobody listed is a member here yet, nobody's role changes until someone
is linked.

**Correcting a record.** "Correct the last one" writes a new record that
replaces the most recent one. The old one stays on the Record.

**Entered in error.** If a record should never have been made, such as a
duplicate, an admin can mark it entered in error. Nothing is deleted. The
record stays on the Record, crossed out, with who marked it, and from then
on it counts for nothing. If it was the record in force, the one before it
is in force again, and admin roles go back to what that one says. If there
is no record before it, nobody's role changes. Only leadership records can
be marked this way, and only while the patch chooses its admins elsewhere.

### Recording an adopted text

When proposals are decided elsewhere, a proposal can still be made and
discussed here, but a member's proposal has no vote and no deadline. When
the meeting adopts a text, an admin uses "Record an adopted text" on the
document, and the adopted text replaces the document's whole text. A
mistaken one is fixed by recording the right text.

The governance rules and the lining are never recorded this way.

### Minutes

On a patch that decides anything elsewhere, any member can add a governance
document, such as the minutes of a meeting. A member's document starts
visible to members only, and an admin decides whether to publish it.

So a meeting usually produces up to three things here: the minutes, as a
document any member can add; a leadership decision, if the meeting changed
who runs the patch; and an adopted text, if it changed a charter.

## Passkeys and confirming

Some acts ask you to confirm it is really you, even though you are signed
in: naming a successor, recording a leadership decision, marking one
entered in error, and recording an adopted text. These change who holds
power or what a charter says. You confirm with your passkey, or with one of
your recovery codes from Settings → Security.

## Questions people ask

**Can I change my template later?** There is no switch from one template to
another. Change your governance rules instead, within what the rules editor
offers (see "Changing your rules").

**Our term ended and no election happened. Are we still the council?** Yes.
A term ending never removes anyone. If the election found no candidates, or
too few people voted, the sitting council stays.

**Somebody on our council stopped showing up. Can we remove them?** The
inactivity period does this for an admin who stops taking part. Another
admin can also change their role in Settings → Roles and requests, as long
as they are not the last admin. There is no member vote for removing an
admin.

**Why can't I just make someone an admin on our elected patch?** Because the
patch elects its council. Fill an empty seat by nomination, or wait for the
next election.

**Do followers get a vote?** No. Following costs nothing and asks nobody, so
it gives no vote. Ask the person to become a member.

**Our meeting decided something. Do we vote again here?** Not if the patch
is set to decide that kind of thing elsewhere. Record the leadership
decision or the adopted text, and put anything else in the minutes. Setting
a patch to decide elsewhere is itself a rules change, so on a patch that
votes, that change is put to a vote first.

**I recorded a leadership decision by mistake. Can I take it back?** Mark it
entered in error. It stays visible, crossed out, and stops counting.
