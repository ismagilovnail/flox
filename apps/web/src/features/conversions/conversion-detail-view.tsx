"use client";

import { Badge } from "@/components/ui/badge";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { StatCard } from "@/components/ui/stat-card";
import { Mono } from "@/components/ui/typography";
import { useConversionTimeline } from "@/hooks/use-conversions";
import { useCampaign } from "@/hooks/use-campaigns";
import { useNetwork } from "@/hooks/use-networks";
import type { CpaStatus } from "@/lib/api/conversions";
import { ConversionTimeline } from "@/features/conversions/conversion-timeline";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

export function ConversionDetailView({ id }: { id: string }) {
  const timelineQuery = useConversionTimeline(id);
  const campaignQuery = useCampaign(timelineQuery.data?.campaignId ?? "");
  const networkQuery = useNetwork(timelineQuery.data?.networkId ?? "");

  if (timelineQuery.isPending) {
    return <LoadingState label="Loading conversion…" />;
  }

  if (timelineQuery.isError) {
    return (
      <ErrorState
        title="Conversion not found"
        description={timelineQuery.error.message}
        onRetry={() => timelineQuery.refetch()}
      />
    );
  }

  const timeline = timelineQuery.data;
  const conversionEvents = timeline.events.filter((e) => e.isConversion);
  const latest = conversionEvents.at(-1);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              <Mono>{timeline.clickId}</Mono>
            </h1>
            {latest && (
              <Badge variant={STATUS_VARIANT[latest.type as CpaStatus]}>{latest.type.replace("CPA_", "")}</Badge>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            {campaignQuery.data?.name ?? timeline.campaignId}
            {timeline.networkId ? ` · ${networkQuery.data?.name ?? timeline.networkId}` : ""}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <StatCard
          label="Revenue"
          value={latest?.hasUsdValue ? `${(latest.revenue ?? 0).toFixed(2)} ${latest.currency}` : "—"}
        />
        <StatCard label="Status" value={latest ? latest.type.replace("CPA_", "") : "No conversion yet"} />
        <StatCard label="Events recorded" value={timeline.events.length} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Timeline</CardTitle>
          <CardDescription>Every recorded event for this click, in order.</CardDescription>
        </CardHeader>
        <CardContent>
          <ConversionTimeline events={timeline.events} />
        </CardContent>
      </Card>
    </div>
  );
}
