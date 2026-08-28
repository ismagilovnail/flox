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

# ---------- build ----------
FROM golang:1.27-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

# Dependency manifests first, so `go mod download` is cached until they change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./api

# ---------- migrate (goose, apps/api/migrations) ----------
# Run as: docker run --rm flox/api:migrate "$DATABASE_URL" up
# ("postgres" is baked into ENTRYPOINT as goose's driver arg; DSN and
# command are the two args `docker run`/compose's `command:` supplies —
# DATABASE_URL is a positional arg to `goose`, not read from the
# environment, matching api/migrations/README.md's own command reference
# unchanged). No default CMD on purpose: a DSN is required, so there is
# no meaningful "run with no arguments" default to provide.
FROM builder AS migrate
WORKDIR /src
ENTRYPOINT ["go", "tool", "goose", "-dir", "api/migrations", "postgres"]

# ---------- runtime ----------
FROM alpine:3.21 AS api

# ca-certificates: apps/api makes outbound HTTPS calls (FX rate provider,
# domain/DNS providers, Facebook/TikTok ad account sync — all behind
# interfaces per CLAUDE.md non-negotiable #11) that need a real CA bundle;
# alpine's base image does not carry one, unlike the Go builder stage.
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
