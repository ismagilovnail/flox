"use client";

import { useTranslation } from "react-i18next";
import { MousePointerClick } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { DateBasis } from "@/lib/api/analytics";

const BASES: DateBasis[] = ["conversion", "click"];

/** Which day conversions and revenue count on: the conversion's own day or
 * its click's (tech audit AN5). Clicks and spend are by click day either way. */
export function DateBasisSelect({
  value,
  onValueChange,
}: {
  value: DateBasis;
  onValueChange: (value: DateBasis) => void;
}) {
  const { t } = useTranslation("analytics");
  return (
    <Select value={value} onValueChange={(v) => onValueChange(v as DateBasis)}>
      <SelectTrigger className="w-52" size="sm" aria-label={t("dateBasis.label")}>
        <span className="text-muted-foreground">{t("dateBasis.label")}</span>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {BASES.map((b) => (
          <SelectItem key={b} value={b}>
            {t(`dateBasis.options.${b}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Shown while conversions are dated by click: recent days are still filling
 * in, and conversions without a click kept their own day. */
export function DateBasisNotice({ dateBasis, datedByConversion }: { dateBasis: DateBasis; datedByConversion: number }) {
  const { t } = useTranslation("analytics");
  if (dateBasis !== "click") return null;
  return (
    <Alert>
      <MousePointerClick aria-hidden="true" />
      <AlertTitle>{t("dateBasis.notice.title")}</AlertTitle>
      <AlertDescription>
        <p>{t("dateBasis.notice.description")}</p>
        {datedByConversion > 0 && <p>{t("dateBasis.notice.withoutClick", { count: datedByConversion })}</p>}
      </AlertDescription>
    </Alert>
  );
}
