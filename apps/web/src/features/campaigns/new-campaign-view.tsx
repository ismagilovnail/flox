"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreateCampaign } from "@/hooks/use-campaigns";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";

export function NewCampaignView() {
  const { t } = useTranslation("campaigns");
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
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">{t("newCampaign.title")}</h1>
      <CampaignForm defaultValues={{}} submitLabel={t("newCampaign.submitButton")} onSubmit={handleSubmit} />
    </div>
  );
}
