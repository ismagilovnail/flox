# Предложения правок к FLOX-master-prompt-v3.md

**Статус:** предложение, НЕ применено. Правки в спеку и в CLAUDE.md не внесены.
**Дата:** 2026-08-13
**Контекст:** перед стартом PHASE 22 — Attribution.
**Источник:** разбор `reference/` (сторонний трекер «adstracker», ~16.5k строк Go),
см. отчёт в истории сессии. Ниже — только те находки, которые затрагивают
**спеку**, а не реализацию: если их не зафиксировать до Phase 22–23, они станут
миграцией живых денежных данных.

Каждая правка приведена как точный before/after по текущему тексту спеки с
номерами строк на момент написания.

---

## Резюме

| № | Правка | Раздел | Тип | Риск невнесения |
|---|---|---|---|---|
| A1 | Ключ дедупа — трёхчастный, с явной ролью txn id | §45 + CLAUDE.md #3 + §59 | разночтение | Потерянные повторные депозиты **или** задвоенные инвойсы |
| A2 | Запрет отката статуса конверсии назад в HOLD | §45 (новый блок) + §59 | пробел | Ночные реплеи партнёров вынимают выручку из закрытых отчётов |
| A3 | Weighted pick детерминирован по ключу визита, а не по ГСЧ | §38 + §39 + §58 | уточнение | Реплики трекера расходятся в решениях; A/B-данные шумят |

A1 и A2 — блокирующие для Phase 23 (Conversion Engine). A3 — блокирующая для
Phase 22 только в части фикстуры (§6-SHARED); саму реализацию можно поменять и
позже, но фикстуру дешевле зафиксировать сейчас.

---

## A1. Ключ дедупа постбэка: сделать трёхчастным явно

### Проблема

Спека противоречит сама себе. Нормативная строка (§45, строка 2354):

```text
DEDUP KEY: (click_id, status)  — NOT click_id alone.
```

а поясняющий абзац сразу под ней говорит уже о другом ключе:

```text
Redeposits are distinguished by an additional event identifier (network txn id
if provided, else a monotonic sequence), so N distinct redeposits are N events,
but a re-sent identical one is dropped.
```

`CLAUDE.md` инвариант #3 повторяет только **первую** формулировку:

```text
3. **Postback dedup key = (click_id, status)** (§45), NOT click_id alone.
```

CLAUDE.md читается в начале каждой сессии, §45 — только перед своей фазой.
Реализовано будет по двухчастному ключу, и тогда второй депозит одного клика
(`CPA_REDEP` №2) молча схлопнется в первый. Это прямая недоплата клиенту.

Вторая проблема — в самом пояснении. «Else a monotonic sequence» **отключает
дедуп полностью**: если сеть не прислала txn id и мы подставляем свой
инкремент, то каждый повтор одного и того же постбэка получает новый номер и
записывается как новая конверсия. Ошибка направлена в сторону задвоения, то
есть в сторону неверного инвойса.

Третья — не сказано, что txn id участвует в ключе **только для повторяемых
статусов**. Если сеть ретраит один и тот же `CPA_ACCEPT`, подставляя каждый раз
свежий txn id (так делают), то ключ с txn id для ACCEPT снова означает отсутствие
дедупа. В `reference/pkg/conversion/status.go` txn id добавляется к ключу при
любом статусе — это их недоработка, повторять её не надо.

### Правка

**§45, строки 2354–2360.**

Было:

```text
DEDUP KEY: (click_id, status)  — NOT click_id alone.
  Rationale: the same click legitimately produces CPA_HOLD, then CPA_ACCEPT,
  then multiple CPA_REDEP. Dedup on click_id alone would drop the deposit after
  the registration. Redeposits are distinguished by an additional event
  identifier (network txn id if provided, else a monotonic sequence), so N
  distinct redeposits are N events, but a re-sent identical one is dropped.
```

Стало:

