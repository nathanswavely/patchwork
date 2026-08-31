# ADR 076: The bulletin is the one broadcast, and it ships off

Date: 2026-08-31. Status: accepted. Decided while grilling discoverability
on the home quilt.

## Context

Patchwork has thirty-nine notification types in five categories —
Proposals, Governance, Membership, Events, Admin
(`internal/notifications/types.go`) — and every one of them is a consequence
of a relationship the recipient already has. A proposal at your patch. Your
membership changing. An event at a patch you follow. Your claim, your admin
queue. **Not one is a broadcast.** That line has never been crossed, and it
is what makes the bell trustworthy: opening it always means something
happened to something of yours.

Notifications also reach email, and ADR 018 states email's purpose narrowly:
magic links, plus "notification delivery to people who aren't checking the
site." Anything monthly aimed at people who aren't checking the site is, in
mechanism, re-engagement mail — the other half of the machine this project
declines to run. The intro card promises every anonymous visitor "that no
algorithm runs it" (ADR 040), and that promise is about who decides what is
put in front of someone.

Against that: a community organizing platform where nobody ever learns that
new communities have arrived is failing at its literal job. Word of mouth is
the thing the platform exists to scale. A monthly, complete, unranked list of
arrivals is not a feed — it is a bulletin, and neighbourhood bulletins are an
old and good form.

## Decision

**One broadcast exists: a monthly bulletin naming the patches that have
become active since the last send.** Four constraints, each load-bearing.

**It ships off.** `DefaultEnabled` returns false for it. Opt-in is the whole
of what keeps the front-door promise true: the *person* decided this should
reach them, and the app deciding is precisely what "no algorithm runs it"
rules out.

**It is offered as two named choices at the end of the discovery flow (ADR
075), not as a checkbox.** ADR 040 deleted a checkbox from Welcome — "double-
signing the same agreement minutes apart is consent theater" — and this does
not restore it: what ADR 040 removed was a *signature*, and a mail preference
is a *setting*. But the register binds, so the model is the intro card's
worded decline: "Once a month, tell me who's new" beside "No thanks", both
dignified, neither pre-selected. A pre-checked box would be default-on
wearing opt-in's clothes. The discovery flow is the right home rather than
onboarding, because onboarding is spent — everyone already on the live
instance would otherwise be excluded from the offer forever.

**It is complete and unranked.** Every patch that became active in the
period, in arrival order. There are no highlights and no picks: the moment
something selects among the arrivals, the bulletin is a feed and the promise
on the front door is false.

**It is a sixth category, named for what it is.** Not folded into Events or
Admin. The exception has to stay legible so the next person proposing a
broadcast has to argue for it rather than inherit it.

**"Became active" is not "a row was inserted."** The reference instance is
twenty-six unclaimed listings of twenty-seven; an admin backfilling the
directory must not fire "twenty-three patches joined." A patch joins the
quilt when a community arrives — a claim completes, or someone creates one —
which is the line CONTEXT.md already draws at patch setup.

**It is built last.** With a standing door into discovery mode and a
"recently added" order, the bulletin is a nice-to-have. Building the push
before the place it pushes to is how it ships as a nag.

## Considered options

- **"X patches since your last discovery."** The original proposal, and
  rejected on three counts: it needs per-person state (`users.last_discovery_at`,
  a migration, a seamrip boundary decision); it excludes anonymous visitors,
  who have the greatest discovery need and no "last discovery"; and it
  converts a public fact into private state with no gain in truth. "Three
  patches joined this month" says the same thing, for everyone, logged out.
- **A recurring in-app prompt to re-enter discovery.** Rejected on the
  glossary: Interruption is "a closed category, deliberately small"
  (CONTEXT.md), and every dismissal pattern in the app — intro card,
  onboarding, unlock panel, setup checklist — is one-shot. A recurring prompt
  inverts all of them.
- **Default on, with easy opt-out.** Rejected. This is recorded here
  deliberately because default-off will look like a failure in any metrics
  review, and the decision should be settled now rather than under that
  pressure later.
- **No bulletin at all.** Rejected: see the context. The job is real.

## Consequences

- **`nodes` needs an arrival timestamp.** There is no `activated_at` column,
  and `updated_at` cannot stand in — completing a claim writes
  `status = 'active', updated_at = ?` (`internal/handler/claims.go`) and then
  every subsequent edit moves it. Both the bulletin and ADR 074's "Recently
  added" order depend on this column; `TreeNode` must carry it.
- A sixth `Category` in `internal/notifications/types.go`, one line in
  `DefaultEnabled`, and a monthly pass on the existing
  `StartReminderWorker` — no new worker.
- The bell carries one thing that is not about you. The cost is stated here
  so it stays at one.
- Instances without SMTP lose nothing: the bulletin is a notification like
  any other and appears in-app (ADR 018 — email is optional and stays so).
