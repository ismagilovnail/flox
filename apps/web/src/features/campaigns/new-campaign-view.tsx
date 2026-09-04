"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { LinkIcon, RouteIcon } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { useCreateCampaign } from "@/hooks/use-campaigns";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";

/** Same two-column layout as the campaign detail page (tracking links full
 * width, settings + stream sets below) — at the user's request, so "New
 * Campaign" doesn't drop you on a bare form before showing where tracking
 * links and stream sets live. Both panels stay locked previews (same
 * title/description strings the real ones use) until the campaign exists:
 * TrackingLinks/StreamSetList are keyed by a real campaignId and query/
 * mutate against it, so there's nothing honest to render there before the
 * POST succeeds. Once it does, onSuccess still routes straight to
 * /campaigns/{id} — the real detail page, fully live — same as it always
 * has. */
export function NewCampaignView() {
  const { t } = useTranslation(["campaigns", "streamSets"]);
  const router = useRouter();
  const createCampaign = useCreateCampaign();

  function handleSubmit(values: CampaignFormValues) {
    createCampaign.mutate(
      {
        name: values.name,
        trafficSourceId: values.trafficSourceId,
        fallbackUrl: values.fallbackUrl,
        externalCampaignId: values.externalCampaignId ?? "",
        notes: values.notes ?? "",
      },
      {
        onSuccess: (created) => {
          toast(t("toast.created"), { description: created.name });
          router.push(`/campaigns/${created.id}`);
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("newCampaign.title")}</h1>

      <Card>
        <CardHeader>
          <CardTitle>{t("trackingLinks.title")}</CardTitle>
          <CardDescription>{t("trackingLinks.description")}</CardDescription>
        </CardHeader>
        <CardContent>
          <EmptyState
            icon={LinkIcon}
            title={t("newCampaign.lockedTitle")}
            description={t("newCampaign.trackingLinksLockedDescription")}
          />
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[360px_1fr] lg:items-start">
        <CampaignForm defaultValues={{}} submitLabel={t("newCampaign.submitButton")} onSubmit={handleSubmit} />

        <Card>
          <CardHeader>
            <CardTitle>{t("list.title", { ns: "streamSets" })}</CardTitle>
            <CardDescription>{t("list.description", { ns: "streamSets" })}</CardDescription>
          </CardHeader>
          <CardContent>
            <EmptyState
              icon={RouteIcon}
              title={t("newCampaign.lockedTitle")}
              description={t("newCampaign.streamSetsLockedDescription")}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
