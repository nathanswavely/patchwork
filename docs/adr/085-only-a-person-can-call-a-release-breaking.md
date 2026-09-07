# ADR 085: Only a person can call a release breaking

Date: 2026-09-07. Status: **accepted**; implemented. Closes the gap that
docs/DEPLOYMENT.md has been papering over in prose since 2026-07-19.

## Context

CI publishes images. A push to main moves `:latest`; a `v*` tag publishes
`:X.Y.Z`, multi-arch, hard-gated on the whole test pipeline (the `image` and
`manifest` jobs in `.github/workflows/ci.yaml`). All of that machinery answers
"did this build and does it work". None of it answers the only question an
operator actually has at update time: **is this one safe to apply without
reading it?**

Today the answer is a person reading a diff. That is fine for one instance and
it does not survive contact with a second. Quilthost — the hosting layer that
runs a fleet of pinned release tags — staged that decision into its own
docs/adr/0008 and wrote that breaking releases are held for manual promotion
*"via upstream machine-readable release notes, when they exist."* They did not
exist, so the clause was unimplementable and the gate was a human.

The self-hoster has the same problem with none of the tooling. DEPLOYMENT.md
carries a standing warning about the pre-2026-07-19 working-directory move —
older images ran with their working directory on the ephemeral container layer,
so a relative `database.path` wrote outside the volume and recreating the
container destroyed the database. That paragraph *is* this feature, written by
hand, once, for one incident, in a file nothing can parse.

Two things are worth noticing about that incident before designing anything.

**It was not visible in a schema or an API.** It was a `WORKDIR` in a
Dockerfile. Any heuristic that reads migrations, config structs or route tables
for signs of breakage would have called that release clean.

**It raised two separate questions, and they are not the same question.**
"Will this break my instance?" and "can I put the old image back?" have
different answers on different releases. A release that drops a column is
perfectly compatible going forward and unrecoverable going backward. A release
that renames a config key is the reverse — trivially rolled back, and it will
not start until you edit a file. Collapsing them into one "safe" bit throws
away the distinction exactly where an operator needs it, which is deciding
whether to take a backup first.

There is also a GitHub releases page. It exists, it is decent, and it is
entirely hand-made: `gh release list` shows v0.6.0 through v0.8.0 with titles
somebody typed. CI has never touched it. Whatever we build should stop that
being two jobs.

## Decision

**1. A release's notes are one hand-written file: `release-notes/vX.Y.Z.md`.**

Named for the git tag verbatim, leading `v` and all, so that
`release-notes/$GITHUB_REF_NAME.md` is the entire lookup and there is no second
set of rules for turning a tag into a path. YAML front matter carries the
facts; the body below the fence is Markdown prose for a human.

One file, not two. A JSON file beside a Markdown file is two things that drift,
and the drift is silent: the prose says "this changes where your database
lives" and the flag still says `false`, and only the flag is read. Front matter
puts both halves in one commit, under one review, in one diff.

**2. Three required facts, and `breaking` must be a real boolean.**

```yaml
breaking: true|false
irreversible_migrations: true|false
summary: <one line, under 120 characters>
```

`irreversible_migrations` is the second question from above, asked separately:
can the previous image be put back against this database. `summary` is one line
because it is also the release title — the releases page already reads
"v0.8.0 — claims complete through setup", and that is the right length.

Both flags are decoded as YAML booleans and nothing else. `"true"` is a string
and is rejected, because a quoted `"false"` is truthy to most things that would
read this, and the entire value of the field is that a machine can trust it.
Unknown keys are rejected too: at this size an unrecognised key is a typo far
more often than an extension, and the reader always ships in the same commit as
the files it reads.

**3. The gate is on the image, not on the release page.**

A `release-notes` job validates the directory on every run and, on a tag,
insists that this tag has a file. `image` needs it, alongside `test`, `smoke`
and `e2e`. So a tag with no notes builds nothing and publishes nothing.

The alternative — publish the image, skip the release page — was rejected
because it fails quietly. The image is the artifact people deploy; a missing
release page is invisible until somebody goes looking for it, by which time the
thing it was supposed to describe has already rolled out.

The job runs on every trigger rather than only on tags, which is not
decoration: a *skipped* job skips everything that needs it, so gating this job
on `github.ref_type == 'tag'` would have skipped `image` on main pushes and
silently stopped publishing `:latest`. Running always also means a malformed
notes file is caught in the PR that adds it, which is hours before it matters.

**4. `release.json` is the artifact, attached to the GitHub release.**

```json
{
  "version": "v0.26.0",
  "image": "ghcr.io/patchwork-toolkit/patchwork:0.26.0",
  "breaking": false,
  "irreversible_migrations": false,
  "summary": "the noticeboard, and events keep their own link"
}
```

