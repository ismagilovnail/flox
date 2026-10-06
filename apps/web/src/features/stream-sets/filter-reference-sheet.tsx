"use client";

import { useTranslation } from "react-i18next";

import { Mono } from "@/components/ui/typography";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { FILTER_OPERATOR_I18N_KEY, FILTER_OPERATORS, type FilterOperator } from "@/lib/filters";

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      <div className="flex flex-col gap-2 text-sm text-muted-foreground">{children}</div>
    </section>
  );
}

function FieldEntry({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 border-l-2 border-border py-0.5 pl-3">
      <Mono className="text-xs font-medium text-foreground">{id}</Mono>
      <p className="text-xs text-muted-foreground">{children}</p>
    </div>
  );
}

const OPERATOR_DESCRIPTION: Record<FilterOperator, string> = {
  IS: "значение совпадает с одним из указанных",
  IS_NOT: "значение не совпадает ни с одним из указанных",
  IN: "значение входит в список (то же, что IS, для многозначных условий)",
  NOT_IN: "значение не входит в список",
  CONTAINS: "текст содержит подстроку",
  NOT_CONTAINS: "текст не содержит подстроку",
  STARTS_WITH: "текст начинается с указанного",
  ENDS_WITH: "текст заканчивается указанным",
  MATCHES: "текст соответствует regex-шаблону (RE2, без lookaround/backreferences)",
  EXISTS: "поле присутствует и не пустое — значение не задаётся",
  NOT_EXISTS: "поле отсутствует или пустое — значение не задаётся",
  GT: "больше указанного числа",
  GTE: "больше или равно",
  LT: "меньше указанного числа",
  LTE: "меньше или равно",
  BETWEEN: "значение внутри диапазона [от; до]",
};

/** Reference content for the field-picker modal's "Справка" panel — built
 * from FLOX's actual field/operator catalog (lib/filters.ts), not the
 * adset.pro doc the user linked as a style reference. That doc documents
 * several fields (Query Parameter, X-Requested-With, IPv4, IPv6, User
 * Agent) with no classifier support in this app's routing engine yet —
 * listing them here as if they worked would be exactly the "fake
 * capability" CLAUDE.md forbids, so the "Пока не поддерживается" section
 * below names them honestly instead. RU-only for now (this is a large
 * prose reference, not UI chrome) — same scoped-localization tradeoff
 * already made for the Domains page. */
