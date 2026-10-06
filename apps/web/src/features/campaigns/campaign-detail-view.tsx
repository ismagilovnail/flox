"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { CampaignStatus } from "@/lib/api/campaigns";
import { useCampaign, useUpdateCampaign } from "@/hooks/use-campaigns";
import { usePermission } from "@/hooks/use-auth";
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
  const canWrite = usePermission("campaign.write");
  const campaignQuery = useCampaign(id);
  const updateCampaign = useUpdateCampaign(id);

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
  const readOnly = isArchived || !canWrite;

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
          <Button type="submit" form={SETTINGS_FORM_ID} disabled={readOnly}>
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
              disabled={readOnly}
              archived={isArchived}
            />
            <StreamSetList campaignId={campaign.id} disabled={readOnly} />
          </div>
        </TabsContent>

        <TabsContent value="cost" className="flex flex-col gap-6">
          <CampaignCostEntries campaignId={campaign.id} />
        </TabsContent>

        <TabsContent value="simulator" className="flex flex-col gap-6">
          <RoutingSimulatorView campaignId={campaign.id} campaignName={campaign.name} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
