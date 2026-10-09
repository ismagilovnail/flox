# Postgres with WAL-G, and the backup sidecar built on it (internal/backup,
# docs/backup.md). Build context is apps/, like the other Go images:
#
#   docker build -f apps/cmd/backup/Dockerfile --target postgres -t flox/postgres apps/
#   docker build -f apps/cmd/backup/Dockerfile --target backup   -t flox/backup   apps/
#
# Both targets sit on postgres:16-alpine, the image the database already
# runs: the same data directory format, the same pg_ctl for the drill, the
# same uid (70) that owns the data files the sidecar reads.

# ---------- wal-g ----------
# Built from its tag: v3 tags do not carry a /v3 module path, so
# `go install ...@v3` cannot fetch them. CGO against musl, for alpine.
FROM golang:1.27-alpine AS walg
RUN apk add --no-cache git build-base
ARG WALG_VERSION=v3.0.8
RUN git clone --depth 1 --branch ${WALG_VERSION} https://github.com/wal-g/wal-g /walg
WORKDIR /walg
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o /out/wal-g ./main/pg

# ---------- flox-backup ----------
FROM golang:1.27-alpine AS builder
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
