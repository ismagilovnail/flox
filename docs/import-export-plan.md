# Импорт/экспорт: Keitaro, Binom, собственный формат — анализ и план (INT-2)

Статус: **план, ждёт решения владельца** (2026-10-16). Кода нет.

Поля Keitaro сверены с `admin-api.docs.keitaro.io/openapi.json` частично. (?) = не подтверждено, сверить с живым трекером.

## 1. Что уже есть

| Файл / пакет | Что делает | Пригодно |
|---|---|---|
| `apps/internal/cost/csvimport.go`, `POST /campaigns/{id}/cost-entries/import` | CSV расходов, всё-или-ничего, ошибки по строкам, BOM, `;` | образец для импорта конверсий |
| `apps/internal/orgexport/*` | GDPR-выгрузка: ZIP с JSONL таблиц Postgres (без секретов) и CSV событий ClickHouse, фоновая задача в worker | не формат переноса (сырые таблицы, внутренние id); пригодна схема фоновой задачи |
| `apps/internal/conversion/replay.go`, `POST /postback-logs/replay-incoming` | одна конверсия вручную через `conversion.Service` (dedup, запрет возврата в HOLD, курс на дату) | импорт конверсий = пакетная версия |
| `apps/internal/mappingreplay`, `eventmapping` | статусы сети → статусы FLOX, повторная запись после появления маппинга | статусы Keitaro lead/sale/rejected/rebill |
| `Duplicate` в campaign/streamset/offer/network/source | копирование через сервисы | образец «через сервис с валидацией» |
| `components/ui/data-table.tsx`, `features/analytics/report-table.tsx` | CSV отчёта в браузере (только видимые строки) | серверного CSV нет |
| `landing/handler.go` | загрузка лендинга ZIP | для лендингов с файлами |

Импорта/экспорта кампаний, потоков, офферов и источников нет.

## 2. Маппинг