A GitHub release asset, because it is fetchable by anything: `gh release
download`, or a plain `curl` against the public API on a box with no tooling at
all. `image` is passed down from the `manifest` job's own output rather than
reconstructed from the tag, so the file names the reference that actually
exists.

Fields are only ever added, never removed or retyped. That is the whole
compatibility promise, and it is enough of one.

**5. The release job runs after `manifest`, and is idempotent.**

After, because release.json names an image and a release announcing a tag that
does not resolve is worse than no release — an updater would gate on it and
then fail to pull. Idempotent, because tag builds get re-run (a registry
permission fix, a retried flake), and a second run must update the release it
already made rather than dying on "already exists": otherwise the image
republishes and the notes silently do not.

**6. The validator is Go, in `cmd/`.**

Following the existing separate-main pattern (`cmd/gazetteer`, `cmd/export`).
Go is what the publishing path already installs, `gopkg.in/yaml.v3` is already a
direct dependency, and putting it in `cmd/` means the same rules are exercised
three ways from one implementation: `go test ./...`, the CI job, and
`make release-notes-check` before a maintainer pushes a tag.

**7. No backfill.** Old releases have no asset. Consumers are told, in
DEPLOYMENT.md and in `release-notes/README.md`, to treat a missing
`release.json` as "ask a human" — absence is not `false`. Inventing flags now
for twenty-five past releases would mean asserting, from memory, the exact
thing this ADR says only a person at the time can assert.

## Consequences

Cutting a release gains one step: write the file, then push the tag. The
failure mode when you forget is loud and early — the pipeline goes red and no
image is published — rather than a release that shipped without saying what it
was. Fixing it means a commit and moving the tag, which is the correct amount
of friction for the thing that tells a fleet whether to auto-deploy.

The releases page stops being hand-made. The body of the notes file becomes the
release notes and the summary becomes the title, so the human artifact and the
machine artifact are written once, together, by the person who knows.

Quilthost's ADR 0008 clause becomes implementable, and self-hosters get the
same gate in two lines of shell, which is the point: this is not a hosting
feature that self-hosters can also use, it is one fact published once.

The honest cost: a wrong `false` is now a *confident* wrong answer, where
before there was no answer and everybody read the diff. Nothing in the pipeline
can catch it — that is what it means for this to be an assertion. The
mitigations are all social and all cheap: the flag is written at the moment the
person knows, it is in a diff a reviewer sees, `release-notes/README.md` says
plainly that when it is arguable the answer is `true`, and the cost of a false
`true` is one human looking at a release while the cost of a false `false` is
somebody's database.

Prerelease tags (`v1.0.0-rc.1`) validate and publish like any other, and the
release job marks them prerelease so an `-rc` never becomes "Latest" on the
releases page.

## Rejected alternatives

**Conventional Commits, with a `BREAKING CHANGE:` footer.** The standard
answer, and it fails on two counts here. It moves the assertion to the moment
of writing a commit, when nobody yet knows what the *release* will contain —
and a release is often breaking in combination, from two commits that are each
harmless. It would also mean replacing this repo's commit style (plain-language
sentence subjects, a body explaining why — CLAUDE.md) with a machine grammar,
which is a large change to how everyone writes in exchange for a worse answer.

**A changelog generated from PR titles** (release-drafter and its kin).
Produces a list of what changed, which is a different artifact from a decision
about whether to apply it. An operator with a list still has to read it, which
is where we started. Worth having one day as *well*; it is not this.

**Heuristics over the diff** — flag a release breaking if it adds a migration,
touches the config struct, or removes a route. Every one of these has false
positives, and the 2026-07-19 incident is the proof of the fatal case: it was a
Dockerfile `WORKDIR`, and all three heuristics call it clean. A gate that is
wrong quietly is worse than no gate, because people stop reading.

**A `breaking` label on the PR.** The nearest miss, and tempting because it
lives where the change does. But a release is a *set* of PRs and the assertion
belongs to the set; and a label is mutable after the fact with no record in the
repository, so the thing a fleet gates on would live outside git. The file is
in git: reviewable, diffable, and carried by anyone who forks the repo.

**Semver alone — "a breaking change bumps the minor."** The project is 0.x,
where semver explicitly promises nothing, and an operator cannot tell an unsafe
0.26.0 from a safe one. Version numbers also cannot carry the second question
at all: nothing about 0.26.0 says whether its migrations go backwards.

**One `safe_to_auto_update` boolean.** Collapses the two independent questions
into one bit and loses the case that matters most — a release that is
compatible and irreversible, where the right behaviour is not "hold for a
human" but "take a backup and proceed".

**An OCI image label** (`org.opencontainers.image.*` or a custom one), read
with `docker inspect`. Attractive because it travels with the artifact itself,
and unusable for the actual job: you cannot inspect a label without first
pulling the image you are trying to decide whether to pull, and reading it
needs a registry client where reading a release asset needs `curl`. The label
answers the question after it stops mattering.
