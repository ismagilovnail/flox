# FLOX dev workflow — start/stop/restart the whole containerized stack.
#
# Everything (postgres/clickhouse/redis/minio/mailhog/prometheus plus
# api/tracker/worker/web) runs as a container in
# infra/docker-compose.dev.yml — see that file's own header comment.
# api/tracker/worker/web hot-reload (air / `next dev`) over a bind-mounted
# apps/ tree, so editing code needs no rebuild or restart; `make
# <svc>-restart` is only for when a Dockerfile itself changes (new
# dependency, base image bump, etc).
#
# `make check` (fmt/vet/test/lint) still runs against the HOST Go/Node
# toolchains, not in containers — it's validation tooling, not a service,
# and needs no container of its own.
#
# Env vars come from .env if present (see .env.example), else the same
# dev defaults .env.example documents.

SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE := docker compose -f infra/docker-compose.dev.yml
GO_DIR  := apps
WEB_DIR := apps/web

ifneq (,$(wildcard .env))
include .env
export
endif

DATABASE_URL ?= postgres://flox:flox@localhost:5432/flox?sslmode=disable

.PHONY: help \
        up down restart status build \
        logs infra-logs api-logs tracker-logs worker-logs web-logs \
        api-restart tracker-restart worker-restart web-restart \
        backup-now backup-drill backup-list \
        migrate-up migrate-down migrate-status dev-reset dev-fresh dev-status fixtures reconcile \
        test-stack-up test-stack-down test-stack-restart test-stack-logs \
        fmt vet test lint check clean devpanel audit-tracker

help: ## Show this help
	@echo "FLOX — make targets"
	@echo ""
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Whole stack (infra/docker-compose.dev.yml) --------------------------

up: ## Build (if needed) and start the whole dev stack in the background
	$(COMPOSE) up -d --build
	@echo ""
	@echo "web:     http://localhost:3000"
	@echo "api:     http://localhost:8080"
	@echo "tracker: http://localhost:8081"
	@echo "worker:  http://localhost:8082"
	@echo "mailhog: http://localhost:8025"
	@echo "logs: 'make logs' or 'make <svc>-logs'"

down: ## Stop the whole dev stack (data volumes are kept)
	$(COMPOSE) down

restart: down up ## Restart the whole dev stack

status: ## Show container status for the whole dev stack
	$(COMPOSE) ps

build: ## Rebuild every service's image without starting anything
	$(COMPOSE) build

# --- Logs ------------------------------------------------------------------

logs: ## Tail logs for every service
	$(COMPOSE) logs -f

infra-logs: ## Tail postgres/clickhouse/redis/minio/mailhog/prometheus logs
	$(COMPOSE) logs -f postgres clickhouse redis minio mailhog prometheus

api-logs: ## Tail apps/api logs
	$(COMPOSE) logs -f api

tracker-logs: ## Tail apps/tracker logs
	$(COMPOSE) logs -f tracker

worker-logs: ## Tail apps/worker logs
	$(COMPOSE) logs -f worker

web-logs: ## Tail apps/web logs
	$(COMPOSE) logs -f web

# --- Per-service rebuild+restart (Dockerfile changes only — hot reload -----
# handles code changes on its own, see the header comment above)

api-restart: ## Rebuild and restart just apps/api's container
	$(COMPOSE) up -d --build api

tracker-restart: ## Rebuild and restart just apps/tracker's container
	$(COMPOSE) up -d --build tracker

worker-restart: ## Rebuild and restart just apps/worker's container
	$(COMPOSE) up -d --build worker

web-restart: ## Rebuild and restart just apps/web's container
	$(COMPOSE) up -d --build web

# --- Postgres migrations (goose, apps/api/migrations) ----------------------
# infra/docker-compose.dev.yml's own `migrate` one-shot service already
# applies every pending migration on `make up`. These targets are for
# re-running them by hand afterwards (e.g. after `make migrate-down`)
# without a full `make restart` — run from the host against Postgres's
# host-published port (localhost:5432), so they need no container of
# their own.

migrate-up: ## Apply all pending Postgres migrations
	cd $(GO_DIR) && go tool goose -dir api/migrations postgres "$(DATABASE_URL)" up

migrate-down: ## Roll back one Postgres migration
	cd $(GO_DIR) && go tool goose -dir api/migrations postgres "$(DATABASE_URL)" down

migrate-status: ## Show Postgres migration status
	cd $(GO_DIR) && go tool goose -dir api/migrations postgres "$(DATABASE_URL)" status

# --- Dev data: reset, fixtures, status ------------------------------------
# All three go through infra/dev-data.sh, which prints each step and a clear
# result, and touches only the services of infra/docker-compose.dev.yml.
#
#   make dev-fresh CONFIRM=yes   wipe everything and fill in the demo accounts — the usual one
#   make dev-reset CONFIRM=yes   wipe only (Postgres re-migrated, ClickHouse/Redis/MinIO emptied)
#   make fixtures                demo accounts + demo campaign into an empty database
#   make dev-status              what is in the dev stores right now

