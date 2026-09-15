# 097. A window that closes settles something

**Status:** accepted, 2026-09-14

## Context

The first simulated month of governance (docs/adr/096) found two things
about an ordinary proposal's voting window, both on the co-op's first
proposal under the Formal template.

**Nothing closed it.** Elections have had a sweep since docs/adr/051 —
nominations close on the clock, votes end on the clock, the council is
seated by nobody. Ordinary proposals resolved in exactly two places:
when somebody opened the detail page after `voting_ends_at`, and when a
sole eligible voter cast a decisive ballot. Until a reader happened by,
the list said "voting", the governance overview counted it among the
votes a member still owed, and the "resolved" notice went out at
whatever hour that reader arrived, in their name. A window with no
mechanism that ends it is a window in the copy only — docs/adr/049's
rule, stated for the rules editor, applied to the clock.

**Under quorum it never closed at all.** `resolveProposal` read "quorum
not met — leave open" and returned, whether the window was still running
or had ended a month ago. The vote endpoint, meanwhile, refused ballots
after the deadline. So a proposal that missed quorum sat `open`/`voting`
forever, could not be voted on, resolved to nothing, told nobody, and kept
counting toward every member's "needs your vote". On the Formal defaults
(quorum 50 %, thirty days' tenure before a vote counts) this is the first
proposal a new community will ever run, because most of its members are
inside the tenure window when it opens.

Elections already had the answer to the second problem: a contest that
settles nothing closes as unsettled, the council holds over, everybody is
told, and the calendar tries again one contest length later. Proposals
had no word for the same thing.

## Decision

**The clock ends a vote.** `handler.SweepProposals` runs from the same
hourly loop as the election sweep and resolves every ordinary proposal
whose window has closed — the production `resolveProposal`, not a second
implementation — so the state a member sees is the state the vote is in,
not the state it was in when someone last looked. The read-time path
stays, because a reader ahead of the sweep should still see the truth;
it is no longer the only path. An advisory vote lands back on the
maintainer on the hour rather than on the next read (docs/adr/092 asked
for exactly that and got it only by accident).

**A vote that misses quorum at close lapses.** Nobody decided anything,
so it is neither approved nor rejected as those words are used
everywhere else. It gets `state = 'lapsed'` and `status = 'rejected'` —
the status because the schema's CHECK allows only `open`, `approved`,
`rejected`, `withdrawn`, and adding a fifth would be a migration for a
word; the state because the state column is where the product already
tells `awaiting_admin` from `voting` and `in_effect` from `approved`, and
the UI reads state first. The banner, the list row, the badge and the
notice all say *not decided*, never *rejected*, and the tally is shown,
because a vote that few turned up to is a fact the community should see.
Audited as `proposal.lapsed` with no actor: the clock closed it.

**Under quorum while the window runs, nothing changes.** Votes may still
come. The lapse is decided at close, by the same code on the same read.

**Born elected means adopted elected.** *Superseded the same day by
docs/adr/098: a founder alone has nobody to elect from, so creation now
seats the founder for a founding term and the calendar opens the first
real contest before that term ends. The intent — that the calendar runs
at all — stands.* Applied here rather than decided:
docs/adr/051 says adopting elected leadership starts an election, and a
patch created under the Formal template adopts it at birth. The creation
path never fired that trigger — only a rules edit did — and since seats
are created only by a resolved election, a founder held no seat, nothing
came due, and the calendar never ran. `CreateNode` now calls the same
`StartElectionOnAdoption` a rules edit calls. The founder holds over
through that first contest like any sitting admin, and an electorate that
is still inside its tenure window lets the contest settle nothing and try
again — which is holdover doing what it is for.

## Consequences

- A "Rejected" filter now also lists lapsed proposals, each labelled
  *lapsed*. The filter is by status; the row is by state. Splitting the
  chip was considered and not done: three outcome chips for a list most
  patches never fill is a fourth tab for the sake of a word.
- `proposal_state_backfill` and every reader of `state` gain one value
  and no migration. Anything that renders a state it does not know falls
  through to the status, which reads *rejected* — the one word this ADR
  says not to use, which is why every surface was taught the state
  explicitly rather than left to fall through.
- A patch that reaches quorum only after the deadline is closed to it.
  Late enthusiasm reopens nothing; a member who wants the question asked
  again raises it again, with the record of the lapse behind it.
- The simulation's `advance` runs this sweep too (cmd/sim), because the
  point of docs/adr/096 is to run the product's own passes.
