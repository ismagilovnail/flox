import { useTranslation } from "react-i18next";

import { StatCard } from "@/components/ui/stat-card";
import { formatUsd, formatInt } from "@/lib/format";
import { pctDelta, type PeriodMetrics } from "@/features/dashboard/metrics";

const pct = (n: number) => `${(n * 100).toFixed(2)}%`;

function trendProps(curr: number, prev: number) {
  const d = pctDelta(curr, prev);
  if (d === null) return {};
  return {
    trend: (Math.abs(d) < 0.5 ? "flat" : d > 0 ? "up" : "down") as "up" | "down" | "flat",
    delta: `${d > 0 ? "+" : ""}${d.toFixed(1)}%`,
  };
}

export function KpiGrid({
  current,
  previous,
}: {
  current: PeriodMetrics;
  previous: PeriodMetrics;
}) {
  const { t } = useTranslation("dashboard");
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <StatCard label={t("kpi.revenue")} value={formatUsd(current.revenue)} {...trendProps(current.revenue, previous.revenue)} />
      <StatCard
        label={t("kpi.spend")}
        value={current.spend === null ? "—" : formatUsd(current.spend)}
        direction="up-is-bad"
        {...(current.spend !== null && previous.spend !== null ? trendProps(current.spend, previous.spend) : {})}
      />
      <StatCard
        label={t("kpi.profit")}
        value={current.profit === null ? "—" : formatUsd(current.profit)}
        {...(current.profit !== null && previous.profit !== null ? trendProps(current.profit, previous.profit) : {})}
      />
      <StatCard
        label={t("kpi.roi")}
        value={current.roi === null ? "—" : `${current.roi > 0 ? "+" : ""}${current.roi.toFixed(1)}%`}
        {...(current.roi !== null && previous.roi !== null
          ? trendProps(current.roi, previous.roi)
          : {})}
      />
      <StatCard label={t("kpi.clicks")} value={formatInt(current.clicks)} {...trendProps(current.clicks, previous.clicks)} />
      <StatCard
        label={t("kpi.conversions")}
        value={formatInt(current.conversions)}
        {...trendProps(current.conversions, previous.conversions)}
      />
      <StatCard label={t("kpi.cvr")} value={pct(current.cvr)} {...trendProps(current.cvr, previous.cvr)} />
      <StatCard
        label={t("kpi.cpa")}
        value={current.cpa === null ? "—" : formatUsd(current.cpa)}
        direction="up-is-bad"
        {...(current.cpa !== null && previous.cpa !== null
          ? trendProps(current.cpa, previous.cpa)
          : {})}
      />
      <StatCard
        label={t("kpi.blocked")}
        value={formatInt(current.blocked)}
        direction="up-is-bad"
        {...trendProps(current.blocked, previous.blocked)}
      />
      <StatCard
        label={t("kpi.blockedRate")}
        value={current.blockedRate === null ? "—" : `${current.blockedRate.toFixed(2)}%`}
        direction="up-is-bad"
        {...(current.blockedRate !== null && previous.blockedRate !== null
          ? trendProps(current.blockedRate, previous.blockedRate)
          : {})}
      />
      <StatCard
        label={t("kpi.bots")}
        value={formatInt(current.bots)}
        direction="up-is-bad"
        {...trendProps(current.bots, previous.bots)}
      />
      <StatCard
        label={t("kpi.botRate")}
        value={current.botRate === null ? "—" : `${current.botRate.toFixed(2)}%`}
        direction="up-is-bad"
        {...(current.botRate !== null && previous.botRate !== null
          ? trendProps(current.botRate, previous.botRate)
          : {})}
      />
    </div>
  );
}
