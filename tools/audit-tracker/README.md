# audit-tracker

Локальная доска по находкам аудита `docs/audit-product-2026-10-15.md`: что
сделано, что нет, открытые баги, риски и карта реализованных функций.

```
make audit-tracker            # из корня репозитория
# или
cd tools/audit-tracker && go run .
```

Открыть: http://127.0.0.1:8099

- База — SQLite, `tools/audit-tracker/data/tracker.db` (в `.gitignore`).
  Удалите файл, чтобы начать заново с исходного списка.
- При пустой базе загружается `seed/items.json` и `seed/features.json`.
  `go run . -reseed` добавляет недостающие строки из сида и ничего не
  перезаписывает.
- Каждая смена статуса, приоритета или названия пишется в историю задачи
  (таблица `item_changes`), видно в карточке задачи.
- Флаги: `-addr` (по умолчанию `127.0.0.1:8099`), `-db`.

Входа нет, поэтому сервер слушает только loopback и отклоняет запросы с
чужим `Host` и cross-origin записи. Не выставляйте его наружу.

Отдельный Go-модуль: зависимость SQLite (`modernc.org/sqlite`, без cgo) не
попадает в `apps/go.mod`.

API: `GET /api/items`, `POST /api/items` (`code` необязателен: `ABC-12`,
иначе `NEW-n`), `PATCH /api/items/{id}`
(`status`, `priority`, `severity`, `notes`, `title`, `fix`, `area`),
`DELETE /api/items/{id}`, `GET /api/items/{id}/changes`,
`GET /api/features`, `PATCH /api/features/{id}` (`status`).
