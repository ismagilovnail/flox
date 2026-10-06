# FLOX: технический аудит перед production (2026-10-06)

Охват: Go-монолит (`apps/tracker`, `apps/worker`, `apps/api`, `apps/internal/*`),
47 миграций Postgres, 12 схем ClickHouse, `infra/`, Dockerfiles, `docs/`.
Код читался построчно на критических путях: redirect, postback, event pipeline,
attribution, отчёты, auth и tenancy. `go build ./...` и `go vet ./...` проходят чисто.

**Что не проверено** (выводы по этим пунктам помечены NEEDS VERIFICATION):

- нет production-топологии: нет k8s-манифестов, Terraform и выбранного облака;
- нет CI;
- тесты в этой сессии не запускались, оценка дана по их составу;
- фронтенд (`apps/web`, 427 файлов) проверен только на заголовки безопасности;
- реальные `EXPLAIN` на объёмах не снимались. Оценки нагрузки ниже сделаны по
  коду и по `docs/performance.md`, где все замеры получены на loopback.

---

## 1. Executive Summary

Кодовая база для своей стадии чистая: тонкие handler'ы, tenant-скоупинг в
репозиториях, параметризованный SQL, RE2, SSRF-защита исходящих postback'ов,
секрет на входящих postback'ах, hashed-токены, единственная реализация
routing. Это заметно выше среднего MVP-трекера.

Но **pipeline данных о деньгах сейчас не является надёжным**. Найдено
несколько **детерминированных** путей потери или дублирования событий, а не
теоретических рисков:

1. **Каждый деплой или рестарт worker'а может навсегда «подвесить» пачку
   событий** в статусе `processing`. Reaper'а нет. Касается кликов, конверсий
   и исходящих postback'ов.
2. **Событие конверсии попадает в ClickHouse через «бросаемый» in-memory
   буфер**, хотя в Postgres оно уже записано как `success`. При сбое отчёт
   теряет выручку навсегда: повтор от сети получит ответ `duplicate`.
3. **Исходящий postback (S2S в FB/TikTok/источник) теряется молча**, если его
   insert упал после записи конверсии. Ошибка только логируется, сети
   возвращается 200.
4. **Курсы валют (`fx_rates`) никто не заполняет.** Любая не-USD выручка и
   не-USD расход в отчётах становятся «нет USD-значения», то есть выпадают из
   Revenue/ROI.
5. **Частичный сбой insert'а в ClickHouse повторяет весь батч** в таблицы
   `MergeTree` без ключа идемпотентности. Клики и конверсии дублируются.
6. **Postgres — единственная точка отказа redirect'а**: на каждый клик идут
   6 последовательных запросов без кэша и без таймаута. При деградации БД
   встаёт весь трафик.
7. **Конверсия, пришедшая раньше клика в ClickHouse** (секунды при норме,
   минуты-часы при backlog), или пришедшая, когда ClickHouse был недоступен
   на старте tracker'а, остаётся `unattributed` навсегда. Переатрибуции нет.

Плюс два узких места, которые проявятся уже на десятках миллионов строк.
Поиск клика по `click_id` в ClickHouse — это full-scan данных организации.
`LastStatus` в Postgres тоже идёт без подходящего индекса.

**Вердикт: NO** в текущем виде для трафика, по которому считаются деньги.
P0-список закрывается примерно за **7–12 инженерных дней**, переписывать
систему не нужно. После этого: **YES, WITH CONDITIONS** до ~1 000 rps.

---

## 2. Architecture Review

**Текущее состояние.** Модульный монолит, 4 бинарника из одного Go-модуля.

```
                  ┌──────────── Postgres (control plane + event_queue + postbacks) ───────────┐
ad click ──► tracker ─6×SELECT─┘        ▲ INSERT batch (eventbuf, 10k chan)                   │
   │ 302 / landing / PWA                 │                                                    │
   │                                     │                                                    │
postback ─► tracker /postback ── SELECT network, mapping, LastStatus ─► INSERT postbacks       │
                │                    └─► ClickHouse ByClickID (sync)                          │
                └─► eventbuf (CPA event) ─► event_queue ─► worker ─► ClickHouse                │
                └─► INSERT postback_deliveries ─► worker ─► HTTP to partner                    │
api ─► Postgres (CRUD) + ClickHouse (reports) ; web (Next.js) ─► api                          │
```

**Что сделано хорошо**

- Чёткие слои handler → service → repository; tenant scope в репозитории
  (`routingstore.LoadRoutingConfig`: org-фильтр даже там, где хватило бы
  `campaign_id`).
- Routing — чистая функция (`internal/routing`), детерминированный
  `pickWeighted` (FNV-1a по visit key), объяснимые решения. Simulator
  использует тот же код.
- Redirect не делает внешних HTTP-вызовов и аналитических запросов. Offer cap
  считается фоновым снапшотом (`offercap.Monitor`), а не запросом на hot path.
- Интерфейсы провайдеров (geo/ASN/cost/DNS/registrar), SSRF-защищённый
  транспорт, секрет postback'а, constant-time compare.
- Postgres-очередь с `FOR UPDATE SKIP LOCKED`. Для Stage 1–2 это правильный
  выбор без лишнего брокера.

**Слабые места**

| # | Компонент | Проблема |
|---|---|---|
| A1 | `routingstore` | Нет кэша конфигурации: Postgres стоит на каждом клике (§14) |
| A2 | `eventbuf` + `event_queue` | Нет outbox и reaper'а, при ошибке события бросаются (§10, §12) |
| A3 | `conversion.Service.Record` | Три независимые записи (PG `postbacks`, eventbuf, `postback_deliveries`) без транзакции |
| A4 | `attribution` | Синхронный lookup в ClickHouse на postback-пути, eventual consistency без компенсации |
| A5 | Tracker и ClickHouse | Соединение создаётся один раз при старте, без reconnect (fallback на `MemoryResolver` навсегда) |

**Что сломается первым при росте нагрузки:** Postgres. Сначала пул соединений
tracker'а (по умолчанию `max(4, NumCPU)`, 6 запросов на клик), затем
`event_queue`: на каждое событие INSERT, UPDATE и DELETE, то есть постоянный
churn, bloat и нагрузка на autovacuum.

**Что изменить до production:** A1–A5 (см. §21–22).

**Что можно оставить как есть:** монолит, Postgres как control plane,
ClickHouse как аналитическое хранилище, Postgres-очередь до ~1–2k rps,
routing engine, auth-модель, структуру пакетов.

---

## 3. Traffic Tracking Review

Путь: `GET /t/{slug}` → `track()` (`apps/tracker/handler.go:76`).

1. `ResolveTrackingLink`: 1 SELECT, JOIN domains/campaigns по `lower(domain)`
   и `slug`.
2. `LoadRoutingConfig`: 5 SELECT (campaign, stream_sets, filter_groups,
   filter_conditions, flows+offers+landings+pwas+LATERAL offer_links).
3. `Classify` (MaxMind локально), `Resolve`, `idgen.New()` (ULID) → `click_id`.
4. `events.Enqueue` (неблокирующий) → 302, internal landing (+1 SELECT по PK)
   или PWA.

**Click ID.** ULID, генерируется сервером, коллизии исключены. ✔
`stickyFlowKeepClickId` переиспользует `click_id` **из cookie без подписи**
(см. T5).

### Проблемы

**T1 [HIGH]. Конверсия раньше клика или при недоступном ClickHouse становится
`unattributed` навсегда.**

- **Где:** `attribution.Service.byClickID` → `chstore.ClickResolver.ByClickID`;
  `tracker/main.go:156-170`.
- **Почему.** Клик попадает в `click_events` только после цепочки eventbuf
  (до 1 с) → `event_queue` → poll worker'а (до 2 с) → insert в CH. При backlog
  или простое CH это минуты и часы. Если ClickHouse недоступен **при старте
  tracker'а**, подключается `MemoryResolver`, и все конверсии получают
  `unknown_click` до следующего рестарта. Postback при этом записывается как
  `success` с `attribution_outcome=unattributed`. Повтор от сети даёт
  `duplicate`, исправить уже нельзя.
- **В production.** Instant-конверсии (регистрация / `CPA_HOLD`, TG_START
  через S2S) и любые конверсии во время backlog'а. В отчётах по кампаниям
  выручки нет, а в Postgres она есть.
