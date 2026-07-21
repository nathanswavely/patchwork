# ADR 016: Drop CGO — pure-Go SQLite via modernc.org/sqlite

Date: 2026-07-19. Status: **proposed**, not implemented. Spiked and measured
(see Evidence); the spike was reverted and nothing in the tree depends on it.

## Context

`github.com/mattn/go-sqlite3` compiles SQLite's C amalgamation, so the binary
needs a C toolchain and `CGO_ENABLED=1`. Everything downstream of that has to
carry a compiler around: the Dockerfile installs `gcc musl-dev` and links
statically with `-linkmode external -extldflags "-static"` to land on
distroless.

The concrete cost is CI. A Go build with cgo cannot cross-compile without a
target-architecture C toolchain, so the arm64 half of the multi-arch image
built under QEMU emulation on an amd64 runner. Measured on run 29701066404
(commit c9b4ade):

| Step | amd64 (native) | arm64 (QEMU) |
|---|---|---|
| `go build` | 93.7s | **788.7s** |
| `npm run build` | 39.3s | 309.1s |

ADR-less CI work in #21 removed the 309s (the frontend stage is now pinned to
`$BUILDPLATFORM`) and stopped paying the arm64 cost per-merge by gating it to
version tags. That took the `image` job from ~20 min to 2m14s. But it is a
mitigation, not a fix: the 789s is still there, it now lands on release tags
instead of every push, and arm64 — the Raspberry Pi story CLAUDE.md commits to
— is the thing that got slower to validate.

Two facts make this cheaper to fix than it looks.

**The coupling surface is three lines.** There are zero references to the
`sqlite3.` package anywhere in the Go tree — no `RegisterFunc`, no
`sqlite3.Error` type assertions, no backup API, no custom collations. The
driver is reached only through `database/sql`. The entire binding is a blank
import, two `sql.Open` driver-name strings, and one DSN, all in
`internal/database/database.go`. go-sqlite3 is also the *only* cgo dependency
in `go.mod`; go-git, go-webauthn, yaml, and x/time are all pure Go.

**`CGO_ENABLED=0` today is a runtime trap, not a build error.** go-sqlite3
ships a stub for builds without cgo, so the binary compiles and links happily
and then fails on first query:

```
database: integrity check: Binary was compiled with 'CGO_ENABLED=0',
go-sqlite3 requires cgo to work. This is a stub
```

The process does not exit non-zero — it logs and continues. Anyone who
disables cgo to get a cross-build gets a binary that starts, serves, and
cannot read its own database.

## Decision

**Replace `mattn/go-sqlite3` with `modernc.org/sqlite` and build with
`CGO_ENABLED=0`.**

modernc.org/sqlite is SQLite transpiled from C to Go. It is a
`database/sql` driver like the current one, so the change is:

- `_ "github.com/mattn/go-sqlite3"` → `_ "modernc.org/sqlite"`
- `sql.Open("sqlite3", …)` → `sql.Open("sqlite", …)` (two call sites)
- the DSN's PRAGMA syntax, which is the one genuine incompatibility:
  `?_journal_mode=WAL&_foreign_keys=ON&…` becomes
  `?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&…`

That DSN is load-bearing and deserves calling out. `Open` also runs a
multi-statement `PRAGMA` Exec after opening the pool, but under
`database/sql` that Exec runs on *one* pooled connection — it is not a
substitute for the DSN. PRAGMAs like `foreign_keys` are per-connection, so if
the DSN syntax is silently wrong, foreign keys are off on every connection
except one and nothing announces it. This is verified below rather than
assumed.

The Dockerfile then drops `apk add gcc musl-dev`, the `CGO_ENABLED=1`, and the
external-static link flags. Stage 2 can build for any target with plain
`GOOS`/`GOARCH`, so multi-arch stops needing QEMU at all.

## Evidence

Spiked on the real tree at 98a42ef and then reverted. Unless noted, measured on
Windows amd64 (go1.26.5, 16 logical CPUs) — **not** on a Pi.

**It works.** Full suite green with `CGO_ENABLED=0`: `go vet` clean, and all of
`ap`, `auth`, `config`, `database`, `governance`, `handler`, `middleware`,
`seamrip` pass. A built binary starts, applies all 20 migrations from scratch,
answers `GET /api/v1/instance` and `GET /api/v1/nodes/tree` with 200, and
leaves a file with `journal_mode=wal` and `PRAGMA integrity_check = ok`.
All 20 migrations applying is also decent evidence that modernc's bundled
SQLite is new enough for the SQL this project actually uses.

