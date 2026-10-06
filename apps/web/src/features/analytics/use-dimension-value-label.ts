"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { useCampaigns } from "@/hooks/use-campaigns";
import { useLandings } from "@/hooks/use-landings";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { useOffers } from "@/hooks/use-offers";
import { usePwas } from "@/hooks/use-pwas";
import type { DimensionKey } from "@/features/analytics/registry";

/**
 * Display label for one dimension value. The report API groups by ids
 * (chstore's whitelist columns), never names — names can change and two
 * entities can share one. This maps ids to current names for display
 * only; filters, exports of the raw query and the URL keep the id.
 *
 * Resolved: `campaign`, `source` (via its campaign), `landing`, `offer` and `pwa` — all group by ULIDs that read as noise
 * on their own, and both have an empty value that is a real bucket worth
 * naming ("no landing shown", "not routed to an offer"). Every other
 * dimension returns its value unchanged — the same rendering as before
 * this hook. An id with no current entity (deleted since) falls back to
 * the raw id rather than vanishing.
 */
export function useDimensionValueLabel(): (dimension: DimensionKey, value: string) => string {
  const { t } = useTranslation("analytics");
  const campaigns = useCampaigns().data?.campaigns;
  const sources = useTrafficSources().data?.trafficSources;
  const landings = useLandings().data?.landings;
  const offers = useOffers().data?.offers;
  const pwas = usePwas().data?.pwas;

  const campaignNames = React.useMemo(() => new Map((campaigns ?? []).map((c) => [c.id, c.name])), [campaigns]);
  // The API's `source` dimension is a proxy for campaign (chstore.DimSource
  // groups by campaign_id — clicks don't carry a traffic source id yet), so
  // its values are campaign ids: resolve campaign → its source's name.
  const sourceNames = React.useMemo(() => {
    const byId = new Map((sources ?? []).map((s) => [s.id, s.name]));
    return new Map(
      (campaigns ?? []).flatMap((c) => {
        const name = byId.get(c.trafficSourceId);
        return name ? [[c.id, name] as const] : [];
      }),
    );
  }, [campaigns, sources]);
  const landingNames = React.useMemo(() => new Map((landings ?? []).map((l) => [l.id, l.name])), [landings]);
  const offerNames = React.useMemo(() => new Map((offers ?? []).map((o) => [o.id, o.name])), [offers]);
  const pwaNames = React.useMemo(() => new Map((pwas ?? []).map((p) => [p.id, p.name])), [pwas]);

  return React.useCallback(
    (dimension: DimensionKey, value: string) => {
      switch (dimension) {
        case "campaign":
          return campaignNames.get(value) ?? value;
        case "source":
          return sourceNames.get(value) ?? value;
        case "landing":
          return value === "" ? t("values.noLanding") : (landingNames.get(value) ?? value);
        case "offer":
          return value === "" ? t("values.noOffer") : (offerNames.get(value) ?? value);
        case "pwa":
          return value === "" ? t("values.noPwa") : (pwaNames.get(value) ?? value);
        default:
          return value;
      }
    },
    [t, campaignNames, sourceNames, landingNames, offerNames, pwaNames],
  );
}
