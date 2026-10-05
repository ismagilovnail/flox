"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { useLandings } from "@/hooks/use-landings";
import type { DimensionKey } from "@/features/analytics/registry";

/**
 * Display label for one dimension value. The report API groups by ids
 * (chstore's whitelist columns), never names — names can change and two
 * entities can share one. This maps ids to current names for display
 * only; filters, exports of the raw query and the URL keep the id.
 *
 * Only `landing` is resolved so far: the `landing` dimension's values are
 * landing ULIDs that read as noise on their own, and its empty value is a
 * real bucket ("no landing shown") worth naming. Every other dimension
 * returns its value unchanged — the same rendering as before this hook.
 * A landing id with no current landing (deleted since) falls back to the
 * raw id rather than vanishing.
 */
export function useDimensionValueLabel(): (dimension: DimensionKey, value: string) => string {
  const { t } = useTranslation("analytics");
  const landings = useLandings().data?.landings;

  const landingNames = React.useMemo(() => new Map((landings ?? []).map((l) => [l.id, l.name])), [landings]);

  return React.useCallback(
    (dimension: DimensionKey, value: string) => {
      if (dimension !== "landing") return value;
      if (value === "") return t("values.noLanding");
      return landingNames.get(value) ?? value;
    },
    [t, landingNames],
  );
}