- **Вероятность:** высокая при backlog'е или рестартах, средняя в норме.
- **Исправление:**
  (a) при `unknown_click` записывать конверсию со статусом
  `pending_attribution` и переатрибутировать в worker'е с backoff
  (30 с, 2 мин, 10 мин, 1 ч, 6 ч). Событие в CH эмитить только после
  атрибуции или финального отказа;
  (b) `ClickResolver` с lazy-reconnect. Пока CH недоступен, postback отвечает
  **503**, чтобы сеть повторила, а не писала `unattributed`.

**T2 [HIGH]. Одна и та же конверсия может разойтись между PG и CH, а одна
атрибуция может выпасть.** См. D1 (outbox).

**T3 [MEDIUM]. Время конверсии — время прихода postback'а, а не время
события у сети.**

- **Где:** `tracker/postback.go`, `OccurredAt: time.Now().UTC()`.
- Ночные реплеи и задержки сетей сдвигают конверсии в «сегодня», и FX берётся
  по дате прихода. Это осознанное решение, но параметра `event_time`/`ts` нет
  вообще.
- **Исправление:** принимать необязательный `event_time` (unix или RFC3339)
  в разумных пределах (не в будущем, не старше 90 дней). Хранить оба поля
  (`occurred_at`, `received_at`); отчёты строить по `occurred_at`, FX брать по
  нему же.

**T4 [MEDIUM]. Конверсии не связываются с несколькими кликами, но и
«разворотов» нет.**

- Одна конверсия к нескольким кликам не привязывается: `ByClickID` →
  `LIMIT 1`, `ByExternalClickID` при >1 совпадении даёт `ambiguous`.
  Проверено. ✔
- Обратная сторона: `CPA_DECLINE`/`CPA_TRASH` после `CPA_ACCEPT` выручку не
  уменьшает. Отчёт суммирует `usd_value` по `type IN ('CPA_ACCEPT','CPA_REDEP')`
  (`chstore/report.go:556`). Если сеть делает chargeback, Revenue и ROI
  завышены. **NEEDS VERIFICATION:** проверить по спецификации §45, должна ли
  отмена вычитаться.

**T5 [MEDIUM]. Sticky cookie без подписи.**

- **Где:** `tracker/sticky.go:26`, `routing/engine.go:66 trySticky`.
- `sf_{campaignId}=setId:flowId[:clickId]` принимается как есть.
  `trySticky` проверяет только, что set/flow ещё существуют, **фильтры
  stream set'а не перепроверяются**. Значит:
  (1) кто угодно, увидев значения из реального визита, ставит cookie и
      обходит гео/бот-фильтры, например попадает в «денежный» поток вместо
      safe-page;
  (2) при `keepClickId` в цепочку пишется произвольный `click_id` без
      валидации: подмена атрибуции чужого клика и мусорные строки до ~4 КБ
      в ClickHouse.
- **Исправление:** HMAC-подпись значения ключом tracker'а (`LANDING_LINK_KEY`
  уже есть), `idgen.IsValid(clickID)`. Нужно ли перепроверять фильтры для
  sticky — продуктовое решение, но поддельная cookie должна просто
  игнорироваться.

```go
// value = setID:flowID[:clickID].base64url(hmac256(key, campaignID|value))[:22]
func signSticky(key []byte, campaignID, v string) string {
    m := hmac.New(sha256.New, key); m.Write([]byte(campaignID + "|" + v))
    return v + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))[:22]
}
```

**T6 [LOW]. Трекинг работает на неверифицированных доменах.**
`ResolveTrackingLink` не проверяет `domains.status`/`verified_at`.
`domains_domain_key` уникален глобально, поэтому любой тенант может
«занять» чужой домен до владельца: это squatting и DoS онбординга.
**Исправление:** верификация через TXT-запись перед `status='active'`;
резолвить только `status='active'`.

**Проверено и в порядке:** IPv4/IPv6 (`net.IP.String()`), разбор
X-Forwarded-For только от `TRUSTED_PROXIES`, 302 вместо 301, 404 на
неактивные кампании, SOURCE_FILTER для заблокированного трафика,
кросс-тенантная изоляция slug (`domain_id` + `slug`).

---

## 4. Database Review

### Postgres

**DB1 [HIGH]. У `postbacks` нет индекса под горячие запросы.**

- **Где:** `conversion/postgres.go: LastStatus`, `FindSuccessID`.
- Запрос: `WHERE organization_id=$1 AND click_id=$2 AND result='success'
  ORDER BY created_at DESC LIMIT 1`.
- Доступные индексы: `(organization_id)`, `(network_id)` и частичный
  уникальный `(organization_id, click_id, status, event_ref) WHERE NOT
  network_accepts_duplicates AND result='success'`. Частичный индекс
  планировщик использовать не может: в запросе нет предиката
  `NOT network_accepts_duplicates`. Значит, каждый postback читает **все**
  postback'и организации. На 1М строк это миллисекунды, на 10М+ — сотни мс,
  на 100М — секунды.
- **Исправление:**

```sql
CREATE INDEX CONCURRENTLY postbacks_click_lookup_idx
  ON postbacks (organization_id, click_id, created_at DESC)
  WHERE result = 'success';
```

**DB2 [MEDIUM]. Таблица `postbacks` растёт без ограничений** (все попытки,
включая duplicate/error), retention нет. Через год активных сетей это
десятки и сотни миллионов строк. **Исправление:** партиционирование по
месяцу `created_at`, retention для `result IN ('duplicate','error','ignored')`
90 дней. Строки `success` — это source of truth, они хранятся.

**DB3 [MEDIUM]. `event_queue` / `postback_attempt_queue` работают как
очередь в Postgres.** На каждое событие: INSERT (JSONB 1–2 КБ), UPDATE на
claim, DELETE. На 1k событий/с это ~3k row-ops/с, нормально. На 10k/с это
~30k row-ops/с, ~30–60 МБ/с WAL, постоянный bloat. Нужен агрессивный
`autovacuum_vacuum_scale_factor=0.01` на этих таблицах. Дальше Stage 2 —
брокер (§19).

**DB4 [LOW]. `ResolveSession` делает `UPDATE memberships SET last_active_at`
на каждый API-запрос** (`auth/repository.go:206`). Это запись и row lock на
каждый GET, а дашборды поллят. **Исправление:** обновлять не чаще раза в
минуту (`WHERE last_active_at < now() - interval '1 min'`).

**DB5 [LOW]. Нет очистки протухших `sessions`, `pending_totp_logins`,
инвайтов.** Нужна cron-задача в worker'е.

**Хорошо:** ULID (сортируемые), `organization_id` во всех tenant-таблицах,
FK, CHECK-ограничения статусов, `numeric(14,4)` для денег в PG, goose
up/down, триггеры `updated_at`.

### Рост таблиц пропорционально кликам

Клики в Postgres не хранятся, и это правильно. Таблица `click_events` в CH:

| Объём | Состояние |
|---|---|
| 1М | Без проблем |
| 10М | Отчёты нормально; `ByClickID` уже читает все гранулы организации (см. CH1) |
| 100М | Отчёты за 30 дней — секунды, если не используются MV; каждый postback сканирует ГБ данных |
| 1 млрд | Без TTL/retention диск растёт линейно; `uniqExact` на unique clicks требует много RAM; postback-путь непригоден |

### ClickHouse

**CH1 [HIGH]. Point lookup по `click_id` — это full-scan данных
организации.**

- **Где:** `chstore/attribution.go: ByClickID`.
- `ORDER BY (organization_id, event_at, campaign_id, click_id)`, в WHERE нет
  `event_at`, skip-индекса на `click_id` в `click_events` нет (есть только
  на `external_click_id` и `country`).
- **Исправление:** отдельная таблица под lookup.

```sql
CREATE TABLE click_lookup (
  organization_id String, click_id String, event_at DateTime64(3),
  campaign_id String, /* ...поля attribution.Click... */
) ENGINE = ReplacingMergeTree
ORDER BY (organization_id, click_id);  -- point lookup = O(log n)
CREATE MATERIALIZED VIEW click_lookup_mv TO click_lookup AS
  SELECT ... FROM click_events WHERE type = 'SOURCE_CLICK';
```

  Быстрый минимум: `ALTER TABLE click_events ADD INDEX idx_click_id click_id
  TYPE bloom_filter(0.001) GRANULARITY 1; MATERIALIZE INDEX idx_click_id`.