dev-reset: ## DESTRUCTIVE: wipe all dev data (make dev-reset CONFIRM=yes)
	@if [ "$(CONFIRM)" != "yes" ]; then \
		echo "dev-reset удаляет ВСЕ dev-данные: Postgres, ClickHouse, Redis, MinIO."; \
		echo "Запуск: make dev-reset CONFIRM=yes   (или make dev-fresh CONFIRM=yes — сброс + фикстуры)"; \
		exit 1; fi
	@infra/dev-data.sh reset
	@infra/dev-data.sh wait
	@echo ""
	@echo "Данные удалены. База пустая — заполнить: make fixtures"

fixtures: ## Create dev accounts + demo data in an empty database (apps/cmd/devfixtures)
	@cd $(GO_DIR) && go run ./cmd/devfixtures

dev-fresh: ## DESTRUCTIVE: wipe all dev data and create the demo accounts (make dev-fresh CONFIRM=yes)
	@if [ "$(CONFIRM)" != "yes" ]; then \
		echo "dev-fresh удаляет ВСЕ dev-данные и заново создаёт демо-аккаунты."; \
		echo "Запуск: make dev-fresh CONFIRM=yes"; \
		exit 1; fi
	@infra/dev-data.sh reset
	@infra/dev-data.sh wait
	@cd $(GO_DIR) && go run ./cmd/devfixtures
	@echo ""
	@infra/dev-data.sh status

dev-status: ## Show what is in the dev stores (services, Postgres, ClickHouse, Redis, MinIO)
	@infra/dev-data.sh status

reconcile: ## Compare+repair ClickHouse conversions vs Postgres: make reconcile FROM=YYYY-MM-DD TO=YYYY-MM-DD [REPORT_ONLY=1]
	@if [ -z "$(FROM)" ] || [ -z "$(TO)" ]; then echo "usage: make reconcile FROM=YYYY-MM-DD TO=YYYY-MM-DD [REPORT_ONLY=1]"; exit 1; fi
	cd $(GO_DIR) && go run ./cmd/reconcile -from $(FROM) -to $(TO) $(if $(REPORT_ONLY),-report-only,)

# --- Backups (internal/backup, docs/backup.md) ----------------------------
# Run inside the backup sidecar: it has wal-g, pg_ctl and the data directory.

backup-now: ## Back up Postgres, ClickHouse and the files bucket now
	$(COMPOSE) exec backup flox-backup now

backup-drill: ## Restore every store into a scratch copy and check it
	$(COMPOSE) exec backup flox-backup drill

backup-list: ## What the backup bucket holds, WAL archiving state, last drill
	$(COMPOSE) exec backup flox-backup list

# --- Full containerized stack (infra/docker-compose.test.yml) --------------
# Production-target build of all 8 §61 services (no hot reload) — its own
# `flox-test` compose project, isolated from docker-compose.dev.yml (see
# infra/README.md). Not part of the day-to-day dev loop above; a
# pre-deploy sanity check that the deployment images actually build and
# boot together.

test-stack-up: ## Build and start the full production-target stack
	docker compose -f infra/docker-compose.test.yml up --build -d

test-stack-down: ## Stop the full production-target stack (drops its volumes)
	docker compose -f infra/docker-compose.test.yml down -v

test-stack-restart: test-stack-down test-stack-up ## Restart the full production-target stack

test-stack-logs: ## Tail the full production-target stack's logs
	docker compose -f infra/docker-compose.test.yml logs -f

# --- Validation (CLAUDE.md's per-phase checklist) ---------------------------
# Runs against the HOST Go/Node toolchains, not inside containers.

fmt: ## gofmt -l (Go) — lists unformatted files
	cd $(GO_DIR) && gofmt -l .

vet: ## go vet ./...
	cd $(GO_DIR) && go vet ./...

test: ## go test -count=1 ./... + vitest run (apps/web)
	cd $(GO_DIR) && go test -count=1 ./...
	cd $(WEB_DIR) && npm run test

lint: ## eslint (apps/web) + tsc --noEmit (apps/web)
	cd $(WEB_DIR) && npm run lint
	cd $(WEB_DIR) && npx tsc --noEmit

check: fmt vet test lint ## Run the full validation suite (fmt+vet+test+lint)

# --- Dev panel (tools/devpanel, its own Go module) --------------------------

devpanel: ## Local dev panel — dashboard + task tracker: http://127.0.0.1:8099 (SQLite in tools/devpanel/data)
	cd tools/devpanel && go run .

audit-tracker: devpanel ## Old name of devpanel

# --- Cleanup -----------------------------------------------------------------

clean: down ## Stop everything (data volumes are kept — see 'docker volume rm' to drop them)
