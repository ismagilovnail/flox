# Аудит FLOX — продукт, данные, безопасность, рынок (2026-10-15)

Проверялся закоммиченный код (HEAD `63fa3e2`) в отдельном git worktree, без
незакоммиченных правок по видео в галерее. Код не менялся. api/tracker/worker
собраны из HEAD и запущены на хосте против dev-стека; по живому стеку прошли
сценарии postback, гонки, изоляции тенантов, CSRF и нагрузки; интерфейс
просмотрен в браузере. Цены конкурентов — из сторонних агрегаторов, не сверены
с официальными сайтами (ссылки в конце).

Трекер статусов по находкам из этого отчёта — интерактивный дашборд
(ссылка в описании коммита / у владельца).

---

## A. Executive Summary

**Что это за продукт.** Облачный (multi-tenant) трекер для арбитража с PWA и
push. Объединяет:

- маршрутизацию уровня Keitaro: стрим-сеты, фильтры, веса, sticky, симулятор;
- конструктор PWA, лендингов и прелендингов;
- push и Telegram;
- учёт CPA-статусов HOLD/ACCEPT/REDEP/DECLINE и LTV;
- импорт расходов из FB, TikTok и Google Ads.

Целевой сегмент — iGaming/CPA-команды, которые льют FB/TikTok-трафик через PWA.
Ближайший аналог по составу — Adset.Pro, а не Binom или BeMob.

**Стадия.** Инженерно зрелый: 60K строк Go, 42K строк тестов, 44K строк TS,
86 миграций, всё зелёное. Коммерчески pre-launch: биллинга нет, в production
не развёрнут, нет подтверждённых клиентов и нагрузочных замеров на реальной
инфраструктуре.

**Пять главных рисков**

1. **Критические уязвимости во фронтенде.** Next.js 16.3.0 — advisory с RCE в
   Image Optimization (AVIF) и `next/og`, SSRF; `next/image` используется в
   4 местах. Плюс 12 достижимых уязвимостей в Go stdlib, `x/net`, `grpc`.
2. **Выручка без валюты молча обнуляется.** Подтверждено вживую: FTD на $50 без
   `currency` не вошёл в Revenue/ROI, предупреждения нет.
3. **Нет биллинга, тарифов, лимитов и учёта потребления.**
4. **Онбординг postback.** Готовый входящий URL нигде не собирается; секрет
   показывается один раз; имена параметров пользователь должен знать сам.
5. **Не проверено в бою.** Нет production, CI, restore-drill; нагрузка только на
   loopback; GDPR: IP+UA хранятся бессрочно, удаления и экспорта нет.

**Пять сильных сторон**

1. Честная денежная модель: дедупликация (click_id, status, event_ref), запрет
   отката в HOLD, сторно через DECLINE — подтверждено вживую, включая гонку
   40 параллельных postback'ов.
2. Трекер + PWA + push + Telegram в одном облаке.
3. Объяснимая маршрутизация: симулятор поверх той же Go-логики.
4. Нативный CPA-жизненный цикл и LTV/когорты для iGaming.
5. Быстрый горячий путь: 0 потерь на 30 000 кликов при 3000 rps, p99 0,24 мс
   (loopback).

**Вывод.** Брать деньги с клиентов сейчас нельзя: уязвимость №1, денежный
дефект №2, нет биллинга. После P0 (оценка 1–3 недели) — закрытая бета на
5–10 команд с ручным выставлением счетов.

---

## B. Product Reality Map

