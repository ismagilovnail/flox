import type { TFunction } from "i18next";

import type { PushRecurrence } from "@/lib/api/push";

export const ISO_WEEKDAYS = [1, 2, 3, 4, 5, 6, 7] as const;

/** Short weekday name for ISO day d (1 = Monday) in locale. 2024-01-01 is
 * a Monday. */
export function weekdayName(d: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { weekday: "short", timeZone: "UTC" }).format(new Date(Date.UTC(2024, 0, d)));
}

/** "Mon, Wed, Fri · 19:00" / "Every day · 19:00", plus "until …" (push
 * namespace). */
export function describeRecurrence(rec: Omit<PushRecurrence, "timezone">, t: TFunction<"push">, locale: string): string {
  const days =
    rec.weekdays.length === 7
      ? t("recurrence.everyDay")
      : [...rec.weekdays].sort((a, b) => a - b).map((d) => weekdayName(d, locale)).join(", ");
  const base = `${days} · ${rec.time}`;
  return rec.until ? `${base} · ${t("recurrence.until", { date: rec.until })}` : base;
}
