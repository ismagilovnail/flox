import { CampaignList } from "@/features/campaigns/campaign-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="campaign.read">
      <CampaignList />
    </RequirePermission>
  );
}