| Функция | Статус | Доказательства | Ограничения |
|---|---|---|---|
| Клик → маршрутизация → редирект | PASS | живой клик 302; e2e `TestFullFunnel`, `TestFilterMatrix` | — |
| Sticky, взвешенный выбор потока | PASS (тесты) | `tracker/sticky_test.go`, `internal/routing` | живьём не проверял |
| Postback: дедупликация, прогрессия | PASS | 14 живых сценариев, гонка 40→1 | REDEP без `txn_id` = дубль |
| Сторно (DECLINE) | PASS | ClickHouse: +100 / −100 | ACCEPT после DECLINE невозможен |
| FX по дате события | PASS | EUR 30 → 33.62 USD | без `currency` → USD=0 (FAIL) |
| Отчёты revenue/profit/ROI | PARTIAL | ROI = null без расходов; HOLD не в выручке | дефект валюты |
| Unique clicks | PARTIAL | `chstore/report.go:452`: кампания+IP+UA | гипотеза: CGNAT |
| Изоляция тенантов | PASS | IDOR → 404; `api/tenant_isolation_routes_test.go` | — |
| CSRF / сессия | PASS | чужой Origin → 403; HttpOnly + Lax | — |
| Импорт расходов FB/TikTok/Google | NOT VERIFIED | `adaccount/*` | нет реальных токенов |
| Пиксели FB CAPI / TikTok | NOT VERIFIED | `pixelsend/providers.go` | то же |
| PWA / лендинги / postlanding | PASS (тесты) | `tracker/pwa_test.go`, `landing_test.go` | установка на устройстве не проверялась |
| Push, Telegram | NOT VERIFIED | код и тесты | внешние сервисы не настроены |
| Биллинг / тарифы / квоты | NOT IMPLEMENTED | — | — |
| Онбординг-визард, тестовая конверсия | NOT IMPLEMENTED | — | есть replay журнала |
| OpenAPI | NOT IMPLEMENTED | — | API-ключи есть |
| Импорт из Keitaro/Binom, шаблоны партнёрок | NOT IMPLEMENTED | — | пресеты макросов FB/TikTok |
| GDPR: удаление, экспорт, анонимизация IP | NOT IMPLEMENTED | `docs/retention.md` | — |
| Production, CI, restore-drill | NOT IMPLEMENTED | `ROADMAP.md` | k8s-манифесты есть |

---

## C. Customer Fit

| Клиент | Соответствие | Почему откажется | Что исправить |
|---|---|---|---|
| Начинающий арбитражник | низко-среднее | сам собирает postback URL; нет визарда и тестовой конверсии; сложные термины | визард «источник → оффер → ссылка → тестовый клик → тестовый postback» |
| Опытный байер | высокое по логике | выручка без валюты = 0; нет импорта из текущего трекера; нет истории | валюта по умолчанию; импорт; статус-пейдж |
| Руководитель команды | среднее-высокое (6 ролей, RBAC, аудит, алерты) | нет per-user scoping (гипотеза); нет отчёта по байерам | scoping и отчёт по сотрудникам |
| Партнёрка / рекламодатель | среднее | нет кабинета партнёра и выплат | не целевой сегмент |
| Технарь клиента | среднее | нет OpenAPI, sandbox, исходящих вебхуков | OpenAPI, «Test postback» |

---

## D. Technical Audit

| # | Проблема | Severity | Доказательство | Решение |
|---|---|---|---|---|
| T1 | Next.js 16.3.0 (RCE AVIF, RCE `next/og`, SSRF, cache poisoning); sharp < 0.35.5 | Critical | `npm audit`: 1 critical, 3 high | `next@16.4.1`, sharp |
| T2 | Go 1.26.6, `x/net` 0.58, `grpc` 1.83.0 — 12 достижимых уязвимостей | High | govulncheck | Go 1.26.9, `x/net` ≥ 0.60, `grpc` ≥ 1.83.1 |
| T3 | Выручка без `currency` → $0 в Revenue/ROI/LTV | High | живой тест: 65.62 вместо 115.62 | валюта по умолчанию у сети + предупреждение |
| T4 | Нет биллинга, квот, метринга | High (бизнес) | поиск по коду | метринг → лимиты → платёжный провайдер |
| T5 | Входящий postback URL не показан в UI | High (UX→деньги) | `PostbackSecretDialog` | готовый URL с макросами на странице сети |
| T6 | Исходящий `postbackUrl` обязателен | Medium | API 422 | необязательное поле |
| T7 | Ошибка FK при удалении сети — «referenced by flows» | Low | `network/repository.go:147` (держит `postbacks`) | различать constraint'ы, предлагать архив |
| T8 | Тест изоляции оставляет организации в dev-БД | Low | 500 «Isolation …» | cleanup |
| T9 | События клика в памяти до flush | Low (гипотеза) | `eventbuf.go` | измерить окно / WAL |
| T10 | Нагрузка только на loopback | Medium | `docs/performance.md` | staging-тест |
| T11 | Нет CI | Medium | решение владельца | на усмотрение владельца |

