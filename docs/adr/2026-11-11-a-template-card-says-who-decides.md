# A template card says who decides

**Status:** accepted, 2026-11-11. Amends docs/adr/104.

## Context

docs/adr/104 made the rules editor state what a rule costs the patch being
edited, and deliberately left the four template cards at patch creation
alone: they have no patch to do arithmetic about. It recorded F-032 as
standing.

F-032 is that the cards routed real organisations to the wrong mechanics.
Each led with a kind of group and a size: "Venues, co-ops, makerspaces
(20 to 100 people)", "Arts districts, coalitions (100+ people)". A
forty-member venue board with an elected board needs the one template that
elects a council, and the size sent it to the meritocratic one. A
four-person press was nearly put off the template that fit by "100+". Nothing
described "two of us run it, no votes", and nothing said a community could
decide at a meeting, which is what several of the simulated organisations
actually do. Every preview then led with quorum, including Minimal's, which
read "Quorum 0%, any number of votes counts" under "Admin decides".

## Decision

**A card says who decides and how, then who it tends to suit, with no size.**
The four descriptions, in `internal/governance/defaults.go` and in
`web/src/pages/PatchForm.svelte` verbatim:

- **Minimal:** You decide. Members can propose, and you approve or decline,
  or ask them to vote first.
- **Casual:** Members vote for three days, and a proposal passes with more
  approvals than rejections. The admins run things day to day.
- **Collaborative:** Members vote for a week, and a quarter of them must
  take part. Admins nominate new admins, and members confirm them.
- **Formal:** Members elect a council for one-year terms. A proposal needs
  half the members to vote, and fails if anyone rejects it.

The examples stay as examples, without head counts, and Formal now names
boards first, since a board is the commonest organisation that elects its
leadership. Casual's leadership label said "Maintainer → Meritocratic" on
the server and "Maintainer" on the form; its rules say maintainer, so both
say that.

**Every card says a decision can be made at a meeting instead.** Where admins
are chosen and where proposals are decided are settings every patch has
(docs/adr/052, docs/adr/053), whichever template it starts from, so the line
is the same on all four: "Decisions can be made at a meeting instead, and
recorded here." The preview adds where that is set.

**The preview leads with who decides.** Leadership first, then the decision
method, and only on a template that votes, the quorum and the voting window.
Where the admin decides there is no vote for those numbers to describe; an
advisory vote the maintainer asks for is covered by the decision row.

**The two copies are held together by a test.** The form carries the card
text so it renders before any request returns.
`TestTemplateCardsMatchTheServer` fails when it drifts from the server's
`TemplateInfo`, and when a card names a group size again.

## Consequences

- A card can be wrong about a mechanic in the same way a document can, so
  each sentence was checked against the rule it describes. "Members vote"
  on Casual is the template's default; a patch that changes its rules
  afterwards is described by its rules page, not by the card it started
  from.
- Picking a venue at creation is still not offered. The card says it can be
  done; doing it stays in the governance rules, where it has always been.
