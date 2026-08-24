"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LineMetricChart } from "@/components/charts/line-metric-chart";
import { CHART_COLORS } from "@/lib/chart-theme";
import { formatUsd } from "@/lib/format";
import type { CampaignStatus } from "@/lib/api/campaigns";
import { useArchiveCampaign, useCampaign, useUpdateCampaign } from "@/hooks/use-campaigns";
import { useCampaignDailyClicks, useCampaignDailyRevenue } from "@/hooks/use-campaign-analytics";
import { useCampaignDailySpend } from "@/hooks/use-cost-entries";
import { CampaignRowActions } from "@/features/campaigns/campaign-row-actions";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";
import { CampaignCostEntries } from "@/features/campaigns/campaign-cost-entries";
import { StreamSetList } from "@/features/stream-sets/stream-set-list";
import { RoutingSimulatorView } from "@/features/routing-simulator/routing-simulator-view";

const STATUS_VARIANT: Record<CampaignStatus, "success" | "warning" | "outline" | "secondary"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
  archived: "secondary",
};

export function CampaignDetailView({ id }: { id: string }) {
  const { t } = useTranslation(["campaigns", "common"]);
  const campaignQuery = useCampaign(id);
  const updateCampaign = useUpdateCampaign(id);
  const archiveCampaign = useArchiveCampaign();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  if (campaignQuery.isPending) {
    return <LoadingState label={t("detail.loading")} />;
  }

  if (campaignQuery.isError) {
    return (
      <ErrorState
        title={t("detail.loadError")}
        description={campaignQuery.error.message}
        onRetry={() => campaignQuery.refetch()}
      />
    );
  }

  const campaign = campaignQuery.data;

  function handleSettingsSubmit(values: CampaignFormValues) {
    updateCampaign.mutate(
      {
        name: values.name,
        trafficSourceId: values.trafficSourceId,
        fallbackUrl: values.fallbackUrl,
        notes: values.notes ?? "",
        status: values.status,
      },
      {
        onSuccess: () => toast(t("toast.updated"), { description: values.name }),
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      },
    );
  }

  function handleArchive() {
    archiveCampaign.mutate(campaign.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: campaign.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{campaign.name}</h1>
            <Badge variant={STATUS_VARIANT[campaign.status]}>{t(`status.${campaign.status}`, { ns: "common" })}</Badge>
          </div>
        </div>
        <CampaignRowActions campaign={campaign} />
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t("detail.overviewTab")}</TabsTrigger>
          <TabsTrigger value="cost">{t("detail.costTab")}</TabsTrigger>
          <TabsTrigger value="simulator">{t("detail.simulatorTab")}</TabsTrigger>
          <TabsTrigger value="settings">{t("detail.settingsTab")}</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-6">
          <CampaignOverview campaignId={campaign.id} />
          <StreamSetList campaignId={campaign.id} />
        </TabsContent>

        <TabsContent value="cost" className="flex flex-col gap-6">
          <CampaignCostEntries campaignId={campaign.id} />
        </TabsContent>

        <TabsContent value="simulator" className="flex flex-col gap-6">
          <RoutingSimulatorView campaignId={campaign.id} campaignName={campaign.name} />
        </TabsContent>

        <TabsContent value="settings" className="flex flex-col gap-6">
          <CampaignForm
            defaultValues={{
              name: campaign.name,
              trafficSourceId: campaign.trafficSourceId,
              fallbackUrl: campaign.fallbackUrl,
              notes: campaign.notes,
              status: campaign.status,
            }}
            showStatus
            submitLabel={t("detail.saveChanges")}
            onSubmit={handleSettingsSubmit}
          />

          <Card className="border-danger/30">
            <CardHeader>
              <CardTitle>{t("detail.dangerZoneTitle")}</CardTitle>
              <CardDescription>{t("detail.dangerZoneDescription")}</CardDescription>
            </CardHeader>
            <CardContent>
              {campaign.status === "archived" ? (
                <p className="text-sm text-muted-foreground">{t("detail.alreadyArchived")}</p>
              ) : (
                <Button variant="destructive" onClick={() => setConfirmArchive(true)}>
                  {t("detail.archiveButton")}
                </Button>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("detail.archiveConfirmTitle", { name: campaign.name })}</DialogTitle>
            <DialogDescription>{t("detail.archiveConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmArchive(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button variant="destructive" onClick={handleArchive} disabled={archiveCampaign.isPending}>
              {t("rowActions.archive")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** Real stats from apps/internal/analytics + apps/internal/cost. Spend/
 * Profit/ROI/CPA (dropped entirely in Phase 27 pending a cost pipeline)
 * are real as of Phase 27-COST. Per CLAUDE.md #6: Spend itself is a
 * direct sum, shown as $0.00 when genuinely no entries exist for the
 * range — but Profit/ROI/CPA are ratios/derivations against spend, so
 * they render "—" (never a false-positive computed against an implicit
 * zero) whenever hasCost is false. See docs/frontend-integration.md. */
function CampaignOverview({ campaignId }: { campaignId: string }) {
  const { t } = useTranslation("campaigns");
  const clicksQuery = useCampaignDailyClicks(campaignId);
  const revenueQuery = useCampaignDailyRevenue(campaignId);
  const spendQuery = useCampaignDailySpend(campaignId);

  if (clicksQuery.isPending || revenueQuery.isPending || spendQuery.isPending) {
    return <LoadingState label={t("overview.loading")} />;
  }
  if (clicksQuery.isError) {
    return (
      <ErrorState
        title={t("overview.clickLoadError")}
        description={clicksQuery.error.message}
        onRetry={() => clicksQuery.refetch()}
      />
    );
  }
  if (revenueQuery.isError) {
    return (
      <ErrorState
        title={t("overview.revenueLoadError")}
        description={revenueQuery.error.message}
        onRetry={() => revenueQuery.refetch()}
      />
    );
  }
  if (spendQuery.isError) {
    return (
      <ErrorState title={t("overview.spendLoadError")} description={spendQuery.error.message} onRetry={() => spendQuery.refetch()} />
    );
  }

  const totalClicks = clicksQuery.data.counts
    .filter((c) => c.type === "SOURCE_CLICK")
    .reduce((sum, c) => sum + c.eventCount, 0);
  // "Conversions" here is total deposit events (CPA_ACCEPT + CPA_REDEP) —
  // matches §26.5's own total_deposits definition, not just first deposits.
  const totalConversions = revenueQuery.data.revenue
    .filter((r) => r.type === "CPA_ACCEPT" || r.type === "CPA_REDEP")
    .reduce((sum, r) => sum + r.eventCount, 0);
  const totalRevenue = revenueQuery.data.revenue.reduce((sum, r) => sum + r.revenueUsd, 0);
  const cvr = totalClicks > 0 ? (totalConversions / totalClicks) * 100 : 0;

  const hasCost = spendQuery.data.spend.length > 0;
  const totalSpend = spendQuery.data.spend.reduce((sum, s) => sum + s.amountUsd, 0);
  const profit = totalRevenue - totalSpend;
  const roi = totalSpend > 0 ? (profit / totalSpend) * 100 : 0;
  // CPA is undefined (never $0.00) with zero conversions — that's a
  // division-by-zero, not "free," same principle as CLAUDE.md #12 for
  // custom metrics.
  const cpa = totalConversions > 0 ? totalSpend / totalConversions : null;

  const revenueByDay = new Map<string, number>();
  for (const r of revenueQuery.data.revenue) {
    revenueByDay.set(r.day, (revenueByDay.get(r.day) ?? 0) + r.revenueUsd);
  }
  const dailyPoints = Array.from(revenueByDay.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, value]) => ({ date, value }));

  return (
    <>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label={t("overview.revenue")} value={formatUsd(totalRevenue, 2)} />
        <StatCard label={t("overview.clicks")} value={totalClicks.toLocaleString("en-US")} />
        <StatCard label={t("overview.conversions")} value={totalConversions.toLocaleString("en-US")} />
        <StatCard label={t("overview.cvr")} value={`${cvr.toFixed(2)}%`} />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label={t("overview.spend")} value={formatUsd(totalSpend, 2)} />
        <StatCard label={t("overview.profit")} value={hasCost ? formatUsd(profit, 2) : "—"} />
        <StatCard label={t("overview.roi")} value={hasCost ? `${roi.toFixed(2)}%` : "—"} />
        <StatCard label={t("overview.cpa")} value={hasCost && cpa !== null ? formatUsd(cpa, 2) : "—"} />
      </div>

      <LineMetricChart
        title={t("overview.revenueChartTitle")}
        points={dailyPoints}
        color={CHART_COLORS.success}
        valueFormatter={(v) => formatUsd(v, 0)}
      />
    </>
  );
}
