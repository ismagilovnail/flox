#!/usr/bin/env bash
# Dev data commands behind `make dev-reset`, `make dev-fresh` and
# `make dev-status`. Talks only to the services of infra/docker-compose.dev.yml
# (compose project "infra") and to Postgres on localhost:5432 for migrations —
# never to another database on this machine.
#
#   infra/dev-data.sh reset    wipe Postgres, ClickHouse, Redis, MinIO; re-migrate
#   infra/dev-data.sh wait     wait until api, tracker and worker answer /health
#   infra/dev-data.sh status   what is in the dev stores right now
#
# Every step prints what it does and whether it worked; on failure it says
# which step failed and exits non-zero.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/infra/docker-compose.dev.yml")
DATABASE_URL="${DATABASE_URL:-postgres://flox:flox@localhost:5432/flox?sslmode=disable}"
LOG="$(mktemp -t flox-dev-data.XXXXXX)"
trap 'rm -f "$LOG"' EXIT

if [ -t 1 ]; then OK=$'\e[32m✓\e[0m' BAD=$'\e[31m✗\e[0m' DIM=$'\e[2m' RST=$'\e[0m'; else OK="✓" BAD="✗" DIM="" RST=""; fi

CURRENT=""
step() { CURRENT="$1"; printf '  %s… ' "$1"; }
done_() { printf '%s %s\n' "$OK" "${1:-}"; }
fail() {
	printf '%s\n\nОшибка на шаге «%s».\n' "$BAD" "$CURRENT" >&2
	if [ -s "$LOG" ]; then printf 'Вывод команды:\n%s\n' "$(tail -n 20 "$LOG")" >&2; fi
	exit 1
}