```text
DEDUP KEY: (click_id, status, event_ref)
  — NOT click_id alone, and NOT (click_id, status) alone.

  Rationale: the same click legitimately produces CPA_HOLD, then CPA_ACCEPT,
  then multiple CPA_REDEP. Dedup on click_id alone would drop the deposit after
  the registration; dedup on (click_id, status) would drop every redeposit
  after the first.

  event_ref is defined by the status, not by what the network happened to send:

    REPEATABLE STATUS (CPA_REDEP only)
      event_ref = the network's transaction id.
      A second deposit by the same user is a second conversion, and the txn id
      is the only thing that tells it apart from a re-send of the first.

      Network sends no txn id → event_ref = "" → exactly ONE redeposit is
      recorded per click. This is deliberate: a missed redeposit is a support
      ticket, a double-counted one is an incorrect invoice, so the failure is
      aimed at the recoverable side. Do NOT substitute a locally generated
      sequence number — a fresh number per delivery makes every re-send look
      distinct and disables deduplication entirely.

    NON-REPEATABLE STATUSES (CPA_HOLD / CPA_ACCEPT / CPA_DECLINE / CPA_TRASH)
      event_ref = "" ALWAYS, even when the network sends a transaction id.
      Networks commonly retry with a fresh txn id per attempt; including it
      here would turn every retry into a new conversion.
      The txn id is still STORED on the event — it is just not part of the key.
```

**CLAUDE.md, инвариант #3.**

Было:

```text
3. **Postback dedup key = (click_id, status)** (§45), NOT click_id alone. Long
   Redis TTL + durable DB unique constraint. Store original currency + USD value
   at event time. `acceptDuplicates` override per network.
```

Стало:

```text
3. **Postback dedup key = (click_id, status, event_ref)** (§45) — NOT click_id
   alone, and NOT (click_id, status). `event_ref` = network txn id for
   CPA_REDEP (the only repeatable status), empty string for every other status
   even if a txn id was sent. Long Redis TTL + durable DB unique constraint.
   Store original currency + USD value at event time. `acceptDuplicates`
   override per network.
```

**§59 (Conversion test cases), строка 2889.**

Было:

```text
duplicate conversion (same click_id + status → dropped)
```

Стало:

```text
duplicate conversion, non-repeatable status (same click_id + status → dropped,
  including when the network sends a different txn id on the retry)
duplicate conversion, repeatable status (same click_id + REDEP + same txn id
  → dropped)
distinct redeposits (same click_id + REDEP + different txn ids → all recorded)
redeposits with no txn id (same click_id + REDEP, network sends none
  → exactly one recorded, not N)
```

### Влияние на реализацию

- Уникальный constraint в PG и `ORDER BY` в ClickHouse должны включать
  `transaction_id` — иначе бэкстоп противоречит быстрому пути. Ср.
  `reference/migrations/clickhouse/0004_conversions.sql`:
  `ORDER BY (team_id, campaign_id, click_id, status, transaction_id)`.
- Redis-ключ: `conv:{click_id}:{status}[:{txn}]`.
- Фаза 23 не начата, кода на выброс нет.

---

## A2. Запрет отката статуса конверсии назад в HOLD

### Проблема

В спеке **нет ни одного правила о порядке статусов**. Проверено grep'ом по
`transition`, `regress`, `re-send`, `out of order` — совпадений по существу нет.
§59 требует, чтобы последовательность `HOLD → ACCEPT → REDEP` записывалась
целиком, но нигде не сказано, что делать с последовательностью, пришедшей
**не в том порядке**.

А приходит она не в том порядке регулярно. Партнёрские сети переигрывают весь
день ночным джобом и заново шлют исходный `HOLD` — уже после того, как
конверсия была одобрена. По ключу из A1 этот `HOLD` не дубликат (у него другой
статус, ключ свободен), значит он будет записан. Результат: конверсия
возвращается в pending, выручка исчезает из уже показанного клиенту отчёта за
вчера, а причина не логируется нигде, потому что формально всё отработало
штатно.

Это не гипотеза: в `reference/pkg/conversion/status.go` под это заведена
отдельная функция `Follows(last, next)` с комментарием ровно про ночной реплей.

### Правка

**§45, новый блок сразу после блока `## DEDUPLICATION`** (перед строкой
`Deduplicate. Log every postback...`, строка 2376):

````markdown
## STATUS PROGRESSION (order-independence — money correctness)

Postbacks arrive out of order. Networks replay a whole day on a nightly job and
re-send the original CPA_HOLD hours after the conversion was approved. That
re-send is not a duplicate under the dedup key (§45 above) — the status differs,
so the key is free — and recording it would move an approved conversion back to
pending, removing revenue from a report the client has already seen. Nothing
would be logged, because formally every step succeeded.

