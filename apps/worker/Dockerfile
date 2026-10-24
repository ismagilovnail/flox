# Multi-stage build for apps/worker, the async event/postback processor
# (Phase 24+; containerized §61, Phase 33).
#
# Build context is apps/, same reasoning as apps/api/Dockerfile — see that
# file's comment:
#
#   docker build -f apps/worker/Dockerfile -t flox/worker apps/
#
# The `dev` target (infra/docker-compose.dev.yml) is what actually runs
# locally day to day: air hot-reload over a bind-mounted apps/ tree,
# reachable by other containers as "worker" on the compose network — no
# host networking involved.

# ---------- build ----------
FROM golang:1.27.2-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/worker ./worker

# ---------- dev (air hot reload, infra/docker-compose.dev.yml) ----------
# Not layered on `builder` on purpose: the whole point of this stage is
# that `apps/` gets bind-mounted over /src at container start (compose's
# `volumes:`), overwriting whatever COPY put there at build time — only
# the go.mod/go.sum download below needs to happen at build time, so a
# source change never requires an image rebuild, just an air restart.
FROM golang:1.27.2-alpine AS dev

ENV CGO_ENABLED=0 GOOS=linux
RUN go install github.com/air-verse/air@v1.61.7
# ffmpeg/ffprobe: Content Gallery video transcoding (docs/content-gallery.md
# "Transcoding"), the same package as the runtime image.
RUN apk add --no-cache ffmpeg

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

EXPOSE 8082
# tmp_dir (worker/.air.toml) has to exist before air's watcher will
# create it under it — and since apps/ is bind-mounted over /src at
# container start (replacing whatever COPY put there at build time),
# that can only happen here, not via an image-build-time RUN.
ENTRYPOINT ["sh", "-c", "mkdir -p tmp/worker && exec air -c worker/.air.toml"]

# ---------- runtime ----------
FROM alpine:3.21 AS worker

# ca-certificates: apps/worker's own outbound HTTPS calls are real and
# frequent — outgoing postback delivery to a network operator's
# postback_url (§46, urlsafety-guarded but still a real TLS connection)
# Facebook/TikTok ad spend sync (§74/§27-COST) and the ECB FX rate sync
# (internal/fx) all need a working CA bundle, unlike apps/tracker.
# ffmpeg (with ffprobe): Content Gallery video transcoding — MOV/AVI/MKV…
# to MP4 (docs/content-gallery.md "Transcoding").
RUN apk add --no-cache ca-certificates ffmpeg && adduser -D -u 10001 -h /app app

WORKDIR /app
COPY --from=builder /out/worker /usr/local/bin/worker

USER app
EXPOSE 8082

# WORKER_URL's port digits are all apps/internal/config.LoadWorker reads
# from this variable — see apps/api/Dockerfile's identical note.
ENV WORKER_URL=http://0.0.0.0:8082

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8082/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["worker"]
