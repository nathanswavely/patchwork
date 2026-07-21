#!/usr/bin/env bash
# Recreate-survival smoke test.
#
# Proves that a containerized instance running the untouched example-style
# config (RELATIVE database path) keeps its data across
# `docker compose up --force-recreate` — the operation every image update
# performs. This is the failure mode that destroyed a production database
# on 2026-07-19: the database landed in the container's ephemeral layer
# while the /data volume stayed empty.
#
# Usage: scripts/smoke-recreate.sh   (from anywhere; needs docker + curl)
set -euo pipefail
cd "$(dirname "$0")/.."

PROJECT=patchwork-smoke
COMPOSE="docker compose -p $PROJECT -f scripts/smoke/compose.yaml"
BASE=http://127.0.0.1:18080
MARKER="Recreate Survivor"

fail() { echo "FAIL: $*" >&2; $COMPOSE logs patchwork >&2 || true; exit 1; }

cleanup() { $COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
cleanup

wait_healthy() {
  for _ in $(seq 1 60); do
    if curl -fsS "$BASE/api/v1/health" >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  fail "app never became healthy at $BASE"
}

echo "==> Building image and starting instance"
$COMPOSE up -d --build
wait_healthy

# The image's own HEALTHCHECK runs `patchwork -healthcheck` inside the
# distroless container. Assert Docker sees it pass — a wrong config path or
# a renamed flag would otherwise leave every deployment stuck "unhealthy".
echo "==> Checking the image's built-in HEALTHCHECK"
CID=$($COMPOSE ps -q patchwork)
for _ in $(seq 1 30); do
  STATUS=$(docker inspect --format '{{.State.Health.Status}}' "$CID" 2>/dev/null || echo none)
  [ "$STATUS" = healthy ] && break
  sleep 3
done
[ "$STATUS" = healthy ] || fail "container HEALTHCHECK never reported healthy (status: $STATUS)"

echo "==> Creating account (magic link prints to the log without SMTP)"
curl -fsS -X POST "$BASE/api/v1/auth/magic-link" \
  -H 'Content-Type: application/json' -H 'X-Patchwork-Request: true' \
  -d '{"email":"smoke@example.com"}' >/dev/null
sleep 1
TOKEN=$($COMPOSE logs patchwork 2>&1 | grep -oE 'auth/verify/[A-Za-z0-9_-]+' | tail -1 | awk -F/ '{print $3}')
[ -n "$TOKEN" ] || fail "no magic link found in app log"

# On a new email, verify does not sign you in — it hands back a signup token
# and asks for a username (docs/adr/013). The session cookie comes from
# completing signup with that token.
SIGNUP=$(curl -fsS -H 'Accept: application/json' "$BASE/api/v1/auth/verify/$TOKEN" \
  | grep -oE '"signup_token":"[a-f0-9]+"' | cut -d'"' -f4)
[ -n "$SIGNUP" ] || fail "verify did not return a signup token"

COOKIE=$(curl -fsS -D - -o /dev/null -X POST "$BASE/api/v1/auth/signup" \
  -H 'Content-Type: application/json' -H 'X-Patchwork-Request: true' \
  -d "{\"token\":\"$SIGNUP\",\"username\":\"smoketest\",\"display_name\":\"Smoke Test\"}" \
  | grep -i '^set-cookie:' | head -1 | sed 's/^[Ss]et-[Cc]ookie: *//' | cut -d';' -f1 | tr -d '\r')
[ -n "$COOKIE" ] || fail "signup did not return a session cookie"

echo "==> Creating a patch through the API"
curl -fsS -X POST "$BASE/api/v1/nodes" \
  -H "Cookie: $COOKIE" -H 'Content-Type: application/json' -H 'X-Patchwork-Request: true' \
  -d "{\"name\":\"$MARKER\"}" >/dev/null
curl -fsS "$BASE/api/v1/nodes" | grep -q "$MARKER" || fail "created patch not visible"

echo "==> Confirming the database actually lives in the volume"
docker run --rm -v ${PROJECT}_smoke_data:/v alpine find /v -name patchwork.db \
  | grep -q patchwork.db || fail "patchwork.db is NOT in the data volume — writes are going to the ephemeral layer"

echo "==> Force-recreating the container (what every image update does)"
$COMPOSE up -d --force-recreate
wait_healthy

curl -fsS "$BASE/api/v1/nodes" | grep -q "$MARKER" \
  || fail "DATA LOST: patch \"$MARKER\" missing after recreate"

echo "PASS: data survived container recreation"