**CH2 [HIGH]. Дубли при повторе батча.** Подробно в D2.

**CH3 [MEDIUM]. Нет TTL и retention ни на одной таблице.** Добавить
`TTL event_at + INTERVAL 13 MONTH` для raw `tracking_events` и
`postback_events`. Агрегаты (MV) хранятся дольше.

**CH4 [MEDIUM]. Соединение без настроек.** `chconn.NewConn`: HTTP без TLS,
без `max_execution_time`, без `DialTimeout`, без лимитов пула, без
сжатия. Один тяжёлый отчёт из API может занять CH, на котором висит
атрибуция postback'ов. **Исправление:** отдельные пользователи и профили CH
для `api` (`max_execution_time=30`, `max_memory_usage`) и для
`tracker`/`worker`.

**CH5 [LOW]. Миграции CH** применяют три бинарника параллельно на старте,
без таблицы версий. Сейчас все операторы идемпотентны, но первая
неидемпотентная миграция сломается. **Исправление:** `schema_migrations` в
CH и применение одним job'ом.

**Нужен ли ClickHouse?** Да, именно этой архитектуре. Отчёты с произвольными
измерениями по сырым событиям, unique clicks, LTV-когорты и funnel на 10М+
событий в сутки в Postgres без rollup-слоя не работают. Решение
обоснованное. Использовать его как OLTP-индекс для атрибуции нельзя (CH1).

---

## 5. Performance Review

**Запросы к БД на один клик:**

| Путь | PG reads | PG writes (амортизированно) | CH | Redis | External |
|---|---:|---:|---:|---:|---:|
| `/t/{slug}` → 302 | **6** (5 при пустых фильтрах) | 1 INSERT в батче + 1 UPDATE + 1 DELETE в worker'е | 0 | 0 | 0 |
| `/t/{slug}` → internal landing | 7 | ×2 события (SOURCE_CLICK + LAND_VIEW) | 0 | 0 | 0 |
| `/postback/{id}` | 4–5 (network, mapping, LastStatus, fx, +dup) | 1–2 INSERT + delivery INSERT + event | **1 sync** (`ByClickID`) | 0–2 | 0 |

Все шесть чтений redirect'а **последовательные**, и каждое — отдельный
acquire из пула. По `docs/performance.md` на loopback это 0.5 мс на клик.
В облаке с RTT 0.3–1 мс до managed-Postgres выйдет 3–8 мс на клик плюс
очередь за соединением.

**Ёмкость (оценка по коду, без замеров, NEEDS VERIFICATION нагрузочным
тестом в целевой сети):**

| rps | Что происходит |
|---|---|
| 100 | Без проблем |
| 1 000 | 6k SELECT/с + 3k row-writes/с в PG. Работает, но пул по умолчанию (`max(4,NumCPU)`) у tracker'а уже даёт ожидание; нужен `pool_max_conns` |
| 10 000 | 60k SELECT/с только на конфиг + 30k row-ops/с в очереди. Одиночный PG упирается в CPU и WAL; autovacuum не успевает на `event_queue`; буфер eventbuf (10k) переполняется при любой паузе записи и начинает **ронять события** |
| 50 000+ | Невозможно без кэша конфига и брокера вместо PG-очереди |

**P1. Кэш routing-конфигурации в процессе tracker'а** — главная
оптимизация. Она снимает 6 из 6 запросов с hot path:

```go
type entry struct { link TrackingLink; cfg CampaignRouting; loadedAt time.Time }
// key: lower(host)+"/"+slug ; TTL 5–10s; singleflight на промах;
// stale-on-error: при ошибке PG отдавать последний известный конфиг до 10 минут;
// инвалидация: LISTEN flox_config_changed (NOTIFY из API в той же транзакции,
// что UPDATE campaigns/stream_sets/flows) → удалить ключи кампании.
// Negative cache на 404 (30 с) — защита от перебора slug'ов.
```

Это не нарушает инвариант #1: логика routing остаётся одна, кэшируется
только вход.

**Прочее:**

- Postback-путь: `ByClickID` (CH1) и `LastStatus` (DB1) — две линейные по
  объёму операции на каждую конверсию.
- `uniqExactIf(concat(campaign_id,'|',ip,'|',user_agent))` в
  `chstore/report.go:328` даёт RAM, пропорциональный числу уникальных
  посетителей в окне. На 100М+ кликов за период нужен `uniqCombined64` или
  MV с предрасчётом.
- Списки сущностей (offers, networks, landings, pwas, tags, streamsets) без
  LIMIT. При сотнях записей это нормально, при тысячах — P3.

---

## 6. API Review

**Хорошо:**

- Сессия в HttpOnly-cookie, `SameSite=Lax`, Secure по конфигу.
- CSRF через проверку Origin на мутирующих методах.
- RBAC `RequirePermission` на всех write-маршрутах, scope API-ключей.
- Rate limit: общий 300/мин и auth 20 на 15 мин по IP.
- `middleware.Timeout(30s)`, единый `apierror`, лимиты на upload'ы
  (`MaxBytesReader`), защита от zip-bomb и zip-slip в `landing/bundle.go`,
  диапазон отчётов не больше 366 дней, tz валидируется через
  `time.LoadLocation` и regex перед подстановкой в SQL.

**Проблемы:**

- **API1 [MEDIUM].** `ratelimit.Allow` делает `INCR`, затем отдельный
  `EXPIRE`. Если `EXPIRE` не выполнился (обрыв сети или рестарт между
  вызовами), ключ остаётся **без TTL**, и этот IP или ключ получает 429
  навсегда. **Исправление:** Lua-скрипт или
  `SET key 0 EX window NX` + `INCR`, либо `INCR` + `EXPIRE ... NX` в одном
  `MULTI`.
- **API2 [LOW].** При недоступном Redis лимитер пропускает всё. Для `/auth/*`
  это неограниченный перебор паролей на время простоя Redis. Лимита на
  аккаунт (а не на IP) нет. **Исправление:** fail-closed для auth-группы или
  счётчик неудачных входов в `users`.
- **API3 [LOW].** Нет версионирования (`/v1`). Для внешнего API по ключам
  его стоит ввести до первого внешнего клиента.
- **«1 млн запросов»:**
  - `/auth/*` упирается в лимит;
  - общий API — 300/мин на ключ или сессию;
  - **`/t/*` и `/postback/*` на tracker'е без лимитов.** На `/t/unknown`
    каждый запрос стоит 1 SELECT, на валидный slug — 6 SELECT и INSERT.
    На `/postback/{validUlid}` — SELECT по `networks`. Миллион запросов
    означает миллион и больше запросов в PG. Защищает только кэш с negative
    cache (§5) и лимит на edge (WAF/LB).

---

## 7. Security Review

| Класс | Статус |
|---|---|
| SQL injection | Не найдено: везде placeholders; динамические колонки отчётов берутся из whitelist (`qualify`, `reportClickColumns`), tz проходит через regex |
| XSS | Пользовательский HTML лендингов отдаётся с домена tracker'а, а не app; app-cookie host-only. Приемлемо |
| CSRF | Origin-check + SameSite=Lax. Отсутствие `Origin` пропускается: приемлемо при Lax, но см. S2 |
| SSRF | `urlsafety.SafeDialContext` на исходящих postback'ах и Slack. ✔ Для Namecheap/Cloudflare/FB/TikTok/Google URL фиксированы. ✔ |
| IDOR / tenant | Проверено сканом всех SQL-строк: запросы без `organization_id` — только глобальные (users, roles, fx, очереди) или внутри транзакции после org-проверки. Ссылки flow → offer/landing/pwa проверяются в `streamset.Service`. ✔ |
| Пароли | bcrypt, dummy hash против user enumeration по таймингу. ✔ |
| Токены | Хранятся только sha256-хэши сессий, инвайтов, API-ключей и секретов postback'ов. ✔ |
| Postback auth | `secret` в query + constant-time compare. ✔ Replay-защиты нет (подписи с timestamp нет), но дедуп-ключ гасит повтор той же конверсии. Для отрасли норма |
| Secrets | `.env` в `.gitignore`, не закоммичен. ✔ |

**Проблемы:**

- **S1 [MEDIUM].** Неподписанная sticky cookie (T5).
- **S2 [LOW-MEDIUM].** У web/API нет `Content-Security-Policy`,
  `X-Frame-Options`/`frame-ancestors`, HSTS (`apps/web/next.config.ts`,
  `httpserver`). Админку можно встроить во фрейм (clickjacking на
  деструктивные действия).