```text
RULE: the only refused transition is BACK TO CPA_HOLD.

  last = "" (no prior status)            → accept
  last = next (same status again)        → handled by the dedup key, not here
  next = CPA_HOLD and last != CPA_HOLD   → REFUSE, record as outcome=ignored
  everything else                        → accept

Everything else is allowed on purpose: approvals really are reversed
(chargebacks → CPA_DECLINE after CPA_ACCEPT) and reversals really are undone
(CPA_ACCEPT after CPA_DECLINE). Only the return to "not decided yet" is
meaningless.

STORAGE: the last seen status per click_id lives next to the dedup keys
  (Redis, same long TTL) so the check costs one lookup on a path that is
  already doing one.

REDIS UNAVAILABLE: fall through and record the event. A missing progression
  check is a wrong report; a refused postback is a lost conversion. Never lose
  the conversion.

acceptDuplicates DOES NOT bypass this rule. That flag is about duplicate
  deliveries, not about time travel.
```

A refused postback is still logged in full (§45 postback log) with
outcome=ignored, so "the network says it re-sent, where did it go" stays
answerable.
````

**§59 (Conversion test cases)** — добавить:

```text
late HOLD after ACCEPT (nightly replay → ignored, conversion stays ACCEPT)
late HOLD after ACCEPT with Redis down (→ recorded, never lost)
chargeback (ACCEPT then DECLINE → recorded, revenue reversed)
chargeback undone (DECLINE then ACCEPT → recorded)
refused postback is still visible in the postback log with outcome=ignored
```

### Влияние на реализацию

Одна функция в `apps/internal/conversion` + один Redis-ключ. Дешевле любой
альтернативы: без правила расхождение обнаруживается через недели, когда клиент
сверяет отчёт с инвойсом сети, и восстановить корректную историю уже нечем.

---

## A3. Weighted pick детерминирован по ключу визита, а не по ГСЧ

### Проблема

Спека требует детерминизма, но не говорит, откуда он берётся. §38, строка 2062:

```text
Routing must be deterministic where configuration requires deterministic behavior.
```

«Where configuration requires» читается как «когда включён sticky». Текущая
реализация `apps/internal/routing/weighted.go` соответствует буквально:
`pickWeighted(flows []Flow, rand01 func() float64)` — жеребьёвка случайная,
детерминизм обеспечивается только sticky-cookie **после** первого ответа.

Что из этого следует:

1. **Первый выбор не воспроизводим.** Повторно проигранный запрос (ретрай
   клиента, префетч, дубль от сети) до установки cookie попадает в другой flow.
   Посетитель видит вторую страницу, а конверсия оказывается атрибуцируема двум
   вариантам сразу.
2. **Реплики расходятся.** Два инстанса трекера за одним балансировщиком не
   договариваются ни о чём; рестарт перетасовывает распределение.
3. **Фикстура §6-SHARED усложняется.** Единственную conformance-фикстуру
   (`inputs → expected route decisions`, объявленную «core correctness
   guarantee») приходится строить вокруг инжектируемого ГСЧ. `expected` при
   случайном выборе — это либо фиксированный seed (проверяет реализацию ГСЧ, а
   не роутинг), либо статистика (не даёт точного равенства). При
   детерминированной функции фикстура становится обычной таблицей.

Решение из `reference/services/tracker/internal/rotate/rotate.go`: брать не
случайное число, а хэш от стабильного признака визита. Оба свойства держатся
одновременно — хороший хэш равномерен, значит доли сходятся к весам; хэш —
чистая функция, значит реплей попадает туда же. Там же обосновано, почему
именно FNV-1a, а не `hash/maphash`: последний засеивается случайно на процесс,
и две реплики разошлись бы ровно так же, как при ГСЧ.

Отдельно: sticky-cookie это **не отменяет** (инвариант #4 в силе — cookie
остаётся источником правды). Хэш решает, что происходит **до** cookie и при её
потере, где сейчас нет ничего.

### Правка

**§38, строка 2062.**

Было:

```text
Routing must be deterministic where configuration requires deterministic behavior.
```

Стало:

````text
Routing must be deterministic ALWAYS, not only when sticky is enabled.

The same request must resolve to the same flow on every replica and after every
restart. Weighted selection therefore draws from a hash of a stable property of
the visit, never from a random number generator:

