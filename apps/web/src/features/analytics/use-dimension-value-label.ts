"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { useLandings } from "@/hooks/use-landings";
import { useOffers } from "@/hooks/use-offers";
import type { DimensionKey } from "@/features/analytics/registry";

/**
 * Display label for one dimension value. The report API groups by ids
 * (chstore's whitelist columns), never names — names can change and two
 * entities can share one. This maps ids to current names for display
 * only; filters, exports of the raw query and the URL keep the id.
 *
 * Resolved: `landing` and `offer` — both group by ULIDs that read as noise
 * on their own, and both have an empty value that is a real bucket worth
 * naming ("no landing shown", "not routed to an offer"). Every other
 * dimension returns its value unchanged — the same rendering as before
 * this hook. An id with no current entity (deleted since) falls back to
 * the raw id rather than vanishing.
 */
export function useDimensionValueLabel(): (dimension: DimensionKey, value: string) => string {
  const { t } = useTranslation("analytics");
  const landings = useLandings().data?.landings;
  const offers = useOffers().data?.offers;

  const landingNames = React.useMemo(() => new Map((landings ?? []).map((l) => [l.id, l.name])), [landings]);
  const offerNames = React.useMemo(() => new Map((offers ?? []).map((o) => [o.id, o.name])), [offers]);

  return React.useCallback(
    (dimension: DimensionKey, value: string) => {
      switch (dimension) {
        case "landing":
          return value === "" ? t("values.noLanding") : (landingNames.get(value) ?? value);
        case "offer":
          return value === "" ? t("values.noOffer") : (offerNames.get(value) ?? value);
        default:
          return value;
      }
    },
    [t, landingNames, offerNames],
  );
}
