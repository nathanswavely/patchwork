# ADR 088: Withdrawing a request is not leaving

Date: 2026-09-07. Status: **accepted**; supersedes the `LeaveNode`
relaxation merged in PR #219. Grilled in session against CONTEXT.md and
the ADRs below, one branch of the design tree at a time; every decision
was put as a question with a recommendation and answered. One
recommendation was withdrawn mid-session because the code disproved it —
recorded below, so the bad argument is not re-derived.

## Context

On an `approval_required` patch, asking to join writes
`memberships (role='member', status='pending')`. The row says member
because that is what it will become.

Two branches found the same consequence independently, within hours of
each other, and fixed it two different ways. That is the first fact worth
recording: this is not an obscure corner. Both branches were reading the
same schema and both concluded, correctly, that something was lying.

- **PR #219** relaxed `LeaveNode` to accept `status IN ('active','pending')`,
  branching the audit action internally, on the reasoning that "a request
  was the one relationship a person could enter and not get out of."
- **This branch** added `POST /api/v1/nodes/{slug}/withdraw` and left
  `leave` alone.

What the codebase already said, and what bound the design:

- **The server already disagreed with itself.** `GET /nodes/{slug}` sets
  `membership_role` only for `status='active'` (`nodes.go`), while
  `GET /me/nodes` sends `role: 'member'` for a pending row
  (`memberships.go`). Two endpoints, one question, two answers.
- **Every other surface excludes a pending row.** `userHasNodeRole`
  checks status; the member list defaults to active; the member count is
  "admins plus members" (CONTEXT.md); a pending row draws no thread and
  earns no role mark.
- **`role` on a pending row carries no information.** All three paths that
  create one set `role='member'`, and there is no path that sets anything
  else — you cannot request admin, and following is frictionless and
  immediate. The field that leaked was a constant.
- **CONTEXT.md contradicted itself.** The Join sheet entry placed a person
  "standing as a member or requester", while Standing control enumerated
  standing exhaustively as Following / Member / Admin and Role mark gave
  exactly three marks. The glossary had the word and no room for it.
- **ADR 042** — an absent door beats a 403 at the end of a ceremony.
- **docs/adr/030** models a *claim* as its own object with its own
  `POST /claims/{id}/withdraw`, distinct from rejection because "nobody
  reviewed anything."

## Decisions

**1. A requester is outside the ladder.** Not a fourth rung: a requester
holds an outstanding request, not a relationship. This is the root
decision and the rest follow from it. CONTEXT.md gains **Requester**, and
the Join sheet entry no longer grants one standing.

**2. The wire stops asserting a role for a row with no standing.**
`GET /me/nodes` omits `role` unless `status='active'`, matching what
`GET /nodes/{slug}` already does. The client-side bug — a pending request
reading as membership on the quilt, on discovery, in the unlock panel, on
event pages — was every client defending itself against a payload that
lied. Fixing it in the clients fixes those clients. Fixing it on the wire
fixes the next one too. Both branches had done only the former.

**3. Withdrawing is its own route; `leave` goes back to active-only.**
`POST /nodes/{slug}/withdraw` takes a pending row and refuses everything
else. This reverts PR #219's relaxation, whose stated reason — that a
request is a relationship a person cannot get out of — does not survive
decision 1.

The deciding argument is not tidiness but a stale page:

> Settings shows **Requested / Withdraw**. An admin approves. The person
> clicks Withdraw.
>
> With separate routes, `/withdraw` sees an active row, returns 400, and
> nothing happens; they reload and find they are a member. With a relaxed
> `leave`, they resign a membership they never knew they had.

They meant "cancel my request" and got "quit the patch." The general form:
when the route names the object it acts on, acting on the wrong object
fails for free. One route with an internal branch would have to be told
what the client believed in order to check it; two routes put that belief
in the URL. This is why `POST /claims/{id}/withdraw` is not
`POST /claims/{id}/leave`.

Withdraw clears `join_message` as rejection does, notifies nobody as
`WithdrawClaim` does, and is scoped to the caller's own row — an admin
turning a request down is `UpdateMember`'s reject, which is a decision and
is logged as one.

**4. A request control, not a standing control.** The patch page states an
outstanding request in the place a standing control sits, with Withdraw in
its menu, because the menu discipline is about consequential acts and not
about standing: retracting costs the deliberate step departure costs. It
is a sibling of the standing control and never one of them — no role mark,
muted where a standing is not. CONTEXT.md gains **Request control**.

## An argument withdrawn

This branch originally justified the separate route on the audit log:
that one handler could not tell "changed their mind before anyone
answered" from "was here and left." **That was wrong about PR #219's
code**, which already selects `membership.withdraw` for a pending row.
The argument is recorded here as rejected so it is not reached for again.
What decided it was decision 1 and the stale page, not the audit trail —
though the trail does stay honest either way.

## Consequences

- PR #219's `LeaveNode` relaxation is reverted. Anyone reading that commit
  should land here. Its other work — `node_status` on `me/nodes`, gating
  the member rung on it, per-exit wording — stands and is kept.
- A conditionally-absent JSON field is a footgun of its own: a client
  reading `m.role` on a pending row now gets `undefined` rather than a
  wrong answer. That is the intended trade — loud beats plausible — but it
  is a trade.
- `memberships` keeps the request as a row state rather than becoming its
  own table. A claim earns a table because it has request-shaped state of
  its own (method, token, send-count window, expiry); a membership request
  has one nullable text column, and `UNIQUE(user_id, node_id)` already
  buys "one open request per person per patch."
- Two verbs must stay apart. `TestLeaveStillRefusesAPendingRequest` exists
  to fail if `leave` ever accepts a pending row again.
