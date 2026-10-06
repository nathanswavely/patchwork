.PHONY: build run dev seed seed-force export import test test-e2e smoke-recreate gazetteer \
        release-notes-check sim sim-personas sim-advance sim-sweep sim-status sim-now sim-reset \
        copy-sync copy-stats copy-review copy-draft copy-pull copy-apply copy-check \
        copy-report script-test errcheck errcheck-baseline

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

# Build the local place index from an OpenStreetMap extract (docs/adr/082).
# Run this on a machine with disk and memory to spare, then copy the result
# next to patchwork.db — the server never parses an extract.
#   make gazetteer IN=lancaster.osm.bz2
# Expect to convert first. Geofabrik publishes regions as .osm.pbf only, and
# this reads XML (see cmd/gazetteer), so osmium sits between the download and
# the build. Crop to a box while you are there: the builder reads the file
# twice, so cropping first is the difference between seconds and many minutes.
#   osmium extract -b <west>,<south>,<east>,<north> region.osm.pbf -o local.osm.bz2
gazetteer:
	go run ./cmd/gazetteer/ -in $(IN)

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

# CI runs this as scripts/errcheck-check.sh directly; the target exists so
# the same check is one command to run locally.
errcheck:
	bash scripts/errcheck-check.sh

# Regenerates errcheck.baseline from the current tree — the only way the
# backlog it grandfathers ever shrinks. Review the diff before committing:
# it should only remove lines (fixed discards) or add ones you meant to add.
errcheck-baseline:
	go run github.com/kisielk/errcheck@v1.20.0 ./... 2>&1 | grep -E '^[^:]+:[0-9]+:[0-9]+:' | sed -E 's/^([^:]+):[0-9]+:[0-9]+:/\1:/' | tr '\\' '/' | sort > errcheck.baseline

# Prove instance data survives `docker compose up --force-recreate`
# (i.e. an image update). Needs docker + curl. See docs/DEPLOYMENT.md.
smoke-recreate:
	bash scripts/smoke-recreate.sh

# --- Governance simulation (docs/adr/096) ------------------------------------
# A throwaway instance that cmd/sim can move through time. The product never
# learns it is being simulated: `sim-advance` slides every stored instant into
# the past and runs the server's own hourly passes once. See
# docs/testing/governance-simulation.md.
SIM_DB ?= data/sim/patchwork.db
SIM_CONFIG ?= cmd/sim/patchwork.sim.yaml
SIM_PERSONAS ?= cmd/sim/personas.example.yaml

sim: build
	$(PATCHWORK_BIN) -config $(SIM_CONFIG)

sim-personas:
	go run ./cmd/sim/ -db $(SIM_DB) personas $(SIM_PERSONAS)

#   make sim-advance BY=30d
sim-advance:
	go run ./cmd/sim/ -db $(SIM_DB) advance $(BY)

sim-sweep:
	go run ./cmd/sim/ -db $(SIM_DB) sweep

sim-status:
	go run ./cmd/sim/ -db $(SIM_DB) status

sim-now:
	go run ./cmd/sim/ -db $(SIM_DB) now

sim-reset:
	rm -rf $(dir $(SIM_DB))

# Validate release-notes/ before cutting a tag (docs/adr/085). Bare, it checks
# every file parses; with TAG= it also insists that release exists, which is
# exactly the check CI runs on a `v*` tag — run it before you push the tag,
# because a tag with no notes publishes no image.
#   make release-notes-check
#   make release-notes-check TAG=v0.9.0
release-notes-check:
	go run ./cmd/releasenotes $(if $(TAG),-tag $(TAG),)

# --- Copy ledger -----------------------------------------------------------
# Who wrote the words a visitor reads, recorded by SlopChop
# (https://www.npmjs.com/package/slopchop), which grew out of this repo's
# tools/copy-ledger. Scope and frozen constants live in .slopchop.json.
# `copy-check` runs in CI; the rest are for writing.
#
# Pinned to an exact version, the npm equivalent of pinning an action to a
# SHA: a published version cannot change underneath us.
SLOPCHOP ?= npx --yes slopchop@0.2.0

copy-sync:
	$(SLOPCHOP) sync

copy-stats:
	$(SLOPCHOP) stats

copy-review:
	$(SLOPCHOP) review

# Review as Markdown instead, for anywhere the local UI can't reach —
# GitHub's web editor, a laptop on a train, a phone. `FILE=` scopes it to
# one source file so you get a page of work rather than all of it.
# SLOPCHOP_DRAFTS_DIR points the drafts at a separate (private) checkout.
copy-draft:
	$(SLOPCHOP) draft $(if $(FILE),--file $(FILE),) $(if $(TIER),--tier $(TIER),)

# `REDRAFT=1` re-cuts any draft whose markers no longer match the source,
# saving writing that has nowhere to land first.
copy-pull:
	$(SLOPCHOP) pull $(if $(REDRAFT),--redraft,)

# Dry run by default — writeback edits source, so it shows you the plan
# first. `make copy-apply APPLY=1` writes.
copy-apply:
	$(SLOPCHOP) apply $(if $(APPLY),--apply,)

copy-check:
	$(SLOPCHOP) check

# scripts/audit-signatures.sh is allowed to exit 0 on one kind of red, so the
# branch that decides which kind is tested against a stubbed npm.
script-test:
	node --test scripts/audit-signatures.test.js

copy-report:
	$(SLOPCHOP) report
