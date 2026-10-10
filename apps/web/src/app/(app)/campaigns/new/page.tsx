import { NewCampaignView } from "@/features/campaigns/new-campaign-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="campaign.read">
      <NewCampaignView />
    </RequirePermission>
  );
}
