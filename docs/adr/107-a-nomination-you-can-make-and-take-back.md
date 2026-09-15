# 107. A nomination you can make, and take back

**Status:** accepted, 2026-09-15

## Context

The shipped bylaws say "any member may nominate themselves **or another
member** for a council seat" (docs/adr/051, `internal/governance/defaults.go`).
`AddCandidate` has always accepted a `user_id` for somebody else. Four
surfaces told members they could do it: the nominations notification
("Stand, or put someone forward"), the governance hub's banner, its link
("Stand, or put someone forward →"), and the vacant-seat blurb.

No surface had it. `ElectionPanel.stand()` posted an empty body, so the one
button on the page stood *you*, whatever you had come to do. And nothing
anywhere took a name back off a slate: there was no DELETE for a candidacy.

In epoch 4 of the governance simulation (docs/adr/096) that combination did
real damage inside one sitting. Ana came to put Kofi forward — he does the
work, he would never volunteer, and she had told him she would. She followed
the promise, found the only button, and pressed it:

> I pressed it and it put **me** on the ballot. No "are you sure?", no chance
> to pick a name... I stood for the board of a co-op by accident and I cannot
> un-stand. Twenty years of minutes and I have never seen a nomination that
> could not be withdrawn before the papers closed.

With no way off, she wrote a comment in the proposal's Discussion tab asking
members not to vote for her. Four of them read it and each withheld an
approval — on a three-seat ballot with two names. Kofi, meanwhile, was told by
two people to press "Stand for election", could not find it (nominations had
closed by the time he arrived), and was never on the slate at all. Priya spent
"four or five minutes, genuinely" hunting for the same missing control.

Six people, one missing route and one missing button.

## Decision

**The picker exists.** Nominations offer, beside "Stand for election", a
member chooser and "Put forward". It lists the patch's active members and
admins, minus whoever is already standing, **minus yourself** — standing is
its own button, and offering your own name inside a control labelled "put
another member forward" is how the accident happened in the first place. The
list is paged through to the end (docs/adr/095 pages that endpoint), because a
picker that stops at twenty silently makes the twenty-first member
un-nominatable.

**`DELETE /api/v1/proposals/{id}/candidates/me` takes your own name back**,
and the route says whose. Your own and nobody else's: a nomination you did not
ask for is withdrawn by *you*, because you are the person it is about, and a
nomination somebody made by mistake is their own candidacy and theirs to take
back. Both cases are covered without recording who put a name up, which
`election_candidates` does not.

**Only while nominations are open.** Once the ballot is running people are
approving a slate, and a name leaving it mid-vote would silently discard
ballots already cast for it — docs/adr/047's frozen terms applied to the
slate rather than to the rules. The refusal says so.

**A nomination seats nobody and needs no acceptance.** This is the question
docs/adr/098's consent rule raises, and the answer differs here for two
reasons. A candidacy is not a membership: it puts a name on a ballot for a
window, it grants nothing, and the electorate decides. And there *is* a
calendar — docs/adr/102 accepted an un-consented interim promotion partly
because nothing would retry it if nobody answered, whereas an unanswered
nomination simply closes with the window. What makes it acceptable is the
exit: withdrawal is the consent, taken after the fact rather than before, and
it costs one click. Without the DELETE route this decision would not be
defensible, which is why they ship together.

## Consequences

- The four surfaces that promised this become true rather than being reworded.
  That was the cheaper fix and the wrong one: the bylaws promise it, the
  server does it, and the product was the only part that did not.
- A member can now put a colleague on a public ballot without asking them
  first. That is what the bylaws say and what most small organisations do; the
  nominee finds out through the ordinary notification and can step off. If it
  proves to need an invitation instead, that is a change to the *act*, and
  the withdrawal route stays either way.
- Nothing stops somebody nominating the same person again after they withdraw.
  A slate is small and public and the patch's members can see it happening;
  a block-list on a fortnight-long window would be machinery for a problem
  nobody has had yet.
- A candidate who withdraws leaves the slate shorter, and an empty slate now
  settles at the close of nominations (docs/adr/106). Withdrawing last can
  therefore end a contest, which is correct — a contest with nobody standing
  is a contest with nobody standing.
