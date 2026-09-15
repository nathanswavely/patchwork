# Governance simulation

How Patchwork's governance is proved out without a community risking its
first election on it. Five fictional organizations are created and run
through months of decisions on a throwaway instance whose calendar
`cmd/sim` can move (docs/adr/096). What comes out is a regression suite, a
list of gotchas, and the raw material for the governance guide.

This document is the public half: the tool, the org matrix, the epoch
loop, and what an auditor checks. Persona briefs, agent journals and the
working findings ledger are a QA program in progress and live outside the
tree (`notes/` is gitignored for exactly this).

## The tool

```sh
make sim-personas            # fresh data/sim/patchwork.db, one account per persona, a fixed session token each
make sim                     # boot the instance on :8097 (config: cmd/sim/patchwork.sim.yaml)
make sim-status              # what every patch's governance is doing right now
make sim-advance BY=30d      # a month passes: every stored instant moves back, the sweeps run once
make sim-now                 # the simulated date
make sim-reset               # rm -rf data/sim
```

Act as a persona with the cookie the tool prints (`patchwork_session=sim-<username>`)
and the CSRF header every mutation needs:

```sh
curl -b 'patchwork_session=sim-priya' -H 'X-Patchwork-Request: true' \
     -H 'Content-Type: application/json' \
     http://localhost:8097/api/v1/auth/me
```

In a browser, set the same cookie on `localhost` and the app is that person.
Playwright specs add it with `context.addCookies`, as `web/e2e/setup.js`
does for the seeded suite.

**Advance in whole days.** Date-only columns (a seat's term end) round a
fractional day down. The server keeps running across an advance — it
reads the database with the real clock and finds the moved world.

**Two artefacts the auditor must not report.** Governance git repos keep
their real commit dates, so a charter's history view can show "3 minutes
ago" for an amendment the record dates months back. Prose in notifications
already sent ("voting ends September 20") is not rewritten. Neither is a
product finding. Everything else that disagrees with the simulated date is.

## The organizations

Five, on one instance, because cross-membership is what a quilt is. The
roster is in `cmd/sim/personas.example.yaml`; a few people are in two orgs.

| Org | Model | Venue | What it stresses |
|---|---|---|---|
| Tin Roof Choir (a band) | `maintainer`, `decision_method: admin`, minimal template | patchwork | successor designation; the maintainer's approve/decline verbs and consult-first votes (docs/adr/092); the maintainer leaves |
| Bobbin Hall Co-op (a venue) | `elected`, 6-month staggered terms | patchwork | nomination window → ballot → seating; quorum failure and holdover; wholesale replacement; two full cycles; a member whose tenure is too short to vote |
| Northwest Corridor Neighbors (a coalition) | `elected` on paper | both `elsewhere` | Patchwork as a results-recording surface: leadership attestations (docs/adr/052), amendment attestations (docs/adr/053), a proposal born `elsewhere` with no ballot |
| Loose Threads Press (a collective) | `meritocratic`, collaborative template | patchwork | nominate-and-ratify; charter amendment proposals with diffs; a rules edit; subject recusal |
| The Remnant Room (the chaos org) | starts `maintainer`, changes | changes | model switched mid-term; venue switched with a vote open; template switched after customizing; the sole admin tries to delete their account; a seamrip mid-election and whether the fork keeps holding elections |

## The epoch loop

A simulation is a sequence of epochs. In each:

1. **Personas act.** Each has a calendar of intentions for this epoch
   ("Devon decides not to stand", "Rosa votes on the last day", "Ivo
   proposes changing the charter's third paragraph"). Some intentions are
   to *not* act — an election where nobody stands is a case.
2. **Time passes.** `make sim-advance BY=<days>`. The sweeps run. The
   tool prints every proposal whose status or state moved, seats gained or
   lost, and notifications created.
3. **The auditor checks.** Database, audit log, notification rows, and
   what each persona *saw* are compared against what should be true.

Three kinds of agent run this, in layers:

- **The scripted harness** — Playwright specs, one per org, deterministic
  and replayable. Each spec is an epoch loop with assertions between
  advances. This layer catches true errors and is the permanent
  regression net. It grows from the failures the other layers find.
- **Persona agents** — one per persona, given a brief and a goal but no
  script, driving the browser. They keep a journal: what they expected,
  what they saw, where they hesitated, what they got wrong. This is the
  feedback a small org would have given, taken before a small org has to.
- **The auditor** — reads everything after each epoch and checks the
  invariants below.

## What the auditor checks

- Every vote has a voter and every proposal an author (docs/adr/086).
- Seats agree with the record: who the governance record page says was
  seated, when, and until when is who `seats` holds.
- Holdover held: an election that settled nothing left the council exactly
  as it was, and said so (docs/adr/051).
- A vote was judged by the terms it opened with, not the rules edited
  since (docs/adr/047).
- Every notification maps to an obligation the recipient actually has
  (docs/adr/093), and nobody was notified of a room they are not in.
- On an `elsewhere` patch, nothing was tallied; on a `patchwork` patch,
  nothing was attested (docs/adr/052).
- The proposal list, the proposal page, the governance record, and the
  notification bell tell one story about the same proposal.
- Anything that disagrees with the simulated date, except the two
  artefacts named above.

## Findings

Findings carry one of five tags: **bug** (the product did the wrong
thing), **gap** (a mechanic the model promises and nothing runs), **ux**
(a persona could not find or understand a working thing), **copy** (the
words were the problem), **docs** (it works, nobody could have known).
Each bug fix lands with an epoch in the scripted harness that would have
caught it.

The documentation phase runs last and is measured: the persona journals
are the guide's raw material — every "I expected" is a sentence in it —
and the persona agents are rerun with the guide in hand to see whether
the hesitation count drops.
