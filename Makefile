.PHONY: build run dev seed seed-force export import test test-e2e smoke-recreate

build:
	go build -o patchwork ./cmd/patchwork/

run: build
	./patchwork

dev: build
	@echo "Starting Go backend (server.port from patchwork.yaml; the Vite proxy expects 8090) and Vite dev server on :5173..."
	@trap 'kill 0' EXIT; \
	./patchwork & \
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
