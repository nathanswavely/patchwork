# Release notes

One file per release: `release-notes/vX.Y.Z.md`, named for the git tag exactly
as it is pushed, leading `v` and all. The front matter is read by machines, the
body is read by people, and CI refuses to publish an image for a tag whose file
is missing or malformed.

See docs/adr/085 for why this is a hand-written file rather than something
generated from commits.

## Who writes it, and when

**The person cutting the tag, in the commit before they cut it.** The file for
`v0.9.0` has to be on `main` when `v0.9.0` is pushed. There is no grace period
and no backfill step: the `release-notes` job in `.github/workflows/ci.yaml`
runs on every tag and the `image` job needs it, so a tag with no notes builds
nothing and publishes nothing. Fixing it means committing the file and moving
the tag.

Validate before you tag:

```sh
make release-notes-check              # every file in the directory
make release-notes-check TAG=v0.9.0   # and: v0.9.0 has one
```

## The fields

Front matter is YAML between two `---` fences at the very top of the file.
Three keys are required, one is optional, and anything else is an error — an
unrecognised key is nearly always a typo.

| Key | Type | Meaning |
|---|---|---|
| `breaking` | boolean, required | Does rolling this out unattended risk breaking a running instance? |
| `irreversible_migrations` | boolean, required | Does any migration in this release refuse to go backwards? |
| `summary` | string, required | One line, under 120 characters. It becomes the release title. |
| `version` | string, optional | The version again, if you want it stated twice. Must match the filename. |

`breaking` and `irreversible_migrations` must be real YAML booleans — `true` or
`false`, unquoted. `"true"` is a string and is rejected, because a quoted
`"false"` is truthy to most things that would read this.

### What counts as `breaking: true`

Anything where an operator who updates without reading gets a worse instance
than they had. Some examples:

- A config key in `patchwork.yaml` is renamed, removed, or newly required.
- A deployment shape changes — a volume path, a port, the container's working
  directory (this is what happened on 2026-07-19).
- An API response or endpoint changes in a way an existing client notices.
- A default flips in a way that changes who can see something.

Adding an optional field, a new endpoint, or a new setting with a safe default
is not breaking. When it is genuinely arguable, say `true` — the cost of a
false `true` is a human looking at it, and the cost of a false `false` is
somebody's instance.

### What counts as `irreversible_migrations: true`

Whether the migrations in *this* release can be undone by restoring the
previous image against the same database. A dropped column, a rewritten value,
a table collapsed into another one: `true`. A new table, a new nullable column,
a new index: `false`. This is a separate question from `breaking` — a release
can be perfectly compatible and still leave a database an older binary cannot
read, which is exactly the case where an updater wants to take a backup first.

### The body

Everything below the closing fence, in Markdown, is published verbatim as the
GitHub release notes. Write for the self-hoster reading the release page at
eight in the morning: what changed, and what — if anything — they have to do.
When `breaking` is `true`, the body must say what breaks and what to do about
it; that is the sentence the flag exists to point at.

## Example

Copy this block into a new file and edit it.

```markdown
---
breaking: false
irreversible_migrations: false
summary: noticeboard replies, and the map stops guessing
---

The noticeboard grew replies, and a patch's map marker is now only ever placed
by a person confirming it.

## Upgrading

Nothing to do. `docker compose pull && docker compose up -d`.

## Changes

- Noticeboard notices take flat replies; reports route to the patch's own queue.
- Address suggestions are provisional until confirmed.
```

A breaking one differs only in the flag and in having an "Upgrading" section
worth reading:

```markdown
---
breaking: true
irreversible_migrations: true
summary: the container's working directory moved into the data volume
---

Older images ran with their working directory on the ephemeral container
layer, so a relative `database.path` wrote outside the `data` volume and
recreating the container destroyed the database.

## Upgrading

**Take a backup and read docs/DEPLOYMENT.md § Updating before pulling this.**
Check where your database actually is:

    docker run --rm -v patchwork_data:/data alpine ls -la /data

If the volume is empty, copy the database out of the container layer with
`docker cp` before recreating.
```

## What CI does with it

On a `v*` tag:

1. The `release-notes` job validates the whole directory and insists that this
   tag has a file. `image` and `manifest` need that job, so nothing publishes
   without it.
2. After `manifest` pushes the multi-arch tag, the `release` job creates or
   updates the GitHub release for the tag, using the body as its notes, and
   attaches `release.json`:

```json
{
  "version": "v0.9.0",
  "image": "ghcr.io/patchwork-toolkit/patchwork:0.9.0",
  "breaking": false,
  "irreversible_migrations": false,
  "summary": "noticeboard replies, and the map stops guessing"
}
```

An unattended updater fetches that one file and decides. See
docs/DEPLOYMENT.md § "Deciding whether an update is safe to apply".