---

## E. Data Accuracy

**Работает правильно (живые тесты):** дубль ACCEPT → `duplicate`; HOLD после
ACCEPT → `ignored`; REDEP с разными `txn_id` — оба, с одинаковым — дубль;
40 параллельных ACCEPT → 1 запись; DECLINE → −100; отрицательная сумма и
`event_time` из будущего → 400; неизвестный клик → `unknown_click` с
повторной атрибуцией (~7 ч); 36 000 кликов → 36 000 строк.

**Риски:**

1. [High, подтверждено] выручка без валюты = 0 (T3).
2. [Medium] REDEP без `txn_id` — повторные депозиты теряются; с
   `acceptDuplicates` ретраи двоят выручку.
3. [Medium] повторное одобрение после DECLINE невозможно.
4. [Medium, гипотеза] unique clicks = кампания+IP+UA — склейка на CGNAT.
5. [Low] неизвестный статус → HTTP 200 `"error"`; сеть не повторит, нужен replay.
6. Часовые пояса: отчёты — пояс организации, расходы — пояс аккаунта (живьём не проверялось).
7. Восстановление: reaper, переатрибуция, ежечасная сверка; restore на production не проводился.

---

## F. Security Audit

**Подтверждено:** T1 (Critical), T2 (High).

**Без проблем:** IDOR между тенантами (404), CSRF (403), без сессии (401),
секрет postback в постоянное время, троттлинг отказов 30/10 мин, bcrypt,
HttpOnly + Lax, RE2.

**Потенциальные риски:** секрет postback в query string (логи прокси/CDN);
`TRUSTED_PROXIES` по умолчанию; нет rate limit на `/t/*` в приложении.

**GDPR (не юридическое заключение):** IP+UA в `click_events` бессрочно, нет
анонимизации, удаления организации/аккаунта и DSAR-экспорта; нужен DPA.

**Не проверялось:** XSS в ZIP-лендингах и изоляция их origin, SSRF исходящих
postback, логи tracker/edge, ротация ключей шифрования.

---

## G. Competitor Gap Analysis

| | FLOX | Keitaro | Binom | RedTrack | BeMob | Adset.Pro |
|---|---|---|---|---|---|---|
| Модель | облако | в основном self-hosted | self-hosted + Cloud | SaaS | SaaS | SaaS |
| Ориентир цены | нет | от ~€49/мес | ~$83–149, Cloud ~$199–299 | ~$69–149 на входе | free до 100K событий, от ~$49 | usage-based, есть free |
| PWA / push | есть | нет (по моим данным) | нет (по моим данным) | нет | нет | есть |
| CPA-статусы / LTV | глубоко | частично | частично | есть | базово | не проверено |
| Шаблоны партнёрок | нет | много | много | очень много | много | не проверено |
| История на рынке | нет | годы | годы | годы | годы | есть |

Прямой конкурент — Adset.Pro; частичные — Keitaro, Binom, RedTrack; заменитель —
«Keitaro + PWA-сервис + push-сервис». Уникальность против Adset.Pro не доказана.

---

## H. Commercial Readiness

- **10 клиентов:** P0, production с мониторингом и restore-drill, ручные счета,
  сопровождение настройки postback, метрика «время до первой конверсии».
