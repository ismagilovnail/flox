"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { SOURCES, TRACKING_DOMAINS } from "@/lib/mock/campaigns";
import { useCampaignsStore } from "@/stores/campaigns";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";

export function NewCampaignView() {
  const router = useRouter();
  const addCampaign = useCampaignsStore((s) => s.addCampaign);

  function handleSubmit(values: CampaignFormValues) {
    const id = addCampaign({
      name: values.name,
      source: values.source,
      trackingDomain: values.trackingDomain,
      fallbackUrl: values.fallbackUrl,
      notes: values.notes ?? "",
    });
    toast("Campaign created", { description: values.name });
    router.push(`/campaigns/${id}`);
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">New Campaign</h1>
      <CampaignForm
        defaultValues={{ source: SOURCES[0], trackingDomain: TRACKING_DOMAINS[0] }}
        submitLabel="Create campaign"
        onSubmit={handleSubmit}
      />
    </div>
  );
}
