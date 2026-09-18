# ADR: A name nobody has to ask for

Date: 2026-09-16. Status: accepted. Closes the ADR number space at 115 and the
migration number space at 074 (amended 2026-09-18: at 117 and 075, see the
addendum); new records in both directories are named from
the clock and a sentence. This ADR is the first record to carry no number, and
it is named under the rule it establishes.

## Context

`docs/adr/` and `migrations/` are both sequentially numbered. A number is
claimed at creation, from a counter whose true state is the union of every
branch in flight. No branch can see that union. Two branches each read "the
highest number on disk", each pick the same next one, and each are right
locally. The merge is clean, `schema_migrations` keys on the whole filename so
both migrations apply in a defined order, and nothing anywhere complains.

The cost has been paid repeatedly:

- Two ADR 017s reached main.
- Two migration 066s reached main, and unpicking one of them is why
  `renamedMigrations` exists.
- ADR 063 was claimed against a clean board and was wrong three hours later,
  while it sat in review.
- On 2026-09-16 a session claimed 114 after verifying it free across
  `origin/main`, every remote and every local branch. PR #285 pushed its own
  114 shortly after. It was caught only by the mandated re-check before merge
  and renumbered to 115 in PR #288.

CLAUDE.md answered this with a ritual: check `git ls-tree origin/main`, then
`gh pr list --state open`, then `git branch --list`, then re-check before
merge, and again after any merge of main into a long-lived branch. The ritual
is documented honestly, including its own insufficiency. It asks a question
whose answer expires. "Is 114 free?" was true when asked and false three hours
later, and no amount of asking earlier fixes that.

**The ritual scales with parallelism, and this repo's parallelism is the
point.** Work here happens in concurrent agentic worktrees. Every additional
worktree adds a board to check and shortens the shelf life of the answer. A
process whose cost grows with the thing the project is built around is not a
process, it is a tax.

### What the numbers are actually doing

Before proposing to change them, we counted what would break.

| | Count |
|---|---|
| `docs/adr/NNN` path citations | 2,890 |
| ...of which are **number-only** (`docs/adr/082`) | 2,884 |
| ...of which carry the slug | 6 |
| Prose `ADR NNN` citations | 680 |
| Files carrying at least one ADR citation | 650 |
| Citations to migrations by number | 125, in 83 files |

Two things follow.

First, **the number is the citation format**, and it is a citation format that
does not resolve. `docs/adr/082` is not a file. The file is
`082-a-gazetteer-suggests-a-person-places.md`. Every one of those 2,884
references is a handle a human or an agent has to glob for.

Second, **the back catalogue is the entire cost of any change**. A fresh ADR
carries between 3 and 41 citations across 3 to 19 files, measured across ADRs
108 through 115. Renaming all 115 costs roughly two hundred times what
renumbering one colliding ADR costs. Any scheme that rewrites history pays
that; any scheme that does not, does not.

## Decision

**Cut over. Do not migrate.**

ADRs 001 through 115 and migrations 001 through 074 keep their names forever.
Every existing citation keeps resolving. Nothing is swept. (The addendum below
extends this to 117 and 075.)

From this record forward:

- A new ADR is `docs/adr/YYYY-MM-DD-slug.md`, where the slug is the decision
  stated as a sentence, in the style the existing 115 already use.
- A new migration is `migrations/YYYYMMDDTHHMMSS_slug.sql`.

Neither number space is ever drawn from again. 116 was free when this ADR was
written, and is declined.

### Why the identity is minted from a clock

A name you can choose alone is a name nobody has to ask for. The clock is
available in every worktree, it agrees with every other worktree without being
consulted, and it does not have a shelf life. Two agents will not produce the
same timestamp, and two agents will not write the same sentence. Where a
collision remains conceivable it is the same path with different content,
which git refuses loudly rather than merging clean.

This is not novel and we should not pretend it is. Sequential
`NNNN-title-with-dashes.md` is the ADR standard, from Nygard's original
convention through `adr-tools` to MADR, and it is precisely the convention
failing here, because it assumes one person writes ADRs occasionally. Projects
running parallel agents have been hitting this and converging on date-prefixed
ADR filenames independently. For migrations the timestamp answer is a decade
old and is what Rails Active Record and EF Core ship, adopted for this exact
reason.

### Why the two halves differ in precision

The asymmetry is deliberate and it is the whole distinction between the two
directories.

**For an ADR, the date is a label.** Two ADRs written the same day are told
apart by their sentences, and nothing cares which came first. `YYYY-MM-DD` is
what a person reads in a directory listing.

**For a migration, the timestamp is the order.** Two migrations written the
same day must have a determinate one, and falling back to alphabetical-by-slug
would let the name of a change decide when it runs. Second precision gives
true creation order.

`database.go` sorts with a plain `sort.Strings`, so `074_` sorts before
`2026...` and the legacy range runs first, which is correct because it is
older. Order is creation order rather than merge order, which is exactly what
claiming "the next free number" already gave, so nothing regresses.

### Why nothing is renamed, especially not a migration

The `renamedMigrations` hazard is entirely a *renaming* hazard. The runner
records the whole filename in `schema_migrations`, migrations are not
idempotent, and `database.Open`'s error is `log.Fatalf`, so renaming a file
that has reached a real database is a fleet that will not boot. A cutover
renames nothing. No entry is added to `renamedMigrations`, and the existing
entry stays, because entries there are permanent.

