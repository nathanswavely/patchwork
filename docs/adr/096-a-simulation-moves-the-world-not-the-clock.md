# 096. A simulation moves the world, not the clock

**Status:** accepted, 2026-09-14

## Context

Governance runs on a calendar. A vote has a window, an election takes
nominations for a number of days and then votes for a number more, a seat
holds a term measured in months, and a council is found due when its term
runs out inside the time a contest takes (docs/adr/051). None of that can
be exercised by a person clicking through the app in an afternoon, and the
communities that would exercise it for real are the ones we least want to
hand an untested election. Proving the mechanics over eighteen months of a
co-op's life needs eighteen months to pass.

The Go tests make them pass by writing `voting_ends_at` into the past and
calling the sweep by hand. That works inside a test binary and nowhere
else: a scripted run or an agent driving the browser needs the *running
server* to see the same thing, and the server reads the wall clock.

Three ways of giving it something else to read were weighed.

**A clock seam in the product.** An `internal/clock` package whose `Now()`
carries an offset, with a dev-only endpoint to turn it. It is the idiomatic
answer and the repo already has two such seams (`inbox.go`, `ap/remote.go`).
It fails on scale: the moment a timestamp is written is not one place.
Go's `time.Now()` appears at roughly a hundred sites; the SQL literal
`strftime(..., 'now')` at sixty more inside Go queries; and twenty-six
migrations declare column defaults that evaluate `'now'` at insert, which
SQLite cannot alter afterwards. A seam that covers the first source and
not the other two produces a database in which a vote was cast before the
proposal it answers was created, and the auditor reading that record would
be chasing the harness, not the product. Covering all three is a sweep of
some hundred and fifty product sites and an audit of every INSERT that
leans on a default, undertaken so that a test lever can exist.

**A build-tagged lever.** The same seam, with the endpoint compiled out of
release binaries by `//go:build sim`. It solves the "not in the product"
worry cleanly and leaves the scale problem exactly where it was.

**Moving the world.** Leave the product reading the wall clock and slide
every stored instant into the past by the amount of time meant to pass.
A vote due to close next week closed three weeks ago; a term that ran to
spring ran out in winter. The server then finds, with its own clock and its
own sweeps, what it would have found had the month gone by. This is what
the Go tests do with one column; done to all of them it is a simulation.

## Decision

A simulation advances by moving the world. `cmd/sim advance <duration>`
opens the database, subtracts the duration from every stored instant, and
then runs the server's own hourly passes once — `handler.SweepElections`
and `notifications.RunReminders`, the production functions, not copies.
The product contains no notion of simulated time, no endpoint, no build
tag and no flag. `RunReminders` is exported for this; that is the whole
footprint.

Which values move is decided by shape, not by a list. Every column of every
table is inspected and anything that is, in its entirety, an ISO 8601
instant or date is moved and written back in the layout it was read in
(`.000Z` stays `.000Z`; RFC 3339 stays RFC 3339; a bare date stays a bare
date). Zoned instants embedded inside longer text — an activity's JSON, an
audit entry's detail, frozen voting terms — move too. A bare date inside
prose does not, because a street number and a meeting date look the same
from here. A new `*_at` column is covered the day it is added, without
anyone remembering the tool exists.

What does not move is named, with a reason, in `excludedTables`: sessions,
magic links, invite links, signup tokens, WebAuthn credentials, recovery
codes, the instance actor's keys, the outbound delivery queue, and the
migration ledger. These are instants measured against a person's real
clock — a session that expires because the simulation aged it a year is
the harness logging everyone out, not a finding. A test fails if a name on
that list stops being a table, so a rename cannot leave a stale exclusion
that silently starts aging sign-ins.

The tool refuses any database it did not mark. `cmd/sim personas` mints
accounts under `@sim.localhost` with a fixed session token each, refuses
a database that holds anyone else (the seed's rule, docs/adr/009), and
writes a `SIMULATION` marker next to the file; `advance` and `sweep`
refuse without it.

## Consequences

- **The product cannot behave differently under simulation**, because it
  cannot tell. Every code path a persona exercises is the one a community
  will.
- **Coverage is by construction.** The clock-seam approach would have
  been complete on the day it merged and incomplete on the day someone
  wrote the next `strftime('now')`. Shape-based selection has no such day.
- **Two things stay still, and the auditor is told so.** Governance git
  repos keep their real commit dates, so a charter history view shows an
  amendment "three minutes ago" that the database says was months back.
  Prose inside notifications already sent ("voting ends September 20")
  is not rewritten. Both are recorded in the harness guide as known
  artefacts rather than product findings.
- **Date-only columns move in whole days.** Advancing by a fraction of a
  day rounds them down. The tool says so when asked for one; the guide
  says to advance in days.
- **Direction is fixed.** Time only goes forward, so the world only moves
  back; there is no rewind. A simulation that wants to try a branch twice
  copies the database file first.
- **This is a testing tool and not a feature.** Stripe made test clocks a
  product; Patchwork does not, and if a community ever asks for one, the
  answer is a fresh ADR and not a flag on this one.
