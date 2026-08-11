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
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <StatCard label="Revenue" value={formatUsd(current.revenue)} {...trendProps(current.revenue, previous.revenue)} />
      <StatCard
        label="Spend"
        value={formatUsd(current.spend)}
        direction="up-is-bad"
        {...trendProps(current.spend, previous.spend)}
      />
      <StatCard label="Profit" value={formatUsd(current.profit)} {...trendProps(current.profit, previous.profit)} />
      <StatCard
        label="ROI"
        value={current.roi === null ? "—" : `${current.roi > 0 ? "+" : ""}${current.roi.toFixed(1)}%`}
        {...(current.roi !== null && previous.roi !== null
          ? trendProps(current.roi, previous.roi)
          : {})}
      />
      <StatCard label="Clicks" value={formatInt(current.clicks)} {...trendProps(current.clicks, previous.clicks)} />
      <StatCard
        label="Conversions"
        value={formatInt(current.conversions)}
        {...trendProps(current.conversions, previous.conversions)}
      />
      <StatCard label="CVR" value={pct(current.cvr)} {...trendProps(current.cvr, previous.cvr)} />
      <StatCard
        label="CPA"
        value={current.cpa === null ? "—" : formatUsd(current.cpa)}
        direction="up-is-bad"
        {...(current.cpa !== null && previous.cpa !== null
          ? trendProps(current.cpa, previous.cpa)
          : {})}
      />
    </div>
  );
}
