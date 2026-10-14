import type { TFunction } from "i18next";

import type { PushTrigger } from "@/lib/api/push";

export type DelayUnit = "minutes" | "hours" | "days";

export const DELAY_UNIT_MINUTES: Record<DelayUnit, number> = { minutes: 1, hours: 60, days: 24 * 60 };

/** The largest unit a delay is a whole number of: 90 → 90 minutes,
 * 120 → 2 hours, 2880 → 2 days. */
export function splitDelay(minutes: number): { amount: number; unit: DelayUnit } {
  if (minutes % DELAY_UNIT_MINUTES.days === 0) return { amount: minutes / DELAY_UNIT_MINUTES.days, unit: "days" };
  if (minutes % DELAY_UNIT_MINUTES.hours === 0) return { amount: minutes / DELAY_UNIT_MINUTES.hours, unit: "hours" };
  return { amount: minutes, unit: "minutes" };
}

/** "1 d after subscribing · without CPA_ACCEPT" (push namespace). */
export function describeTrigger(tr: PushTrigger, t: TFunction<"push">): string {
  const { amount, unit } = splitDelay(tr.delayMinutes);
  const delay = t("trigger.after", { delay: t(`trigger.units.${unit}`, { count: amount }) });
  if (tr.condition === "none") return delay;
  return `${delay} · ${t(`trigger.conditionShort.${tr.condition}`, { statuses: tr.statuses.join(", ") })}`;
}
