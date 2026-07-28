# Succession follows the leadership model

A patch on the Formal template tells its members it is "governed by an
elected council of up to 7 admins," that "council members serve 12-month
terms," and — as a stated *value* — that "power rotates. No one stays in
charge indefinitely." None of it is true. docs/adr/049 swept
`GovernanceConfig` and found six fields stored, rendered, and read by
nothing; docs/adr/050 found the same disease in `follower_permissions` on
the same row. This is the third sweep and the first where the answer is
to build rather than retract, because a community that chose elected
leadership chose something real.

The first draft of this ADR tried to build "elections" as *the* answer,
and invented a mechanism to do it: any member could call an election on a
lapsed seat, with the nomination window as the anti-griefing filter. That
was too clever, and the error was upstream of the cleverness — it assumed
leadership rotates one way and went looking for the one way.

**`leadership_model` already names three, and the copy already on screen
describes three different mechanics:**

> **maintainer** — "One person maintains this patch. They handle
> day-to-day decisions and can designate a successor."
> **meritocratic** — "Admins earn their role through sustained
> contribution. When a seat opens, existing admins nominate from active
> members and the community ratifies."
> **elected** — "The community elects admins for fixed terms. Regular
> elections ensure power rotates."

Only the third is calendar-shaped. The first is succession by
designation. The second is the open-source model: event-driven, triggered
by a seat opening rather than by a date, resolved by nomination and
ratification rather than by a contest.

We decided **there is no single succession mechanic. A patch picks one
when it picks a leadership model, and each model gets the mechanic its
own description already promises.**

| Model | Seats | Terms | How an admin is made |
|---|---|---|---|
| `maintainer` | no | no | the maintainer designates a successor |
| `meritocratic` | no | no | admins nominate, the community ratifies |
| `elected` | yes | yes | calendared election |

- **A seat is the thing a term attaches to, so only `elected` has one.**
  An earlier revision of this table gave meritocratic seats, on the
  strength of its own copy saying "when a seat opens." Building it showed
  that was reading a figure of speech as a data model: with no term and
  no cap, a meritocratic seat would carry an occupant and nothing else,
  which is what the membership row already is. "A seat opens" there means
  a human decided the patch could use another admin — a judgement, not a
  tracked state. Under `elected` a seat holds a term end, survives its
  occupants, and can be filled or vacant, and only then is it an entity
  worth having.

  Terms and seats therefore arrive together rather than being separable
  as this ADR first claimed. What *is* separable, and what the first
  draft actually got wrong, is **succession from elections**: two of the
  three models rotate leadership with no seat, no term, and no ballot
  between candidates.

  A `maintainer` patch has none of it, and a band never meets any of
  these words. There is no fourth role anywhere: an admin is an admin,
  however they got there.

- **Meritocratic ratification needs no new voting machinery.** The admins
  put forward one name and the community votes yes or no — an ordinary
  proposal on the existing `votes` table. The objection that killed
  single-nominee ballots for elections (two people want one seat, both
  proposals pass) cannot arise here, because by definition there is one
  nominee. This is the cheapest of the three to build and it makes
  `leadership_model` honest for one of its values immediately.

- **Elections are calendared, not called.** This is the correction the
  shipped templates were already carrying and the first draft talked past.
  The succession plan lays out a 42-day annual cycle — nominate 14 days,
  campaign 14, vote 14, seated on day 43 — and the bylaws say "elections
  are held annually." Nobody triggers an election; the calendar does.
  "Annually" is not fixed either: term length is `admin_term_months`, and
  12 is Formal's choice, not the model's.

  So the question "who may call an election" dissolves, and with it the
  petition threshold, the anti-griefing filter, and the seat-state table
  the first draft spent its effort on. The only unscheduled election is
  the one the template already defines: more than half the seats vacant
  triggers an emergency cycle on compressed 7-day phases.

