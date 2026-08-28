# Multi-stage build for apps/worker, the async event/postback processor
# (Phase 24+; containerized §61, Phase 33).
#
# Build context is apps/, same reasoning as apps/api/Dockerfile — see that
# file's comment:
#
#   docker build -f apps/worker/Dockerfile -t flox/worker apps/

# ---------- build ----------
FROM golang:1.27-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/worker ./worker

# ---------- runtime ----------
FROM alpine:3.21 AS worker

# ca-certificates: apps/worker's own outbound HTTPS calls are real and
# frequent — outgoing postback delivery to a network operator's
# postback_url (§46, urlsafety-guarded but still a real TLS connection)
# and Facebook/TikTok ad spend sync (§74/§27-COST) both need a working CA
# bundle, unlike apps/tracker.
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 -h /app app

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
