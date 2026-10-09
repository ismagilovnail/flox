# Multi-stage build for apps/tracker, the hot-path click/redirect service
# (§41, Phase 21; containerized §61, Phase 33).
#
# Build context is apps/, same reasoning as apps/api/Dockerfile — see that
# file's comment:
#
#   docker build -f apps/tracker/Dockerfile -t flox/tracker apps/
#
# The `dev` target (infra/docker-compose.dev.yml) is what actually runs
# locally day to day: air hot-reload over a bind-mounted apps/ tree,
# reachable by other containers as "tracker" on the compose network — no
# host networking involved.

# ---------- build ----------
FROM golang:1.27-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tracker ./tracker

# ---------- dev (air hot reload, infra/docker-compose.dev.yml) ----------
# Not layered on `builder` on purpose: the whole point of this stage is
# that `apps/` gets bind-mounted over /src at container start (compose's
# `volumes:`), overwriting whatever COPY put there at build time — only
# the go.mod/go.sum download below needs to happen at build time, so a
# source change never requires an image rebuild, just an air restart.
FROM golang:1.27-alpine AS dev

ENV CGO_ENABLED=0 GOOS=linux
RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

EXPOSE 8081
# tmp_dir (tracker/.air.toml) has to exist before air's watcher will
# create it under it — and since apps/ is bind-mounted over /src at
# container start (replacing whatever COPY put there at build time),
# that can only happen here, not via an image-build-time RUN.
ENTRYPOINT ["sh", "-c", "mkdir -p tmp/tracker && exec air -c tracker/.air.toml"]

# ---------- runtime ----------
FROM alpine:3.21 AS tracker

# ca-certificates: the redirect path makes no outbound calls (CLAUDE.md
# #9), but the tracker does reach its stores and services over TLS in a
# real deployment — Postgres with sslmode=verify-full, rediss://, an
# https:// ClickHouse, S3 for landing assets, Sentry. alpine:3.21 happens
# to ship ca-certificates-bundle in its base image; installed explicitly
# so a base-image change cannot silently break every TLS connection
# (tech audit DO4).
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 -h /app app

WORKDIR /app
COPY --from=builder /out/tracker /usr/local/bin/tracker

USER app
EXPOSE 8081

# TRACKER_URL's port digits are all apps/internal/config.LoadTracker
# reads from this variable — see apps/api/Dockerfile's identical note.
ENV TRACKER_URL=http://0.0.0.0:8081

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8081/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["tracker"]