- **S3 [MEDIUM].** `revenue` парсится через `strconv.ParseFloat`, который
  принимает `NaN`, `Inf` и отрицательные значения (`tracker/postback.go`).
  Один `revenue=NaN` от сети (или от любого, кто знает URL с секретом)
  превращает `sum()` в CH в NaN за весь период: колонка Revenue в отчётах
  ломается целиком. Значение больше 10^10 переполняет `numeric(14,4)`:
  INSERT падает, ответ 500, сеть ретраит бесконечно (poison message).
  **Исправление:** `math.IsNaN/IsInf`, диапазон
  `0 ≤ v < 1e9`, иначе 400.
- **S4 [LOW].** Squatting доменов (T6).

---

## 8. Multi-tenancy Review

**Изоляция в Postgres сделана хорошо и последовательно.** `organization_id`
берётся только из сессии или API-ключа, в postback'е — из `networks` по
`networkId`. Репозитории фильтруют по нему. ClickHouse: все запросы с
`organization_id = ?`, и он первый в sort key. Redis-ключи содержат org
(`flox:conversion:last-status:{org}:{click}`).

**Где tenant может потеряться:**

- **M1 [LOW].** `offercap.Monitor.Capped()` возвращает глобальный
  `map[offerID]`. Безопасно, потому что offer id — глобально уникальные
  ULID. Это стоит зафиксировать комментарием.
- **M2.** Воркеры (`event_queue`, `postback_deliveries`) обрабатывают
  строки всех тенантов в одном батче. Org сохраняется в payload, утечки нет.
  Но один тенант с флудом событий задерживает остальных: fairness нет.
  P3.
- **M3.** Логи: `click_id`/`network_id` пишутся вместе с `organization_id`.
  PII (IP, UA) в логи tracker'а не попадает. ✔
- **M4. NEEDS VERIFICATION:** подписанные ссылки лендингов и PWA
  (`landinglink`) — нужно убедиться, что `org` в claims сверяется с
  резолвленным ресурсом во всех handler'ах `/l/*` и `/pwa/*`. Беглый
  просмотр это подтверждает, но полного разбора `tracker/pwa.go` не было.

---

## 9. Redis / Cache Review

Redis используется только как кэш `LastStatus` (TTL 30 дней) и для
rate-limit'ов. Redirect от Redis **не зависит**. Это правильно (инвариант #4:
sticky живёт в cookie).

- **R1 [MEDIUM].** Гонка кэша `LastStatus`: две параллельные конверсии
  записывают значения в обратном порядке, и в кэше остаётся более старый
  статус на 30 дней. Решение то же, что для D3 (сериализация по click).
- **R2.** Ключи `flox:conversion:last-status:*` растут на каждый
  сконвертировавшийся click по ~100 байт × 30 дней. На 10М конверсий в месяц
  это около 1 ГБ. Нужен `maxmemory-policy allkeys-lru`. NEEDS VERIFICATION:
  конфиг Redis в production.
- **Что должно быть:**
  - кэш — routing config (в процессе), `LastStatus`, negative 404;
  - persistent — PG (конфиг, `postbacks`) и CH (события);
  - ephemeral — rate-limit счётчики.

---

## 10. Queue / Worker Review

**Q1 [CRITICAL]. Строки в `processing` зависают навсегда.**
**Статус: исправлено** (миграция 00048, `internal/queuereaper`, см. CHANGELOG).

- **Где:** `eventqueue/postgres.go: ClaimDue` (ставит
  `status='processing'`), `postback/postgres.go: ClaimDue`,
  `postbacklog/postgres.go: ClaimDue`. **Reaper'а нет нигде.**
- **Механизм 1 (каждый деплой).** `worker/main.go`: по SIGTERM `ctx`
  отменяется, `run()` сразу выходит, poll-циклы не ждут. Текущий `RunOnce`
  получает отменённый ctx: `InsertBatch` падает с `context canceled`, а
  `Requeue(ctx, …)` вызывается **с тем же отменённым ctx** и тоже падает.
  Строки остаются `processing` навсегда.
- **Механизм 2.** OOM, SIGKILL или паника между claim и delete/requeue.
- **Влияние:**
  - клики и конверсии не доходят до CH и пропадают из отчётов. Данные
    физически лежат в PG, их можно вытащить руками, но никто не узнает, что
    это нужно;
  - **исходящие postback'и** в источник трафика не отправляются никогда.
    Для FB/TikTok S2S это прямая потеря оптимизации;
  - `event_queue_depth` растёт, алерт `> 10000` при малом объёме не
    сработает.
- **Вероятность:** при постоянной нагрузке цикл почти всегда внутри батча,
  то есть **почти каждый деплой**.
- **Исправление:**

```sql
ALTER TABLE event_queue ADD COLUMN claimed_at timestamptz;
-- ClaimDue: SET status='processing', claimed_at=now()
-- reaper (worker, раз в минуту):
UPDATE event_queue SET status='queued', claimed_at=NULL, next_attempt_at=now()
WHERE status='processing' AND claimed_at < now() - interval '5 minutes';
-- то же для postback_attempt_queue; для postback_deliveries — вернуть в 'retrying'
```

```go
// worker: Requeue/Delete/Mark* — на context.WithoutCancel(ctx) с таймаутом 5s;
// run(): wg.Wait() для всех PollLoop перед выходом (graceful drain ≤ 25s).
```

**Q2 [HIGH]. Нет dead-letter для «ядовитых» событий** в `event_queue`.
Если CH отвергает батч из-за одной плохой строки (тип, размер), весь батч
ретраится каждые 10 с бесконечно, и очередь за ним встаёт (ORDER BY
`next_attempt_at`). **Исправление:** `attempt_count`; после N неудач делить
батч пополам до одной строки, после чего строку переводить в `dead` и
поднимать алерт.

**Q3 [MEDIUM]. Нет fairness и упорядочивания.** Порядок событий одного
`click_id` не гарантирован. Для CH это нормально: агрегаты не зависят от
порядка.

**Может ли один conversion/postback обработаться дважды?**

- **Входящий postback:** нет. Уникальный индекс
  `(org, click_id, status, event_ref)` срабатывает и при гонке. ✔
- **Событие конверсии в CH:** **да**, через Q1 + D2 (повтор батча после
  частичного успеха).
