# A vote keeps the terms it opened with

A patch's governance rules were read live, at the moment a vote was
counted. `resolveProposal` loaded `governance_config` from the node and
divided by whatever it found. So a rules edit on Tuesday judged a vote
that opened on Monday, by terms nobody knew when they cast their ballots.
An admin who dislikes where a vote is heading can raise `quorum_percent`
until it cannot pass, or move `decision_method` from majority to
consensus and turn a winning vote into a losing one. None of that
requires bad faith — a well-meant edit does it just as well, and the
edit is invisible from the proposal page it changes.

ADR 044 made this reachable in a way it had not been. It fixed
`resolveProposal` to divide by `eligibleVoters`, which applies
`min_voting_tenure_days`; before that the denominator ignored tenure, so
raising the requirement mid-vote moved nothing. Now it moves the
denominator, while `countedBallot` — the numerator — still has no tenure
term. Ten members, two of them past a thirty-day requirement, six ballots
cast while the requirement was zero: raise it to thirty and quorum
evaluates `6 * 100 / 2`, three hundred percent turnout, decided by six
people of whom four are no longer in the electorate. That is the same
numerator-over-denominator divergence ADR 044 was written to end, in a
corner it did not look at. Its claim that the two sets are now one is
true only while the rules hold still.

We decided **a vote is judged by the rules in force when it opened**.

- **The whole config is photographed, not the tenure line.** The
  proposal carries a copy of `governance_config` taken when voting
  opened, and resolution reads the copy. Freezing only the field that
  produced this bug would leave `quorum_percent` and the two thresholds
  live, and those are the worse cases: a moved electorate miscounts a
  result, a moved threshold reverses one. It would also be a list of
  fields to keep in sync — a fifth place the rules are described, and
  the failure this whole run of ADRs keeps finding is a new field or a
  new surface that nobody told. A copy of the whole thing cannot be
  forgotten.

- **The precedent is already in the schema.** `duration_hours` and
  `voting_ends_at` freeze the voting window at creation; `base_sha`
  (migration 016) freezes the document the proposal was drafted against,
  for conflict detection. A proposal already refuses to let the ground
  move under it in two ways. This is the third, and the one that decides
  outcomes.

- **A rules amendment resolves under the old rules.** This falls out of
  the decision rather than being designed, and it is worth keeping on
  purpose: you do not get to use the new rules to pass the change to the
  new rules. A patch lowering its own amendment threshold must clear the
  old one to do it.

- **`amendment_auto_apply` is not a term of the contest.** It is the only
  field read live. It does not decide who wins; it decides whether an
  approved amendment applies itself or waits for a person. That makes it
  a safety valve, and a safety valve has to take effect when it is
  flipped — a community that switches auto-apply off because an
  amendment surprised them must not then watch three in-flight
  amendments apply themselves under a photograph taken before they
  flipped it. The line is: rules that decide the vote freeze, rules that
  decide what happens afterward do not.

- **The vote states its own terms, always.** Every open proposal says
  which rules it is running under and when they were fixed. This is not
  an alert; it is the vote being honest about itself, and it costs
  nothing to show. It also answers a question nothing else did: a member
  inside the tenure window sees no vote buttons, and before this there
  was no text anywhere explaining why. "Voting opened 21 July under rules
  requiring 30 days' membership" is that explanation, and it is the same
  sentence, not a second one written for the refused.

- **Two notification types, and exactly one fires per edit.**
  `governance.rules_changed` — declared in the notification registry
  since it was written and **never once fired** — carries the routine
  case at normal priority. `governance.rules_changed_midvote` carries
  the same edit landing while votes are open, at high priority, and says
  which votes the new rules do not reach.

  The split is forced by two mechanics, not by taste. Preferences key on
  the *type*, so one type would mean a member who mutes their patch's
  config churn also loses the warning that a vote they are trying to
  join is running under terms they do not meet. And `DefaultEnabled`
  turns email on for `PriorityHigh` alone, so one high-priority type
  would mail every member on every config edit — which is how a whole
  category gets filtered, and then the one that mattered is filtered
  with it. Normal for routine, high for the case with a deadline
  attached.

  They are mutually exclusive: a mid-vote change is still a rules
  change, and saying it twice teaches people to ignore both. The
  proposal *making* the change is excluded from the count of votes left
  behind — on the direct-change path the sync runs before that
  proposal's status leaves `open`, so without the exclusion a rules
  change reports itself as a vote its own terms no longer reach.

