# ADR 084: A restore rebuilds the mirror, and the mirror says so

Date: 2026-09-07. Status: accepted (implemented same day).

## Context

docs/adr/011 settled which of a patch's two governance stores is
authoritative: the `governance_docs` row is canonical, the bare git repo is a
derived history mirror. docs/adr/002 drew the portability boundary the same
way round — the rows travel in a seamrip, the repos deliberately do not.

Both decisions are right, and together they describe a state neither of them
handles. An instance restored from its SQLite file alone — the ordinary
backup, the one every operator actually has — comes up with every charter's
text intact and no repos under `data/governance/` at all. So does a seamrip
import, by design. The derived store is missing and nothing derives it: the
repo is created by `ForkForNode` at patch creation and never again.

What that costs is not history. History is already gone in that scenario and
no decision here brings it back. What it costs is the *future*: every
governance write opens the repo first. A direct edit, an amendment branch, a
merge, `WriteRules`, `ReadRules` — each begins at `openBare`, and on a
restored instance each fails or silently falls back to defaults. The
community can read its charters and can never change them again. The failure
is quiet, it is permanent, and it lands on exactly the communities that had
a bad enough day to need a restore.

There was already half a fix. `BackfillNodeGovernanceRepos` ran on every boot
and created repos that were absent — but it built them by forking the
*casual template* and then mirroring the rows over the top, and it wrote them
under the ordinary "Patchwork System" author with a "Backfill …" message. So
a repaired patch came back carrying an operating agreement nobody had adopted,
running whatever rules the casual template ships rather than the ones it had
been governed by, and looking in its history exactly like a patch that had
never lost anything.

## Decision

**One repair pass, `governance.Repair`, reads the canonical rows and
reconciles the mirror. It runs in two reaches: create-missing on every boot,
and the full reconciliation on an operator's command.**

**Idempotent, and no-op on a healthy instance.** A repo whose files already
equal their rows is not opened for writing; HEAD does not move. This is what
makes the boot half safe to leave on forever.

**It never deletes.** Not a ref, not a commit, not a file. A repo that
carries a document the database has never heard of keeps it. The pass fills
absences and corrects staleness; it does not make the mirror a mirror by
subtraction.

**A rebuilt repo is one commit, not one per document.** Both are fictions —
none of these commits happened when they say they did. One commit tells the
true version of the fiction: this entire tree arrived from the database in a
single moment. A run of them would fabricate an order and a set of dates the
community never lived through, and would leave each charter's history several
entries long when what that patch actually has is one. Inside a repo that
*exists*, the opposite holds: the surrounding history is real and
`GetHistory` filters by filename, so each corrected file gets its own commit
and a document that was already current gains no entry for its neighbour's
repair.

**Only what the database attests to is written.** A rebuild does not fill the
gaps with template text. A governance document nobody adopted, sitting in a
patch's repo wearing the same authority as one they voted on, is worse than a
document that is missing.

**The rules file is the exception, and comes back from the DB cache.**
`governance-rules.json` has no `governance_docs` row; git is normally its
canonical store and `nodes.governance_config` / `membership_policy` /
`follower_permissions` are the derived cache. But a repo that is gone has no
rules to be canonical about, and the cache is what the instance was actually
enforcing. So a rebuild reconstructs the file from those columns — the exact
inverse of `SyncRulesToDB`. Falling back to `DefaultRules()` instead would
silently demote an elected patch to `maintainer`, which is the same class of
bug as the one docs/adr/002's amendment found in `seats`: the safety
mechanism stripping the machinery that rotates leadership. Inside an existing
repo the rules file is only ever restored when it is *absent*, never when it
merely differs — a difference there is ordinary cache lag, and rewriting on
one would mint a no-op commit on every healthy instance on earth.

**A repair commit is visibly not a person's.** It is authored as
`Patchwork repair <repair@patchwork.local>`, and a rebuild's message is fixed
text: `Rebuilt from database; original history unavailable`. `GetHistory`
reads the marker off the author identity — not off the message, so a member
who happens to write those words in an amendment is not mislabelled — and
carries it to the API as `repair: "rebuilt" | "restored"`. The charter's
history view says it out loud: a note reading "History rebuilt from the
database on ⟨date⟩. The current text is intact; versions from before the
rebuild are not available", and a chip on each repair entry. The v1 entry of
a rebuilt document no longer claims to be "the initial version", because it
isn't one — it is the oldest version that survived.

