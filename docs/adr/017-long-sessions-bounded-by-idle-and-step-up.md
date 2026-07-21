# ADR 017: Sessions stay long, bounded by idle timeout and step-up auth

Date: 2026-07-20. Status: accepted.

## Context

`SessionExpiry` was a hardcoded `30 * 24 * time.Hour` const with no config
surface, and expiry was **absolute**: written once at insert, never
extended, never shortened. The sessions table had no `last_used_at`, so
neither a sliding window nor an idle timeout was expressible without a
migration. Nothing rotated on privilege change, and no one could see or
revoke their own sessions.

Reviewing it surfaced a sharper problem than the lifetime itself.
`AdminWipe` — which erases every patch, person, event, proposal, and
governance record — was reachable from a month-old cookie, gated only by
`AdminRequired` plus typing the instance name. The handler's own comment
notes the audit log is wiped too, so the only surviving trace is a server
log line. Instance export is the same shape: `AdminRequired` alone, and it
moves every member's email address (docs/adr/012).

The instinct is to shorten the session. That is the wrong lever. Thirty
days is unremarkable for a community platform, and the people it would
punish most are exactly the ones Patchwork is for — volunteer organizers
checking in from a phone every few weeks, on an instance where SMTP is
optional and re-authenticating may mean finding an invite link. Shortening
sessions trades a large, certain usability cost against a small slice of
the actual risk, because the risk was never "the cookie is old." It was
"an old cookie is sufficient proof to destroy everything."

## Decision

**Session lifetime stays long and becomes configurable.** `session:` keys
in patchwork.yaml set the absolute maximum (default 30 days) and an idle
timeout (default 14 days). Both are read at startup; neither requires a
recompile.

**Expiry becomes two-sided.** A new `last_used_at` column is stamped on
validation, and a session dies at whichever comes first: the absolute
expiry, or `last_used_at + idle timeout`. An active organizer is never
logged out; an abandoned session on a lost phone closes itself. The
stamping is throttled (at most once per hour per session) so a read-heavy
page does not turn every request into a write on a Pi's SD card.

**Destructive actions require step-up authentication.** A fresh WebAuthn
assertion, valid for a five-minute sudo window recorded on the session,
gates:

- instance wipe
- instance export / seamrip (it moves other people's emails)
- promoting an account to instance admin

Holding a valid session is no longer proof of presence for these three.
The window is per-session and does not survive logout. An admin with no
passkey enrolled must enroll one before performing these actions — this
does not lock anyone out, since enrollment needs only the session they
already hold, and it means the highest-consequence actions on an instance
always have a hardware-backed proof behind them.

Suspension already destroys sessions and stays as it is. Role *demotion*
does not rotate the session, which is acceptable: `ValidateSession` re-reads
the role from the users join on every request, so the authorization
decision is always fresh even when the session identifier is stale.

## Considered options

- **Shorten the session to 7 or 14 days.** Rejected — pays a certain,
  recurring usability cost in exchange for narrowing a window that was
  never the actual vulnerability. A 6-day-old cookie wipes the instance
  just as thoroughly as a 30-day-old one.
- **Sliding expiry with no absolute ceiling.** Rejected — a session that
  renews forever is a permanent credential; the absolute ceiling is what
  guarantees every session eventually ends.
- **Step-up for every admin action.** Rejected — re-prompting for routine
  moderation trains admins to approve prompts reflexively, which is how
  step-up auth stops working. Reserve it for the irreversible.
- **Password/email re-entry instead of WebAuthn.** Rejected — Patchwork has
  no passwords by design, and email re-auth is unavailable on the SMTP-less
  deployments the project explicitly supports.

## Consequences

Idle timeout is now the lever that matters, and 14 days is a guess. It
should be revisited once a real instance has usage data — if organizers are
being logged out, the number is wrong.

Step-up makes a passkey a soft prerequisite for instance administration.
That is a deliberate raise in the floor, and it needs to be visible in the
admin UI *before* someone hits a wall at the moment they are trying to
export their data.

`last_used_at` makes sessions legible for the first time, which is the
groundwork for a user-facing session inventory (see the follow-up issue) —
that feature is what makes a long session genuinely safe rather than merely
tolerated, and it is not built here.

Expired rows are still deleted lazily on next use, so sessions belonging to
people who never return persist in the table indefinitely. Harmless at
community scale; worth a sweeper if it ever isn't.
