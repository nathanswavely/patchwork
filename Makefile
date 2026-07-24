.PHONY: build run dev seed seed-force export import test test-e2e smoke-recreate

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
