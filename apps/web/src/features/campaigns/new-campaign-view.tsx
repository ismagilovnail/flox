"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useCreateCampaign } from "@/hooks/use-campaigns";
import { CampaignForm, type CampaignFormValues } from "@/features/campaigns/campaign-form";

export function NewCampaignView() {
  const router = useRouter();
  const createCampaign = useCreateCampaign();

  function handleSubmit(values: CampaignFormValues) {
    createCampaign.mutate(
      {
        name: values.name,
        trafficSourceId: values.trafficSourceId,
        fallbackUrl: values.fallbackUrl,
        notes: values.notes ?? "",
      },
      {
        onSuccess: (created) => {
          toast("Campaign created", { description: created.name });
          router.push(`/campaigns/${created.id}`);
        },
        onError: (err) => toast.error("Couldn't create campaign", { description: err.message }),
      },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">New Campaign</h1>
      <CampaignForm defaultValues={{}} submitLabel="Create campaign" onSubmit={handleSubmit} />
    </div>
  );
}
