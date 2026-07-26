# The electorate is one set

A follower could vote. `VoteOnProposal` gated on `userHasMembership`,
which counts *any* active membership row, while `eligibleVoters` — the
denominator every quorum calculation divides by — counts only active
admins and members. The two halves of the same arithmetic were reading
two different tables' worth of people, so a follower's ballot landed in
the numerator and was invisible to the denominator. `totalVotes * 100 /
activeMemberCount` could exceed 100. A patch with two members and fifty
followers met any quorum on follower votes alone, and under the Casual
and Collaborative defaults (`amendment_auto_apply: true`) that resolved
straight into an applied charter amendment.

This was the third appearance of one mistake. ADR 042 found the second —
`CreateEvent` testing direct-post rights with the same helper, so a
follower's event published straight to a patch's calendar — and fixed
that door while explicitly leaving the helper alone, on the reasoning
that "comments and proposal participation legitimately admit followers
under `follower_permissions`." That reasoning does not survive contact
with the vote gate. `follower_permissions` is `{events, proposals,
charters, members}`, and it is a **read** gate — `canReadPatchDocs`
returns `fp.Charters` to decide who may *see* a charter. There is no
field in it that could grant voting, and none that grants commenting
either. The sentence named a permission mechanism that does not exist,
which is how a helper that had just been caught being wrong about one
door was cleared for two more.

We decided **the set of people who may vote and the set of people who
are counted are one set, expressed once**:

- **The gate is the electorate.** `VoteOnProposal` now applies exactly
  the condition `eligibleVoters` applies. Not a condition that happens
  to agree today — the same one, so the two cannot drift apart again.
  Following is frictionless precisely because it carries no write
  rights; a follower is an interested observer, not a member
  (CONTEXT.md, "Member count").

- **Tallies filter at read time; votes are never purged.** Membership
  moves underneath an open proposal: a member can vote and then be
  demoted to follower, leave (`LeaveNode` sets status `left` rather
  than deleting the row), or be banned. Nothing cleaned up their vote,
  so the same numerator/denominator divergence was reachable without
  any follower involved. The fix is a shared `countedBallot` predicate
  applied by every surface that counts — list, detail, voter list, and
  resolution — rather than a purge on demotion. Purging would put
  correctness at the mercy of every membership-mutation path
  remembering to call it, and would destroy the record of who voted,
  which is worth keeping. A vote is a fact; whether it counts is a
  question asked fresh each time it is read.

- **An instance admin does not vote in a patch.** The gate carried the
  usual `user.Role == "admin"` bypass, so a site-wide admin could vote
  in a patch they hold no role in — and, since `eligibleVoters` never
  counted them either, do so as another uncounted ballot. An instance
  admin "curates instance-wide options; does not override per-patch
  choices" (CONTEXT.md, "Instance admin"), and ADR 026 already refused
  instance authority reaching into an active patch over the far smaller
  matter of an event queue. A patch's vote is its own. An instance
  admin who is also a member votes as that member, like anyone else.

- **Instance admins may speak, not decide.** The same bypass survives
  in `CreateProposal` and `CreateComment`, deliberately. The principle
  is about overriding a patch's *choices*: proposing and commenting put
  something in front of the members, who still decide what comes of it.
  Voting is deciding. The line is drawn at the decision, not at the
  door.

  > **Narrowed by the fourth amendment below.** This grouped proposing
  > with commenting as speech. Proposing has since become a member act,
  > and the bypass went with it — an instance admin keeps it for
  > commenting and for stewardship, not for raising proposals.

- **`is_member` is not narrowed, and is not the member test.** The node
  payload sets `is_member` for any active membership, followers
  included. Narrowing it at the source is the cleaner-looking fix and
  is wrong: there are call sites where the follower-inclusive meaning
  is the intended one — `PatchProfile` uses it to decide whether to
  *fetch* governance, and a follower with `fp.proposals` should get
  that fetch. The field answers "does this viewer have standing here?",
  which is a real question; it is simply not the question "is this
  viewer a member?" Write rights derive from `membership_role`
  (`isMemberOrAdmin`, ADR 042) — never from `is_member`.

- **The entry point and the route are gated separately.** The
  governance hub offered "Propose a change to these rules" to
  followers, and `RulesProposalEditor` — reachable by URL — had no
  membership guard at all, so hiding the button would have hidden the
  entrance while leaving the door. Both now derive from
  `membership_role`. A gate that exists only on the control that leads
  to a surface is not a gate on that surface.

**Considered and rejected: deleting `userHasMembership`.** Three
appearances of one bug is a strong argument for removing the helper and
forcing every call site to name its roles. The remaining caller —
`CreateComment` — genuinely admits followers, so it would become
`userHasNodeRole(…, "follower", "member", "admin")`: the same behavior,
stated. That is still the right cleanup, but it is a rename dressed as a
fix; what actually made this bug survivable was that the gate and the
electorate were written in two places. Fixing the duplication is the
load-bearing change, and the helper's fate can follow it. Left as
follow-up rather than smuggled in here.

> Done, once the duplication was fixed and `CreateProposal` stopped calling
> it — see the third amendment below. Deferring was right: the helper turned
> out to be wrong at a fourth door too, and deleting it early would have
> renamed that bug rather than found it.

