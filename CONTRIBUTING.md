# Contributing to Patchwork

If you see any issues or have any ideas, please reach out! I'm building Patchwork as a labor of love, and there are many things that I may have missed. There may also be security concerns that I in my front-end focused experienced missed. Please feel free to PR, big or small.

## Dev setup

Requirements: Go 1.25+, Node 20+.

```sh
cp patchwork.yaml.example patchwork.yaml   # edit name, domain, geography
cd web && npm ci && cd ..
make dev        # Go backend + Vite dev server with hot reload
make seed       # optional demo data (fictional; see docs/adr/009)
```

`make build` writes the server binary to `./patchwork`. Set `PATCHWORK_BIN`
to an absolute path to send it somewhere else. This is useful on Windows, where
the firewall keys its rule to the executable's full path, so a per-worktree
binary prompts as a new program every time. Pointing every worktree at one
path means approving once. The tradeoff: worktrees then share a binary and
can't run servers simultaneously.

Before opening a PR:

```sh
make test       # go test + vitest
make test-e2e   # Playwright suite (seeds its own test DB)
```

CI runs the same suites plus `go vet`, `govulncheck`, and npm audit.

## Finding your way around

- CONTEXT.md: the vocabulary glossary. Backend code uses generic terms
  (node, event, proposal); the UI speaks "textile" (patch, quilt, charter,
  thread). Code, database columns, and API endpoints always use the
  backend terms; only UI copy translates.
- docs/adr/: design decisions, indexed by status in
  [docs/adr/README.md](docs/adr/README.md). If a change contradicts an
  accepted ADR, the PR should either follow the ADR or include a new one.
- CLAUDE.md: orientation for coding agents. If you work with one, point
  it there; if you don't, you can ignore it.

## Conventions

- Go stdlib router, no frameworks. SQLite with the PRAGMAs listed in
  CLAUDE.md. Cursor pagination on all list endpoints. UUIDv7 IDs,
  ISO 8601 TEXT timestamps.
- Behavior changes come with tests. The e2e suite is the backbone.
  Regression tests reference the issue or ADR that motivated them.
- The binary must stay comfortable on a Raspberry Pi 4 with 2GB RAM; be
  suspicious of dependencies.

## ADR and migration numbers

Both are sequentially numbered and both can be claimed by branches that
can't see each other. Before claiming a number, check what's in flight,
not just what's on disk:

```sh
git ls-tree --name-only origin/main docs/adr/   # or migrations/
gh pr list --state open
```

Numbers are never reused. A retired ADR keeps its number and gains a
status line; `migrations/006` is intentionally absent.

## AI-assisted contributions

Acceptable. Much of Patchwork is built via AI coding. Two rules: disclose it
(keep the `Co-Authored-By` trailer your tool adds), and review what you
submit. You are the author of your PR; "the model wrote it" is not a
review process.

## Security issues

Never through the public tracker — see [SECURITY.md](SECURITY.md).