- **The cycle is anchored by nothing configured.** Adopting `elected`
  leadership starts an election; every cycle after is scheduled from when
  the last one seated the council. No founding anniversary, no date
  picked at setup, no instance-wide date — all three were considered and
  all three require a decision, or inherit an arbitrary date, that has
  nothing to do with the patch's governance. A patch founded on 24
  December should not hold elections over the holidays because of it.

  This also settles what adoption does to the sitting admins: they hold
  over through that first election and either win their seats or do not.
  Nobody is ejected by a rules change, and nobody collects a twelve-month
  mandate from one either.

- **The clock belongs to the seat, not the person.** Someone appointed to
  a mid-term vacancy serves out the remainder, not a fresh term. The
  argument is integrity rather than tidiness: under fresh terms a council
  can reset its own clocks — a seat opens, an ally is appointed, and that
  person holds a full term without ever having faced the electorate at a
  scheduled cycle. Done repeatedly, the council outruns its own election
  calendar indefinitely. When the seat carries the term, appointment can
  fill a gap but can never manufacture a mandate. It also keeps the
  answer to "when do we next elect?" a single date rather than one per
  seat.

  `term_ends_at` therefore lives on the seat. Which makes **staggering a
  policy rather than machinery**: aligned seats share a date, staggered
  seats differ, and spreading a first cohort is just setting shorter
  initial dates on some of them — the way real bylaws classify at a first
  election — after which inheritance keeps the pattern stable by itself.
  No class entity, no migration, and nothing here forecloses it. Not
  shipped initially, but only because it is optional, not because it is
  hard.

- **No clock removes anybody.** Where terms exist, a term ending makes
  the *seat* contestable at the next cycle; the holder keeps serving
  until a successor is elected. "Directors serve until their successors
  are elected and qualified" is boilerplate in real bylaws for a reason,
  and it is what covers an election with no candidates and an election
  that misses quorum — both resolve to the incumbents continuing.
  Auto-demotion at expiry was rejected: it collides with the last-admin
  floor, and its failure mode lands on the least organized communities,
  where a patch that forgets a cycle gets decapitated by a cron job.

- **`succession_policy` is reachable after all, and the first draft was
  wrong to call it structurally inert.** The succession plan names the
  rule: total vacancy means "the three longest-tenured active members
  become interim admins" — which is precisely
  `succession_policy: "longest_tenure"`. The claim that a patch can never
  reach zero admins holds only for *voluntary* exit; leaving, banning,
  and demoting the last admin are refused, but the inactivity path
  vacates a seat without anyone choosing to, and that path can reach
  zero. `longest_tenure` is the rule that fires when it does.

- **`inactivity_days` has teeth, and the template supplies them:** day 30
  notify, day 45 the council discusses, day 60 the seat is declared
  vacant. docs/adr/049 listed it among the six and suggested it might be
  best left descriptive because "may be asked to step down" reads
  honestly. That was reading the overview's hedge rather than the
  succession plan's procedure. It is a vacancy rule, and it is what makes
  `succession_policy` live.

- **A seat cannot be dissolved while it is occupied.** Two different acts
  want the same button — *dissolving a seat* (the body shrinks) and
  *removing a person from a seat* (the seat survives, refillable). Fused,
  a patch can dissolve an occupied seat to remove somebody and
  permanently shrink itself as a side effect of a grudge, and the
  timeline cannot tell a reader which argument won. Removal is in scope
  as its own act: the bylaws already promise it ("a council member can be
  removed by a supermajority vote of the full membership," brought by a
  10% petition), and a patch whose whole council has gone bad should be
  able to fix itself without seamripping.

