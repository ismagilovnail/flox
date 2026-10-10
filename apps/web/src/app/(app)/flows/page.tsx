import { FlowList } from "@/features/flows/flow-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="campaign.read">
      <FlowList />
    </RequirePermission>
  );
}