The same reasoning is why "numbers are never reused" survives in a stronger
form. The number space is not merely never reused, it is **closed**.
`migrations/006` remains intentionally absent, and a retired ADR keeps its
number and its status line.

## Alternatives rejected

**Slug-only for everything, including the back catalogue.** Right in the
abstract and collision-proof. Rewrites roughly 3,500 citations across 650
files, in a repo where the defining constraint is concurrent branches, which
makes it the single most collision-inducing commit available. The cure costs
two hundred times the disease.

**Keep numbers, add a duplicate-prefix guard.** A test mirroring
`TestMigrationNumbersAreUnique`, plus branch protection with `strict: true` so
a stale PR must re-merge and re-run before landing. This would work, and it is
the cheapest thing that could. It was rejected because a guard arbitrates
contention rather than removing it: under N concurrent worktrees, "caught"
means "one of them renumbers", every time two ADRs are written in the same
window. It converts a rare silent failure into a frequent loud one, and the
frequency tracks parallelism. It also does nothing about the known blind spot,
where a conflicting merge ref produces no checks at all and `gh pr checks`
reports `no checks reported`, which reads like a clean board.

**Reserved number ranges per worktree.** Already CLAUDE.md's advice for
parallel agents. It needs an allocator, which is the thing that is missing,
and it failed on 2026-09-16 precisely because the colliding branch was not one
the session had assigned.

**An append-only allocator file, designed to conflict.** Trades a rare silent
failure for a constant loud one. `copy/ledger.json` is this repo's own
evidence that a file which conflicts on every parallel branch becomes its own
tax, up to and including PRs that sit with no CI at all because there is no
mergeable ref to build.

**Draft-then-number at merge.** ADRs live at `docs/adr/draft-<slug>.md` on a
branch and take a number on main, where there is one board and no concurrency.
Genuinely collision-proof. Rejected because every citation written during a
branch's life is a long slug anyway, so it pays slug-only's ergonomic cost and
still keeps a renaming step forever.

**Alembic-style revision ids with explicit parent pointers.** The rigorous
answer, and it makes branching and merging first-class. Far heavier than a
single-binary project with 74 migrations needs.

## Consequences

**Citations get longer and start resolving.** "amends 021" becomes "amends
`a-patch-may-ask-for-a-word`". That is the real cost and it is worth naming
plainly. Partly offset: a slug citation is a working path, where
`docs/adr/082` is a handle that resolves to nothing, and the slug says what
was amended without opening anything.

**The directory holds two shapes forever.** This is honest rather than untidy.
It records that the project changed its mind on a specific date, which is what
the directory is for.

**Chronology stays in the filename.** `ls docs/adr/` still reads in order
within each range, so order does not depend solely on a hand-maintained index.
Every new ADR must carry its `Date:` line; 69 of the existing 115 do, and
every recent one does.

**The one guard worth keeping is a drift check, not a collision check.** The
realistic way this decays is that an agent reads 115 numbered files, pattern
matches, and writes `116-foo.md`. `TestNewRecordsAreNotNumbered` in the root
package fails on any new `NNN-` in `docs/adr/` or `NNN_` in `migrations/`,
against a frozen list of the legacy names. It arbitrates nothing and can never
block one branch on another's work, because after this ADR there is no
contended resource. `TestMigrationNumbersAreUnique` stays as it is, now
guarding a closed range.

**Forks inherit this.** The scheme ships with the product, and a fork that
starts parallel work on day one gets the answer rather than the ritual.

**CLAUDE.md, CONTRIBUTING.md and docs/adr/README.md lose the ritual.** The
"Claiming a number" section is replaced by the naming rule. What survives from
it is the part that was never about numbers: a clean merge is not a working
merge, so build and run the suites after merging main. Two branches adding the
same helper name, or registering the same route twice, still compile apart and
fail together, and no naming scheme touches that.

## The irony, recorded deliberately

This ADR was assigned a number by the ritual it retires. The check was run in
full on 2026-09-16: `origin/main` topped out at 115, no open PR added an ADR,
and no local or remote branch held 116. The number was free, and by CLAUDE.md's
own account that fact had a shelf life measured in hours. Declining it is the
shortest available demonstration of the argument.

## Addendum, 2026-09-18: the spaces closed one step later

Two branches were in flight when this ADR merged, and both landed numbered
records after it: ADR 116 and 117, and migration 075, in PR #297. That is the
blind spot the Context section describes, arriving on schedule, and
`TestNewRecordsAreNotNumbered` caught it, which is what the test is for. It
also made CI on main red for every branch that followed.

The repair is not a rename. Renaming a merged migration needs a permanent
`renamedMigrations` entry in `database.go` to rescue every database that
already recorded the old name, tested from a database carrying that name,
and the two ADRs are cited from over thirty places in code comments. Paying
that to defend a number is the cost this ADR exists to stop paying.

So the line moved instead: the ADR space closed at 117 and the migration
space at 075. The three files keep their names, `docs/adr/116` and
`docs/adr/117` keep resolving, and the test's constants say so. Nothing about
the rule changed. Anything numbered from here on is still mimicry and still
fails the build.
