"use client";

import * as React from "react";
import { toast } from "sonner";

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
  const campaignQuery = useCampaign(id);
  const updateCampaign = useUpdateCampaign(id);
  const archiveCampaign = useArchiveCampaign();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  if (campaignQuery.isPending) {
    return <LoadingState label="Loading campaign…" />;
  }

  if (campaignQuery.isError) {
    return (
      <ErrorState
        title="Couldn't load campaign"
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
        onSuccess: () => toast("Campaign updated", { description: values.name }),
        onError: (err) => toast.error("Couldn't update campaign", { description: err.message }),
      },
    );
  }

  function handleArchive() {
    archiveCampaign.mutate(campaign.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast("Campaign archived", { description: campaign.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error("Couldn't archive campaign", { description: err.message });
      },
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{campaign.name}</h1>
            <Badge variant={STATUS_VARIANT[campaign.status]}>{campaign.status}</Badge>
          </div>
        </div>
        <CampaignRowActions campaign={campaign} />
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="cost">Cost</TabsTrigger>
          <TabsTrigger value="simulator">Simulator</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
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
            submitLabel="Save changes"
            onSubmit={handleSettingsSubmit}
          />

          <Card className="border-danger/30">
            <CardHeader>
              <CardTitle>Danger zone</CardTitle>
              <CardDescription>Archived campaigns stop routing traffic immediately.</CardDescription>
            </CardHeader>
            <CardContent>
              {campaign.status === "archived" ? (
                <p className="text-sm text-muted-foreground">This campaign is archived.</p>
              ) : (
                <Button variant="destructive" onClick={() => setConfirmArchive(true)}>
                  Archive campaign
                </Button>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Archive &ldquo;{campaign.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Archived campaigns stop routing traffic and are hidden from the active list.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmArchive(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleArchive} disabled={archiveCampaign.isPending}>
              Archive
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
  const clicksQuery = useCampaignDailyClicks(campaignId);
  const revenueQuery = useCampaignDailyRevenue(campaignId);
  const spendQuery = useCampaignDailySpend(campaignId);

  if (clicksQuery.isPending || revenueQuery.isPending || spendQuery.isPending) {
    return <LoadingState label="Loading analytics…" />;
  }
  if (clicksQuery.isError) {
    return <ErrorState title="Couldn't load click analytics" description={clicksQuery.error.message} onRetry={() => clicksQuery.refetch()} />;
  }
  if (revenueQuery.isError) {
    return (
      <ErrorState
        title="Couldn't load revenue analytics"
        description={revenueQuery.error.message}
        onRetry={() => revenueQuery.refetch()}
      />
    );
  }
  if (spendQuery.isError) {
    return <ErrorState title="Couldn't load spend" description={spendQuery.error.message} onRetry={() => spendQuery.refetch()} />;
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
        <StatCard label="Revenue" value={formatUsd(totalRevenue, 2)} />
        <StatCard label="Clicks" value={totalClicks.toLocaleString("en-US")} />
        <StatCard label="Conversions" value={totalConversions.toLocaleString("en-US")} />
        <StatCard label="CVR" value={`${cvr.toFixed(2)}%`} />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="Spend" value={formatUsd(totalSpend, 2)} />
        <StatCard label="Profit" value={hasCost ? formatUsd(profit, 2) : "—"} />
        <StatCard label="ROI" value={hasCost ? `${roi.toFixed(2)}%` : "—"} />
        <StatCard label="CPA" value={hasCost && cpa !== null ? formatUsd(cpa, 2) : "—"} />
      </div>

      <LineMetricChart
        title="Revenue (last 30 days)"
        points={dailyPoints}
        color={CHART_COLORS.success}
        valueFormatter={(v) => formatUsd(v, 0)}
      />
    </>
  );
}
