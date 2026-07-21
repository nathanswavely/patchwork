# Stage 1: Build Svelte frontend
# Pinned to BUILDPLATFORM: web/dist is static JS/CSS and is byte-identical
# across target arches, so building it once natively avoids a redundant
# ~5min QEMU-emulated npm run build in the arm64 leg of multi-arch builds.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /app/web
# .npmrc carries ignore-scripts=true and must be copied with the manifests, not
# with the source below — npm reads it at `npm ci` time, and without it here the
# image build would run dependency install scripts even though CI does not.
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
# go-sqlite3 needs CGO; link statically so the binary runs on distroless/static.
FROM golang:1.25-alpine AS backend
RUN apk add --no-cache gcc musl-dev git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
# Passed in by CI (the tag on a release, else the full commit SHA). The old
# `git describe` here depended on the build context carrying a .git with tags
# fetched — CI's shallow checkout has neither, so it silently produced a bare
# abbreviated SHA and never a version. Local builds still default to "dev".
ARG VERSION=dev
RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w -linkmode external -extldflags \"-static\" -X github.com/patchwork-toolkit/patchwork/internal/handler.Version=${VERSION}" \
    -o /patchwork ./cmd/patchwork
# Staged so the runtime image can COPY an empty, correctly-owned /data
# (distroless has no shell to mkdir/chown with).
RUN mkdir -p /data

# Stage 3: Distroless runtime
# Config is NOT baked into the image — mount your patchwork.yaml at
# /patchwork.yaml (docker-compose.yaml does this for you).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /patchwork /patchwork
# /data is the durable home for all state (SQLite db, governance repos).
# It must be owned by nonroot (uid 65532) in the IMAGE: a fresh named
# volume mounted here copies this ownership on first use, so the app can
# write without a manual chown. WORKDIR /data makes relative paths in
# patchwork.yaml (the example's "data/patchwork.db") resolve inside the
# volume instead of the ephemeral container layer.
COPY --from=backend --chown=65532:65532 /data /data
WORKDIR /data
EXPOSE 8080
# Distroless has no shell and no curl, so the usual `CMD curl -f .../health`
# is unrunnable. Exec form invokes the app's own binary in probe mode: it
# reads the same config the server does (so it cannot probe the wrong port)
# and exits non-zero when /api/v1/health is unreachable or returns 503.
# start-period covers migrations on a large database; the app is not
# considered failed during it.
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["/patchwork", "-config", "/patchwork.yaml", "-healthcheck"]
USER nonroot:nonroot
ENTRYPOINT ["/patchwork"]
CMD ["-config", "/patchwork.yaml"]