- **The newly eligible are told too, not only the newly excluded.** They
  are the more confused group. Someone who becomes a member in good
  standing under the new rules, reads the requirement on the page, meets
  it, and still finds no vote buttons on the three proposals already
  running has been told the truth twice and shown a contradiction. The
  frozen terms are the explanation and they should have it without
  filing a bug first.

**Considered and rejected: evaluating tenure at read time**, the way
role and status are evaluated. It is the smaller change and it is what
ADR 044's own principle sounds like it wants — "a vote is a fact;
whether it counts is a question asked fresh each time it is read."
Applied to tenure it is wrong, because tenure is not a fact about a
person the way role is; it is a comparison against a rule, and the rule
can move. Demotion already kills a ballot under ADR 044 and we accepted
that, but demotion is one person at a time and shows on the member list.
Raising a tenure requirement voids many ballots at once, silently, from a
settings edit. Same effect, different order of magnitude, and no trace.

**Considered and rejected: freezing the electorate itself** — snapshot
the roll when voting opens, the way an election closes registration.
It kills this class completely and it is the more familiar model. We
did not take it because it contradicts a decision ADR 044 made for good
reasons: someone who leaves a patch mid-vote should not still be
deciding its charter, and `countedBallot` filtering at read time is how
that stays true without purging votes. Membership churn is a real change
in who belongs; a rules edit is not. Freezing the rule keeps ADR 044
intact and fixes only the thing that was broken.

**The cost, stated plainly:** a patch that reforms its governance waits
for in-flight votes to clear before the reform governs them — up to the
window length, which is 336 hours on the Formal defaults. Two weeks of a
patch's own new rules not applying to the votes in front of it. We think
that is correct, and the mitigation is to say so on the page rather than
to avoid it.

**The photograph is taken at creation because that is when voting opens.**
`CreateProposal` starts a proposal in state `voting` (or `in_effect` for a
direct change); the `draft` and `discussion` states in the migration-016
state machine have no path into `voting` — the SPA's "Submit for voting"
control PATCHes `{state: "voting"}` and `UpdateProposal` accepts only
`title`, `body`, and `status`, so the field is dropped. If that
transition is ever wired up, the photograph moves with it: the terms must
be fixed when the vote starts, not when the draft was written, or a
proposal could sit in discussion for a month and be judged by rules that
predate the discussion.

*Settled by docs/adr/048: that transition is not being wired up. The
`draft` and `discussion` states are retired, `UpdateProposal` refuses
`state` by name, and the SPA control is gone. Creation is when voting
opens, so the photograph and the start of the vote are the same instant
by construction — the conditional above cannot come due.*

**This closes the tenure half of `countedBallot` without touching it.**
The open question going in was whether `countedBallot` — which filters
ballots by role and status but not tenure — should gain a tenure term, so
that the numerator and the denominator could not disagree. Under the
freeze it cannot disagree. A ballot only exists because its caster passed
the tenure check when they cast it; the requirement is now fixed for that
vote's life, and tenure only grows, so anyone whose ballot still counts is
still in the electorate. Every counted ballot's caster is in the
denominator, which is the property adding a tenure term to
`countedBallot` was meant to buy. Adding one anyway would be dead
weight — and worse, it would be the retroactive disenfranchisement
rejected above, wearing a different hat.

The exception is a proposal with no photograph, which falls back to the
live config: resolved proposals (harmless, nothing is being decided) and
rows inserted outside `CreateProposal`, which today means the seeder and
test fixtures. Those can still show a turnout above 100% if a patch's
tenure requirement rises under them. Rendered, it reads "Quorum met (3 of
1 voted, 50% needed)" — which is how this was found.

**Open:** the notification tells people the terms diverged, but nothing
tells them *what* diverged. A diff between the photograph and the live
config is a better message than a sentence, and the structured rules
diff component already exists (`StructuredRulesDiff`). Left as a
follow-up rather than widening this change.
