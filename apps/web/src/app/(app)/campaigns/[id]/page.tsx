import { CampaignDetailView } from "@/features/campaigns/campaign-detail-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default async function Page(props: PageProps<"/campaigns/[id]">) {
  const { id } = await props.params;
  return (
    <RequirePermission permission="campaign.read">
      <CampaignDetailView id={id} />
    </RequirePermission>
  );
}