- **100 клиентов:** биллинг с метрингом, визард, шаблоны 10–20 партнёрок, импорт
  из Keitaro/Binom, OpenAPI, статус-пейдж, GDPR, CI.
- **1 000 клиентов:** квоты и изоляция тяжёлых тенантов в ClickHouse,
  multi-region, self-serve, партнёрская программа.

**Unit economics:** данных нет. Себестоимость ≈ события × (байт строки ×
цена хранения × срок + CPU записи) + доля фиксированной инфраструктуры +
поддержка. Измерить: размер строки `click_events` (`system.parts`), CPU на
1 000 rps на staging, время поддержки на онбординг.

---

## I. Prioritized Roadmap

1. **До первых клиентов (P0):** T1+T2; T3; T5+T6; production,
   `TRUSTED_PROXIES`, restore-drill, staging-нагрузка.
2. **До 100 (P1):** биллинг и квоты; визард и «Test postback»; диагностика
   «REDEP без txn_id» / «revenue без currency»; повторное одобрение после
   DECLINE; шаблоны партнёрок, импорт; OpenAPI; GDPR.
3. **До 1 000 (P2):** квоты ClickHouse по тенантам; проверка уников на
   реальном трафике; отчёт по байерам, per-user scoping; CI (решение владельца).
4. **Дальше (P3):** multi-region; WAL буфера трекера; транскодинг видео,
   Google Play.

---

## J. Final Verdict

1. **Кто:** iGaming/CPA-команды 3–20 байеров, FB/TikTok через PWA.
2. **За что платят:** один сервис вместо трёх и честные цифры HOLD→ACCEPT→REDEP/LTV.
3. **Почему выберут:** гипотеза «трекер уровня Keitaro + PWA уровня Adset» — доказать на бете.
4. **Что мешает:** биллинг, уязвимости, обнуление выручки, сложный postback, нет production и истории.
5. **Три изменения:** готовый postback URL + шаблоны; валюта по умолчанию + предупреждение; биллинг с метрингом.
6. **Нельзя игнорировать:** T1/T2, T3, GDPR.
7. **Нужные данные:** staging-нагрузка; стоимость хранения; 5–10 интервью; официальные цены и ручное сравнение с Adset.Pro; логи tracker/edge.

---

## Использованные проверки

- `go build`, `go vet` — OK; `go test` (полный env) — 86 пакетов ok, 1 208 тестов, exit 0.
- e2e против живых сервисов — 4/4 PASS.
- `tsc` — 0 ошибок; eslint — 0 ошибок, 1 предупреждение; vitest — 158/158.
- npm audit — 1 critical, 3 high; govulncheck — 12 достижимых.
- vegeta: 300 rps × 20 с и 3 000 rps × 10 с — все 302, 0 потерь.

Код: `apps/tracker/handler.go`, `apps/tracker/postback.go`,
`apps/internal/conversion/conversion.go`, `apps/internal/eventbuf/eventbuf.go`,
`apps/internal/eventstream/sink.go`, `apps/internal/chstore/report.go`,
`apps/internal/auth/handler.go`, `apps/internal/network/repository.go`,
`apps/web/src/features/networks/*`.

Источники по конкурентам: [G2 Keitaro](https://www.g2.com/products/keitaro-tracker/pricing) ·
[affninja Binom](https://affninja.com/binom-pricing/) ·
[affinco Binom](https://affinco.com/binom-pricing/) ·
[RedTrack pricing](https://cloudfront-80d18e61f0e6c55c.redtrack.io/pricing/) ·
[toolradar RedTrack](https://toolradar.com/tools/redtrack/pricing) ·
[affninja BeMob](https://affninja.com/bemob-pricing/) ·
[afflift BeMob](https://afflift.com/f/threads/updates-to-bemob-basic-free-plan-its-not-bad.8379/) ·
[Capterra Adset.Pro](https://www.capterra.com/p/10050670/Adset-Pro/) ·
[GetApp Adset.Pro](https://www.getapp.com/all-software/a/adset-pro/)