- **An election is a proposal that carries candidates.** Not a parallel
  entity — docs/adr/041 settled that shape ("one entity keeps one history
  surface"), and a separate entity would need its own quorum, terms
  freeze, notifications, AP broadcast, and seamrip boundary, five things
  it took docs/adr/044, 047, and 048 to get right once. As a proposal it
  inherits the electorate (044), the voting-terms freeze (047), the
  timeline, notifications, federation, and seamrip already correct.
  Quorum is unchanged; only the threshold differs — most-approved take
  the open seats.

  The `votes` table is untouched. It is `UNIQUE(proposal_id, user_id)`
  over three values, so it says yes or no to one question and nothing
  else. Election ballots get their own table.

- **Approval voting, not ranked-choice.** The bylaws say ranked-choice,
  and that text is the same category as "14 days of discussion before
  voting begins" — aspirational prose docs/adr/048 and 049 established
  gets corrected rather than obeyed. Approval has no vote-splitting
  pathology at community scale and needs no tie-break doctrine. An IRV
  tally nobody in the patch can verify by reading the page sits badly
  with docs/adr/047's "the vote states its own terms, always."

- **Candidacy is a member act with no tenure condition.** It falls out of
  docs/adr/044: tenure asks whether someone has been here long enough to
  *decide*, and a candidate is being decided *about*. Same gate as
  authoring a proposal — any admin or member, never a follower. Which is
  also what the bylaws already say: "any member may nominate themselves
  or another member."

- **An election has a nomination window, and this amends docs/adr/048.**
  The templates put nominations ahead of voting, so creation and the
  start of voting are not the same instant. 048 said they were, "by
  construction," when it closed docs/adr/047's open conditional. **For
  elections, and only elections, the voting-terms photograph is taken
  when nominations close and voting opens** — which is what 047 asked for
  originally. `voting_ends_at` likewise runs from the start of voting.

  048's *argument* does not forbid this: it objected that
  `GovernanceConfig` had "no field that says how long discussion lasts or
  what ends it," and a nomination window has a length, a purpose, and a
  defined end. Its *consequence* did forbid it, and that is what is
  amended. We are aware this argues for a pre-voting state shortly after
  deleting one; the distinction is that `draft` and `discussion` died of
  never having been specified. If a nomination window dies the same way,
  it will be for the same reason and this paragraph is where to look.

- **`max_admins` is retired as a rule, and the guard from docs/adr/049 is
  retracted.** How many admins a patch has follows from how it governs —
  under `elected`, seats are created and dissolved by governance act and
  the count is however many exist; under the other two, it is however
  many people have been designated or ratified. An instance-level cap was considered and rejected:
  every instance setting today governs the instance's own presentation or
  its own discovery surface, and CONTEXT.md is explicit that an instance
  admin "does not override per-patch choices."

  049's guard made a number binding that no surface can edit —
  `RulesProposalEditor` is the only writer of `governance-rules.json` and
  `StructuredRulesEditor` preserves `max_admins` without offering a
  control — while migration 041 had backfilled `"max_admins": 3` into, in
  its own words, nearly every patch. A live patch with three admins
  wanting a fourth got a 409 with no recourse. Enforcing a claim is only
  honest when the claim is reachable; 049 had the principle right and the
  instance wrong.

**Considered and rejected: one universal succession mechanic.** It is
what the first draft assumed and it is the source of every wrong turn in
it. Three mechanics means three code paths and three sets of copy to keep
honest — which is how docs/adr/049 happened — but two of the three are
small, all three end at the same place (someone holds `admin`), and the
alternative is forcing a band and a coalition through the same ceremony.

**Considered and rejected: seats for every patch, with designation as an
unelected way to fill one.** Uniform, and the overview would say one
thing everywhere. It loses on the spectrum principle: a three-piece band
would acquire seats it never asked for, and every surface would have to
explain a structure most patches will never use.

- **Mid-term vacancy under `elected` borrows meritocratic's mechanic**,
  because the template already says so: "the council may appoint a
  replacement from active members. The appointment must be ratified by
  the community within 14 days." That is nomination-plus-ratification,
  which is the same act meritocratic runs on and the same ordinary
  proposal. The appointee inherits the seat's remaining term, per above.

## Open

Nothing structural. What remains is implementation detail — the shape of
the seats table, where the cycle scheduler lives in a single-process
binary with no queue, and the copy for each of the three models'
overview sections.

Two things are deliberately deferred rather than undecided: **first-cohort
staggering** (a policy over the existing schema, addable whenever) and
**ranked-choice** (rejected for approval voting, and revisitable only
with a tally members can verify by reading the page).

**Status: adopted as a design boundary — implementation is backlog,
except the `max_admins` retraction, which is a live fix and ships here.**
Build order runs cheapest-first and honest-soonest: maintainer
designation, then meritocratic ratification (no new voting machinery),
then elected.
