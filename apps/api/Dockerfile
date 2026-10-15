# Multi-stage build for apps/api, the control-plane HTTP server (§33,
# Phase 16; containerized §61, Phase 33).
#
# Build context is apps/ (the single Go module root apps/api, apps/tracker
# and apps/worker share — CLAUDE.md's "modular monolith... separate
# binaries inside the same Go module"), NOT this directory:
#
#   docker build -f apps/api/Dockerfile -t flox/api apps/
#
# This file also builds the `migrate` target, a one-shot goose runner for
# apps/api/migrations (docs/migrations.md) — reusing this same build
# rather than a second Dockerfile, since it needs the identical module +
# migrations directory anyway.
#
# The `dev` target (infra/docker-compose.dev.yml) is what actually runs
# locally day to day: air hot-reload over a bind-mounted apps/ tree,
# reachable by other containers as "api" on the compose network — no
# host networking involved.

# ---------- build ----------
FROM golang:1.27.2-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

# Dependency manifests first, so `go mod download` is cached until they change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./api

# ---------- dev (air hot reload, infra/docker-compose.dev.yml) ----------
# Not layered on `builder` on purpose: the whole point of this stage is
# that `apps/` gets bind-mounted over /src at container start (compose's
# `volumes:`), overwriting whatever COPY put there at build time — only
# the go.mod/go.sum download below needs to happen at build time, so a
# source change never requires an image rebuild, just an air restart.
FROM golang:1.27.2-alpine AS dev

ENV CGO_ENABLED=0 GOOS=linux
RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

EXPOSE 8080
# tmp_dir (api/.air.toml) has to exist before air's watcher will create it
# under it — and since apps/ is bind-mounted over /src at container start
# (replacing whatever COPY put there at build time), that can only happen
# here, not via an image-build-time RUN.
ENTRYPOINT ["sh", "-c", "mkdir -p tmp/api && exec air -c api/.air.toml"]

# ---------- migrate (goose, apps/api/migrations) ----------
# Run as: docker run --rm flox/api:migrate "$DATABASE_URL" up
# ("postgres" is baked into ENTRYPOINT as goose's driver arg; DSN and
# command are the two args `docker run`/compose's `command:` supplies —
# DATABASE_URL is a positional arg to `goose`, not read from the
# environment, matching api/migrations/README.md's own command reference
# unchanged). No default CMD on purpose: a DSN is required, so there is
# no meaningful "run with no arguments" default to provide.
#
# goose is compiled here at the version go.mod's `tool` directive pins, not
# at container start: `go tool goose` in the builder image recompiled goose
# and every driver on each run, which took minutes and was OOM-killed under
# a 1 Gi Kubernetes limit (infra/k8s/migrate).
FROM builder AS goose
RUN go build -trimpath -ldflags "-s -w" -o /out/goose github.com/pressly/goose/v3/cmd/goose

FROM alpine:3.21 AS migrate
# ca-certificates: managed Postgres with sslmode=verify-full.
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 -h /app app
COPY --from=goose /out/goose /usr/local/bin/goose
COPY api/migrations /migrations
USER app
ENTRYPOINT ["goose", "-dir", "/migrations", "postgres"]

# ---------- runtime ----------
FROM alpine:3.21 AS api

# ca-certificates: apps/api makes outbound HTTPS calls (domain/DNS
# providers, Facebook/TikTok ad account sync — all behind
# interfaces per CLAUDE.md non-negotiable #11) that need a real CA bundle.
# alpine:3.21's base carries ca-certificates-bundle today; installed
# explicitly so a base-image change cannot remove it.
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 -h /app app

WORKDIR /app
COPY --from=builder /out/api /usr/local/bin/api

USER app
EXPOSE 8080

# API_URL's port digits are all apps/internal/config.Load reads from this
# variable (it decides the bind port, nothing else) — 8080 matches
# EXPOSE/the healthcheck below and .env.example's own default, so this is
# just keeping the container's default consistent with dev rather than a
# behavior the binary requires.
ENV API_URL=http://0.0.0.0:8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["api"]