No new table. The marker is a property of the commits, which is where the
question is asked.

**Two reaches, and the boot one is the strictly safe half.**

| | on every boot | `patchwork -repair-governance` |
|---|---|---|
| repo absent | rebuilt from the rows | rebuilt from the rows |
| document absent from an existing repo | left | committed |
| document stale against its row | left | committed |
| prints a per-patch summary | no (one log line) | yes |

Doing both is deliberate. The startup half is the one that fires without
anybody remembering it exists, and a restored instance is precisely the case
where nobody knows to look — so it must be the half that cannot do damage: it
only ever creates what is entirely absent, and never opens a repo that is
already there. Writing commits into live history is a different act, and it
should be somebody's decision.

**It is a flag on `cmd/patchwork`, not a `cmd/repair`.** The other tools in
`cmd/` are separate mains and this could have been one, but the distroless
runtime image ships exactly one executable, `/patchwork`. A separate binary
would be unrunnable in the deployment that needs it most — a Docker instance
restored from a database backup. Riding along here also means the repair
reads the same `patchwork.yaml` the server reads and derives the data
directory by the same rule (the database file's parent), so it cannot repair
a directory the server will not open. Run it with the server stopped.

## Consequences

- A database-only restore is now a supported procedure rather than a
  one-way door. docs/DEPLOYMENT.md carries it.
- docs/adr/002's known gap is narrowed but not closed: repos still do not
  *travel*, and no decision here recovers a history that was never exported.
  A forked or restored community gets a working mirror and an honest note
  saying where it came from.
- `governanceFilename` moved to `governance.Filename`. The title→filename
  mapping is the only link between a canonical row and its history in git,
  and it now has one definition instead of a copy in each package that
  crosses by it.
- `BackfillNodeGovernanceRepos` is a thin call into the create-missing reach.
  Backfilled repos changed shape: they no longer carry casual-template text,
  and they keep the rules the patch was running.

## Rejected

**Make the repos travel in a seamrip.** The honest fix to the seamrip half,
and it does nothing for the restore half — a SQLite backup is not a seamrip,
and it is what operators have. It also reopens what docs/adr/011 closed: a
repo that travels is a second thing to keep consistent with the rows on the
far side, and the ADR's whole argument is that two authoritative writers is
the drift it exists to end. Repo transfer stays where docs/adr/002 put it.

**Repair on write instead — heal the repo when `openBare` fails.** Attractive
because it needs no command and no boot pass: the repair happens exactly when
it matters. Rejected because it puts a history-fabricating write in the path
of an ordinary member's edit, at the moment they are least able to judge it,
and because a read path that silently self-heals is how a damaged instance
stays undiagnosed. It would also fire under conditions that are not damage at
all — a disk that is briefly unreadable would get a rebuilt repo rather than
an error.

**Let the boot pass do the whole reconciliation.** Then nobody needs to run
anything, which is the argument for it. Rejected: a restart would rewrite
files inside live repos on the strength of a comparison, and a bug in that
comparison is a commit on every charter on the instance, made by nobody, at
3am, because a container restarted. The asymmetry is the point — creating
what is absent cannot destroy anything, and writing into what is present can.

**Silent repair.** The smallest diff and the worst outcome. A synthetic
commit that looks like history *is* a claim about what the community did, and
a governance record that quietly contains one is worth less than a record
with an admitted hole in it. The whole value of the mirror is that people can
read it; a mirror that lies about its own provenance has nothing left to
offer.

**A `repos_rebuilt` table (or a column) recording the repair.** Considered
for the history note, and unnecessary: the commits already carry the fact,
and reading it off them cannot drift from what the repo actually contains. It
would also have had to answer `TestEveryTableHasABoundaryDecision` — and the
answer is that it stays behind, which means a seamrip would strip the note
off a fork while leaving the rebuilt commits it describes.

**Fill a rebuilt repo from the patch's template.** What the old backfill did.
The template chosen at creation is not persisted, so "the patch's template"
means "casual" — and the text it lands is governance the community never
adopted, in a store that is supposed to mirror what they did.
