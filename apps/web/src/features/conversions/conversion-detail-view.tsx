"use client";

import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { StatCard } from "@/components/ui/stat-card";
import { Mono } from "@/components/ui/typography";
import { useConversionTimeline } from "@/hooks/use-conversions";
import { useCampaign } from "@/hooks/use-campaigns";
import { useNetwork } from "@/hooks/use-networks";
import { CPA_STATUS_I18N_KEY, type CpaStatus } from "@/lib/api/conversions";
import { ConversionTimeline } from "@/features/conversions/conversion-timeline";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

export function ConversionDetailView({ id }: { id: string }) {
  const { t } = useTranslation("conversions");
  const timelineQuery = useConversionTimeline(id);
  const campaignQuery = useCampaign(timelineQuery.data?.campaignId ?? "");
  const networkQuery = useNetwork(timelineQuery.data?.networkId ?? "");

  if (timelineQuery.isPending) {
    return <LoadingState label={t("detail.loading")} />;
  }

  if (timelineQuery.isError) {
    return (
      <ErrorState
        title={t("detail.notFoundTitle")}
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
              <Badge variant={STATUS_VARIANT[latest.type as CpaStatus]}>
                {t(CPA_STATUS_I18N_KEY[latest.type as CpaStatus])}
              </Badge>
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
          label={t("detail.revenueLabel")}
          value={latest?.hasUsdValue ? `${(latest.revenue ?? 0).toFixed(2)} ${latest.currency}` : "—"}
        />
        <StatCard
          label={t("detail.statusLabel")}
          value={latest ? t(CPA_STATUS_I18N_KEY[latest.type as CpaStatus]) : t("detail.noConversionYet")}
        />
        <StatCard label={t("detail.eventsRecordedLabel")} value={timeline.events.length} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("detail.timelineTitle")}</CardTitle>
          <CardDescription>{t("detail.timelineDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <ConversionTimeline events={timeline.events} />
        </CardContent>
      </Card>
    </div>
  );
}
