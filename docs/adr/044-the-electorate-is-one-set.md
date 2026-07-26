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

**Open, deliberately:** followers can still author proposals
(`CreateProposal` admits any active membership), while the shipped
Collaborative operating agreement says "Any member can propose changes
to this operating agreement." That is a governance policy question — how
open deliberation should be — not a defect in the tally, and it wants a
decision rather than a patch.
