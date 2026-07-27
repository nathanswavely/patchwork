# Elected leadership: seats, terms, and elections

A patch on the Formal template tells its members it is "governed by an
elected council of up to 7 admins," that "council members serve 12-month
terms," that "any member may nominate themselves or another member for a
council seat," and — as a stated *value* — that "power rotates. No one
stays in charge indefinitely." None of it is true. docs/adr/049 swept
`GovernanceConfig` and found six fields stored, rendered, and read by
nothing; docs/adr/050 found the same disease in `follower_permissions` on
the same row. This is the third sweep and the first one where the answer
is to build the thing rather than retract the claim, because a community
that chose elected leadership chose something real and there is no honest
way to describe what it currently gets.

We decided **elected leadership is a governed structure of seats, terms,
and elections — and no clock ever removes anybody.**

- **Seats exist only where leadership is elected.** A patch whose
  `leadership_model` is `elected` has seats; every other patch has admins
  the way it always has, and never encounters the word. Governance is a
  spectrum (CLAUDE.md), and seat-as-entity is real ceremony — a band must
  not inherit it for having a governance column. There is no fourth role:
  a seat's holder holds the ordinary `admin` role, with no permission an
  appointed admin lacks. Admin is the rung; a seat is one of two ways to
  be standing on it.

- **A term lapsing removes no one.** At expiry the *seat* becomes
  contestable and its holder keeps serving — every permission, no
  interruption. The teeth are in what expiry unlocks rather than what it
  takes: **a lapsed term lets the electorate call an election for that
  seat**, where before only an admin could open one. That is a real
  transfer of power on a real deadline, and it is the mechanism that makes
  "power rotates" true.

  Auto-demotion at expiry was rejected outright. It collides with the
  last-admin floor, and its failure mode lands on the least organized
  communities: a patch that simply forgets to hold an election gets
  decapitated by a cron job. A clock must never be able to leave a patch
  leaderless. The cost we accept is that a patch can drift with lapsed
  seats indefinitely if nobody exercises the right — which is correct,
  because the alternative is the platform overruling a community that is
  content with its leadership. The drift is public, and a council that
  has not faced election in three years shows it.

- **`succession_policy` stays inert, honestly.** A patch cannot reach
  zero admins — leaving, banning, and demoting the last admin are each
  refused in `internal/handler/memberships.go` — so succession answers a
  question those three guards prevent anyone from asking, and holdover
  keeps it that way. Making it relevant would mean first deciding a patch
  may be left leaderless, which is adjacent to docs/adr/012 and is not
  decided here. This is the one field from 049's six that stays exactly
  as it was, and that is a result rather than an omission.

- **Seat count is emergent, and `max_admins` is retired as a rule.** The
  number of admins is a function of how a patch governs, not a ceiling it
  configures: seats are created and dissolved by governance act, and the
  count is however many exist. `max_admins` therefore stops being a rule
  at any level — including instance level, which was considered and
  rejected because every instance setting today (`instance_name`,
  `icon_design`, the legal docs, `hide_amended_linings`) governs the
  instance's own presentation or its own discovery surface, and none
  reaches into a patch's composition. CONTEXT.md is explicit: an instance
  admin "curates instance-wide options; does not override per-patch
  choices."

  **This retracts the guard shipped with docs/adr/049.** That guard made
  a number binding that no surface can edit — `RulesProposalEditor` is
  the only writer of `governance-rules.json` and `StructuredRulesEditor`
  preserves `max_admins` without offering a control — and migration 041
  had backfilled `"max_admins": 3` into, in its own words, nearly every
  patch. A live patch with three admins wanting a fourth got a 409 with
  no recourse. Enforcing a claim is only honest when the claim is
  reachable; 049 got the principle right and the instance wrong.

  If a backstop against abuse is ever wanted, it belongs with rate
  limiting: absurdly high, invisible, and never rendered on a governance
  surface. The moment a number appears in the UI as "seats," members read
  it as their charter, and we are back in 049.

- **A seat cannot be dissolved while it is occupied.** Two different acts
  want to be the same button — *dissolving a seat* (structural: the
  council shrinks) and *removing a person from a seat* (personnel: the
  seat survives and can be refilled). Fused, a patch can dissolve an
  occupied seat to get rid of somebody and permanently shrink its council
  as a side effect of a grudge, and the timeline cannot tell a reader
  which argument actually won. To shrink, a patch either lets a term
  lapse and declines to refill, or removes the person as its own act and
  then dissolves the vacant seat. Two decisions, two votes, two records.

- **Removal from a seat is in scope.** The Formal bylaws already promise
  it — "a council member can be removed by a supermajority vote of the
  full membership" — so shipping seats and elections without it leaves a
  seventh false sentence standing, and a patch whose whole council has
  gone bad would have no lever short of seamrip. The escape valve should
  not be the first resort; a patch should be able to fix itself without
  forking.

