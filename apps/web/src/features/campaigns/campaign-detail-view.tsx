"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { CopyIcon, LinkIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { EmptyState } from "@/components/ui/empty-state";
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
import { generateCampaignDaily, type CampaignStatus } from "@/lib/mock/campaigns";
import { useCampaignsStore } from "@/stores/campaigns";
import { CampaignRowActions } from "@/features/campaigns/campaign-row-actions";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";

const STATUS_VARIANT: Record<CampaignStatus, "success" | "warning" | "outline" | "secondary"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
  archived: "secondary",
};

export function CampaignDetailView({ id }: { id: string }) {
  const campaign = useCampaignsStore((s) => s.getById(id));
  const updateCampaign = useCampaignsStore((s) => s.updateCampaign);
  const setStatus = useCampaignsStore((s) => s.setStatus);
  const router = useRouter();
  const [confirmArchive, setConfirmArchive] = React.useState(false);

  const daily = React.useMemo(() => (campaign ? generateCampaignDaily(campaign.id) : []), [campaign]);

  if (!campaign) {
    return (
      <ErrorState
        title="Campaign not found"
        description="It may have been created in a previous session — mock data resets when the app reloads."
        onRetry={() => router.push("/campaigns")}
      />
    );
  }

  const trackingUrl = `https://${campaign.trackingDomain}/t/${campaign.trackingId}`;
  const profit = campaign.spend === null ? null : campaign.revenue - campaign.spend;
  const roi = campaign.spend && campaign.spend > 0 ? ((profit as number) / campaign.spend) * 100 : null;
  const cvr = campaign.clicks > 0 ? (campaign.conversions / campaign.clicks) * 100 : 0;
  const cpa = campaign.spend !== null && campaign.conversions > 0 ? campaign.spend / campaign.conversions : null;

  function copyTrackingUrl() {
    navigator.clipboard.writeText(trackingUrl);
    toast("Tracking URL copied", { description: trackingUrl });
  }

  function handleSettingsSubmit(values: CampaignFormValues) {
    updateCampaign(campaign!.id, values);
    if (values.status) setStatus(campaign!.id, values.status);
    toast("Campaign updated", { description: values.name });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{campaign.name}</h1>
            <Badge variant={STATUS_VARIANT[campaign.status]}>{campaign.status}</Badge>
          </div>
          <button
            type="button"
            onClick={copyTrackingUrl}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <LinkIcon className="size-3" />
            <span className="font-mono">{trackingUrl}</span>
            <CopyIcon className="size-3" />
          </button>
        </div>
        <CampaignRowActions campaign={campaign} />
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-6">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <StatCard label="Revenue" value={formatUsd(campaign.revenue, 2)} />
            <StatCard label="Spend" value={campaign.spend === null ? "—" : formatUsd(campaign.spend, 2)} />
            <StatCard label="Profit" value={profit === null ? "—" : formatUsd(profit, 2)} />
            <StatCard label="ROI" value={roi === null ? "—" : `${roi > 0 ? "+" : ""}${roi.toFixed(1)}%`} />
            <StatCard label="Clicks" value={campaign.clicks.toLocaleString("en-US")} />
            <StatCard label="Conversions" value={campaign.conversions.toLocaleString("en-US")} />
            <StatCard label="CVR" value={`${cvr.toFixed(2)}%`} />
            <StatCard label="CPA" value={cpa === null ? "—" : formatUsd(cpa, 2)} />
          </div>

          <LineMetricChart
            title="Revenue (last 30 days)"
            points={daily.map((p) => ({ date: p.date, value: p.revenue }))}
            color={CHART_COLORS.success}
            valueFormatter={(v) => formatUsd(v, 0)}
          />

          <Card>
            <CardHeader>
              <CardTitle>Stream Sets</CardTitle>
              <CardDescription>Priority-ordered rules that route this campaign&apos;s traffic.</CardDescription>
            </CardHeader>
            <CardContent>
              <EmptyState
                title="Not built yet"
                description="Stream sets, filters, and flows land in Phase 7-9. Until then, all traffic falls back to the URL configured in Settings."
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings" className="flex flex-col gap-6">
          <CampaignForm
            defaultValues={{
              name: campaign.name,
              source: campaign.source,
              trackingDomain: campaign.trackingDomain,
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
            <Button
              variant="destructive"
              onClick={() => {
                setStatus(campaign.id, "archived");
                setConfirmArchive(false);
                toast("Campaign archived", { description: campaign.name });
              }}
            >
              Archive
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
