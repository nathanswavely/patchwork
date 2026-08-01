# The record is assembled, not written

docs/adr/053 named the minute book as a separate decision: whether
Patchwork records decisions that change nothing, and if so under what
name and what gate.

Working through it, the premise turned out to be wrong in an instructive
way. 053 assumed the gap was a **writing** gap, and put its value in
being "a public, federated, portable record". Two things undercut that:

- **A charter already is one.** A community can create a governance
  document called "Minutes 2026" today. Versioned, per-document
  visibility, federated when public, travels in seamrip, full git
  history. Every property 053 named, a charter has. A second document
  store for the same thing is duplication.
- **CONTEXT.md already ruled on the word.** The attestation entry lists
  under *Avoid*: "minutes (those are the community's own document)."
  Minutes are a text the community writes and owns. Patchwork holding
  "the minutes" claims something that isn't ours to claim.

The gap that is real is a **reading** gap, and it is demonstrable rather
than asserted. Patchwork already holds every decision a patch has made:
proposals with their outcomes, elections and who they seated, both kinds
of attestation, direct changes. No route shows them as a sequence. The
hub shows counts, the proposals list shows one status at a time, and a
member asking "when did we change the quorum?" goes hunting.

So we decided **a record that assembles from what already exists, with no
table, no write path, and nothing to keep in step.**

## What follows from assembling rather than storing

- **It federates and travels for free.** Every entry is a view of a row
  another feature owns, and those rows already federate (docs/adr/054)
  and already cross the seamrip boundary (docs/adr/002). A stored record
  would have needed both taught to it, and would have been a fifth place
  for governance history to drift out of step with the other four.
- **It cannot disagree with the pages it links to**, because there is no
  second copy to disagree with.
- **Nothing new is disclosed.** It is a public read, like the proposals
  it draws from, and every entry links to a page that was already public.
- **It is called the Record.** Not the minute book: we are not holding
  anyone's minutes, and CONTEXT.md said so before this was written.

## Only settled things

An open proposal is an argument in progress. Putting it here would make
the record a to-do list, and the proposals list is already that.

A superseded council attestation stays out too. Corrections supersede
rather than edit (docs/adr/052), and both stay readable on the governance
hub where the correction sits beside what it corrects; in a chronological
list they would read as two councils seated on the same day.

## No tally, and this one was found on screen

The first version showed the vote counts beside the outcome. On the first
seeded patch it rendered:

> Did not carry. 2 for, 1 against.

Two for, one against, and it did not carry. Both numbers were right and
the sentence was nonsense, because they come from different places. The
outcome is written into `status` when a vote resolves and never moves
again. The tally is recomputed on every read, and drops ballots from
people who have since left the patch (docs/adr/044). The counts that
actually decided it were never stored — only the votes, re-filtered
forever after.

So the record states the outcome, which is fixed, and links to the
proposal, which holds the voter list in full and marks which ballots
still count. Losing the at-a-glance number is a real cost. A governance
record whose arithmetic changes as people leave is a worse one.

The general form is worth keeping: **a stored conclusion and a
recomputed explanation must not be rendered as one sentence.**

## What this leaves open

- **Recording decisions Patchwork had no part in** — the writing half of
  053's minute book. Deliberately unbuilt. The reading gap was
  demonstrable; the writing gap is asserted, and a community that wants
  to record "we approved the kiln" can raise a proposal today, which on
  an `elsewhere` patch is exactly the discussion-with-no-ballot 053
  built. If it comes back, it comes back with evidence that a charter and
  a proposal both failed to serve.
- **Pagination.** The record returns everything. A patch with a decade of
  governance will want a cursor, like every other list endpoint
  (CLAUDE.md). Not built because no patch is near it, and the shape of
  the fix is already established.

**Status: adopted and implemented.** `GET /nodes/{slug}/governance/record`
and a Record tab in the governance workspace.
