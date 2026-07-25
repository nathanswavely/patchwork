# Governance ceremony follows the rules in force

A sole admin on a Minimal-template patch clicked "Propose a change to
these rules," saw a form prefilled with phantom defaults, and had to
wait out a 72-hour voting window to change their own settings — despite
the Minimal template declaring `decision_method: "admin"` and the
backend already carrying an instant-apply path for it. The feature was
dead on arrival because `CreateNode` forked the template's governance
repo but never called `SyncRulesToDB`, so every patch's cached
`governance_config` stayed empty and every read path saw voting
defaults. Fixing the plumbing forced the design question: what
legitimately decides whether an admin can change governance without
ceremony?

We decided **the patch's own rules decide — never its headcount, and
never a template heuristic**:

- **`decision_method == "admin"` is the sole trigger for direct
  change.** A structural trigger (exactly one admin, no members) was
  rejected: governance behavior would silently flip when the second
  member joined or left — a power shift nobody voted on. A patch that
  chose majority vote keeps majority vote even when its electorate
  dwindles to one.
- **The Casual fast-track is removed.** The old code auto-applied any
  admin's proposal on maintainer-led zero-quorum patches, even though
  Casual's decision method is majority and its own shipped operating
  agreement promises "Proposals are open for 3 days. Majority vote
  wins." That was an admin bypass the rules never granted — the rules
  on screen were not the rules in force. Members of a Casual patch now
  get the vote their charter says they get, admins included.
- **Sole-voter early close.** On voting patches where exactly one
  person is eligible to vote, their explicit vote resolves the proposal
  immediately — a deliberation window for an electorate of one protects
  nobody. This is a resolution rule, not a submission side effect:
  submitting still doesn't count as voting (proposing and endorsing
  stay distinct acts; the author can sleep on it or withdraw). Voting
  patches with two or more eligible voters are untouched — the window
  still holds space to reconsider.
- **A direct change is an instantly-applied proposal record, not a
  parallel edit path.** One entity keeps one history surface: the
  governance timeline tells the whole story for every patch, followers
  still get notified through the same permissions, AP broadcast and
  seamrip carry it unchanged. Only the framing differs — the UI never
  says "propose," "submit," or "vote" for one (see "Direct change" in
  CONTEXT.md).
- **Ceremony in the UI matches ceremony in force.** When the decision
  method is admin-decides, the voting knobs (quorum, voting period,
  amendment threshold, auto-apply, voting tenure) are hidden, not
  disabled — their stored values persist and reappear if the patch
  adopts a voting method. New-patch creation preselects Minimal:
  the default patch is one person running a listing, and upgrading
  ceremony later is one direct change, while downgrading on a grown
  patch rightly costs a member-visible vote.

Consequences: the startup backfill that repairs `governance_config`
(SyncRulesToDB for every node) changes live behavior — Minimal-template
patches on existing instances become admin-decides, which is what their
template declared all along; pre-template patches with no repo rules
file get the explicit voting defaults they were already effectively
under. Anyone reading the removed fast-track as a regression should
read this first: it was removed because it contradicted the patch's own
charter, and the legitimate instant path is choosing the admin-decides
method.
