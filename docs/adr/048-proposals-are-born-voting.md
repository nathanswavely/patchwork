# Proposals are born voting — draft and discussion are retired

The proposal page has shown a "Submit for voting" button to the author of
a `draft` proposal since migration 016 introduced the state machine. It
has never worked. It PATCHes `{state: "voting"}` to
`/api/v1/proposals/{id}`, and `UpdateProposal` decodes only `title`,
`body`, and `status` — the field is dropped, `setClauses` comes back
empty, and the API answers 400 "no valid fields to update." Nobody
reported it, because nobody could reach it: `CreateProposal` starts every
proposal in `voting` (or `in_effect` for a direct change under
docs/adr/041), and no other path writes `draft` or `discussion`. The
control sits behind a state the system cannot produce, and the button
that would leave it is the half of the loop that got noticed first.

docs/adr/047 found the same dead transition from the other side and left
it open, because the answer changes where the voting-terms photograph is
taken. This is that answer.

We decided **a proposal opens for voting when it is created; `draft` and
`discussion` are retired**.

- **The two states were never governed.** A staged flow has to say how
  long discussion lasts and what ends it, and `GovernanceConfig` has no
  field that says either — no discussion period, no promotion rule, no
  setting on any template. `duration_hours` and `quorum_percent` and
  `min_voting_tenure_days` all describe the vote. Wiring the transition
  would not be finishing a feature; it would be inventing the governance
  rule the feature needs and then finishing it. That is a design, not a
  repair, and nothing on record asks for it.

- **The voting clock already starts at creation.** `voting_ends_at` is
  stamped `now + duration_hours` at INSERT. A proposal parked in `draft`
  burns its own window while it sits there, and a month-old draft would
  open for voting already expired. The schema was written for proposals
  that are born voting; the states were added beside it and never given
  the columns they would need.

- **docs/adr/041 re-decided initial state and produced two outcomes.** It
  is the most recent deliberate thinking about proposal ceremony, it
  rewrote `CreateProposal`'s state selection from scratch, and its
  spectrum is direct change versus vote. A draft phase is not mentioned
  once. If the staged flow were live intent, that is the ADR it would
  have survived in.

- **Discussion already happens — during the vote, not before it.** The
  proposal page has a Discussion tab with threaded comments, open the
  whole time voting runs. The deliberation the staged flow was meant to
  stage is in the product; it is concurrent rather than sequential. What
  was missing was never the conversation.

- **A draft was unreachable from both ends.** `CreateProposal` accepts no
  `state`, the proposal form has no "save as draft," and `ProposalList`
  filters on `status` with `open` as its default, so a draft would not
  appear in the list that would have to show it. Repairing the PATCH
  alone would have wired the exit to a room with no entrance.

- **`UpdateProposal` refuses `state` out loud.** It could keep dropping
  the field — the 400 is already correct by accident — but "no valid
  fields to update" reads like a bug in the caller, and the next person
  to re-add the control gets the same twenty minutes back. The handler
  now names the decision at the exact line where it would be
  re-litigated.

**Consequences for docs/adr/047:** the voting-terms photograph stays at
INSERT, and its conditional — "if that transition is ever wired up, the
photograph moves with it" — is now settled rather than pending. Creation
*is* the moment voting opens, so the two are the same instant by
construction and cannot drift apart. 047 landed while this was open; its
conditional paragraph now carries a pointer here rather than reading as
work still owed.

*Amended by docs/adr/051: elections are the exception. An election opens
for nominations and starts voting when they close — the shipped
succession plan puts a nomination period ahead of the vote — so for that
one proposal type creation and the start of voting do drift, and 047's
photograph is taken at the transition rather than at INSERT, which is
what 047 asked for. The claim above holds for every other proposal type,
including the meritocratic ratification vote, which is an ordinary
proposal and is born voting like any other. 051 also explains why a
nomination window is not the flow retired below: what killed `draft` and
`discussion` was that nothing governed them, and a nomination window has
a length, a purpose, and a defined end.*

**Considered and rejected: building the flow.** A draft phase is a real
thing communities want — a place to circulate wording before the clock
starts. It is also a governance rule (who sees a draft, how long
discussion runs, who promotes it, whether promotion is itself a member
act), a schema change (`voting_ends_at` moves to promotion time,
`voting_terms` with it), and three UI surfaces. Doing it as a side effect
of repairing a broken button would ship the smallest possible version of
a decision that deserves its own. If it comes back, it comes back as its
own ADR with the rule written down first.

**What stays:** the `state` column and every state a proposal can
actually be in. `draft` and `discussion` remain in migration 016's
comment and in the `status`-to-`state` guard in migration 043 — an
applied migration is history and is not rewritten, and 043's guard is
written as "not already terminal," which is correct whether or not the
pre-voting states exist. No rows carry either value: the column defaults
to `voting`, `CreateProposal` never writes them, and the seeder omits the
column entirely.