### Кампания
| Keitaro / Binom | FLOX | |
|---|---|---|
| name | name (2–80) | ✅ |
| alias | трекинг-ссылка domain + slug `/t/slug` | 🔁 URL меняется |
| domain_id | tracking_links.domain_id | 🔁 домен нужно верифицировать по TXT |
| traffic_source_id | trafficSourceId | ✅ через таблицу id |
| group | теги | 🔁 |
| notes | notes (≤500) | ✅ |
| state | status | 🔁 |
| type position/weight | порядок стрим-сетов / веса | 🔁 |
| cost_type/value/auto, traffic loss | нет модели цены; только cost_entries / синк кабинета | ❌ в notes, не 0 (инв. #6) |
| uniqueness, cookies_ttl (?), bind_visitors | свои правила sticky / уников | ❌ |
| token, parameters | макросы trackingTemplate | 🔁 частично |
| S2S postbacks | postback источника | 🔁 если один, иначе notes |
| default flow | fallbackUrl | 🔁 |

### Потоки
| Keitaro | FLOX | |
|---|---|---|
| forced | стрим-сеты высшего приоритета | 🔁 |
| regular + position | стрим-сеты по порядку | ✅ |
| regular в weight-кампании | один стрим-сет, веса потоков | 🔁 точно при одинаковых фильтрах |
| default | последний стрим-сет с пустым фильтром / fallback | 🔁 |
| filters accept/reject, filter_or | rootFilter AND/OR, IS/IN vs IS_NOT/NOT_IN | 🔁 |
| country…sub_id_N, bot, proxy | 31 поле FLOX | ✅ после сверки `GET /stream_filters` (?) |
| ip, isp, operator, UA, schedule, interval, limit, uniqueness, empty referrer | нет | ❌ поток импортируется на паузе |
| schema redirect | destination redirect | ✅ |
| schema landings (доли) | N×M потоков, вес = share_L × share_O | 🔁 |
| schema action, triggers, collect_clicks=false | нет | ❌ |

### Прочее
| Keitaro / Binom | FLOX | |
|---|---|---|
| оффер: name, action_payload URL, payout, network, country[] | name, links[], payout (>0!), currency, networkId, countries (≥1!) | 🔁 перевод макросов `{subid}`→`{click_id}`; правило для payout 0 и пустых стран |
| payout_type/auto/upsell, архив оффера | — | ❌ |
| лендинг external / local / action | external / internal ZIP | ✅ / только с архивом / ❌ |
| партнёрка: name, postback_url, offer_param (?) | network + event_mappings | 🔁 новый incomingPostbackUrl, прописать в партнёрке |
| источник | name, type, trackingTemplate, postbackUrl/Statuses | 🔁 |
| домен | domain (pending до TXT) | 🔁, default_campaign ❌ |
| пользователи | — | ❌ только приглашения |
| статистика, клики | — | ❌ |

Binom: `.binom` не описан публично — только по образцу файла или через Binom API, после Keitaro.

## 3. План (каждая буква — отдельная фаза)

Один конвейер: внешний формат → FLOX-бандл → предпросмотр → применение через сервисы.

- **A. Серверный CSV отчётов**: `POST /analytics/report` (+timeseries) с `Accept: text/csv` / `?format=csv`; null → пустая ячейка; валюта в заголовке; BOM; в OpenAPI; кнопка «Экспорт CSV (полный отчёт)».
- **B. Импорт конверсий** `subid,payout,tid,status[,date]`: `POST /conversions/import`, сеть выбирается в форме; каждая строка через `conversion.Service.Record` (RawStatus → маппинг, tid = event_ref, dedup, без возврата в HOLD, курс на дату); subid = click_id, затем external_click_id; предпросмотр по строкам; ≤5000 строк; `subid,0,tid,rejected` → CPA_DECLINE.
- **C. Свой формат** `flox-bundle.json` v1 (+файлы лендингов в ZIP): источники, сети + event mappings, офферы, лендинги, постленды, домены, теги, кампании + стрим-сеты/потоки/фильтры + ссылки; ссылки по локальным id; без секретов. Экспорт всего или выбранных кампаний. Импорт: разбор → предпросмотр → режим «пустая организация» / «слияние» (по имени: пропуск или переименование) → одна транзакция → отчёт. `import_runs` + `import_id_map(source_system, source_id → flox_id)` для идемпотентности. Кампании в draft, урезанные стрим-сеты на паузе; проверка симулятором.
- **D. Keitaro**: Admin API (URL + ключ, ключ не хранится, urlsafety, фоновая задача в worker: networks, sources, offers, landing_pages, domains, groups, campaigns, streams, stream_filters, streams_actions) → конвертер в бандл с журналом потерь → C. JSON Export Flows — по реальному образцу, в существующую кампанию. Тесты на фикстурах ответов + «импорт → симулятор выбирает тот же поток».
- **E. Binom** — после D.

## 4. Риски

1. Статистика не переносится; фабриковать события нельзя. Конверсии Keitaro не к чему привязать (кроме цепочки с external_click_id).
2. CPC/CPM/RevShare: в FLOX нет расчёта за клик — потеря в notes или отдельная фича; 0 нельзя.
3. Версии Keitaro: расхождения в спеке (group_id, cost_type), справочники фильтров с трекера; Admin API только в Business; без живого трекера — только фикстуры.
4. Пустая база vs слияние: без `import_id_map` повторный импорт дублирует.
5. Дубли конверсий: dedup ключ; но `acceptDuplicates` посчитает повтор файла дважды — предупреждать в предпросмотре.
6. Отброшенные фильтры расширяют поток → на паузе; N×M меняет статистику по потокам.
7. Ручные шаги: новые URL ссылок, новый входящий postback в партнёрках, TXT доменов, local-лендинги — чек-лист в отчёте.
8. Безопасность: чужой ключ не хранить/не логировать, urlsafety, лимиты размера, валидация через сервисы, RE2.

## Открытые вопросы владельцу

1. Порядок фаз (предложено A → B → C → D) или сразу Keitaro?
2. Импорт только в пустую организацию или слияние?
3. Есть ли доступ к Keitaro Business (ответы Admin API) и образец Export Flows?
4. CPC/CPM: терять в notes или отдельная фича «модель цены кампании»?
