# Multi-stage build for apps/tracker, the hot-path click/redirect service
# (§41, Phase 21; containerized §61, Phase 33).
#
# Build context is apps/, same reasoning as apps/api/Dockerfile — see that
# file's comment:
#
#   docker build -f apps/tracker/Dockerfile -t flox/tracker apps/

# ---------- build ----------
FROM golang:1.27-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tracker ./tracker

# ---------- runtime ----------
FROM alpine:3.21 AS tracker

# No ca-certificates here on purpose: as currently wired
# (classifier.New(nil, nil, nil) — no real geo/ASN/bot vendor yet, §74),
# apps/tracker makes no outbound HTTPS calls at all on its request path —
# CLAUDE.md non-negotiable #9 keeps the redirect hot path free of exactly
# that kind of dependency. Revisit this line the day a real geo/ASN/bot
# provider (behind CLAUDE.md non-negotiable #11's interfaces) is actually
# wired into apps/tracker and needs one.
RUN adduser -D -u 10001 -h /app app

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
