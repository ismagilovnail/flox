"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
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
import type { CampaignStatus } from "@/lib/api/campaigns";
import { useArchiveCampaign, useCampaign, useUpdateCampaign } from "@/hooks/use-campaigns";
import { CampaignRowActions } from "@/features/campaigns/campaign-row-actions";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";
import { CampaignCostEntries } from "@/features/campaigns/campaign-cost-entries";
import { TrackingLinks } from "@/features/campaigns/tracking-links";
import { StreamSetList } from "@/features/stream-sets/stream-set-list";
import { RoutingSimulatorView } from "@/features/routing-simulator/routing-simulator-view";

const STATUS_VARIANT: Record<CampaignStatus, "success" | "warning" | "outline" | "secondary"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
  archived: "secondary",
};

const SETTINGS_FORM_ID = "campaign-settings-form";

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
  const isArchived = campaign.status === "archived";

  function handleSettingsSubmit(values: CampaignFormValues) {
    updateCampaign.mutate(
      {
        name: values.name,
        trafficSourceId: values.trafficSourceId,
        fallbackUrl: values.fallbackUrl,
        externalCampaignId: values.externalCampaignId ?? "",
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
        <div className="flex items-center gap-2">
          <Button type="submit" form={SETTINGS_FORM_ID} disabled={isArchived}>
            {t("detail.saveChanges")}
          </Button>
          <CampaignRowActions campaign={campaign} variant="inline" />
        </div>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t("detail.overviewTab")}</TabsTrigger>
          <TabsTrigger value="cost">{t("detail.costTab")}</TabsTrigger>
          <TabsTrigger value="simulator">{t("detail.simulatorTab")}</TabsTrigger>
          <TabsTrigger value="settings">{t("detail.settingsTab")}</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-6">
          <TrackingLinks campaignId={campaign.id} campaignName={campaign.name} disabled={isArchived} />
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-[360px_1fr] lg:items-start">
            <CampaignForm
              defaultValues={{
                name: campaign.name,
                trafficSourceId: campaign.trafficSourceId,
                fallbackUrl: campaign.fallbackUrl,
                externalCampaignId: campaign.externalCampaignId,
                notes: campaign.notes,
                status: campaign.status,
              }}
              formId={SETTINGS_FORM_ID}
              onSubmit={handleSettingsSubmit}
              disabled={isArchived}
            />
            <StreamSetList campaignId={campaign.id} disabled={isArchived} />
          </div>
        </TabsContent>

        <TabsContent value="cost" className="flex flex-col gap-6">
          <CampaignCostEntries campaignId={campaign.id} />
        </TabsContent>

        <TabsContent value="simulator" className="flex flex-col gap-6">
          <RoutingSimulatorView campaignId={campaign.id} campaignName={campaign.name} />
        </TabsContent>

        <TabsContent value="settings" className="flex flex-col gap-6">
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