export function FilterReferenceSheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t, i18n } = useTranslation("streamSets");

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full gap-0 overflow-hidden p-0 data-[side=right]:sm:max-w-3xl">
        <SheetHeader className="border-b border-border">
          <SheetTitle>{t("filterReference.title")}</SheetTitle>
          <SheetDescription>{t("filterReference.subtitle")}</SheetDescription>
        </SheetHeader>

        <div className="flex flex-col gap-6 overflow-y-auto p-4">
          {!i18n.language.startsWith("ru") && (
            <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              This reference is only available in Russian for now — the text below is in Russian.
            </p>
          )}

          <Section title="Введение">
            <p>
              Фильтры отбирают трафик внутри стрим-сета кампании. Каждое условие проверяет одно поле визита
              (гео, устройство, UTM-метку и т.д.); группа условий объединяется через И (все должны совпасть) или
              ИЛИ (достаточно одного). Стрим-сеты кампании оцениваются по приоритету сверху вниз — побеждает
              первый набор, чьи фильтры совпали; если ни один не совпал, применяется резервный URL кампании.
            </p>
          </Section>

          <Section title="Категории полей">
            <ul className="list-disc pl-4">
              <li><strong className="text-foreground">Параметры</strong> — источник запроса (реферер).</li>
              <li><strong className="text-foreground">Геолокация</strong> — страна/регион/город, прокси, бот, ASN, тип соединения.</li>
              <li><strong className="text-foreground">Устройство и браузер</strong> — тип устройства, ОС, браузер, язык.</li>
              <li><strong className="text-foreground">Параметры отслеживания</strong> — UTM-метки, внешний click id, sub1–sub10.</li>
            </ul>
          </Section>

          <Section title="Операции">
            <p>Любое поле поддерживает весь набор операций ниже — ограничений «эта операция только для этого поля» нет.</p>
            <div className="overflow-x-auto rounded-lg border border-border">
              <table className="w-full text-left text-xs">
                <thead className="bg-muted/50 text-muted-foreground">
                  <tr>
                    <th className="px-2 py-1.5 font-medium">Операция</th>
                    <th className="px-2 py-1.5 font-medium">Описание</th>
                  </tr>
                </thead>
                <tbody>
                  {FILTER_OPERATORS.map((op) => (
                    <tr key={op} className="border-t border-border">
                      <td className="px-2 py-1.5 align-top">
                        <Mono className="text-xs">{t(FILTER_OPERATOR_I18N_KEY[op])}</Mono>
                      </td>
                      <td className="px-2 py-1.5 align-top">{OPERATOR_DESCRIPTION[op]}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p>
              <Mono className="text-xs">IN</Mono>/<Mono className="text-xs">NOT_IN</Mono> и списковые значения других
              операций принимают несколько значений через запятую (например, <Mono className="text-xs">US, GB, CA</Mono>).
              <Mono className="text-xs"> BETWEEN</Mono> требует обе границы диапазона.
            </p>
          </Section>

          <Section title="Параметры">
            <FieldEntry id="referrer">
              Домен или URL, с которого пришёл пользователь (HTTP Referer). Может отсутствовать или быть подделан —
              не используйте как единственную защиту.
            </FieldEntry>
          </Section>

          <Section title="Геолокация">
            <FieldEntry id="country">Страна по GeoIP, код ISO-3166 alpha-2 (US, GB, DE, UA…).</FieldEntry>
            <FieldEntry id="proxy">Использование прокси/VPN — да/нет.</FieldEntry>
            <FieldEntry id="bot">Трафик определён как бот/краулер — да/нет. Базовая защита от фрода.</FieldEntry>
            <FieldEntry id="asn">Номер автономной системы провайдера — полезно отсекать дата-центры.</FieldEntry>
            <FieldEntry id="connection_type">Тип соединения: wifi, cellular, ethernet, unknown.</FieldEntry>
          </Section>

          <Section title="Устройство и браузер">
            <FieldEntry id="device">Тип устройства: mobile, desktop, tablet.</FieldEntry>
            <FieldEntry id="os">Операционная система: ios, android, windows, macos, linux.</FieldEntry>
            <FieldEntry id="os_version">Версия ОС, свободный текст.</FieldEntry>
            <FieldEntry id="platform">Платформа устройства (тот же словарь, что и os).</FieldEntry>
            <FieldEntry id="browser">Браузер: chrome, safari, firefox, edge, opera, yandex, vivaldi, uc_browser, samsung_internet, huawei_browser, mi_browser, duckduckgo, ie, other. Brave по User-Agent неотличим от Chrome и считается как chrome.</FieldEntry>
            <FieldEntry id="browser_version">Версия браузера, свободный текст.</FieldEntry>
            <FieldEntry id="language">Основной язык браузера (Accept-Language) в виде кода: ru, en, uk. Регион отбрасывается: ru-RU это ru.</FieldEntry>
          </Section>

          <Section title="Параметры отслеживания">
            <FieldEntry id="utm_source">Источник трафика из URL (facebook, google…).</FieldEntry>
            <FieldEntry id="utm_medium">Канал распространения (cpc, cpm, email…).</FieldEntry>
            <FieldEntry id="utm_campaign">Название рекламной кампании из URL.</FieldEntry>
            <FieldEntry id="utm_content">Идентификатор объявления/креатива для A/B-тестов.</FieldEntry>
            <FieldEntry id="utm_term">Ключевое слово платного поиска.</FieldEntry>
            <FieldEntry id="external_click_id">Внешний click id рекламной сети (fbclid, gclid и т.п.).</FieldEntry>
            <FieldEntry id="sub1 – sub10">10 произвольных параметров для собственных данных: площадка, креатив, версия лендинга и т.д.</FieldEntry>
          </Section>

          <Section title="Комбинирование условий">
            <p>
              Переключатель «Любое условие / Все условия» задаёт joiner группы: И — должны совпасть все дочерние
              условия, ИЛИ — достаточно одного. Внутри группы можно добавить вложенную группу со своим joiner —
              так собираются условия вида <Mono className="text-xs">country IS US AND (browser IS chrome OR browser IS safari)</Mono>.
              Пустая группа верхнего уровня соответствует всему трафику.
            </p>
          </Section>

          <Section title="Пока не поддерживается">
            <p>
              В движке маршрутизации ещё нет разбора GET-параметров запроса, заголовка X-Requested-With, IPv4/IPv6
              и полной строки User-Agent как отдельных полей фильтра — добавить их в каталог фильтров будет
              обманом интерфейса, пока для них нет реальной обработки на бэкенде.
            </p>
          </Section>
        </div>
      </SheetContent>
    </Sheet>
  );
}