**PRAGMAs reach every connection.** Holding 8 pooled connections open
simultaneously and querying each: `foreign_keys` is `1` and `journal_mode` is
`wal` on all 8, under both the new DSN syntax and the current driver. Parity,
confirmed rather than hoped for.

**Build time collapses.** Cold arm64 cross-compile (`go build -a`) is **23s**,
against **788.7s** for the same work under QEMU. It also cross-compiles clean
to `linux/arm` (32-bit), which the cgo setup never did.

**Binary size is a non-issue.** Stripped like-for-like with the Dockerfile's
`-ldflags="-s -w"`: cgo amd64 13.5MB, modernc amd64 14.3MB (+0.8MB, +6%),
modernc arm64 13.4MB.

**Runtime is a real trade, and it is not uniform.** Medians of 5 runs,
2000 iterations each:

| Benchmark | mattn (cgo) | modernc | change |
|---|---|---|---|
| Insert, autocommit | 58.9µs | 72.9µs | 1.24× slower |
| Insert, in transaction | 4.5µs | 7.6µs | 1.67× slower |
| Select, indexed point lookup | 8.4µs | 15.2µs | 1.81× slower |
| Select, scan 500 rows | 517µs | 326µs | **1.59× faster** |

Point operations cost 1.2–1.8× more. Multi-row scans get *faster*, which is
not a fluke: cgo pays a call transition per `sqlite3_step`, so a 500-row scan
crosses the boundary 500 times and modernc crosses it zero times. Patchwork's
hot read paths — the tree endpoint, list endpoints — are row scans.

## Considered options

- **`tonistiigi/xx` cross-toolchain helpers** (keep cgo, cross-compile with a
  target-arch musl toolchain): rejected. It solves the build time without the
  runtime trade, and it is the right answer for a project that genuinely needs
  the C library. But it keeps a C compiler and a static-link incantation in the
  build, keeps `CGO_ENABLED=0` as a runtime landmine for anyone building
  outside Docker, and adds a third-party build dependency to a project whose
  stated principle is a single self-contained binary. It buys build speed and
  nothing else.
- **Status quo plus the #21 arm64 gate**: rejected as the end state, accepted
  as the interim. It is what is deployed today and it is fine for merge
  latency, but it makes releases slow and leaves the Pi path the least-tested
  one. Keeping it means the CI cost never actually goes away.
- **`zombiezen.com/go/sqlite`**: rejected. Also pure Go (it sits on modernc's
  translation) and generally faster, but it deliberately does not implement
  `database/sql` — it exposes its own API. That turns a three-line change into
  rewriting every query in the project. Wrong ratio of risk to payoff here.
- **Give up on arm64**: rejected. CLAUDE.md commits to running on a Pi 4 with
  2GB, and self-hosting on cheap hardware is a large part of who this is for.

## Consequences

- **Multi-arch gets cheap enough to un-gate.** With no QEMU in the path, #21's
  tag-gate on arm64 can be reverted and `:latest` can go back to being
  multi-arch. That gate is a workaround for exactly the cost this ADR removes,
  and leaving it in place afterward would be carrying the scar tissue with none
  of the injury.
- **Point-query latency rises ~1.2–1.8×.** On the workload this serves — a
  community instance where SQLite sits on the same box and requests are tens
  per minute, not thousands per second — microseconds per query is not the
  binding constraint. If a hot path ever does become measurable, it is more
  likely a scan, where this is a win.
- **Not measured on a Pi.** Every number above is amd64. The ratios should
  roughly hold, since the difference is transpiled-C-in-Go versus C, but arm64
  code generation and SD-card I/O are different enough that this deserves a
  real measurement on the target before anyone treats the perf trade as
  settled. This is the largest open risk in this ADR.
- **Existing databases were not exercised.** modernc implements the same
  on-disk format, so no data migration should be required, and the spike
  applied all 20 migrations cleanly from empty. But opening a populated
  production-shaped database — Lancaster's, via a copy — was not tested and
  must be before this ships.
- **Loses SQLite extension loading and cgo-only escape hatches.** Nothing uses
  them today. It is a door that closes, and worth knowing it closed.
- **Small cleanups fall out**: the Dockerfile comment on stage 2, the
  `CGO_ENABLED=1` note in `web/playwright.config.js:32`, and
  `dev-machine-go-cgo`-style local setup all stop needing a C compiler —
  contributors on Windows and macOS get a working build with no toolchain
  install at all. That is a real contributor-onboarding win for a project that
  wants to be forked.
- **Sequencing.** This should land on its own, behind its own PR, with the Pi
  measurement and the populated-database check done first. It touches the
  storage layer of a project that has already had one production data-loss
  incident (2026-07-19); it should not be bundled with anything.