**Amended 2026-07-26 — the nudge is a counting surface too.** The governance
hub's "N proposals need your vote" counted every open proposal the viewer
hadn't voted on, with no role or tenure condition at all: a fourth place the
electorate was described, and the one people actually read. Closing the vote
gate turned it into a visible dead end — a follower was told two proposals
needed their vote and the server answered 403 — and a member short of
`min_voting_tenure_days` got the same round trip. Rather than write the
condition a fourth time, it now comes from `electorateFilter`, the single
fragment `eligibleVoters`, `countedBallot`, the vote gate, and the count are
all built from; the gate's tenure check, which had been its own Go-side date
parse, is the same fragment now as well. Asking someone to vote is part of the
same arithmetic as counting them.

**Amended 2026-07-26 — the quorum denominator was never eligibleVoters.**
This ADR says `eligibleVoters` is "the denominator every quorum calculation
divides by." It wasn't. `resolveProposal` — the calculation that actually
decides whether a proposal resolves — counted active admins and members with
its own inline query, ignoring `min_voting_tenure_days` entirely, so it
divided by people the gate refuses. The consequence was worse than a wrong
percentage: on the shipped Formal defaults (quorum 50%, tenure 30 days), a
patch where more than half the members joined inside the tenure window could
not reach quorum at all. Every proposal sat open past its window and never
resolved, with nothing to see — no error, no rejection, just a vote that never
ended. Collaborative (25%, 7 days) is reachable but overstated by the same
arithmetic. The resolution math now divides by `eligibleVoters` like the
displayed count always did.

The client was the far side of the same mistake. `ProposalDetail` derived
`canVote` from `membershipRole`, which cannot see tenure — so a member inside
the window was shown the buttons and refused on click, and the status banner
told them to "cast your vote below" when there was no vote below. The
electorate is not something a client can work out; the proposal payload now
carries `can_vote`, computed by `inElectorate`, and the page renders that
answer rather than reconstructing one. Which surfaces gate is a UI question;
who may vote is not.

**Amended 2026-07-26 — the voter list is the record, not the tally.** The
tally filter above was applied to the displayed voter list as well, which
answered "does this ballot count" by erasing "was this ballot cast". It also
broke direct-change detection: `ProposalDetail` reads an empty voter list as
"no vote ever happened" to recognise a record born applied under admin-decides
rules (docs/adr/041), and that inference is only sound while the list is
complete. Filtered, a proposal that was genuinely voted on and passed would —
once its voters had left or been demoted — render as one applied without a
vote, and suppress its own vote history. A governance record must not describe
a vote that happened as a vote that did not. The list is now every ballot, each
carrying whether it counts; the counts stay filtered. The two legitimately
disagree, and the list says which rows are which.

**Amended 2026-07-26 — proposing is a member act, and the helper is gone.**
The paragraph this replaces recorded followers authoring proposals as an open
policy question. It is decided: they may not. The reasoning is the ladder
rather than the act — a patch that wants open governance already has
`membership_policy: "open"`, where joining makes you a member in one click, so
the release valve is making the step cheap, not granting the rung below it the
right. Someone who wants a hand in governance takes the step. This also puts
the code back in agreement with the shipped Collaborative operating agreement
("Any member can propose changes to this operating agreement"), which it had
been contradicting. The gate was missing on three UI routes and on the server
behind all of them; `mayPropose` now reads its role condition off
`electorateMembership`, omitting the tenure clause, because a minimum *voting*
tenure gates casting a ballot rather than raising the question.

With that, `userHasMembership` had one caller left and is deleted. Four doors
in a row were wrong because a helper that read as "is a member" admitted
followers. `CreateComment` — the one governance act followers keep — now names
them: `userHasNodeRole(…, "follower", "member", "admin")`. Same behaviour,
stated. The rule that outlives all of this: a gate names the roles it admits.

**Amended 2026-07-26 — governance participation needs standing in the patch.**
The two questions left open above are answered, one of them by turning out not
to exist.

The `follower_permissions` gap was misdiagnosed. It was recorded as "a follower
who cannot see proposals can still comment on one", but `fp.Proposals` gates
nothing server-side — it has no backend reference at all — and proposal and
comment reads are `AuthOptional` or fully public. Proposals are public
deliberation by design, which is the same premise `hiddenDocRedactor` rests on
when it withholds the mirrored charter text and leaves the proposal itself
readable. `fp.Proposals` decides whether the workspace surfaces a proposals tab
to followers: curation, not permission. Commenting on a proposal anyone can
read is not incoherent, and there is nothing to fix.

The instance-admin bypass was real, and the ground moved under the "speak, not
decide" line above: it defended the bypass by grouping proposing with
commenting as speech, and proposing has since become a member act. A member act
is a member's alone. `CreateProposal` no longer accepts `user.Role == "admin"`,
so a site-wide admin holding no role in a patch cannot raise a proposal in its
governance — that is instance authority reaching into a per-patch choice, which
CONTEXT.md ("Instance admin") and ADR 026 both refuse over much smaller matters.

The bypass stays where the act is speech or stewardship: `CreateComment`,
because explaining a moderation action in the thread it concerns is a thing
stewardship needs and a comment decides nothing; and withdraw, apply, and
comment moderation, which are stewardship outright. One rule underneath:
**governance participation requires standing in the patch.** Instance admins
moderate; they do not govern other people's patches. Wanting a voice in one is
what joining is for.