```text
pickWeighted(flows, key) where key is derived from the visit
  (click_id when already minted, otherwise a stable fingerprint of the request)

  * uniform hash → observed shares converge to configured weights (§58: within
    2% over 10k picks)
  * pure function → a replayed request lands exactly where the original did

Use a fixed, unseeded hash (FNV-1a 64). NOT hash/maphash: it is seeded randomly
per process, so two tracker replicas behind one load balancer would disagree
about the same visit and a restart would re-bucket every visitor.

Flows with weight <= 0 are SKIPPED, not clamped: pausing one flow must not
require re-balancing the others before traffic stops reaching it.

Shares are relative to the sum of the weights actually in play after filtering,
not to 100 — see §58 "eligibility before the draw".
```

This is independent of sticky (§39-STICKY). The cookie remains the source of
truth for a returning visitor; the hash decides what happens BEFORE a cookie
exists and if it is lost.
````

**§39 (Routing engine order), строка 2097.**

Было:

```text
apply weighted selection (pickWeighted)
```

Стало:

```text
apply weighted selection (pickWeighted — deterministic by visit key, §38)
```

**§58 (Routing test cases)** — заменить одну строку и добавить две.

Было:

```text
weighted routing (distribution within 2% of configured weights over 10k picks)
```

Стало:

```text
weighted routing, distribution (within 2% of configured weights over 10k
  distinct visit keys)
weighted routing, determinism (the same visit key resolves to the same flow
  across repeated calls, across engine instances, and across process restarts)
weighted routing, eligibility before the draw (of two flows at 50/50 where one
  is US-only, ALL non-US traffic goes to the other one — it does not half
  disappear into the fallback)
weighted routing, zero and negative weights (skipped, not clamped; remaining
  flows split the traffic between themselves)
```

### Влияние на реализацию

- `apps/internal/routing/weighted.go`: сигнатура меняется с
  `rand01 func() float64` на `key string`. ~50 строк, тесты рядом.
- `apps/internal/routing/fixture_test.go` (397 строк, самый большой файл в
  Go-части) — фикстура упрощается: пропадает инжекция ГСЧ.
- `apps/web/src/lib/routing-simulate.ts` содержит зеркало `pickWeightedFlow` и
  после этой правки перестаёт соответствовать движку. **Новой дивергенции это
  не создаёт**: файл с самого начала помечен как временный мок контракта
  `POST /routing/simulate`, удаляемый в Phase 27 («once Phase 19 ports the
  decision logic to Go, this file is replaced, not kept running alongside it»).
  Правка лишь делает срок его жизни жёстким. Если Phase 27 далеко — проще
  выровнять мок сразу, чем оставлять симулятор врущим про распределение.

### Замечание про «eligibility before the draw»

Формулировка в §58 выше — отдельная семантическая находка из
`reference/.../httpapi/click.go` (`pickDestination`), и она стоит фиксации
независимо от того, примете ли вы хэш вместо ГСЧ. Кандидатов надо отсеивать
фильтрами **до** жеребьёвки, а не тянуть жребий и потом проверять. Иначе веса
означают не то, что ввёл оператор: при 50/50 с US-only вариантом половина
не-US трафика уходит в fallback вместо того, чтобы целиком достаться второму
варианту. То же касается правила «flow, который прошёл фильтры, но не имеет ни
одного подходящего destination, не останавливает перебор» — иначе трафик,
который ловил следующий stream set, молча выбрасывается.

---

## Порядок применения

1. A1 и A2 — до начала Phase 23. Обе меняют форму ключей и уникальных
   ограничений; после того как в ClickHouse лягут живые конверсии, это миграция
   денежных данных.
2. A3 — до того, как фикстура §6-SHARED будет объявлена замороженной.
3. Правки в `CLAUDE.md` (только инвариант #3) вносить одновременно с §45,
   иначе разночтение просто переедет.

Если правка отклоняется — стоит записать причину прямо в §45/§38, потому что
все три вопроса всплывут снова при первом же расхождении отчёта с инвойсом
партнёра.

## Что НЕ предлагается

Остальные находки из `reference/` (generic batch writer с retry, negative TTL +
singleflight в кэше конфигов, три таблицы money-path, алиасы статусов сетей,
защиты горячего пути) — это реализация, а не спека. Их можно забирать по ходу
соответствующих фаз, ничего в `FLOX-master-prompt-v3.md` не меняя.