psql_() { "${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U flox -d flox -Atc "$1"; }
ch_() { "${COMPOSE[@]}" exec -T clickhouse clickhouse-client -q "$1"; }

require_running() {
	local missing=()
	for svc in postgres clickhouse redis minio; do
		if [ -z "$("${COMPOSE[@]}" ps --status running -q "$svc" 2>/dev/null)" ]; then missing+=("$svc"); fi
	done
	if [ ${#missing[@]} -gt 0 ]; then
		printf '%s Не запущены: %s. Сначала: make up\n' "$BAD" "${missing[*]}" >&2
		exit 1
	fi
}

cmd_reset() {
	require_running
	echo "Сброс dev-данных (Postgres, ClickHouse, Redis, MinIO):"

	step "Останавливаю api, tracker, worker"
	"${COMPOSE[@]}" stop api tracker worker >"$LOG" 2>&1 || fail
	done_

	step "Postgres: удаляю схему public"
	psql_ 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;' >"$LOG" 2>&1 || fail
	done_

	step "Postgres: применяю миграции"
	(cd "$ROOT/apps" && go tool goose -dir api/migrations postgres "$DATABASE_URL" up) >"$LOG" 2>&1 || fail
	done_ "версия схемы $(psql_ 'SELECT max(version_id) FROM goose_db_version')"

	step "ClickHouse: пересоздаю базу flox"
	{ ch_ 'DROP DATABASE IF EXISTS flox SYNC' && ch_ 'CREATE DATABASE flox'; } >"$LOG" 2>&1 || fail
	done_ "таблицы создадут api и worker при запуске"

	step "Redis: очищаю"
	"${COMPOSE[@]}" exec -T redis redis-cli FLUSHALL >"$LOG" 2>&1 || fail
	done_

	step "MinIO: удаляю файлы из бакета flox"
	"${COMPOSE[@]}" exec -T minio sh -c 'mc alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null && (mc rm --recursive --force local/flox >/dev/null 2>&1 || true)' >"$LOG" 2>&1 || fail
	done_

	step "Запускаю api, tracker, worker"
	"${COMPOSE[@]}" start api tracker worker >"$LOG" 2>&1 || fail
	done_
}

cmd_wait() {
	local timeout="${WAIT_TIMEOUT:-180}" start=$SECONDS
	step "Жду готовности сервисов (до ${timeout} с)"
	for port in 8080 8081 8082; do
		until curl -sf "http://localhost:$port/health" >/dev/null 2>&1; do
			if (( SECONDS - start > timeout )); then
				printf '%s\n\nСервис на порту %s не ответил за %s с. Логи: make api-logs / tracker-logs / worker-logs\n' "$BAD" "$port" "$timeout" >&2
				exit 1
			fi
			sleep 2
		done
	done
	# ClickHouse tables appear once api/worker have run their migration.
	until [ "$(ch_ "SELECT count() FROM system.tables WHERE database = 'flox'" 2>/dev/null || echo 0)" -gt 0 ]; do
		if (( SECONDS - start > timeout )); then
			printf '%s\n\nВ ClickHouse так и не появились таблицы. Логи: make worker-logs\n' "$BAD" >&2
			exit 1
		fi
		sleep 2
	done
	done_ "готово за $(( SECONDS - start )) с"
}

health() { curl -sf "http://localhost:$1/health" >/dev/null 2>&1 && echo "работает" || echo "НЕ ОТВЕЧАЕТ"; }

cmd_status() {
	require_running
	local ver orgs users list campaigns offers domains rules notes ch_tables events convs keys files
	ver=$(psql_ 'SELECT max(version_id) FROM goose_db_version' 2>/dev/null || echo "нет")
	orgs=$(psql_ 'SELECT count(*) FROM organizations')
	users=$(psql_ 'SELECT count(*) FROM users')
	# The oldest five (fixtures come first), then how many more.
	list=$(psql_ "SELECT string_agg(name, ', ' ORDER BY created_at, name) || CASE WHEN (SELECT count(*) FROM organizations) > 5 THEN ' и ещё ' || ((SELECT count(*) FROM organizations) - 5) ELSE '' END FROM (SELECT name, created_at FROM organizations ORDER BY created_at, name LIMIT 5) o")
	campaigns=$(psql_ 'SELECT count(*) FROM campaigns')
	offers=$(psql_ 'SELECT count(*) FROM offers')
	domains=$(psql_ 'SELECT count(*) FROM domains')
	rules=$(psql_ 'SELECT count(*) FROM alert_rules' 2>/dev/null || echo "—")
	notes=$(psql_ 'SELECT count(*) FROM notifications' 2>/dev/null || echo "—")
	ch_tables=$(ch_ "SELECT count() FROM system.tables WHERE database = 'flox'" 2>/dev/null || echo 0)
	events=$(ch_ 'SELECT count() FROM flox.tracking_events' 2>/dev/null || echo "—")
	convs=$(ch_ 'SELECT count() FROM flox.conversion_events' 2>/dev/null || echo "—")
	keys=$("${COMPOSE[@]}" exec -T redis redis-cli DBSIZE 2>/dev/null | tr -d '\r')
	files=$("${COMPOSE[@]}" exec -T minio sh -c 'mc alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null && mc ls --recursive local/flox 2>/dev/null | wc -l' 2>/dev/null | tr -d ' \r')

	cat <<EOF
Состояние dev-стенда:

  Сервисы    api $(health 8080) · tracker $(health 8081) · worker $(health 8082)
  Postgres   схема v${ver}; организаций ${orgs}, пользователей ${users}
             кампаний ${campaigns}, офферов ${offers}, доменов ${domains}
             правил оповещений ${rules}, уведомлений ${notes}
             ${DIM}организации: ${list:-нет}${RST}
  ClickHouse таблиц ${ch_tables}; событий трекинга ${events}, конверсий ${convs}
  Redis      ключей ${keys:-?}
  MinIO      файлов ${files:-?}
EOF
	if [ "$orgs" = "0" ]; then echo; echo "База пустая. Заполнить: make fixtures"; fi
}

case "${1:-}" in
reset) cmd_reset ;;
wait) cmd_wait ;;
status) cmd_status ;;
*)
	echo "usage: $0 reset|wait|status" >&2
	exit 2
	;;
esac
