import { NewNetworkView } from "@/features/networks/new-network-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="offer.read">
      <NewNetworkView />
    </RequirePermission>
  );
}
