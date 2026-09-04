"use client";

import { formatDistanceToNow } from "date-fns";
import { enUS, ru } from "date-fns/locale";
import { useTranslation } from "react-i18next";

import { Caption } from "@/components/ui/typography";
import { cn } from "@/lib/utils";

// Same locale map / i18n.language key as components/ui/date-range-picker.tsx
// — kept as its own small map here rather than shared, since pulling it
// into a common module for two call sites isn't worth the indirection yet.
const DATE_FNS_LOCALES = { en: enUS, ru } as const;

/** "X minutes ago" in the table's own language instead of always English —
 * every `formatDistanceToNow` table cell in the app should render through
 * this, not call date-fns directly. */
export function RelativeTime({ date, className }: { date: string | Date; className?: string }) {
  const { i18n } = useTranslation();
  const locale = DATE_FNS_LOCALES[i18n.language as keyof typeof DATE_FNS_LOCALES] ?? enUS;
  return (
    <Caption className={cn(className)}>
      {formatDistanceToNow(typeof date === "string" ? new Date(date) : date, { addSuffix: true, locale })}
    </Caption>
  );
}
