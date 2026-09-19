# 104. A rule says what it costs this patch

**Status:** accepted, 2026-09-15

## Context

The rules editor asks for a quorum as a bare percentage in a number box
labelled "Quorum (%)", a decision method from a dropdown whose most drastic
option is called "Full consensus", and a voting bar as a number of days. It
says nothing about what any of them comes to for the patch being edited.

An eight-person co-op in the governance simulation (docs/adr/096) took the
Formal template's defaults — consensus, 50% quorum, a fortnight's window,
thirty days' tenure — ran three proposals in its first month and carried
none. Two lapsed with no ballots at all. Its founder, reading the editor,
had no way to learn that 50% meant four of her eight members had to act
inside that fortnight, that "Full consensus" meant one reject would defeat
what the other seven approved, or that an abstention counts toward quorum.
The one place anybody in that community learned the last of those was a
notice the founder wrote herself, after working it out.

Nothing was broken. Every number did what it said. The product simply never
said what the numbers *were*, on the one screen where somebody chooses them,
and a founder cannot reason about a governance rule from its label.

Two things made it worse than a missing hint. The first is that the tenure
bar has a rule of its own — a patch younger than its own bar has no bar
(docs/adr/098) — so the honest answer changes with the patch's age, and no
static help text can carry it. The second is that a co-op learns what its
rules cost by running a vote and losing it, which is the most expensive
possible way to find out, and the one that makes people stop using
governance features.

## Decision

**The editor states the arithmetic, for this patch, as the person sets it.**
Beside the quorum box: *"4 of the 8 people who can vote today must cast a
ballot, or a proposal closes with nothing decided. An abstention counts
toward it."* Beside the method: *"One reject defeats a proposal, however many
approve it."* Beside the tenure bar: how many of today's members clear it, or
that it is not in force yet and when it starts to be.

**The numbers come from the server, and the arithmetic mirrors the server's
own.** `GET /api/v1/nodes/{slug}/governance/electorate` sends three facts —
how many people the electorate is drawn from, how long each has been here in
whole days, and how old the patch is. The editor computes `votesNeeded` the
way `votesNeededForQuorum` does (the ceiling, capped at the electorate) and
applies docs/adr/098's cap the way `effectiveTenureDays` does. A sentence
that the gate does not honour would be worse than no sentence: five simulated
members already read a tenure rule that was not the one in force, and one
pressed a button unsure whether his vote had counted.

**Its own endpoint, not a field on the rules payload.** The rules editor
builds its submission from what it loaded, and that endpoint is documented as
having to send the whole rule set and nothing else; a fact about the patch
travelling in it is a fact waiting to be written into a patch's rules file by
an unrelated edit. Facts about the room and rules for the room are two
payloads because they are two kinds of thing.

**Tenures travel as bare sorted numbers, with no names.** The question is
"how many clear thirty days", and answering it does not require saying who.
It is still gated to members and admins — the same people who may raise a
rules proposal at all — because when a patch's people arrived is a fact about
the room rather than about the patch. A follower gets 403, not 404: the rules
themselves are public, and what is private is who votes on them.

**The editor works without it.** The fetch is best-effort and its failure
leaves the sentences off rather than blocking the form. Nobody should be
unable to change their rules because a hint would not load.

## Consequences

- This is documentation that cannot go stale, because it is computed rather
  than written. It is also the first governance help in the product that is
  about *your* patch rather than about patches.
- Two definitions now exist for the quorum ceiling and the tenure cap — one
  in Go, one in the browser. That is the cost, and it is why both are pinned
  by tests naming the Go functions they mirror. A third copy anywhere would
  be a mistake; a surface that needs these numbers should ask this endpoint.
- It does not change any default. The Formal template still ships consensus
  and a 50% quorum, and whether those defaults suit an eight-person co-op is
  a separate question this only makes answerable *before* the first vote
  rather than after the third lapse.
- The template picker at patch creation still describes its options in the
  abstract, and has no patch to do arithmetic about. The simulation's
  finding that the blurbs route real organisations to the wrong template
  (F-032) stands untouched.
