"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { useCampaigns } from "@/hooks/use-campaigns";
import { useFlows } from "@/hooks/use-flows";
import { useLandings } from "@/hooks/use-landings";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { useOffers } from "@/hooks/use-offers";
import { usePwas } from "@/hooks/use-pwas";
import { usePostlandings } from "@/hooks/use-postlandings";
import { useNetworks } from "@/hooks/use-networks";
import type { DimensionKey } from "@/types/analytics";

/**
 * Display label for one dimension value. The report API groups by ids
 * (chstore's whitelist columns), never names — names can change and two
 * entities can share one. This maps ids to current names for display
 * only; filters, exports of the raw query and the URL keep the id.
 *
 * Resolved: `campaign`, `source`, `flow`, `stream_set`, `landing`, `offer`, `network`, `pwa` and `postlanding` — all group by ULIDs that read as noise
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
  const flows = useFlows().data?.flows;
  const landings = useLandings().data?.landings;
  const offers = useOffers().data?.offers;
  const pwas = usePwas().data?.pwas;
  const postlandings = usePostlandings().data?.postlandings;
  const networks = useNetworks().data?.networks;

  const campaignNames = React.useMemo(() => new Map((campaigns ?? []).map((c) => [c.id, c.name])), [campaigns]);
  const sourceNames = React.useMemo(() => new Map((sources ?? []).map((s) => [s.id, s.name])), [sources]);
  // A flow's name alone is ambiguous across campaigns ("Flow 1"), so a name
  // that more than one flow carries gets its campaign appended. Flow ids are
  // re-minted when a stream set is saved, so an id from older history may no
  // longer resolve and falls back to the raw id.
  const flowNames = React.useMemo(() => {
    const count = new Map<string, number>();
    for (const f of flows ?? []) count.set(f.name, (count.get(f.name) ?? 0) + 1);
    return new Map(
      (flows ?? []).map((f) => [f.id, (count.get(f.name) ?? 0) > 1 ? `${f.name} (${f.campaignName})` : f.name]),
    );
  }, [flows]);
  // Stream sets have no org-wide list; every flow carries its set's name.
  const streamSetNames = React.useMemo(
    () => new Map((flows ?? []).map((f) => [f.streamSetId, f.streamSetName])),
    [flows],
  );
  const landingNames = React.useMemo(() => new Map((landings ?? []).map((l) => [l.id, l.name])), [landings]);
  const offerNames = React.useMemo(() => new Map((offers ?? []).map((o) => [o.id, o.name])), [offers]);
  const pwaNames = React.useMemo(() => new Map((pwas ?? []).map((p) => [p.id, p.name])), [pwas]);
  const networkNames = React.useMemo(() => new Map((networks ?? []).map((n) => [n.id, n.name])), [networks]);
  const postlandingNames = React.useMemo(
    () => new Map((postlandings ?? []).map((p) => [p.id, p.name])),
    [postlandings],
  );

  return React.useCallback(
    (dimension: DimensionKey, value: string) => {
      switch (dimension) {
        case "campaign":
          return campaignNames.get(value) ?? value;
        case "source":
          return value === "" ? t("values.noSource") : (sourceNames.get(value) ?? value);
        case "flow":
          return value === "" ? t("values.noFlow") : (flowNames.get(value) ?? value);
        case "stream_set":
          return value === "" ? t("values.noStreamSet") : (streamSetNames.get(value) ?? value);
        case "landing":
          return value === "" ? t("values.noLanding") : (landingNames.get(value) ?? value);
        case "offer":
          return value === "" ? t("values.noOffer") : (offerNames.get(value) ?? value);
        case "pwa":
          return value === "" ? t("values.noPwa") : (pwaNames.get(value) ?? value);
        case "network":
          return value === "" ? t("values.noNetwork") : (networkNames.get(value) ?? value);
        case "postlanding":
          return value === "" ? t("values.noPostlanding") : (postlandingNames.get(value) ?? value);
        case "country":
          return value === "" ? t("values.noCountry") : value;
        default:
          // An empty bucket (unknown region/city/device/…) is real data; a blank
          // cell reads as a rendering bug, so it gets a dash.
          return value === "" ? "—" : value;
      }
    },
    [t, campaignNames, sourceNames, flowNames, streamSetNames, landingNames, offerNames, networkNames, pwaNames, postlandingNames],
  );
}
