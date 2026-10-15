# Postgres with WAL-G, and the backup sidecar built on it (internal/backup,
# docs/backup.md). Build context is apps/, like the other Go images:
#
#   docker build -f apps/cmd/backup/Dockerfile --target postgres -t flox/postgres apps/
#   docker build -f apps/cmd/backup/Dockerfile --target backup   -t flox/backup   apps/
#   docker build -f apps/cmd/backup/Dockerfile --target backup-remote -t flox/backup-remote apps/
#
# backup-remote is ClickHouse and the files bucket only (BACKUP_POSTGRES=off):
# for Kubernetes with a managed Postgres (infra/k8s/components/backup).
#
# Both targets sit on postgres:16-alpine, the image the database already
# runs: the same data directory format, the same pg_ctl for the drill, the
# same uid (70) that owns the data files the sidecar reads.

# ---------- wal-g ----------
# Built from its tag: v3 tags do not carry a /v3 module path, so
# `go install ...@v3` cannot fetch them. CGO against musl, for alpine.
FROM golang:1.27.2-alpine AS walg
RUN apk add --no-cache git build-base
ARG WALG_VERSION=v3.0.8
RUN git clone --depth 1 --branch ${WALG_VERSION} https://github.com/wal-g/wal-g /walg
WORKDIR /walg
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o /out/wal-g ./main/pg

# ---------- flox-backup ----------
FROM golang:1.27.2-alpine AS builder
ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags "-s -w" -o /out/flox-backup ./cmd/backup

# ---------- postgres (archive_command = wal-g wal-push) ----------
FROM postgres:16-alpine AS postgres
COPY --from=walg /out/wal-g /usr/local/bin/wal-g

# ---------- backup sidecar ----------
FROM postgres AS backup
COPY --from=builder /out/flox-backup /usr/local/bin/flox-backup
RUN mkdir -p /var/lib/flox-drill && chown postgres:postgres /var/lib/flox-drill
USER postgres
EXPOSE 8083
HEALTHCHECK --interval=30s --timeout=5s CMD wget -q --spider http://127.0.0.1:8083/health || exit 1
ENTRYPOINT ["flox-backup"]
CMD ["serve"]

# ---------- ClickHouse + files only (BACKUP_POSTGRES=off) ----------
# No wal-g, no Postgres: a managed Postgres is its provider's PITR, and
# WAL-G cannot reach its data directory anyway.
FROM alpine:3.21 AS backup-remote
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 -h /app app
COPY --from=builder /out/flox-backup /usr/local/bin/flox-backup
ENV BACKUP_POSTGRES=off
USER app
EXPOSE 8083
ENTRYPOINT ["flox-backup"]
CMD ["serve"]