- **An election is a proposal that carries candidates.** Not a parallel
  entity. docs/adr/041 settled the general shape — "one entity keeps one
  history surface" — and a separate election entity would need its own
  quorum, terms freeze, notifications, AP broadcast, and seamrip
  boundary, five things that took docs/adr/044, 047, and 048 to get right
  once. As a proposal it inherits the **electorate** (044), the **voting
  terms** freeze (047), the governance timeline, notifications,
  federation, and seamrip for free and already correct. Quorum is
  unchanged: did enough of the electorate cast a ballot. Only the
  threshold differs — most-approved take the open seats, rather than a
  percentage.

  The `votes` table is untouched. It is `UNIQUE(proposal_id, user_id)`
  with `value IN ('approve','reject','abstain')`, which can express
  exactly one thing: yes or no on a single question. Election ballots go
  in their own table, so nothing that currently tallies changes behavior.

- **Approval voting, not ranked-choice.** The Formal bylaws say
  ranked-choice, and that text is in the same category as "14 days of
  discussion before voting begins" — aspirational prose that docs/adr/048
  and 049 have already established gets corrected rather than obeyed.
  Approval has no vote-splitting pathology at community scale, needs no
  tie-break doctrine, and asks the same mental act members already
  perform. An IRV tally nobody in the patch can verify by reading the
  page is worse than no election, and docs/adr/047's "the vote states its
  own terms, always" sits badly with a count you cannot follow.

- **Candidacy is a member act with no tenure condition.** It falls out of
  docs/adr/044 rather than being designed: tenure asks whether someone
  has been here long enough to *decide*, and a candidate is being decided
  *about*. So candidacy takes the same gate as authoring a proposal — any
  admin or member, never a follower, no tenure. This is also exactly what
  the bylaws already say ("any member may nominate themselves or another
  member"), and the brigading defense is already in place one layer up:
  new accounts cannot vote, so they cannot elect their own candidate.

- **An election has a nomination window, and this amends docs/adr/048.**
  Nominations open when the election is created and close when voting
  opens. The alternative — the slate fixed at creation — concentrates in
  the person who calls the election the power to choose who may be
  elected, which is a serious flaw in the one act meant to redistribute
  power, and it breaks the self-nomination the bylaws promise.

  048 said proposals are born voting. Its *argument* does not forbid this:
  048 objected to `draft` and `discussion` because "GovernanceConfig has
  no field that says how long discussion lasts or what ends it," and a
  nomination window has a length, a purpose, and a defined end. Its
  *consequence* does forbid it, and that is the part being amended — 048
  closed docs/adr/047's open conditional by asserting that creation and
  the start of voting "are the same instant by construction and cannot
  drift apart." For an election they drift. **So for elections, and only
  elections, the voting-terms photograph is taken when nominations close
  and voting opens, which is what 047 said it should be all along.**
  `voting_ends_at` is likewise computed from the start of voting, not
  from creation.

  We are aware this argues for a pre-voting state an hour after deleting
  one. The distinction we are resting on is that `draft` and `discussion`
  died of never having been specified — no rule, no clock, no entrance,
  no way in and no way out. If a nomination window dies the same way, it
  will be for the same reason and this paragraph is where to look.

**Considered and rejected: one proposal per seat, approve/reject on a
single nominee.** It needs no new table at all, which is a real
attraction, and it matches how many small organizations actually work.
But two people wanting one seat produces two concurrent proposals that
can both pass, and the fix — one open nomination per seat at a time —
turns elections into sequential ratification, which is neither what the
charter promises nor what "power rotates" implies.

**Considered and rejected: seats for every patch, with appointment as an
unelected way to fill one.** Uniform, and it would make the overview say
one thing everywhere. It loses on the spectrum principle: a three-piece
band would acquire three seats it never asked for, and every surface
would have to explain a structure most patches will never use.

## Open — not decided here

Named rather than quietly assumed, because this ADR is a spine and not a
specification:

- What resolves an election with **no candidates**, or with fewer
  candidates than open seats.
- What **quorum failure** does to an election — does the seat stay lapsed
  and contestable, or is there a cooling-off period before a re-run?
- Whether **one member calling an election** is enough, or whether it
  takes a petition threshold (the bylaws use 10% of active members for
  removal proposals, which suggests a precedent the patch already has).
- When a **term starts** — at the close of the election that filled it,
  or aligned to a shared calendar so a council's terms lapse together.
- Whether **`inactivity_days`** gains teeth here or stays descriptive; it
  is the one field from 049's six that already reads honestly ("may be
  asked to step down") and may be best left alone.
- The mechanics of **removal** beyond the decision that it exists and is
  a separate act from dissolving a seat.

**Status: adopted as a design boundary — implementation is backlog,
except the `max_admins` retraction, which is a live fix and goes first.**
