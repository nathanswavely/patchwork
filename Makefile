.PHONY: build run dev seed seed-force export import test test-e2e smoke-recreate \
        copy-sync copy-stats copy-review copy-draft copy-pull copy-apply copy-check \
        copy-test copy-report

# Where `make build` writes the server binary. Override via the environment to
# build every worktree to one stable path — on Windows the firewall keys its
# allow/block rule to the executable's full path, so per-worktree binaries
# each prompt as a brand-new program. One path, one prompt.
PATCHWORK_BIN ?= ./patchwork

build:
	go build -o $(PATCHWORK_BIN) ./cmd/patchwork/

run: build
	$(PATCHWORK_BIN)

dev: build
	@echo "Starting Go backend (server.port from patchwork.yaml; the Vite proxy expects 8090) and Vite dev server on :5173..."
	@trap 'kill 0' EXIT; \
	$(PATCHWORK_BIN) & \
	cd web && npm run dev & \
	wait

seed:
	go run ./cmd/seed/

seed-force:
	go run ./cmd/seed/ -force

export:
	go run ./cmd/export/ -db data/patchwork.db -out ./export

import:
	go run ./cmd/import/ -db data/patchwork.db -in $(or $(IN),./export)

test:
	go test ./...
	cd web && npx vitest run

test-e2e:
	cd web && npx playwright test

# Prove instance data survives `docker compose up --force-recreate`
# (i.e. an image update). Needs docker + curl. See docs/DEPLOYMENT.md.
smoke-recreate:
	bash scripts/smoke-recreate.sh

# --- Copy ledger -----------------------------------------------------------
# Who wrote the words a visitor reads. See tools/copy-ledger/README.md.
# `copy-check` runs in CI; the rest are for writing.

copy-sync:
	node tools/copy-ledger/cli.js sync

copy-stats:
	node tools/copy-ledger/cli.js stats

copy-review:
	node tools/copy-ledger/cli.js review

# Review as Markdown instead, for anywhere the local UI can't reach —
# GitHub's web editor, a laptop on a train, a phone. `FILE=` scopes it to
# one source file so you get a page of work rather than all of it.
copy-draft:
	node tools/copy-ledger/cli.js draft $(if $(FILE),--file $(FILE),) $(if $(TIER),--tier $(TIER),)

# `REDRAFT=1` re-cuts any draft whose markers no longer match the source,
# saving writing that has nowhere to land first.
copy-pull:
	node tools/copy-ledger/cli.js pull $(if $(REDRAFT),--redraft,)

# Dry run by default — writeback edits source, so it shows you the plan
# first. `make copy-apply APPLY=1` writes.
copy-apply:
	node tools/copy-ledger/cli.js apply $(if $(APPLY),--apply,)

copy-check:
	node tools/copy-ledger/cli.js check

# Guards the rule that makes two review surfaces safe: an untouched draft
# block never writes back over a decision made in the UI since.
copy-test:
	node --test tools/copy-ledger/drafts.test.js

copy-report:
	node tools/copy-ledger/cli.js report
