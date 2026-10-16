# A deployer never edits a tracked file

Date: 2026-10-16. Status: **accepted**. Applies docs/adr/025 (the flagship
gets no special powers) to the deployment files.

## Context

A deployment is a clone of this repository, and docs/DEPLOYMENT.md tells
self-hosters to update it with `git pull`. Most of what a deployer sets was
already outside the tracked tree: `patchwork.yaml`, `.env` and
`docker-compose.override.yml` are gitignored, and each tracked file that
seeds one carries an `.example` suffix.

The `Caddyfile` was the exception. It is tracked and bind-mounted into the
caddy container as it stands, and it asked deployers to uncomment an
access-log block in place. A second commented block, a `www` redirect, was
on its way. Either edit makes the tracked file dirty, and git then does
one of two things with it. `git pull` refuses as soon as the project
changes the `Caddyfile`, which stops an update halfway for a reason the
deployer did not cause. `git reset --hard` reverts it without a word, which
quietly turns a redirect or a log back off.

The reference instance hit exactly this: its deploy routine checks that the
`Caddyfile` is byte-identical to the release tag and has used `reset
--hard`. Quilthost's founding rule is that hosted instances run the
unmodified public Patchwork. So the fix had to be one every deployment
uses, rather than a special case for one instance.

A second trap sat underneath. The `Caddyfile` is mounted as a single file,
and a single-file bind mount follows the inode, not the path. `git pull`
writes a changed file as a new file, so a running caddy container goes on
reading the old one, and `caddy reload` inside it reloads the old one too.
Verified against caddy:2-alpine: after git rewrote the file the host held
v2, the container read v1, and a reload served v1. A container restart
picked up v2.

## Decision

**Tracked files belong to the project. A deployer's own settings live in
untracked files, and the tracked ones are never edited in place.**

For Caddy, that place is `caddy.d/`, gitignored apart from tracked
`*.example` recipes, and mounted read-only into the caddy container as a
directory. The `Caddyfile` imports two globs from it:

- `caddy.d/*.caddy` at the top level, for blocks of their own, such as a
  second site that redirects to this one. Its import is the first line of
  the file, so a global options block works in whichever file sorts first.
- `caddy.d/*.site` inside the Patchwork site's block, for directives on
  that site, such as an access log.

Two import points rather than one because Caddy site blocks cannot nest
and a site address cannot be declared twice: a top-level file can add a
site but can never add a directive to the existing one. The two suffixes
are disjoint, so neither glob can read the other's files.

The reasoning that used to sit in the commented blocks moves to the
recipes, which are still the project's text and still change with it. The
recipe for the access log is `caddy.d/access-log.site.example`.

A glob that matches no file is a warning in Caddy, not an error, so the
shipped state (nothing but recipes in `caddy.d/`) starts clean with two
warning lines. The tracked recipe also means the directory exists on every
clone, so Docker never has to create it; if it does (somebody copied only
the compose file and the `Caddyfile`), it creates an empty root-owned
directory and the same two warnings follow.

The stale single-file mount is documented rather than restructured:
DEPLOYMENT.md says to run `docker compose restart caddy` after an update
that changes the `Caddyfile`, and the compose file says it beside the
mount. Files in `caddy.d/` are read through the directory mount, so a
`caddy reload` sees them.

## Consequences

- An update never stops on, and never reverts, a deployer's Caddy
  settings, and an unattended routine can keep checking that the
  `Caddyfile` is byte-identical to its release.
- A recipe is opt-in by copying, so the project can change a recipe
  without changing any running deployment. A deployer who wants the new
  version copies it again.
- `caddy.d/` is host config: it joins `patchwork.yaml`, `.env` and the
  override in the list of things a backup has to carry, since a clone
  will not bring it back.
- Any future setting a deployer is invited to change gets the same shape:
  an untracked file and a tracked `.example`, never "uncomment this".

## Rejected

**Uncomment in place, with a warning about git.** It is the arrangement
that failed, and the warning is read once, at setup, while the failure
arrives months later during an update.

**One import point.** A single top-level glob cannot reach inside the
Patchwork site, so the access log, the one block the `Caddyfile` already
shipped commented, would have had nowhere to go.

**A snippet the site block imports by name.** Caddy errors on an import of
an undefined snippet and on a redefined one, so there is no way to ship a
default the deployer overrides.

**Mounting the directory that holds the `Caddyfile`, to fix the stale
mount.** It would mean moving the `Caddyfile` out of the repository root,
which every existing deployment's compose file and documentation point at,
for a problem a restart already solves.
