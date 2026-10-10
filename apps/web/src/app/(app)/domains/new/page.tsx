import { NewDomainView } from "@/features/domains/new-domain-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="campaign.read">
      <NewDomainView />
    </RequirePermission>
  );
}