- **Исходящая доставка:** да, если HTTP успел, а `MarkSuccess` упал. Это
  at-least-once, для postback'ов норма. Партнёрам стоит передавать
  идемпотентный `{transaction_id}` (id postback'а) в макросе.

---

## 11. Webhook / Postback Review

**Текущее состояние:** `GET/POST /postback/{networkId}?secret=…&click_id=…&status=…&revenue=…&currency=…&txn_id=…`.

**Хорошо:**

- секрет с constant-time проверкой;
- маппинг статусов per network;
- дедуп `(click_id, status, event_ref)`, где `event_ref` берётся только для
  REDEP;
- запрет отката в HOLD;
- аудит каждой попытки, включая error/ignored;
- 500 на инфраструктурные ошибки, чтобы сеть ретраила;
- `acceptDuplicates` per network.

**Проблемы:**

- **P1 = D1 [CRITICAL].** Три независимые записи без транзакции:
  `postbacks` (PG), событие в CH через **бросаемый** `eventbuf`,
  `postback_deliveries` (ошибка проглатывается, `enqueuer.go:40`). К тому же
  enqueue доставки идёт на `r.Context()`: если сеть закрыла соединение по
  своему таймауту (а CH1 делает postback медленным), insert доставки
  отменяется, конверсия уже записана, и ретрай сети получает `duplicate`.
- **P2 = D3 [MEDIUM].** Гонка progression: `LastStatus` и `Record` не
  атомарны. Параллельные HOLD и ACCEPT для одного клика (реплей вместе с
  live) обе проходят проверку, и HOLD записывается после ACCEPT.
- **P3 [MEDIUM].** S3 (NaN/Inf/overflow).
- **P4 [LOW].** Лимита на `/postback` нет (см. API).

**Целевая архитектура postback-обработки:**

```
POST /postback/{networkId}
  1. auth (secret), parse, validate (revenue finite, 0..1e9; event_time sane) → 400 на мусор
  2. BEGIN
       SELECT pg_advisory_xact_lock(hashtextextended(org || ':' || click_id, 0));
       last := LastStatus(...)                       -- под локом: progression атомарна
       INSERT INTO postbacks ... ON CONFLICT DO NOTHING RETURNING id
       IF inserted:
         INSERT INTO event_queue (outbox: CPA event)  -- тот же tx
         INSERT INTO postback_deliveries (...)        -- тот же tx
     COMMIT
  3. 200 / 409 duplicate
Атрибуция: в worker'е при flush outbox-события (с retry, пока клик не найден ≤ N часов),
а не синхронно в HTTP-запросе сети. Ответ сети ≤ 20 мс и не зависит от CH.
```

Это убирает D1, D3, T1 и CH1 с postback-пути одним изменением. Таблица
`event_queue` уже лежит в том же Postgres, поэтому outbox здесь дешёвый.

---

## 12. Analytics Review

**Хорошо:**

- Cost отсутствует → Cost/Profit/ROI/ROAS/CPC/CPA = `nil` («—»), а не
  деление на 0 (инвариант #6). ✔
- Timezone: границы `[from 00:00, to+1d 00:00)` в зоне клиента,
  `toDate(col, tz)` для бакетов. ✔
- Revenue считается только по `CPA_ACCEPT`/`CPA_REDEP` в USD с
  `has_usd_value`. Конверсии без курса не превращаются в 0, но **выпадают**.
- Unique clicks — по `campaign|ip|ua`, а не по `click_id` (с учётом
  keepClickId). ✔

**Проблемы:**

- **AN1 [CRITICAL для не-USD клиентов].** Таблицу `fx_rates` ничто не
  заполняет: в репозитории нет ни провайдера, ни job'а; INSERT есть только в
  тестах. `PostgresFX.ToUSD` ищет точную `rate_date`, без fallback на
  предыдущий день. В итоге **любая выручка в EUR/RUB/BRL/… и любой расход в
  не-USD имеют `usd_value=NULL`**. Revenue занижена, ROI = «—» или неверный,
  и пересчёта после появления курса нет. Комментарий в
  `apps/api/Dockerfile` упоминает «FX rate provider», которого не
  существует.
  **Исправление:**
  - FX-провайдер за интерфейсом (ECB daily — бесплатный, или
    openexchangerates);
  - ежедневный job в worker'е;
  - lookup «последний курс ≤ даты события» с ограничением 7 дней;
  - job `backfill_usd`, который досчитывает `usd_value IS NULL` в
    `postbacks` и `cost_entries` и переэмитит события.
- **AN2 [HIGH].** Дубли в CH (D2) завышают clicks, conversions и revenue.
  Отчёт и raw-данные в PG (`postbacks`) **могут показывать разные цифры**.
  Сверки (reconciliation) между ними нет.
- **AN3 [MEDIUM].** Отмены после ACCEPT не вычитаются (T4).
- **AN4 [MEDIUM].** Cost хранится по дате (`entry_date`), а отчёт в не-UTC
  зоне режет клики по локальной дате. День с разницей зон показывает cost
  «чужих суток». Это известное ограничение дневных cost'ов, его нужно хотя бы
  обозначить в UI. NEEDS VERIFICATION: в какой зоне costsync пишет даты
  FB/TikTok (у FB это зона рекламного аккаунта).
- **AN5 [LOW].** Конверсии датируются временем конверсии, а не клика. Это
  стандарт, но для ROI по дням кампании нужен режим «по дате клика» (cohort).
  LTV-модуль частично это покрывает.

**Могут ли dashboard и raw расходиться?** Да, по трём механизмам:

1. eventbuf дропнул CPA-событие, которое в PG уже есть (D1);
2. дубли при повторе батча (D2);
3. зависшие `processing` (Q1).

Нужен ночной job сверки: `count/sum` из `postbacks WHERE result='success'`
против `conversion_events` по `(org, day)`. При расхождении — алерт и
переэмит.

---

## 13. Reliability Review

| Сценарий | Что будет сейчас |
|---|---|
| **Redis недоступен 30 с** | Redirect не затронут. Postback'и читают PG напрямую (корректно). Rate-limit fail-open, auth без лимита 30 с. **Деградация допустимая** ✔ |
| **PG отвечает 5 с** | Каждый клик: 6 последовательных запросов **без таймаута**, то есть до 30 с на redirect. Пул tracker'а (`max(4,NumCPU)`) забивается, горутины копятся, пользователи рекламы уходят. **Полная остановка трафика.** eventbuf: `Write` висит до 10 с, буфер 10k переполняется, события дропаются, неудачный батч **выбрасывается** без retry |
| **PG недоступен** | Все клики 502 «tracking unavailable». Tracker `/health` продолжает отвечать `ok` (не проверяет PG), так что LB не выводит инстанс |
| **CH недоступен** | Клики идут (события копятся в PG-очереди, это хорошо). Postback'и: `ByClickID` возвращает ошибку → 500 → сеть ретраит. ✔ Если же CH был недоступен **на старте tracker'а**, все конверсии `unattributed` до рестарта ✘ |
| **Внешний API недоступен** | Исходящие postback'и: backoff 8 попыток (~21 ч), затем dead. ✔ Cost sync: `http.DefaultClient` **без таймаута** на FB/TikTok/Google: зависшее соединение блокирует sync всех организаций до рестарта worker'а ✘ |

**Отдельные проблемы:**

- **RL1 [HIGH].** У tracker'а нет per-request deadline и
  `statement_timeout`. **Исправление:** `context.WithTimeout(r.Context(),
  300ms)` на чтение конфига, `statement_timeout=500ms` в DSN tracker'а и
  stale-on-error кэш (§5).
- **RL2 [HIGH].** eventbuf выбрасывает неудачный батч (`eventbuf.go:155`):
  счётчик `failed` растёт, retry нет. Одна PG-ошибка длиной в секунду (а
  failover managed-PG длится 10–60 с) теряет до 500 событий плюс всё, что
  переполнило канал. **Исправление:** retry с backoff в пределах ёмкости
  буфера; при длительной недоступности — локальный spill-файл (WAL-like
  append-only), который досылается после восстановления.
- **RL3 [MEDIUM].** Паника на shutdown tracker'а: `events.Close()`
  вызывается явно (`tracker/main.go:258`) и ещё раз через
  `defer events.Close()` (`:96`). `close(w.done)` на закрытом канале
  паникует. Данные к этому моменту уже сброшены, но **каждый штатный стоп
  завершается паникой и exit code 2**. Это шум в алертах и маскировка
  настоящих падений. **Исправление:** `sync.Once` в `Close`.
- **RL4 [MEDIUM].** Tracker `/health` не проверяет зависимости, readiness
  нет. **Исправление:** `/ready` с проверкой «PG ping ≤ 200 мс ИЛИ кэш
  конфигурации прогрет».
- **RL5 [LOW].** У `http.Server` tracker'а и API нет
  `ReadTimeout`/`WriteTimeout`/`IdleTimeout`. Медленные клиенты держат
  соединения.

---

## 14. Observability Review

**Есть:**

- slog JSON, request ID (с echo в ответ), OTel tracing (OTLP), Prometheus;
- метрики: `tracking_latency`, `tracking_requests_total{outcome}`,
  eventbuf `enqueued/written/dropped/failed`, `event_queue_depth`,
  `events_requeued`, `postback_deliveries_total{outcome}`, `analytics`
  latency;
- `infra/alerts.yml` с 8 разумными алертами, включая `EventLossDetected`.

**Не хватает (минимальный набор):**

| Метрика | Зачем |
|---|---|
| `postback_incoming_total{result}` + latency p95 | Сейчас входящие конверсии не видны в метриках вообще |
| `attribution_outcome_total{outcome}` | Рост `unknown_click` — индикатор T1 и Q1 |
| `queue_oldest_age_seconds{queue}` (`now() - min(created_at)`) | Depth не ловит зависание при малом объёме; возраст ловит |
| `queue_processing_stuck{queue}` (processing старше 5 мин) | Q1 |
| `pgxpool` stats: `acquired`, `idle`, `max`, `acquire_wait_duration` (по каждому бинарнику) | Исчерпание пула — первая точка отказа |
| `clickhouse_insert_errors_total{table}` | Сейчас только requeued |
| `fx_missing_total{currency}` | AN1 |
| `reconciliation_diff{org}` | Расхождение PG и CH |
| `redis_errors_total` | Fail-open должен быть виден |
| Redirect p50/p95/p99 по outcome, CPU, RSS, GC pause, goroutines | Частично есть через Go collector |

Error tracking (Sentry или аналог) не подключён. Паники тонут в логах.

---

## 15. DevOps Review

**Есть:**

- multi-stage Dockerfiles, non-root (uid 10001), `-trimpath -s -w`, alpine;
- HEALTHCHECK;
- `time/tzdata` вшит (api, offercap) ✔;
- goose up/down;
- `docs/migrations.md` с порядком zero-downtime;
- `docs/backup.md` с командами.

**Нет:**

- **DO1 [HIGH]. CI.** Нет `.github/workflows` или аналога. Ни один merge не
  проверяет `go test`, `tsc`, `eslint`, build и миграции. Для системы с
  деньгами это P1.
- **DO2 [HIGH]. Автоматизации бэкапов и проверенного restore.** Это
  осознанно отложено в `docs/backup.md`, но до production обязательно:
  PITR для PG (WAL archiving), `BACKUP ... TO S3` для CH и ежемесячный
  restore-drill.
- **DO3 [MEDIUM]. Манифестов оркестрации** (requests/limits, PDB,
  HPA, `terminationGracePeriodSeconds`) нет. NEEDS VERIFICATION.
- **DO4 [MEDIUM].** В runtime-образе tracker'а нет `ca-certificates`
  (`apps/tracker/Dockerfile`). При managed-PG с `sslmode=verify-full`,
  Redis с TLS (`rediss://`) или HTTPS-ClickHouse tracker не стартует или не
  подключится.
- **DO5.** ClickHouse только по HTTP без TLS (`chconn`).

**Можно ли задеплоить новую версию без потери clicks/conversions?**

- **Tracker:** почти да. `Shutdown` ждёт запросы до 10 с, потом eventbuf
  сбрасывается. Минусы: паника RL3, а при неудачной записи в PG события
  теряются (RL2).
- **Worker: нет** (Q1). Деплой оставляет батч в `processing` навсегда.
- **API:** да.

Итог: **нет, пока не исправлены Q1 и RL2.**

---

## 16. Testing Review

98 тестовых файлов, ~24k строк. Есть:

- unit-тесты routing, classifier, attribution и conversion;
- интеграционные тесты против реальных PG/CH/Redis/MinIO (пропускаются без
  env, см. memory `flox-full-test-env`);
- conformance fixture routing (Go и UI);
- benchmarks и нагрузочный прогон vegeta до 6k rps (loopback);
- e2e-документ.

**Пробелы — тесты, которые надо добавить:**

| Тест | Что ловит |
|---|---|
| Worker: отменить ctx посреди `RunOnce`, затем reaper, затем строки снова `queued` и доставлены | Q1 |
| `InsertBatch`: clicks ок, tracking падает, retry → нет дублей в CH (по `event_id`) | D2 |
| 50 параллельных postback'ов (HOLD/ACCEPT/REDEP вперемешку) на один click → итоговый статус не HOLD, ровно одна строка на `(click,status,ref)` | D3 |
| Postback, где insert в `postback_deliveries` падает → вся операция откатывается, ответ 500 | D1 |
| Postback до появления клика в CH → через N секунд конверсия атрибутирована | T1 |
| `revenue=NaN`, `Inf`, `-5`, `1e15` → 400 | S3 |
| Поддельная sticky cookie → игнорируется | T5 |
| PG «чёрная дыра» (toxiproxy, latency 5 с) → redirect p95 ≤ 50 мс из кэша | RL1 |
| Нагрузочный тест в целевой сети (tracker и PG на разных хостах), 1k/5k rps, 30 мин, сверка `enqueued == persisted in CH` | §5 |
| Кросс-тенант: API-ключ org A + ID ресурсов org B на **каждом** маршруте (табличный тест по роутеру) | Регрессии tenancy |
| Shutdown tracker'а → exit code 0 | RL3 |

Тестов на сбои, гонки и конкурентность фактически нет: в `_test.go`
используется только одно место с `WaitGroup`/`t.Parallel`.

---

## 17. Data Integrity Review

**Source of truth:**

- Postgres: конфигурация, `postbacks` (`result='success'` — истина по
  конверсиям), `cost_entries`, `postback_deliveries`;
- для кликов истиной служит `click_events` в CH (в PG они живут только
  транзитом).

**Derived:** все CH-таблицы `conversion_events`, MV `*_daily_*`,
`ltv_events`; Redis `last-status`; offer-cap снапшот.

**Проблемы:**

**D1 [CRITICAL]. Нет транзакционного outbox.**
**Статус: исправлено** (одна транзакция `postbacks` + `event_queue` + `postback_deliveries`).

- **Где:** `conversion.Service.Record` → `s.events.Enqueue(...)` (in-memory,
  droppable) и `s.deliveries.Enqueue` (ошибка проглатывается).
- **Почему.** PG говорит «конверсия есть», а CH её не видит. Сеть при
  ретрае получает `duplicate`, исправления не будет. Исходящий S2S
  теряется молча.
- **Исправление:** одна транзакция PG, `INSERT postbacks` +
  `INSERT event_queue` + `INSERT postback_deliveries` (§11).

**D2 [HIGH]. Нет идемпотентности записи в CH.**

- **Где:** `chstore.EventStore.InsertBatch`: три последовательных insert'а
  (clicks → tracking → conversions). Ошибка во втором ведёт к `Requeue`
  всего батча, и clicks вставляются второй раз. Таймаут после фактического
  коммита в CH тоже даёт дубль. Таблицы — обычный `MergeTree`,
  `event_id` нет.
- **Исправление:** `event_id` (ULID) проставлять в `eventbuf.Enqueue` и
  хранить в payload, чтобы он был стабилен между ретраями.

```sql
ALTER TABLE click_events ADD COLUMN event_id String;
-- новая таблица / миграция: ENGINE = ReplacingMergeTree
-- ORDER BY (organization_id, event_at, campaign_id, click_id, event_id)
-- отчёты: SELECT ... FROM click_events FINAL  (или uniqExact(event_id) вместо count())
```

  Альтернатива дешевле: фиксировать состав батча при claim (`batch_id`),
  ретраить тот же состав с `insert_deduplication_token =
  batch_id||table` и `non_replicated_deduplication_window` на таблицах.

**D3 [MEDIUM]. Гонка progression** (§11).
**Статус: исправлено** (advisory lock + `clock_timestamp()`; R1 закрыт этим же).

**D4 [HIGH]. Валюта** (AN1).

**D5 [MEDIUM]. Timezone.** Raw-события в UTC (`DateTime64` +
`EventAt.UTC()`), бакетирование tz-aware. ✔ Cost по дате зоны рекламного
кабинета (AN4).

---

## 18. Failure Scenarios

| # | Сценарий | Что произойдёт | Критичность | Обнаружим? | Авто-восстановление? | Что изменить |
|---|---|---|---|---|---|---|
| 1 | PG недоступен 60 с | Все redirect'ы 502, клики потеряны; eventbuf выбрасывает батчи; postback'и 500 (сети ретраят) | CRITICAL | Частично (`tracking_requests_total{error}`), `/health` = ok | Да, после возврата PG; потерянные клики не вернуть | Кэш конфига + stale-on-error, retry/spill в eventbuf, readiness |
| 2 | PG медленный (5 с) | Redirect до 30 с, пул исчерпан, фактически outage | CRITICAL | Алерт p95 | Да | `statement_timeout`, deadline, кэш |
| 3 | Redis недоступен | Redirect без изменений, auth без лимита | LOW | Только по логам | Да | Метрика `redis_errors`, fail-closed для auth |
| 4 | Очередь (PG) переполнена или bloat | Медленный claim, растёт lag атрибуции → `unattributed` | HIGH | `event_queue_depth` | Частично | Autovacuum-тюнинг, затем брокер, отложенная атрибуция |
| 5 | Worker crash или деплой | Батч навсегда в `processing`: клики и конверсии не в отчётах, S2S не отправлены | CRITICAL | **Нет** | **Нет** | Reaper, `WithoutCancel`, graceful drain |
| 6 | Дубль webhook'а | Корректно `duplicate` | — | Да | — | — |
| 7 | Параллельные HOLD и ACCEPT | Возможен HOLD после ACCEPT | MEDIUM | Нет | Нет | Advisory lock |
| 8 | Всплеск трафика ×10 | Пул PG и eventbuf переполнены, события дропаются, latency растёт | HIGH | Алерты drop/p95 | Да, после спада | Кэш конфига, `pool_max_conns`, HPA |
| 9 | Бот-атака на `/t/*` | 6 SELECT на запрос, нагрузка PG, рост CH | HIGH | `tracking_requests_total` | Нет | Кэш + negative cache, edge rate-limit, бот-фильтры до логирования |
| 10 | Исчерпание соединений PG | API, tracker и worker делят один PG без лимитов в коде | HIGH | Нет метрик пула | Да | pgxpool-метрики, PgBouncer, лимиты per-binary |
| 11 | Медленный отчётный запрос CH | Конкурирует с атрибуцией postback'ов → таймауты → ретраи сетей | MEDIUM | `analytics_query_latency` | Да | CH-профили и квоты, CH1 |
| 12 | Таймаут внешнего API | Postback'и: backoff ✔; cost sync: зависание навсегда | MEDIUM | Нет | Нет (cost sync) | Таймауты клиентов |
| 13 | Сетевой сбой tracker ↔ CH на старте | `MemoryResolver` навсегда, все конверсии `unattributed` | HIGH | Только warn в логе | Нет | Lazy reconnect, 503 на postback |
| 14 | Диск заполнен (CH) | Insert'ы падают, батчи ретраятся, PG-очередь растёт, затем PG тоже упрётся в диск | HIGH | Нет алерта на диск | Нет | TTL, node-exporter + алерт 80% |
| 15 | Утечка памяти или OOM tracker'а | Рестарт: eventbuf (≤10k событий) теряется | MEDIUM | `up == 0` | Да (оркестратор) | Spill-файл, memory limit + алерт |
| 16 | Неудачный деплой (баг routing) | Весь трафик на fallback; откат образа | HIGH | `outcome=blocked` | Нет | Canary-реплика, алерт на долю blocked/fallback |
| 17 | Сломанная миграция PG | goose down существует; CH-миграции forward-only, применяются всеми бинарниками на старте | HIGH | Падение старта | Нет | Миграции отдельным job'ом до rollout, `schema_migrations` в CH |
| 18 | `revenue=NaN` от сети | Revenue в отчётах = NaN за весь период | MEDIUM | Нет | Нет (данные в CH) | Валидация, `sumIf(isFinite)` |
| 19 | FX-курс отсутствует | Выручка в не-USD выпадает из отчётов | HIGH | Нет | Нет | FX-job + backfill |

---

## 19. Scalability Review

### Stage 1: до 100 rps

Текущая архитектура подходит **после P0**:

- архитектура: 1 tracker, 1 worker, 1 api;
- PG: managed, PITR;
- CH: одна нода с бэкапом;
- Redis: одна нода;
- мониторинг: текущий набор плюс §14.

### Stage 2: 100–1 000 rps

- Кэш конфигурации в tracker'е (обязательно).
- 2–4 реплики tracker'а за LB.
- PG: `pool_max_conns` per-binary, PgBouncer (transaction mode) для api.
- `event_queue` в PG ещё нормальна, если тюнить autovacuum.
- 2 реплики worker'а.
- CH: `click_lookup` (CH1), TTL, отдельный профиль для отчётов.
- Мониторинг: метрики пула, возраст очередей, reconciliation.

### Stage 3: 1 000–10 000 rps

- Брокер вместо PG-очереди: **Redpanda/Kafka**, или NATS JetStream как
  более простой вариант. Tracker пишет батчами в топик `events`
  (`acks=1`, локальный spill при недоступности). Worker'ы — consumer group,
  бьющие в CH батчами по 10–50k (или ClickHouse Kafka engine).
- Conversion outbox остаётся в PG (объём конверсий на порядки меньше),
  relay outbox → топик.
- Tracker stateless, конфиг полностью в памяти, PG нужен только для промахов
  кэша.
- CH: 2 реплики (ReplicatedReplacingMergeTree + Keeper). Отчёты читают из
  MV (`daily_campaign` и др.); сырые запросы — только drill-down.
- Redis: только rate-limit и `LastStatus`.
- Мониторинг: consumer lag, SLO redirect p99.

### Stage 4: 10 000–100 000 rps

- Tracker в нескольких регионах (geo-DNS/anycast). Конфиг тянется push'ем
  (snapshot по версии) из control plane, PG на hot path отсутствует совсем.
- Kafka в каждом регионе с MirrorMaker в центральный регион, либо
  региональные CH-шарды.
- CH кластер: шардирование по `organization_id` (cityHash), 3+ шарда × 2
  реплики. Rollup'ы через MV; raw TTL 3–6 мес, cold на S3 (tiered storage).
- Bot-фильтрация до записи событий (дешёвые эвристики в памяти).
- Per-tenant quotas и fairness в consumer'ах.

Не усложнять раньше времени: до Stage 3 брокер не нужен, нужны кэш, outbox и
reaper.

---

## 20. Code Quality Review

**Сильные стороны:**

- последовательная структура пакетов;
- узкие интерфейсы (`Sink`, `ClickResolver`, `Store`, `Mapper`);
- DI через конструкторы;
- нет глобального состояния кроме метрик;
- ошибки оборачиваются с контекстом;
- RE2;
- нет мёртвого кода в горячих путях.

**Проблемы:**

- **CQ1 [LOW, но системно].** Комментарии длиннее кода и пересказывают
  историю фаз и аудитов (`tracker/main.go`, `worker/main.go`,
  `routingstore`). Они полезны как журнал решений, но за этим текстом
  теряются реальные контракты. Например, в `docs/attribution.md` прямо
  написано, что CH-down на старте даёт `unknown_click`: известный дефект
  задокументирован, но не исправлен. История решений должна жить в `docs/`,
  а в коде — инварианты.
- **CQ2.** Почти идентичные `eventqueue` и `postbacklog` (claim, requeue,
  delete, flusher) — это осознанное дублирование. Reaper и dead-letter
  придётся добавлять дважды. Стоит сделать общий
  `pgqueue.Queue[T]` (Go generics).
- **CQ3.** `conversion.Service.Record` смешивает валидацию, атрибуцию, FX,
  запись и три побочных эффекта. После outbox'а это станет «одна транзакция
  + worker».
- **CQ4.** `eventbuf.Close` не идемпотентен (RL3); `Requeue`/`Delete` на
  родительском ctx (Q1). Это один класс ошибок: «cleanup на отменённом
  контексте». Стоит проверить остальные места поиском по `defer`/`Close` и
  `ctx` в shutdown-путях.

**Наиболее проблемные участки:**

1. `apps/internal/eventqueue/flusher.go` + `postgres.go`
2. `apps/internal/conversion/conversion.go: Record`
3. `apps/internal/eventbuf/eventbuf.go: write`
4. `apps/internal/chstore/events.go: InsertBatch`
5. `apps/internal/chstore/attribution.go: ByClickID`
6. `apps/tracker/main.go` (shutdown, CH fallback)
7. `apps/internal/routingstore` (отсутствие кэша)

---

## 21. Critical Issues

| ID | Severity | Где | Кратко |
|---|---|---|---|
| Q1 | CRITICAL | `eventqueue`, `postback`, `postbacklog`: `ClaimDue` + `worker/main.go` | Зависшие `processing`, нет reaper'а, cleanup на отменённом ctx |
| D1 | CRITICAL | `conversion.Service.Record`, `postback/enqueuer.go:40` | Нет outbox: CPA-событие и S2S-доставка теряются молча |
| AN1 | CRITICAL* | `fx_rates`, `conversion.PostgresFX` | Курсы не заполняются, не-USD выручка и расход выпадают (*для клиентов с не-USD) |
| D2 | HIGH | `chstore.EventStore.InsertBatch` | Дубли в CH при частичном сбое и ретрае |
| RL1/A1 | HIGH | `routingstore`, `tracker/handler.go` | PG — SPOF redirect'а, 6 запросов без кэша и таймаута |
| RL2 | HIGH | `eventbuf.write` | Неудачный батч выбрасывается без retry |
| T1 | HIGH | `attribution`, `tracker/main.go:156` | Ранние конверсии и CH-down на старте → `unattributed` навсегда |
| CH1 | HIGH | `chstore.ByClickID` | Full-scan организации на каждый postback |
| DB1 | HIGH | `postbacks` | Нет индекса под `LastStatus` |
| DO1 | HIGH | — | Нет CI |
| DO2 | HIGH | — | Нет автоматизированных бэкапов и restore-drill |

---

## 22. Prioritized Remediation Plan

| Priority | Problem | Impact | Probability | Fix | Effort |
|---|---|---|---|---|---|
| **P0** | Q1: зависшие `processing` (3 очереди) | Потеря кликов, конверсий и S2S в отчётах | Высокая (каждый деплой) | `claimed_at` + reaper; `context.WithoutCancel` для Requeue/Delete/Mark*; `wg.Wait` в worker'е | 1 д |
| **P0** | D1: outbox для конверсий | PG ≠ CH, потеря S2S | Средняя | Одна транзакция: `postbacks` + `event_queue` + `postback_deliveries`; доставка с ошибкой → 500 | 1.5 д |
| **P0** | AN1: FX | Revenue и ROI неверны для не-USD | 100% при не-USD | FX-провайдер + daily job + «последний курс ≤ даты» + backfill | 2 д |
| **P0** | D2: идемпотентность CH | Завышение кликов и выручки | Средняя | `event_id` + ReplacingMergeTree, или стабильный батч + dedup token | 2 д |
| **P0** | RL2: retry в eventbuf | Потеря событий при сбое PG | Средняя | Retry с backoff, затем spill на диск | 1 д |
| **P0** | S3: валидация revenue | NaN ломает отчёты, poison-ретраи | Низкая | `isFinite`, диапазон | 0.25 д |
| **P0** | DO1: CI | Регрессии в деньгах | Высокая | GH Actions: go test (full env через compose), tsc, eslint, build, goose up/down | 1 д |
| **P1** | A1/RL1: кэш конфига + deadline + `statement_timeout` | Outage при деградации PG; потолок ~1k rps | Средняя | In-proc кэш, singleflight, stale-on-error, LISTEN/NOTIFY, negative cache | 2–3 д |
| **P1** | T1: отложенная атрибуция + reconnect CH | Потерянная атрибуция | Высокая при backlog | `pending_attribution` + retry в worker'е; lazy CH; 503 при недоступном CH | 2 д |
| **P1** | CH1: `click_lookup` | Postback-путь непригоден на 100М+ | 100% при росте | Таблица + MV, или bloom-индекс | 1 д |
| **P1** | DB1: индекс `postbacks` | То же для PG | 100% при росте | `CREATE INDEX CONCURRENTLY` | 0.1 д |
| **P1** | D3/R1: advisory lock для progression | HOLD после ACCEPT | Низкая–средняя | В той же транзакции, что D1 | 0.5 д |
| **P1** | DO2: бэкапы, PITR, restore-drill | Невосстановимая потеря | Низкая | Managed PITR + `BACKUP` CH в S3 + runbook | 1–2 д |
| **P1** | RL4: readiness tracker'а; RL3: `sync.Once` в Close | LB шлёт трафик на мёртвый инстанс; ложные падения | Средняя | `/ready`, `Once` | 0.25 д |
| **P1** | Метрики §14 (возраст очередей, пул, postback incoming, attribution outcome, reconciliation) | Слепые зоны | — | Prometheus + alerts | 1.5 д |
| **P1** | Q2: dead-letter `event_queue` | Ядовитый батч блокирует очередь | Низкая | `attempt_count` + split + dead | 1 д |
| **P2** | T5: HMAC sticky cookie | Обход фильтров, подмена `click_id` | Средняя | Подпись + валидация ULID | 0.5 д |
| **P2** | API1: атомарный rate-limit | Вечный 429 | Низкая | Lua / `EXPIRE NX` | 0.25 д |
| **P2** | Таймауты внешних HTTP-клиентов (`http.DefaultClient`) | Зависание cost sync | Низкая | `&http.Client{Timeout: 30s}` | 0.25 д |
| **P2** | CH3/DB2: TTL и партиционирование `postbacks` | Рост диска и стоимости | 100% со временем | TTL, партиции, retention job | 1 д |
| **P2** | CH4: профили и квоты CH, TLS | Отчёты душат атрибуцию | Средняя | Users/profiles, `max_execution_time` | 0.5 д |
| **P2** | T3: `event_time` в postback | Сдвиг конверсий и FX | Средняя | Опциональный параметр | 0.5 д |
| **P2** | DO4: CA-сертификаты в образе tracker'а | Не подключится к TLS-PG/Redis | Средняя | `apk add ca-certificates` | 0.1 д |
| **P2** | S2: CSP / frame-ancestors / HSTS | Clickjacking | Низкая | Заголовки в `next.config.ts` и API | 0.25 д |
| **P2** | T6: верификация доменов | Squatting | Низкая | TXT-challenge | 1 д |
| **P2** | CQ2: общий `pgqueue[T]` | Двойная поддержка | — | Рефакторинг | 1 д |
| **P3** | DB4: throttle `last_active_at` | Write amplification | — | `WHERE … < now()-1m` | 0.1 д |
| **P3** | T4/AN3: вычитание отмен | Завышенная выручка при chargeback'ах | По бизнесу | Решение по спецификации | 1 д |
| **P3** | Брокер вместо PG-очереди | Потолок ~2–3k rps | При росте | Stage 3 | 5–8 д |
| **P3** | Пагинация справочников, `/v1` | — | — | — | 1 д |

**P0 итого: ~8–9 дней. P1: ~11–13 дней.**

---

## 23. Production Readiness Score

| Область | Оценка | Почему |
|---|---:|---|
| Architecture | **7/10** | Правильный монолит, правильные хранилища, чистые границы; минус за PG на hot path и отсутствие outbox |
| Tracking correctness | **4/10** | Routing и дедуп сделаны хорошо, но есть пути потери атрибуции и событий, дубли в CH и неподписанная sticky cookie |
| Database | **5/10** | Хорошая схема PG; нет индекса под горячий запрос, CH-ключ не подходит под point lookup, нет TTL и retention |
| Performance | **6/10** | Hot path быстрый на loopback; потолок ~1k rps на одном PG без кэша; postback-путь деградирует линейно |
| Security | **7/10** | Выше среднего: tenancy, SSRF, хэши, bcrypt, RBAC. Минусы: sticky, NaN, заголовки, rate-limit race |
| Reliability | **3/10** | Q1, RL1, RL2, CH-reconnect: система не переживает штатный деплой и blip БД без потерь |
| Observability | **6/10** | Метрики, трейсинг и алерты есть, но слепы к зависанию очередей, атрибуции и пулу |
| Testing | **5/10** | Объём хороший, интеграционные тесты реальные; нет тестов на сбои и гонки, нет нагрузки в реальной сети, нет CI |
| DevOps | **3/10** | Dockerfiles хорошие; нет CI, нет автоматических бэкапов, нет манифестов оркестрации |
| Code quality | **7/10** | Читаемо и консистентно; перегруженные комментарии, дублированные очереди |
| **Итого** | **53/100** | |

---

## 24. Final CTO Verdict

### Можно ли запускать это в production прямо сейчас?

## **NO**

**Почему.** Для трекера главное свойство — **каждый клик и каждый доллар
учтены ровно один раз**. Сейчас это не так:

- плановый деплой worker'а теряет пачку событий и исходящих postback'ов, и
  никто об этом не узнает (Q1);
- конверсия может оказаться в PG и не оказаться в отчётах, без возможности
  исправить ретраем (D1);
- любой клиент с не-USD сетью видит неверную выручку с первого дня (AN1);
- частичный сбой ClickHouse завышает цифры (D2);
- деградация Postgres останавливает весь платный трафик (RL1).

**Переписывать ничего не нужно.** Архитектура выбрана верно, код чистый.
Все P0 — локальные изменения в 6–7 файлах плюс 2–3 миграции. Это примерно
полторы-две недели одного инженера.

**Условия для «YES, WITH CONDITIONS» (до ~1 000 rps, ограниченный круг
клиентов):**

1. Закрыты все P0 из §22, на каждый есть тест из §16.
2. Кэш конфигурации tracker'а с stale-on-error и `statement_timeout`
   (P1, A1/RL1).
3. Добавлены метрики `queue_oldest_age`, `processing_stuck`,
   `attribution_outcome` и `reconciliation_diff` с алертами.
4. PITR для PG и бэкап CH проверены восстановлением.
5. Нагрузочный тест 1k rps, 30 минут, **в целевой сети**, со сверкой
   `requests == click_events` и `postbacks success == conversion_events`.
