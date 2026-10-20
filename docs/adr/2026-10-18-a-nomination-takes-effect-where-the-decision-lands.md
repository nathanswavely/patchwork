# A nomination takes effect where the decision lands

Date: 2026-10-18. Status: **accepted**; implemented. Reconciles
docs/adr/051 and docs/adr/100 (admins nominate, the community ratifies;
ratification seats the nominee) with docs/adr/092 (on an admin-decides patch
the maintainer decides every proposal). Found while building governance
authoring for the iOS app (issue #408).

## Context

`leadership_model` and `decision_method` are two separate rules. No shipped
template pairs `meritocratic` or `elected` with `admin`, but nothing stops a
patch editing its rules into that pairing, and docs/adr/041 says the
ceremony follows whatever rules are in force.

On such a patch a nomination is reachable — `validateNomination` reads the
leadership model and never the decision method — and was impossible to
carry out. `ratifyNomination` had exactly one caller, `resolveProposal`,
and on admin-decides terms resolution never decides anything: an advisory
window closing hands the proposal back to the maintainer (docs/adr/092).
The three doors that do decide there — an admin's own proposal applied at
creation, `POST /proposals/{id}/decide` with `approve`, and
`POST /proposals/{id}/apply` on an open proposal — each settled the
nomination `in_effect` and promoted nobody. The governance record said a
person had been made an admin; the memberships table said otherwise.

It was worse than one dead control. The role dropdown is closed on
meritocratic patches (docs/adr/051) and on elected ones (docs/adr/100),
precisely so that nomination is the way in. A patch pairing either model
with admin-decides therefore had **no way left to make an admin**, short of
the instance admin's empty-council repair.

## Decision

**A nomination takes effect wherever the patch's decision lands.** On a
voting patch that is resolution, as before. On an admin-decides patch it is
the maintainer: all three doors above now ratify, after the apply has
settled, with the maintainer named as the decider.

- **Ratification is still the one function.** The same re-checks run on
  every door: a nominee who left is not dragged back, one already promoted
  is left alone, and on an elected patch the nominee is seated in a vacant
  chair inheriting its remaining term, or not promoted at all where no
  chair is left. The integrity argument of docs/adr/051 — an appointee
  serves out the remainder, never a fresh term — holds whoever decided.
- **The record says who decided.** `membership.ratified` carries the
  maintainer as its actor where they decided it, and no actor where the
  electorate did. The nominee's notification says "Your nomination was
  approved." rather than "The community ratified your nomination.", which
  would recite a vote that never ran — the failure docs/adr/041 names.
- **The approved → in_effect apply step does not ratify again.** On a
  voting patch resolution already ratified; running it a second time is how
  an approval that found no chair (docs/adr/100's unseatable case) would be
  carried out later by whoever pressed apply.

### Why not refuse nominations on admin-decides patches

The other answer issue #408 offered. It would honour 051's literal "the
community ratifies", and it would leave the patches this ADR is about with
no mechanism for making an admin at all, since the dropdown is closed on
both models. It would also be a rule nobody can see: the patch's own
charter says the maintainer decides, and 092 already settled that "decides
every proposal" means every one. A maintainer on such a patch can rewrite
the rules by direct change in any case, so a mandatory vote here would bind
nobody it was meant to bind.

The maintainer may still ask first. `put_to_vote: true` opens the advisory
vote 092 provides, and the members' answer is on the proposal when the
maintainer decides.

## Related corrections

- **An election is never the maintainer's.** docs/adr/051's holdover rule
  says the electorate closes a contest, on every patch. `decide` already
  refused one, but `GET /proposals/{id}` reported `can_decide: true` and
  `advisory: true` for it, and `apply` — whose admin-decides branch did not
  look at `seats_contested` — would settle it `in_effect` with nobody
  seated. Both flags are now false for an election, `can_decide` also
  requires the states `decide` accepts (`voting`, `awaiting_admin`), and
  `apply` refuses an open election with 409.
- **An applied amendment's page carries the text it replaced.** Once in
  effect, `current_doc_content` equals `proposed_body`, so the proposal's
  own diff was empty. `base_doc_content`, sent only for an amendment in
  effect, is the document at the first parent of the commit that applied
  it: the text it was written against when nothing moved in between, and
  the text it actually displaced when something did. The applied commit is
  used rather than `base_sha` because a revision rebuilds the branch and
  leaves `base_sha` behind. Empty means the amendment created the document;
  absent means the commit is unreadable (a repo rebuilt by docs/adr/084).
  It is the charter's text, so it is withheld wherever `doc_text_hidden` is
  true.

## Not changed

`CreateGovernanceDoc` lets an instance admin holding no role on the patch
create a charter. That is the instance-admin reach docs/adr/115 decides
should end and explicitly defers to issue #286; `UpdateGovernanceDoc` and
`canReadPatchDocs` carry the same reach, and closing one door of the shelf
while the others stay open protects nothing. It is now pinned by
`TestGovernanceDocWrites_WhoMay` so #286 flips it in one place. The proposal
verbs are different because a patch's *decision* was never the steward's
(docs/adr/092), and their bypass is already gone.

## Consequences

- A nomination on an admin-decides patch promotes the moment the
  maintainer approves it, and `membership.ratified` may now carry an actor.
- Clients reading `can_decide` stop offering Approve and Decline on an
  election; clients reading `advisory` stop labelling an election's ballot
  as advice.
- Clients diffing an applied amendment should diff `proposed_body` against
  `base_doc_content` when it is present, and fall back to
  `current_doc_content` when it is not.
