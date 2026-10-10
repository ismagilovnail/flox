import { DomainList } from "@/features/domains/domain-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="campaign.read">
      <DomainList />
    </RequirePermission>
  );
}
