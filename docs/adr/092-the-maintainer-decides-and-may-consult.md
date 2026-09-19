# ADR 092: The maintainer decides, and may consult

Date: 2026-09-07. Status: **accepted**; implemented. Extends docs/adr/041
(ceremony follows the rules in force) and docs/adr/047 (frozen terms);
closes a bypass docs/adr/041 described and did not close.

## Context

The Minimal template says "the maintainer makes all decisions for this
patch" (`decision_method: "admin"`). docs/adr/041 made an admin's proposal
on such a patch a **direct change** — born applied, never voted — and
promised the UI would never say "propose", "submit" or "vote" for one.

Two things were left unsaid, and both were wrong in production.

**A member's proposal on that patch went to a member vote.** CreateProposal
read the decision method only to decide what to do with an *admin's*
proposal. Anyone else's was born voting with the patch's default window and
resolved by `resolveProposal`, whose threshold switch has no `admin` case
and falls through to majority. On a patch whose rules say the maintainer
decides everything, two members could carry a proposal over the
maintainer's reject. The reference instance's sole-admin patch surfaced the
other half of the same gap: the general proposal form offered its admin a
voting-duration picker and a "Submit Proposal" button, then applied the
proposal the instant they clicked, and the page showed a change in effect
that nobody had been able to vote on. The rules editor honoured 041's
promise; the general form and the amendment editor never learned it.

**Any admin could apply any open proposal.** `ApplyProposal` accepted
`state = 'voting'` from any patch admin, and from any instance admin, on
every patch. The UI never offered that button mid-vote, but the endpoint
did, and 041's own words — "every voting method votes, admins included" —
were true of the vote gate and false of the apply gate.

## Decision

On a patch whose decision method is admin-decides, **the maintainer decides
every proposal, and may consult the members before deciding.** Nothing
here touches a patch that decides by majority, supermajority or consensus.

- **A member's proposal is a request to the maintainer.** It is born
  `state = 'awaiting_admin'` with `voting_ends_at` NULL: open, discussable,
  withdrawable, and carrying no ballot. No clock ends it. This mirrors
  docs/adr/053's `elsewhere` shape — an open proposal the tally does not
  decide — rather than reviving docs/adr/048's retired `discussion` stage,
  which was a stage ahead of a vote that would come; here the vote may
  never come.
- **An admin's proposal is a direct change unless they ask first.** The
  create request takes `put_to_vote: true`; without it an admin's proposal
  is born applied as before. With it, or when an admin opens a vote on a
  waiting proposal (`POST /proposals/{id}/open-vote`), the vote is
  **advisory**.
- **An advisory vote decides nothing.** It runs on the ordinary ballot and
  the ordinary clock, the electorate is the ordinary electorate, and its
  window closing moves the proposal back to `awaiting_admin` with the tally
  attached. The sole-voter early close (docs/adr/041) does not fire: the
  sole voter is not who decides. The members can be asked once — a
  proposal whose window has run carries its advice, and a second window
  would be asking again until the answer changed.
- **The maintainer's two verbs.** `POST /proposals/{id}/decide` with
  `approve` or `decline`, available to the patch's admins at any moment the
  proposal is open — mid-vote included. Approve runs the one apply path
  (merge, sync, `applied_by`); decline stamps `declined_by` (migration 067)
  so a rejected row with no tally is never mistaken for a vote that
  failed. The governance record (docs/adr/055) shows both as a person's
  decision: "Applied by …" / "Declined by …", never "did not carry".
- **The gates read the proposal's frozen terms** (docs/adr/047), not the
  patch's live rules. A patch that moves to majority does not turn the
  maintainer's open questions into member verdicts; one that moves to
  admin-decides does not hand a running majority vote to its admin.
- **Apply is closed on voting patches.** `ApplyProposal` now accepts the
  approved → in_effect step on any patch, and an open proposal only where
  its terms are admin-decides — where it *is* the decision. The
  instance-admin bypass is gone, as it already was from the vote gate: a
  patch's decision is its own.
- **Ceremony in the UI matches ceremony in force, on every door.** The
  general proposal form and the amendment editor derive the same
  `directChange` the rules editor does. An admin on an admin-decides patch
  sees "Apply it now" or "Ask the members first" and a button that says
  which; a member sees that their proposal goes to the maintainer; the
  duration picker appears only where a vote will run. The banner leads an
  advisory vote with the word *advisory*, and the vote section says "this
  vote advises; it does not decide" where a quorum line would be.

### Why advisory, not binding

A vote the admin can override at any time is advisory whatever it is
called. Making it binding would have meant either removing the mid-vote
decision — and with it the reason a maintainer would consult at all — or
letting the admin choose after seeing the tally whether it counted, which
is the tally-versus-attestation abuse docs/adr/053 refused. Naming the
vote for what it is costs a word on the banner and nothing else.

### Why not refuse member proposals

The cheapest fix was to hide the form from members on admin-decides
patches. It was rejected because it removes the one thing a member can do
on such a patch — put something in front of the maintainer where the
whole patch can see it and argue about it. The contributor ladder
(follower → member → admin) is the project's core pattern, and a rung
that can only watch is not a rung.

## Consequences

- One new proposal state, `awaiting_admin`, on the migration-016 column;
  no CHECK constrains it. Migration 067 adds `proposals.declined_by`, which
  travels in the seamrip beside `applied_by`.
- The `needs_vote` count skips waiting proposals, since there is no ballot
  to cast, and counts advisory votes, since there is.
- A direct change no longer carries a `voting_ends_at` 72 hours out, and
  `can_vote` is false on anything that is not open — the payload
  contradiction docs/adr/044 was written to end had survived on this one
  row.
- The vote section's threshold label now prefers `amendment_threshold`
  only for amendments, as `resolveProposal` always did; an action proposal
  on a Collaborative patch no longer promises a supermajority the server
  never asks for.
- `internal/handler/proposal_admin_decides_test.go` holds the scenario
  matrix: every template × every role through create, vote and resolve,
  asserting where the decision lands. A template edit that moves power has
  to come here and say so.
- Not done: the e2e suite still exercises the seeded majority patch only.
